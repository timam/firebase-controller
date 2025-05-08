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

const (
	FunctionStatusPending   = "Pending"   // Initial state when created
	FunctionStatusDeploying = "Deploying" // Function is being deployed to Firebase
	FunctionStatusDeployed  = "Deployed"  // Function successfully deployed
	FunctionStatusFailed    = "Failed"    // Deployment failed
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
	// Status represents the current state of the function deployment (Pending, Deploying, Deployed, Failed)
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=Pending;Deploying;Deployed;Failed
	// +kubebuilder:default=Pending
	Status string `json:"status"`

	// Message provides details about the current Status
	// +optional
	Message string `json:"message,omitempty"`

	RetryCount    int          `json:"retryCount,omitempty"`
	LastRetryTime *metav1.Time `json:"lastRetryTime,omitempty"`
	ImageHash     string       `json:"imageHash,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="NAMESPACE",type="string",JSONPath=".metadata.namespace"
// +kubebuilder:printcolumn:name="STATUS",type="string",JSONPath=".status.status"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"

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
