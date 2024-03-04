/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/RicochetStudios/registry"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	polarisv1 "github.com/RicochetStudios/polaris/apis/v1"
)

// PolarisReconciler reconciles a Polaris object
type PolarisReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

const (
	// The number of replicas to be used for the statefulset.
	// Must always be one, game servers cannot be scaled horizontally.
	statefulSetReplicas int32 = 1

	// The number of seconds to wait before marking a pod as unreachable.
	//
	// At 30 seconds, the pod is scheduled to redeploy in 120s total.
	statefulSetUnreachableSeconds int64 = 30

	// statefulSetImagePullPolicy is the image pull policy to be used for the statefulset.
	statefulSetImagePullPolicy apiv1.PullPolicy = "Always"

	// The default resources to be requested by the statefulset container.
	statefulSetRequestResourcesCPU    string = "50m"
	statefulSetRequestResourcesMemory string = "100Mi"

	// templateRegex is a regular expression to validate templates.
	templateRegex string = `^{{ (?P<tpl>(\.\w+)*) }}$`

	// finalizer is the finalizer to be used for the instance.
	polarisFinalizer = "ricochet/finalizer"
)

//+kubebuilder:rbac:groups=polaris.ricochet,resources=polaris,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=polaris.ricochet,resources=polaris/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=polaris.ricochet,resources=polaris/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.0/pkg/reconcile
func (r *PolarisReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)
	l.Info("starting reconcile")
	defer l.Info("reconcile finished")

	// Get the instance of Polaris.
	polaris := &polarisv1.Polaris{}
	err := r.Get(ctx, req.NamespacedName, polaris)
	if apierrors.IsNotFound(err) {
		l.Info("Polaris not found, assuming it was deleted")
		return reconcile.Result{}, nil
	} else if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to get polaris: %w", err)
	}

	// Update the status of the Polaris instance.
	if err := r.setCurrentState(ctx, polaris, l); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to set polaris state: %w", err)
	}

	// Remove the finalizer and return before running any reconciles.
	// This is done to prevent reconcile functions from running when the polaris spec is empty.
	if polaris.GetDeletionTimestamp() != nil {
		l.Info("Polaris is being deleted")
		if controllerutil.ContainsFinalizer(polaris, polarisFinalizer) {
			// // Run logic to perform before deleting the polaris instance.
			// if err := r.finalizePolaris(ctx, polaris, l); err != nil {
			// 	return ctrl.Result{}, err
			// }

			l.Info("Polaris finalizer found, removing")
			controllerutil.RemoveFinalizer(polaris, polarisFinalizer)
			if err := r.Update(ctx, polaris); err != nil {
				return ctrl.Result{}, err
			}
		}
		l.Info("Polaris resources cleaned up")
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(polaris, polarisFinalizer) {
		// This log line is printed exactly once during initial provisioning,
		// because once the finalizer is in place this block gets skipped. So,
		// this is a nice place to tell the operator that the high level,
		// multi-reconcile operation is underway.
		l.Info("ensuring Polaris is set up")
		polaris.Status.State = polarisv1.PolarisStateProvisioning
		if err = r.Status().Update(ctx, polaris); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update polaris status: %w", err)
		}

		controllerutil.AddFinalizer(polaris, polarisFinalizer)
		if err := r.Update(ctx, polaris); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Run logic to perform before setting up the polaris instance.
	if err = r.maybeProvisionPolaris(ctx, polaris, req, l); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to privision polaris: %w", err)
	}

	return ctrl.Result{}, nil
}

// setCurrentState sets the current state of the Polaris instance.
// It checks the phase of the persistent volume claim and the pod.
func (r *PolarisReconciler) setCurrentState(ctx context.Context, polaris *polarisv1.Polaris, l logr.Logger) error {
	// TODO: Get the state of the service.

	// Update the status of the Polaris instance.
	err := error(nil)
	defer func() {
		err = r.Status().Update(ctx, polaris)
		l.Info("Polaris " + string(polaris.Status.State))
	}()
	if err != nil {
		return fmt.Errorf("failed to update polaris status: %w", err)
	}

	// Define the empty states, where it's possible sub resources don't exist.
	emptyStates := []polarisv1.PolarisState{polarisv1.PolarisStateDeleting, polarisv1.PolarisStateUnknown, polarisv1.PolarisStateProvisioning}

	// Get the persistent volume claims for the server.
	pvcs := &apiv1.PersistentVolumeClaimList{}
	err = r.List(ctx, pvcs, client.InNamespace(polaris.Namespace))
	if err != nil {
		return fmt.Errorf("failed to list persistent volume claims: %w", err)
	} else if len(pvcs.Items) == 0 && !slices.Contains(emptyStates, polarisv1.PolarisState(polaris.Status.State)) {
		// If no pvcs are found outside of the empty states, return an error.
		return fmt.Errorf("no persistent volume claims found")
	} else if len(pvcs.Items) == 0 {
		// If no pvcs are found duing empty states, set the state to provisioning.
		polaris.Status.State = polarisv1.PolarisStateProvisioning
		return nil
	}

	// Get the state of the pvc.
	var pvc *apiv1.PersistentVolumeClaim = &pvcs.Items[0]
	pvcStatus := pvc.Status.Phase

	// Get the pods for the server.
	pods := &apiv1.PodList{}
	err = r.List(ctx, pods, client.InNamespace(polaris.Namespace), client.MatchingLabels{"id": polaris.Spec.Id})
	if err != nil {
		return fmt.Errorf("failed to list pods: %w", err)
	} else if len(pods.Items) == 0 && !slices.Contains(emptyStates, polarisv1.PolarisState(polaris.Status.State)) {
		// If no pods are found outside of the empty states, return an error.
		return fmt.Errorf("no pods found")
	} else if len(pods.Items) == 0 {
		// If no pods are found duing empty states, set the state to provisioning.
		polaris.Status.State = polarisv1.PolarisStateProvisioning
		return nil
	}

	// Get the state of the pod.
	var pod *apiv1.Pod = &pods.Items[0]
	podStatus := pod.Status.Phase

	// Get the state of the Polaris server.
	if pvcStatus == apiv1.ClaimBound && podStatus == apiv1.PodRunning {
		polaris.Status.State = polarisv1.PolarisStateRunning
	} else if pvcStatus == apiv1.ClaimPending || podStatus == apiv1.PodPending {
		// If any sub resource is pending, set the state to creating.
		polaris.Status.State = polarisv1.PolarisStateProvisioning
	} else if polaris.DeletionTimestamp != nil {
		// If the polaris is marked for deletion, set the state to deleting.
		polaris.Status.State = polarisv1.PolarisStateDeleting
	} else if pvcStatus == apiv1.ClaimLost || podStatus == apiv1.PodFailed {
		polaris.Status.State = polarisv1.PolarisStateFailed
		// define some unique error messages for the different failure states
		if pvcStatus == apiv1.ClaimLost {
			return fmt.Errorf("polaris failed: persistent volume claim is lost")
		} else if podStatus == apiv1.PodFailed {
			return fmt.Errorf("polaris failed: pod is in a failed state")
		}
	}

	return nil
}

// // finalizePolaris runs logic to perform before deleting the polaris instance.
// func (r *PolarisReconciler) finalizePolaris(ctx context.Context, polaris *polarisv1.Polaris, l logr.Logger) error {
// 	polaris.Status.State = polarisv1.PolarisStateDeleting
// 	if err := r.Update(ctx, polaris); err != nil {
// 		return fmt.Errorf("failed to update polaris status: %w", err)
// 	}

// 	return nil
// }

func (r *PolarisReconciler) maybeProvisionPolaris(ctx context.Context, polaris *polarisv1.Polaris, req ctrl.Request, l logr.Logger) error {
	// Get the schema for the specified game name.
	s, err := registry.GetSchema(polaris.Spec.Game.Name)
	if err != nil {
		return err
	}
	l.Info("Got schema for game " + s.Name)

	// Create the persistent volume claims for the server.
	for _, v := range s.Volumes {
		if err := r.reconcilePvc(ctx, polaris, v, req, l); err != nil {
			return err
		}
	}

	// Create the statefulset for the server.
	if err := r.reconcileStatefulSet(ctx, polaris, s, req, l); err != nil {
		return err
	}

	// Create the service for the server.
	if err := r.reconcileService(ctx, polaris, s, req, l); err != nil {
		return err
	}

	// Update the status of the Polaris instance.
	if err := r.setCurrentState(ctx, polaris, l); err != nil {
		return err
	}

	return nil
}

// reconcileStatefulSet reconciles the statefulset for the server.
func (r *PolarisReconciler) reconcileStatefulSet(ctx context.Context, polaris *polarisv1.Polaris, s registry.Schema, req ctrl.Request, l logr.Logger) error {
	// Create the volumes.
	volumes := []apiv1.Volume{}
	for _, v := range s.Volumes {
		volumes = append(volumes, toSpecVolume(polaris, v))
	}

	// Create the volume mounts.
	volumeMounts := []apiv1.VolumeMount{}
	for _, v := range s.Volumes {
		volumeMounts = append(volumeMounts, toVolumeMount(polaris, v))
	}

	// Create the container ports.
	containerPorts := []apiv1.ContainerPort{}
	for _, n := range s.Network {
		containerPorts = append(containerPorts, toContainerPort(n))
	}

	// Template settings.
	for i, setting := range s.Settings {
		s.Settings[i].Value = templateValue(setting.Value, s, *polaris)
	}

	// Create the container env vars.
	containerEnvVars := []apiv1.EnvVar{}
	for _, setting := range s.Settings {
		containerEnvVars = append(containerEnvVars, toEnvVar(setting))
	}

	// Convert probes to from registry.Probe to apiv1.Probe.
	startupProbe := toProbe(s.Probes.Command, s.Probes.StartupProbe)
	readinessProbe := toProbe(s.Probes.Command, s.Probes.ReadinessProbe)
	livenessProbe := toProbe(s.Probes.Command, s.Probes.LivenessProbe)

	// Define the wanted statefulset spec.
	desiredStatefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: polaris.Namespace,
			Name:      polaris.Name,
			Labels: map[string]string{
				"id":  polaris.Spec.Id,
				"app": polaris.Spec.Game.Name,
			},
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: int32Ptr(statefulSetReplicas),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"id": polaris.Spec.Id,
				},
			},
			Template: apiv1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"id":  polaris.Spec.Id,
						"app": polaris.Spec.Game.Name,
					},
				},
				Spec: apiv1.PodSpec{
					Volumes: volumes,
					Containers: []apiv1.Container{
						{
							// Underscores are not allowed in container names.
							Name:            strings.Replace(polaris.Spec.Game.Name, "_", "-", -1),
							Image:           s.Image,
							ImagePullPolicy: statefulSetImagePullPolicy,
							Ports:           containerPorts,
							Resources: apiv1.ResourceRequirements{
								Requests: apiv1.ResourceList{
									apiv1.ResourceCPU:    resource.MustParse(statefulSetRequestResourcesCPU),
									apiv1.ResourceMemory: resource.MustParse(statefulSetRequestResourcesMemory),
								},
								// Limit resources to the specified size, relevant to the schema of the game.
								Limits: apiv1.ResourceList{
									apiv1.ResourceCPU:    resource.MustParse(s.Sizes[polaris.Spec.Size].Resources.CPU),
									apiv1.ResourceMemory: resource.MustParse(s.Sizes[polaris.Spec.Size].Resources.Memory),
								},
							},
							// Volumes go here.
							VolumeMounts: volumeMounts,
							// Health check probes.
							StartupProbe:   startupProbe,
							ReadinessProbe: readinessProbe,
							LivenessProbe:  livenessProbe,
							// Env var game settings.
							Env: containerEnvVars,
						},
					},
					Tolerations: []apiv1.Toleration{
						// Make pod redeploy faster when running on a risky node.
						// Useful for node failures and Azure Spot VMs.
						{
							Key:               "node.kubernetes.io/unreachable",
							Operator:          apiv1.TolerationOpExists,
							Effect:            apiv1.TaintEffectNoExecute,
							TolerationSeconds: int64Ptr(statefulSetUnreachableSeconds),
						},
						// Deploy to nodes with matching ratios.
						{
							Key:      "polaris/ratio",
							Operator: apiv1.TolerationOpEqual,
							Value:    s.Ratio,
							Effect:   apiv1.TaintEffectNoSchedule,
						},
					},
				},
			},
		},
	}

	// Set polaris as the owner and controller of the statefulSet.
	// This helps ensure that the statefulSet is deleted when polaris is deleted.
	ctrl.SetControllerReference(polaris, desiredStatefulSet, r.Scheme)

	// If the statefulSet does not exist, create.
	statefulSet := &appsv1.StatefulSet{}
	err := r.Get(ctx, types.NamespacedName{Name: polaris.Name, Namespace: polaris.Namespace}, statefulSet)
	if err != nil {
		l.Info("StatefulSet does not exist, creating")
		return r.Create(ctx, desiredStatefulSet)
	} else {
		// If the statefulSet is not equal to the desiredStatefulSet, update.
		if !reflect.DeepEqual(statefulSet, desiredStatefulSet) {
			l.Info("StatefulSet is not as desired, updating")
			return r.Update(ctx, desiredStatefulSet)
		}
	}

	return nil
}

// templateValue takes a value and resolves its template if it is a template.
func templateValue(v string, s registry.Schema, i polarisv1.Polaris) string {
	// Template the env var if needed.
	re, err := regexp.Compile(templateRegex)
	// If the regex is invalid, return an empty string.
	if err != nil {
		return ""
	}

	if re.MatchString(v) {
		// Get the template to target.
		matches := re.FindStringSubmatch(v)
		tplIndex := re.SubexpIndex("tpl")
		tpl := matches[tplIndex]

		// Resolve the templates.
		switch tpl {
		case ".name":
			return i.Spec.Name
		case ".modLoader":
			return i.Spec.Game.ModLoader
		case ".players":
			return fmt.Sprint(s.Sizes[i.Spec.Size].Players)
		}
	}

	// If it is not a template, return an empty string.
	return v
}

// toProbe converts a registry.Probe to a apiv1.Probe.
func toProbe(c []string, p registry.Probe) *apiv1.Probe {
	return &apiv1.Probe{
		ProbeHandler: apiv1.ProbeHandler{
			Exec: &apiv1.ExecAction{
				Command: c,
			},
		},
		InitialDelaySeconds: int32(p.InitialDelaySeconds),
		PeriodSeconds:       int32(p.PeriodSeconds),
		FailureThreshold:    int32(p.FailureThreshold),
		SuccessThreshold:    int32(p.SuccessThreshold),
		TimeoutSeconds:      int32(p.TimeoutSeconds),
	}
}

// toEnvVar converts a registry.Setting to a apiv1.EnvVar.
func toEnvVar(s registry.Setting) apiv1.EnvVar {
	return apiv1.EnvVar{
		Name:  s.Name,
		Value: s.Value,
	}
}

// toContainerPort converts a registry.Network to a apiv1.ContainerPort.
func toContainerPort(n registry.Network) apiv1.ContainerPort {
	return apiv1.ContainerPort{
		Name:          n.Name,
		Protocol:      apiv1.Protocol(strings.ToUpper(n.Protocol)),
		ContainerPort: int32(n.Port),
	}
}

// toSpecVolume converts a registry.Volume to a apiv1.Volume.
func toSpecVolume(i *polarisv1.Polaris, v registry.Volume) apiv1.Volume {
	// Define the name of the PVC and Volume.
	n := v.Name + "-" + i.Name

	return apiv1.Volume{
		Name: n,
		VolumeSource: apiv1.VolumeSource{
			PersistentVolumeClaim: &apiv1.PersistentVolumeClaimVolumeSource{
				ClaimName: n,
			},
		},
	}
}

// toVolumeMount converts a registry.Volume to a apiv1.VolumeMount.
func toVolumeMount(i *polarisv1.Polaris, v registry.Volume) apiv1.VolumeMount {
	// Define the name of the PVC and Volume.
	n := v.Name + "-" + i.Name

	return apiv1.VolumeMount{
		Name:      n,
		MountPath: v.Path,
	}
}

// reconcilePvc reconciles the persistent volume claim for the server.
func (r *PolarisReconciler) reconcilePvc(ctx context.Context, polaris *polarisv1.Polaris, v registry.Volume, req ctrl.Request, l logr.Logger) error {
	// Define the name of the persistent volume claim.
	pvcName := v.Name + "-" + polaris.Name

	// Define the wanted persistent volume claim spec.
	desiredPvc := &apiv1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: polaris.Namespace,
			Name:      pvcName,
			Labels: map[string]string{
				"id":  polaris.Spec.Id,
				"app": polaris.Spec.Game.Name,
			},
		},
		Spec: apiv1.PersistentVolumeClaimSpec{
			AccessModes: []apiv1.PersistentVolumeAccessMode{
				apiv1.ReadWriteOnce,
			},
			// TODO: Add storage class overrides via annotation.
			// StorageClassName: storageClassAnnotation,
			Resources: apiv1.VolumeResourceRequirements{
				Requests: apiv1.ResourceList{
					apiv1.ResourceStorage: resource.MustParse("1Gi"),
				},
				Limits: apiv1.ResourceList{
					apiv1.ResourceStorage: resource.MustParse(v.Size),
				},
			},
		},
	}

	// TODO: Add optional field to choose a VolumeClaimDeletePolicy.
	// Set polaris as the owner and controller of the pvc.
	// This helps ensure that the pvc is deleted when polaris is deleted.
	ctrl.SetControllerReference(polaris, desiredPvc, r.Scheme)

	// If the pvc does not exist, create.
	pvc := &apiv1.PersistentVolumeClaim{}
	err := r.Get(ctx, types.NamespacedName{Name: pvcName, Namespace: polaris.Namespace}, pvc)
	// PersistentVolumeClaims cannot be modified, only created or deleted.
	if err != nil {
		l.Info("PersistentVolumeClaim does not exist, creating")
		return r.Create(ctx, desiredPvc)
	}

	return nil
}

// reconcileService reconciles the service for the server.
func (r *PolarisReconciler) reconcileService(ctx context.Context, polaris *polarisv1.Polaris, s registry.Schema, req ctrl.Request, l logr.Logger) error {
	// Create the servicePorts.
	servicePorts := []apiv1.ServicePort{}
	for _, n := range s.Network {
		servicePorts = append(servicePorts, toServicePort(n))
	}

	// Define the wanted service spec.
	desiredService := &apiv1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: polaris.Namespace,
			Name:      polaris.Name,
			Labels: map[string]string{
				"id":  polaris.Spec.Id,
				"app": polaris.Spec.Game.Name,
			},
		},
		Spec: apiv1.ServiceSpec{
			Selector: map[string]string{
				"id": polaris.Spec.Id,
			},
			Type:  apiv1.ServiceTypeLoadBalancer,
			Ports: servicePorts,
		},
	}

	// Set polaris as the owner and controller of the service.
	// This helps ensure that the service is deleted when polaris is deleted.
	ctrl.SetControllerReference(polaris, desiredService, r.Scheme)

	// If the service does not exist, create.
	service := &apiv1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: polaris.Name, Namespace: polaris.Namespace}, service)
	if err != nil {
		l.Info("Service does not exist, creating")
		return r.Create(ctx, desiredService)
	} else {
		// If the service is not equal to the desiredService, update.
		if !reflect.DeepEqual(service, desiredService) {
			l.Info("Service is not as desired, updating")
			return r.Update(ctx, desiredService)
		}
	}

	return nil
}

// toServicePort converts a registry.Network to a apiv1.ServicePort.
func toServicePort(n registry.Network) apiv1.ServicePort {
	return apiv1.ServicePort{
		Name:       n.Name,
		Protocol:   apiv1.Protocol(strings.ToUpper(n.Protocol)),
		Port:       int32(n.Port),
		TargetPort: intstr.FromInt(n.Port),
	}
}

func int32Ptr(i int32) *int32 { return &i }

func int64Ptr(i int64) *int64 { return &i }

// SetupWithManager sets up the controller with the Manager.
func (r *PolarisReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&polarisv1.Polaris{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&apiv1.PersistentVolumeClaim{}).
		Owns(&apiv1.Service{}).
		Owns(&apiv1.Pod{}).
		Complete(r)
}
