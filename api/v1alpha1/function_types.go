/*
Copyright 2025 Timam.

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

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// FunctionSpec defines the desired state of Function
type FunctionSpec struct {
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=nodejs18;nodejs16
	Runtime string `json:"runtime"`

	// +kubebuilder:validation:Required
	Source SourceSpec `json:"source"`

	// +kubebuilder:validation:Required
	Project ProjectSpec `json:"project"`
}

type SourceSpec struct {
	// +kubebuilder:validation:Required
	Container ContainerSpec `json:"container"`
}

type ContainerSpec struct {
	// +kubebuilder:validation:Required
	Image string `json:"image"`

	// +kubebuilder:default=Always
	ImagePullPolicy string `json:"imagePullPolicy,omitempty"`
}

type ProjectSpec struct {
	// +kubebuilder:validation:Required
	Auth AuthSpec `json:"auth"`
}

type AuthSpec struct {
	// +kubebuilder:validation:Required
	ServiceAccountKey ServiceAccountKeySpec `json:"serviceAccountKey"`
}

type ServiceAccountKeySpec struct {
	// +kubebuilder:validation:Required
	SecretName string `json:"secretName"`

	// +kubebuilder:validation:Required
	MountPath string `json:"mountPath"`
}

// FunctionStatus defines the observed state of Function
type FunctionStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Function is the Schema for the functions API
type Function struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FunctionSpec   `json:"spec,omitempty"`
	Status FunctionStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// FunctionList contains a list of Function
type FunctionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Function `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Function{}, &FunctionList{})
}
