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
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	apiv1 "k8s.io/api/core/v1"
	resource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/RicochetStudios/registry"
	"github.com/go-logr/logr"

	serverv1 "ricochet/polaris/api/v1"
)

// InstanceReconciler reconciles a Instance object
type InstanceReconciler struct {
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
)

//+kubebuilder:rbac:groups=server.ricochet,resources=instances,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=server.ricochet,resources=instances/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=server.ricochet,resources=instances/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Instance object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.16.3/pkg/reconcile
func (r *InstanceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)
	l.Info("starting reconcile")
	defer l.Info("reconcile finished")

	// Get the instance.
	instance := &serverv1.Instance{}
	r.Get(ctx, types.NamespacedName{Name: req.Name, Namespace: req.Namespace}, instance)
	l.Info("Got instance", "spec", instance.Spec, "status", instance.Status)

	// Get the schema for the specified game name.
	if !reflect.ValueOf(instance.Spec).IsZero() {
		s, err := registry.GetSchema(instance.Spec.Game.Name)
		if err != nil {
			return ctrl.Result{}, err
		}
		l.Info("Got schema", "schema", s)

		// Create the persistent volume claims for the server.
		for _, v := range s.Volumes {
			if err := r.reconcilePersistentVolumeClaim(ctx, instance, v, req, l); err != nil {
				return ctrl.Result{}, err
			}
		}

		// Create the statefulset for the server.
		if err := r.reconcileStatefulSet(ctx, instance, s, req, l); err != nil {
			return ctrl.Result{}, err
		}

		// Create the service for the server.
		if err := r.reconcileService(ctx, instance, s, req, l); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// reconcileStatefulSet reconciles the statefulset for the server.
func (r *InstanceReconciler) reconcileStatefulSet(ctx context.Context, instance *serverv1.Instance, s registry.Schema, req ctrl.Request, l logr.Logger) error {
	// Create the volumes.
	volumes := []apiv1.Volume{}
	for _, v := range s.Volumes {
		volumes = append(volumes, toSpecVolume(instance, v))
	}

	// Create the volume mounts.
	volumeMounts := []apiv1.VolumeMount{}
	for _, v := range s.Volumes {
		volumeMounts = append(volumeMounts, toVolumeMount(instance, v))
	}

	// Create the container ports.
	containerPorts := []apiv1.ContainerPort{}
	for _, n := range s.Network {
		containerPorts = append(containerPorts, toContainerPort(n))
	}

	// Template settings.
	for i, setting := range s.Settings {
		s.Settings[i].Value = templateValue(setting.Value, s, *instance)
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
			Namespace: instance.Namespace,
			Name:      instance.Name,
			Labels: map[string]string{
				"id":  instance.Spec.Id,
				"app": instance.Spec.Game.Name,
			},
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: int32Ptr(statefulSetReplicas),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"id": instance.Spec.Id,
				},
			},
			Template: apiv1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"id":  instance.Spec.Id,
						"app": instance.Spec.Game.Name,
					},
				},
				Spec: apiv1.PodSpec{
					Volumes: volumes,
					Containers: []apiv1.Container{
						{
							// Underscores are not allowed in container names.
							Name:            strings.Replace(instance.Spec.Game.Name, "_", "-", -1),
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
									apiv1.ResourceCPU:    resource.MustParse(s.Sizes[instance.Spec.Size].Resources.CPU),
									apiv1.ResourceMemory: resource.MustParse(s.Sizes[instance.Spec.Size].Resources.Memory),
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
							Key:      "instance/ratio",
							Operator: apiv1.TolerationOpEqual,
							Value:    s.Ratio,
							Effect:   apiv1.TaintEffectNoSchedule,
						},
					},
				},
			},
		},
	}

	// Set the instance as the owner and controller of the statefulSet.
	// This helps ensure that the statefulSet is deleted when the instance is deleted.
	ctrl.SetControllerReference(instance, desiredStatefulSet, r.Scheme)

	// If the statefulSet does not exist, create.
	statefulSet := &appsv1.StatefulSet{}
	err := r.Get(ctx, types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, statefulSet)
	if err != nil {
		l.Info("StatefulSet does not exist, creating", "desiredStatefulSet", desiredStatefulSet)
		return r.Create(ctx, desiredStatefulSet)
	} else {
		// If the statefulSet is not equal to the desiredStatefulSet, update.
		if statefulSet != desiredStatefulSet {
			l.Info("StatefulSet is not as desired, updating")
			return r.Update(ctx, desiredStatefulSet)
		}
	}

	return nil
}

// templateValue takes a value and resolves its template if it is a template.
func templateValue(v string, s registry.Schema, i serverv1.Instance) string {
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
func toSpecVolume(i *serverv1.Instance, v registry.Volume) apiv1.Volume {
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
func toVolumeMount(i *serverv1.Instance, v registry.Volume) apiv1.VolumeMount {
	// Define the name of the PVC and Volume.
	n := v.Name + "-" + i.Name

	return apiv1.VolumeMount{
		Name:      n,
		MountPath: v.Path,
	}
}

// reconcilePersistentVolumeClaim reconciles the persistent volume claim for the server.
func (r *InstanceReconciler) reconcilePersistentVolumeClaim(ctx context.Context, instance *serverv1.Instance, v registry.Volume, req ctrl.Request, l logr.Logger) error {
	// Define the name of the persistent volume claim.
	pvcName := v.Name + "-" + instance.Name

	// Define the wanted persistent volume claim spec.
	desiredPersistentVolumeClaim := &apiv1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: instance.Namespace,
			Name:      pvcName,
			Labels: map[string]string{
				"id":  instance.Spec.Id,
				"app": instance.Spec.Game.Name,
			},
		},
		Spec: apiv1.PersistentVolumeClaimSpec{
			AccessModes: []apiv1.PersistentVolumeAccessMode{
				apiv1.ReadWriteOnce,
			},
			// TODO: Add storage class overrides via annotation.
			// StorageClassName: storageClassAnnotation,
			Resources: apiv1.ResourceRequirements{
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
	// Set the instance as the owner and controller of the pvc.
	// This helps ensure that the pvc is deleted when the instance is deleted.
	ctrl.SetControllerReference(instance, desiredPersistentVolumeClaim, r.Scheme)

	// If the persistentVolumeClaim does not exist, create.
	persistentVolumeClaim := &apiv1.PersistentVolumeClaim{}
	err := r.Get(ctx, types.NamespacedName{Name: pvcName, Namespace: instance.Namespace}, persistentVolumeClaim)
	// PersistentVolumeClaims cannot be modified, only created or deleted.
	if err != nil {
		l.Info("PersistentVolumeClaim does not exist, creating", "desiredPersistentVolumeClaim", desiredPersistentVolumeClaim)
		return r.Create(ctx, desiredPersistentVolumeClaim)
	}

	return nil
}

// reconcileService reconciles the service for the server.
func (r *InstanceReconciler) reconcileService(ctx context.Context, instance *serverv1.Instance, s registry.Schema, req ctrl.Request, l logr.Logger) error {
	// Create the servicePorts.
	servicePorts := []apiv1.ServicePort{}
	for _, n := range s.Network {
		servicePorts = append(servicePorts, toServicePort(n))
	}

	// Define the wanted service spec.
	desiredService := &apiv1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: instance.Namespace,
			Name:      instance.Name,
			Labels: map[string]string{
				"id":  instance.Spec.Id,
				"app": instance.Spec.Game.Name,
			},
		},
		Spec: apiv1.ServiceSpec{
			Selector: map[string]string{
				"id": instance.Spec.Id,
			},
			Type:  apiv1.ServiceTypeLoadBalancer,
			Ports: servicePorts,
		},
	}

	// Set the instance as the owner and controller of the service.
	// This helps ensure that the service is deleted when the instance is deleted.
	ctrl.SetControllerReference(instance, desiredService, r.Scheme)

	// If the service does not exist, create.
	service := &apiv1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, service)
	if err != nil {
		l.Info("Service does not exist, creating", "desiredService", desiredService)
		return r.Create(ctx, desiredService)
	} else {
		// If the service is not equal to the desiredService, update.
		if service != desiredService {
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
func (r *InstanceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&serverv1.Instance{}).
		Complete(r)
}
