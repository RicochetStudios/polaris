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

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	polarisv1alpha1 "github.com/RicochetStudios/polaris/apis/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// GatewayReconciler reconciles a Gateway object
type GatewayReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=polaris.ricochet,resources=gateways,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=polaris.ricochet,resources=gateways/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=polaris.ricochet,resources=gateways/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Gateway object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.2/pkg/reconcile
func (r *GatewayReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)
	l.Info("starting reconcile")
	defer l.Info("reconcile finished")

	// Get the gateway.
	gw := &polarisv1alpha1.Gateway{}
	err := r.Get(ctx, req.NamespacedName, gw)
	if apierrors.IsNotFound(err) {
		l.Info("Gateway not found, assuming it was deleted")
		return reconcile.Result{}, nil
	} else if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to get gateway: %w", err)
	}

	// Update the status of the gateway.
	if err := r.setCurrentState(ctx, gw, l); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to set polaris state: %w", err)
	}

	// // Remove the finalizer and return before running any reconciles.
	// // This is done to prevent reconcile functions from running when the gateway spec is empty.
	// if gw.GetDeletionTimestamp() != nil {
	// 	l.Info("Polaris is being deleted")
	// 	if controllerutil.ContainsFinalizer(gw, gatewayFinalizer) {
	// 		// // Run logic to perform before deleting the gateway.
	// 		// if err := r.finalizeGateway(ctx, gw, l); err != nil {
	// 		// 	return ctrl.Result{}, err
	// 		// }

	// 		l.Info("Gateway finalizer found, removing")
	// 		controllerutil.RemoveFinalizer(gw, gatewayFinalizer)
	// 		if err := r.Update(ctx, gw); err != nil {
	// 			return ctrl.Result{}, err
	// 		}
	// 	}
	// 	l.Info("Gateway resources cleaned up")
	// 	return ctrl.Result{}, nil
	// }

	// if !controllerutil.ContainsFinalizer(gw, gatewayFinalizer) {
	// 	// This log line is printed exactly once during initial provisioning,
	// 	// because once the finalizer is in place this block gets skipped. So,
	// 	// this is a nice place to tell the operator that the high level,
	// 	// multi-reconcile operation is underway.
	// 	l.Info("ensuring gateway is set up")
	// 	gw.Status.Conditions = ""
	// 	if err = r.Status().Update(ctx, gw); err != nil {
	// 		return ctrl.Result{}, fmt.Errorf("failed to update gateway status: %w", err)
	// 	}

	// 	controllerutil.AddFinalizer(gw, gatewayFinalizer)
	// 	if err := r.Update(ctx, gw); err != nil {
	// 		return ctrl.Result{}, err
	// 	}
	// }

	// Run logic to perform before setting up the gateway.
	if err = r.maybeProvisionGateway(ctx, gw, l); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to privision gateway: %w", err)
	}

	return ctrl.Result{}, nil
}

func (r *GatewayReconciler) setCurrentState(ctx context.Context, gw *polarisv1alpha1.Gateway, l log.Logger) error {
	// Get the child gateway.
	childGw := &gatewayv1.Gateway{}
	err := r.Get(ctx, types.NamespacedName{Name: gw.Name, Namespace: gw.Namespace}, childGw)
	if err != nil {
		return fmt.Errorf("failed to get gateway child resource: %w", err)
	}

	// Get the services assigned to the gateway.
	_, err = r.getAssignedServices(ctx, gw, l)

	// Set the status of the gateway.
	gw.Status.Addresses = childGw.Status.Addresses
	gw.Status.Conditions = childGw.Status.Conditions

	if err := r.Status().Update(ctx, gw); err != nil {
		return fmt.Errorf("failed to update gateway status: %w", err)
	}

	return nil
}

func (r *GatewayReconciler) getAssignedServices(ctx context.Context, gw *polarisv1alpha1.Gateway, l log.Logger) (corev1.Service, error) {
	// // Get the services assigned to the gateway.
	// services := &corev1.ServiceList{}
	// if err := r.List(ctx, services, client.MatchingLabels(gw.Spec.Selector)); err != nil {
	// 	return fmt.Errorf("failed to list services: %w", err)
	// }

	return corev1.Service{}, nil
}

func (r *GatewayReconciler) maybeProvisionGateway(ctx context.Context, gw *polarisv1alpha1.Gateway, l log.Logger) error {
	// Get the child gateway.
	childGw := &gatewayv1.Gateway{}
	err := r.Get(ctx, types.NamespacedName{Name: gw.Name, Namespace: gw.Namespace}, childGw)
	if err != nil {
		return fmt.Errorf("failed to get gateway child resource: %w", err)
	}

	// Check if the gateway is already provisioned.
	if childGw.Status.ObservedGeneration == gw.Generation {
		l.Info("gateway already provisioned")
		return nil
	}

	// Provision the gateway.
	l.Info("provisioning gateway")
	childGw.Spec = gw.Spec.GatewaySpec
	if err := r.Update(ctx, childGw); err != nil {
		return fmt.Errorf("failed to update gateway child resource: %w", err)
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *GatewayReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&polarisv1alpha1.Gateway{}).
		Complete(r)
}
