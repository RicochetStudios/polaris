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
)

// Game defines the game and modloader to be used for the server.
type Game struct {
	// The name of the game type to be created.
	Name string `json:"name"`

	// The version of the game to be used.
	//
	// +kubebuilder:default:=latest
	// +optional
	Version string `json:"version"`

	// The software used to load mods into the game server.
	// Vanilla will launch the game server as default without any mods.
	//
	// +kubebuilder:default:=vanilla
	// +optional
	ModLoader string `json:"modLoader"`
}

// NetworkType defines the type of network to be used for the server.
// Only one of the following network types may be specified.
// If none of the following types is specified, the default one
// is PrivateNetwork.
// +kubebuilder:validation:Enum=public;private
// +kubebuilder:default:=private
type NetworkType string

const (
	// PublicNetwork will expose the server over a randomly generated external IP.
	PublicNetwork NetworkType = "public"

	// PrivateNetwork will create a Tailscale vpn sidecar and not expose an IP.
	PrivateNetwork NetworkType = "private"
)

// Network defines the network configuration for the server.
//
// This defines how the user can connect to the server.
type Network struct {
	// The type of network to be used for the server.
	//
	// +optional
	Type NetworkType `json:"type"`
}

// ServerSpec defines the desired state of the server.
type ServerSpec struct {
	// The unique identifier of the game server instance.
	Id string `json:"id"`

	// This changes the resources given to the server and the player limit.
	// Valid values are: xs, s, m, l, xl
	//
	// +kubebuilder:validation:Enum:=xs;s;m;l;xl
	// +kubebuilder:default:=xs
	// +optional
	Size string `json:"size"`

	// The name of the server.
	//
	// +kubebuilder:default:=Hyperborea
	// +optional
	Name string `json:"name"`

	// The game and modloader to be used for the server.
	Game Game `json:"game"`

	// The network configuration for the server.
	//
	// +optional
	Network Network `json:"network"`
}

// ServerState defines the current operating condition of the server.
// Only one of the following states may be specified.
// +kubebuilder:validation:Enum=provisioning;starting;running;stopping;stopped;deleting;failed;""
type ServerState string

const (
	ServerStateProvisioning ServerState = "provisioning"
	ServerStateStarting     ServerState = "starting"
	ServerStateRunning      ServerState = "running"
	ServerStateStopping     ServerState = "stopping"
	ServerStateStopped      ServerState = "stopped"
	ServerStateDeleting     ServerState = "deleting"
	ServerStateFailed       ServerState = "failed"
	ServerStateUnknown      ServerState = ""
)

// ServerStatus defines the observed state of Server
type ServerStatus struct {
	State ServerState `json:"state"`
}

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`
// +kubebuilder:resource:scope=Namespaced,shortName=svr,singular=server

// Server is the Schema for the servers API
type Server struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ServerSpec   `json:"spec,omitempty"`
	Status ServerStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ServerList contains a list of Server
type ServerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Server `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Server{}, &ServerList{})
}
