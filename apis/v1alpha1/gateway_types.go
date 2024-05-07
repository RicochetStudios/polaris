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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Address",type=string,JSONPath=`.status.address.value`
// +kubebuilder:resource:shortName=gtw,singular=gateway

// Gateway is the Schema for the gateways API
type Gateway struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GatewaySpec   `json:"spec,omitempty"`
	Status GatewayStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// GatewayList contains a list of Gateway
type GatewayList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Gateway `json:"items"`
}

// GatewaySpec defines the desired state of Gateway
type GatewaySpec struct {
	// GatewayClassName used for this Gateway. This is the name of a
	// GatewayClass resource.
	GatewayClassName gatewayv1.ObjectName `json:"gatewayClassName"`

	// Addresses requested for this Gateway. This is optional and behavior can
	// depend on the implementation. If a value is set in the spec and the
	// requested address is invalid or unavailable, the implementation MUST
	// indicate this in the associated entry in GatewayStatus.Addresses.
	//
	// The Addresses field represents a request for the address(es) on the
	// "outside of the Gateway", that traffic bound for this Gateway will use.
	// This could be the IP address or hostname of an external load balancer or
	// other networking infrastructure, or some other address that traffic will
	// be sent to.
	//
	// If no Addresses are specified, the implementation MAY schedule the
	// Gateway in an implementation-specific manner, assigning an appropriate
	// set of Addresses.
	//
	// The implementation MUST bind all Listeners to every GatewayAddress that
	// it assigns to the Gateway and add a corresponding entry in
	// GatewayStatus.Addresses.
	//
	// Support: Extended
	//
	// +optional
	// <gateway:validateIPAddress>
	// +kubebuilder:validation:MaxItems=16
	// +kubebuilder:validation:XValidation:message="IPAddress values must be unique",rule="self.all(a1, a1.type == 'IPAddress' ? self.exists_one(a2, a2.type == a1.type && a2.value == a1.value) : true )"
	// +kubebuilder:validation:XValidation:message="Hostname values must be unique",rule="self.all(a1, a1.type == 'Hostname' ? self.exists_one(a2, a2.type == a1.type && a2.value == a1.value) : true )"
	Addresses []gatewayv1.GatewayAddress `json:"addresses,omitempty"`

	// Map of string keys and values which services can use to select this Gateway.
	//
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// Infrastructure defines infrastructure level attributes about this Gateway instance.
	//
	// Support: Core
	//
	// <gateway:experimental>
	// +optional
	Infrastructure *gatewayv1.GatewayInfrastructure `json:"infrastructure,omitempty"`
}

// GatewayStatus defines the observed state of Gateway
type GatewayStatus struct {
	// Addresses lists the network addresses that have been bound to the
	// Gateway.
	//
	// This list may differ from the addresses provided in the spec under some
	// conditions:
	//
	//   * no addresses are specified, all addresses are dynamically assigned
	//   * a combination of specified and dynamic addresses are assigned
	//   * a specified address was unusable (e.g. already in use)
	//
	// +optional
	// <gateway:validateIPAddress>
	// +kubebuilder:validation:MaxItems=16
	Addresses []gatewayv1.GatewayStatusAddress `json:"addresses,omitempty"`

	// Services is a list of service names that are bound to this Gateway.
	//
	// +optional
	Services []string `json:"services,omitempty"`

	// Conditions describe the current conditions of the Gateway.
	//
	// Implementations should prefer to express Gateway conditions
	// using the `GatewayConditionType` and `GatewayConditionReason`
	// constants so that operators and tools can converge on a common
	// vocabulary to describe Gateway state.
	//
	// Known condition types are:
	//
	// * "Accepted"
	// * "Programmed"
	// * "Ready"
	//
	// +optional
	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:MaxItems=8
	// +kubebuilder:default={{type: "Accepted", status: "Unknown", reason:"Pending", message:"Waiting for controller", lastTransitionTime: "1970-01-01T00:00:00Z"},{type: "Programmed", status: "Unknown", reason:"Pending", message:"Waiting for controller", lastTransitionTime: "1970-01-01T00:00:00Z"}}
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}
