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

	"github.com/RicochetStudios/registry"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"

	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	polarisv1alpha1 "ricochet/polaris/apis/v1alpha1"
)

// ServerReconciler reconciles a Server object
type ServerReconciler struct {
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
	statefulSetImagePullPolicy corev1.PullPolicy = "Always"

	// The default resources to be requested by the statefulset container.
	statefulSetRequestResourcesCPU    string = "50m"
	statefulSetRequestResourcesMemory string = "100Mi"

	// templateRegex is a regular expression to validate templates.
	templateRegex string = `^{{ (?P<tpl>(\.\w+)*) }}$`

	// finalizer is the finalizer to be used for the instance.
	serverFinalizer = "server.polaris.ricochet/finalizer"
)

//+kubebuilder:rbac:groups=polaris.ricochet,resources=servers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=polaris.ricochet,resources=servers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=polaris.ricochet,resources=servers/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.2/pkg/reconcile
func (r *ServerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)
	l.Info("starting reconcile")
	defer l.Info("reconcile finished")

	// Get the server instance.
	server := &polarisv1alpha1.Server{}
	err := r.Get(ctx, req.NamespacedName, server)
	if apierrors.IsNotFound(err) {
		l.Info("Server not found, assuming it was deleted")
		return reconcile.Result{}, nil
	} else if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to get server: %w", err)
	}

	// Update the status of the server.
	if err := r.setCurrentState(ctx, server, l); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to set polaris state: %w", err)
	}

	// Remove the finalizer and return before running any reconciles.
	// This is done to prevent reconcile functions from running when the server spec is empty.
	if server.GetDeletionTimestamp() != nil {
		l.Info("Polaris is being deleted")
		if controllerutil.ContainsFinalizer(server, serverFinalizer) {
			// // Run logic to perform before deleting the server.
			// if err := r.finalizeServer(ctx, server, l); err != nil {
			// 	return ctrl.Result{}, err
			// }

			l.Info("Server finalizer found, removing")
			controllerutil.RemoveFinalizer(server, serverFinalizer)
			if err := r.Update(ctx, server); err != nil {
				return ctrl.Result{}, err
			}
		}
		l.Info("Server resources cleaned up")
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(server, serverFinalizer) {
		// This log line is printed exactly once during initial provisioning,
		// because once the finalizer is in place this block gets skipped. So,
		// this is a nice place to tell the operator that the high level,
		// multi-reconcile operation is underway.
		l.Info("ensuring Server is set up")
		server.Status.State = polarisv1alpha1.ServerStateProvisioning
		if err = r.Status().Update(ctx, server); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update server status: %w", err)
		}

		controllerutil.AddFinalizer(server, serverFinalizer)
		if err := r.Update(ctx, server); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Run logic to perform before setting up the server.
	if err = r.maybeProvisionPolaris(ctx, server, l); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to privision server: %w", err)
	}

	return ctrl.Result{}, nil
}

// setCurrentState sets the current state of the server.
// It checks the phase of the persistent volume claim and the pod.
func (r *ServerReconciler) setCurrentState(ctx context.Context, server *polarisv1alpha1.Server, l logr.Logger) error {
	// TODO: Get the state of the service.

	// Update the status of the server.
	err := error(nil)
	defer func() {
		err = r.Status().Update(ctx, server)
		l.Info("Server " + string(server.Status.State))
	}()
	if err != nil {
		return fmt.Errorf("failed to update polaris status: %w", err)
	}

	// Define the empty states, where it's possible sub resources don't exist.
	emptyStates := []polarisv1alpha1.ServerState{
		polarisv1alpha1.ServerStateDeleting,
		polarisv1alpha1.ServerStateUnknown,
		polarisv1alpha1.ServerStateProvisioning,
	}

	// Get the persistent volume claims for the server.
	pvcs := &corev1.PersistentVolumeClaimList{}
	err = r.List(ctx, pvcs, client.InNamespace(server.Namespace))
	if err != nil {
		return fmt.Errorf("failed to list persistent volume claims: %w", err)
	} else if len(pvcs.Items) == 0 && !slices.Contains(emptyStates, polarisv1alpha1.ServerState(server.Status.State)) {
		// If no pvcs are found outside of the empty states, return an error.
		return fmt.Errorf("no persistent volume claims found")
	} else if len(pvcs.Items) == 0 {
		// If no pvcs are found duing empty states, set the state to provisioning.
		server.Status.State = polarisv1alpha1.ServerStateProvisioning
		return nil
	}

	// Get the state of the pvc.
	var pvc *corev1.PersistentVolumeClaim = &pvcs.Items[0]
	pvcStatus := pvc.Status.Phase

	// Get the pods for the server.
	pods := &corev1.PodList{}
	err = r.List(ctx, pods, client.InNamespace(server.Namespace), client.MatchingLabels{"id": server.Spec.Id})
	if err != nil {
		return fmt.Errorf("failed to list pods: %w", err)
	} else if len(pods.Items) == 0 && !slices.Contains(emptyStates, polarisv1alpha1.ServerState(server.Status.State)) {
		// If no pods are found outside of the empty states, return an error.
		return fmt.Errorf("no pods found")
	} else if len(pods.Items) == 0 {
		// If no pods are found duing empty states, set the state to provisioning.
		server.Status.State = polarisv1alpha1.ServerStateProvisioning
		return nil
	}

	// Get the state of the pod.
	var pod *corev1.Pod = &pods.Items[0]
	podStatus := pod.Status.Phase

	// Evaluate the state of the server.
	if pvcStatus == corev1.ClaimBound && podStatus == corev1.PodRunning {
		server.Status.State = polarisv1alpha1.ServerStateRunning
	} else if pvcStatus == corev1.ClaimPending || podStatus == corev1.PodPending {
		// If any sub resource is pending, set the state to creating.
		server.Status.State = polarisv1alpha1.ServerStateProvisioning
	} else if server.DeletionTimestamp != nil {
		// If the polaris is marked for deletion, set the state to deleting.
		server.Status.State = polarisv1alpha1.ServerStateDeleting
	} else if pvcStatus == corev1.ClaimLost || podStatus == corev1.PodFailed {
		server.Status.State = polarisv1alpha1.ServerStateFailed
		// define some unique error messages for the different failure states
		if pvcStatus == corev1.ClaimLost {
			return fmt.Errorf("server failed: persistent volume claim is lost")
		} else if podStatus == corev1.PodFailed {
			return fmt.Errorf("server failed: pod is in a failed state")
		}
	}

	return nil
}

// // finalizeServer runs logic to perform before deleting the server.
// func (r *ServerReconciler) finalizeServer(ctx context.Context, server *polarisv1alpha1.Server, l logr.Logger) error {
// 	server.Status.State = polarisv1alpha1.ServerStateDeleting
// 	if err := r.Update(ctx, server); err != nil {
// 		return fmt.Errorf("failed to update server status: %w", err)
// 	}

// 	return nil
// }

// maybeProvisionPolaris runs the logic to provision the server.
func (r *ServerReconciler) maybeProvisionPolaris(ctx context.Context, server *polarisv1alpha1.Server, l logr.Logger) error {
	// Get the schema for the specified game name.
	s, err := registry.GetSchema(server.Spec.Game.Name)
	if err != nil {
		return err
	}
	l.Info("Got schema for game " + s.Name)

	// Create the persistent volume claims for the server.
	for _, v := range s.Volumes {
		if err := r.reconcilePvc(ctx, server, v, l); err != nil {
			return err
		}
	}

	// Create the statefulset for the server.
	if err := r.reconcileStatefulSet(ctx, server, s, l); err != nil {
		return err
	}

	// Create the service for the server.
	if err := r.reconcileService(ctx, server, s, l); err != nil {
		return err
	}

	// Update the status of the server.
	if err := r.setCurrentState(ctx, server, l); err != nil {
		return err
	}

	return nil
}

// reconcileStatefulSet reconciles the statefulset for the server.
func (r *ServerReconciler) reconcileStatefulSet(ctx context.Context, server *polarisv1alpha1.Server, s registry.Schema, l logr.Logger) error {
	// Create the volumes.
	volumes := []corev1.Volume{}
	for _, v := range s.Volumes {
		volumes = append(volumes, toSpecVolume(server, v))
	}

	// Create the volume mounts.
	volumeMounts := []corev1.VolumeMount{}
	for _, v := range s.Volumes {
		volumeMounts = append(volumeMounts, toVolumeMount(server, v))
	}

	// Create the container ports.
	containerPorts := []corev1.ContainerPort{}
	for _, n := range s.Network {
		containerPorts = append(containerPorts, toContainerPort(n))
	}

	// Template settings.
	for i, setting := range s.Settings {
		s.Settings[i].Value = templateValue(setting.Value, s, *server)
	}

	// Create the container env vars.
	containerEnvVars := []corev1.EnvVar{}
	for _, setting := range s.Settings {
		containerEnvVars = append(containerEnvVars, toEnvVar(setting))
	}

	// Convert probes to from registry.Probe to corev1.Probe.
	startupProbe := toProbe(s.Probes.Command, s.Probes.StartupProbe)
	readinessProbe := toProbe(s.Probes.Command, s.Probes.ReadinessProbe)
	livenessProbe := toProbe(s.Probes.Command, s.Probes.LivenessProbe)

	// Define the wanted statefulset spec.
	desiredStatefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: server.Namespace,
			Name:      server.Name,
			Labels: map[string]string{
				"id":  server.Spec.Id,
				"app": server.Spec.Game.Name,
			},
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: int32Ptr(statefulSetReplicas),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"id": server.Spec.Id,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"id":  server.Spec.Id,
						"app": server.Spec.Game.Name,
					},
				},
				Spec: corev1.PodSpec{
					Volumes: volumes,
					Containers: []corev1.Container{
						{
							// Underscores are not allowed in container names.
							Name:            strings.Replace(server.Spec.Game.Name, "_", "-", -1),
							Image:           s.Image,
							ImagePullPolicy: statefulSetImagePullPolicy,
							Ports:           containerPorts,
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse(statefulSetRequestResourcesCPU),
									corev1.ResourceMemory: resource.MustParse(statefulSetRequestResourcesMemory),
								},
								// Limit resources to the specified size, relevant to the schema of the game.
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse(s.Sizes[server.Spec.Size].Resources.CPU),
									corev1.ResourceMemory: resource.MustParse(s.Sizes[server.Spec.Size].Resources.Memory),
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
					Tolerations: []corev1.Toleration{
						// Make pod redeploy faster when running on a risky node.
						// Useful for node failures and Azure Spot VMs.
						{
							Key:               "node.kubernetes.io/unreachable",
							Operator:          corev1.TolerationOpExists,
							Effect:            corev1.TaintEffectNoExecute,
							TolerationSeconds: int64Ptr(statefulSetUnreachableSeconds),
						},
						// Deploy to nodes with matching ratios.
						{
							Key:      "server.polaris.ricochet/ratio",
							Operator: corev1.TolerationOpEqual,
							Value:    s.Ratio,
							Effect:   corev1.TaintEffectNoSchedule,
						},
					},
				},
			},
		},
	}

	// Set server as the owner and controller of the statefulSet.
	// This helps ensure that the statefulSet is deleted when the server resource is deleted.
	ctrl.SetControllerReference(server, desiredStatefulSet, r.Scheme)

	// If the statefulSet does not exist, create.
	statefulSet := &appsv1.StatefulSet{}
	err := r.Get(ctx, types.NamespacedName{Name: server.Name, Namespace: server.Namespace}, statefulSet)
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

// reconcilePvc reconciles the persistent volume claim for the server.
func (r *ServerReconciler) reconcilePvc(ctx context.Context, server *polarisv1alpha1.Server, v registry.Volume, l logr.Logger) error {
	// Define the name of the persistent volume claim.
	pvcName := v.Name + "-" + server.Name

	// Define the wanted persistent volume claim spec.
	desiredPvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: server.Namespace,
			Name:      pvcName,
			Labels: map[string]string{
				"id":  server.Spec.Id,
				"app": server.Spec.Game.Name,
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			},
			// TODO: Add storage class overrides via annotation.
			// StorageClassName: storageClassAnnotation,
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("1Gi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(v.Size),
				},
			},
		},
	}

	// TODO: Add optional field to choose a VolumeClaimDeletePolicy.
	// Set polaris as the owner and controller of the pvc.
	// This helps ensure that the pvc is deleted when polaris is deleted.
	ctrl.SetControllerReference(server, desiredPvc, r.Scheme)

	// If the pvc does not exist, create.
	pvc := &corev1.PersistentVolumeClaim{}
	err := r.Get(ctx, types.NamespacedName{Name: pvcName, Namespace: server.Namespace}, pvc)
	// PersistentVolumeClaims cannot be modified, only created or deleted.
	if err != nil {
		l.Info("PersistentVolumeClaim does not exist, creating")
		return r.Create(ctx, desiredPvc)
	}

	return nil
}

// reconcileService reconciles the service for the server.
func (r *ServerReconciler) reconcileService(ctx context.Context, server *polarisv1alpha1.Server, s registry.Schema, l logr.Logger) error {
	// Create the servicePorts.
	servicePorts := []corev1.ServicePort{}
	for _, n := range s.Network {
		servicePorts = append(servicePorts, toServicePort(n))
	}

	// Define the wanted service spec.
	desiredService := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: server.Namespace,
			Name:      server.Name,
			Labels: map[string]string{
				"id":  server.Spec.Id,
				"app": server.Spec.Game.Name,
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"id": server.Spec.Id,
			},
			Type:  corev1.ServiceTypeLoadBalancer,
			Ports: servicePorts,
		},
	}

	// Set server as the owner and controller of the service.
	// This helps ensure that the service is deleted when the server resource is deleted.
	ctrl.SetControllerReference(server, desiredService, r.Scheme)

	// If the service does not exist, create.
	service := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: server.Name, Namespace: server.Namespace}, service)
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

// templateValue takes a value and resolves its template if it is a template.
func templateValue(v string, s registry.Schema, i polarisv1alpha1.Server) string {
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

// toProbe converts a registry.Probe to a corev1.Probe.
func toProbe(c []string, p registry.Probe) *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			Exec: &corev1.ExecAction{
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

// toEnvVar converts a registry.Setting to a corev1.EnvVar.
func toEnvVar(s registry.Setting) corev1.EnvVar {
	return corev1.EnvVar{
		Name:  s.Name,
		Value: s.Value,
	}
}

// toContainerPort converts a registry.Network to a corev1.ContainerPort.
func toContainerPort(n registry.Network) corev1.ContainerPort {
	return corev1.ContainerPort{
		Name:          n.Name,
		Protocol:      corev1.Protocol(strings.ToUpper(n.Protocol)),
		ContainerPort: int32(n.Port),
	}
}

// toSpecVolume converts a registry.Volume to a corev1.Volume.
func toSpecVolume(i *polarisv1alpha1.Server, v registry.Volume) corev1.Volume {
	// Define the name of the PVC and Volume.
	n := v.Name + "-" + i.Name

	return corev1.Volume{
		Name: n,
		VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: n,
			},
		},
	}
}

// toVolumeMount converts a registry.Volume to a corev1.VolumeMount.
func toVolumeMount(i *polarisv1alpha1.Server, v registry.Volume) corev1.VolumeMount {
	// Define the name of the PVC and Volume.
	n := v.Name + "-" + i.Name

	return corev1.VolumeMount{
		Name:      n,
		MountPath: v.Path,
	}
}

// toServicePort converts a registry.Network to a corev1.ServicePort.
func toServicePort(n registry.Network) corev1.ServicePort {
	return corev1.ServicePort{
		Name:       n.Name,
		Protocol:   corev1.Protocol(strings.ToUpper(n.Protocol)),
		Port:       int32(n.Port),
		TargetPort: intstr.FromInt(n.Port),
	}
}

func int32Ptr(i int32) *int32 { return &i }

func int64Ptr(i int64) *int64 { return &i }

// SetupWithManager sets up the controller with the Manager.
func (r *ServerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&polarisv1alpha1.Server{}).
		Complete(r)
}
