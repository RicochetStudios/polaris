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
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	apiv1 "k8s.io/api/core/v1"
	resource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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
	// Debug line.
	l.Info("Enter Reconcile", "req", req)

	// TODO(user): your logic here

	// Create an empty instance.
	instance := &serverv1.Instance{}
	r.Get(ctx, types.NamespacedName{Name: req.Name, Namespace: req.Namespace}, instance)

	// Debug line.
	l.Info("Enter Reconcile", "spec", instance.Spec, "status", instance.Status)

	// Get the schema for the specified game name.
	s, err := registry.GetSchema(req.Name)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Create the statefulset for the server.
	if err := r.reconcileStatefulSet(ctx, instance, s, req, l); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// reconcileStatefulSet reconciles the statefulset for the server.
func (r *InstanceReconciler) reconcileStatefulSet(ctx context.Context, instance *serverv1.Instance, s registry.Schema, req ctrl.Request, l logr.Logger) error {
	statefulSet := &appsv1.StatefulSet{}
	if err := r.Get(ctx, types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, statefulSet); err != nil {
		l.Info("StatefulSet does not exist, creating", "err", err)
	}

	// Create the container ports.
	containerPorts := []apiv1.ContainerPort{}
	for _, n := range s.Network {
		containerPorts = append(containerPorts, toContainerPort(n))
	}

	// Create the container env vars.
	containerEnvVars := []apiv1.EnvVar{}
	for _, setting := range s.Settings {
		containerEnvVars = append(containerEnvVars, toEnvVar(setting))
	}

	// Define the wanted statefulset spec.
	statefulSet = &appsv1.StatefulSet{
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
							// Probes go here.
							StartupProbe: &apiv1.Probe{
								ProbeHandler: apiv1.ProbeHandler{
									Exec: &apiv1.ExecAction{
										Command: s.Probes.Command,
									},
								},
								InitialDelaySeconds: int32(s.Probes.StartupProbe.InitialDelaySeconds),
								PeriodSeconds:       int32(s.Probes.StartupProbe.PeriodSeconds),
								FailureThreshold:    int32(s.Probes.StartupProbe.FailureThreshold),
								SuccessThreshold:    int32(s.Probes.StartupProbe.SuccessThreshold),
								TimeoutSeconds:      int32(s.Probes.StartupProbe.TimeoutSeconds),
							},
							ReadinessProbe: &apiv1.Probe{},
							LivenessProbe:  &apiv1.Probe{},
							Env:            containerEnvVars,
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

	// Compare the statefulset spec with the instance spec.

	return nil
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

func int32Ptr(i int32) *int32 { return &i }

func int64Ptr(i int64) *int64 { return &i }

// SetupWithManager sets up the controller with the Manager.
func (r *InstanceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&serverv1.Instance{}).
		Complete(r)
}
