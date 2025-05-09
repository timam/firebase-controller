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
	corev1 "k8s.io/api/core/v1"
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
	// Number of successful deployment history to keep
	// +optional
	// +kubebuilder:default=3
	// +kubebuilder:validation:Minimum=1
	SuccessfulDeploymentsHistoryLimit *int32 `json:"successfulDeploymentsHistoryLimit,omitempty"`

	// Number of failed deployment history to keep
	// +optional
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	FailedDeploymentsHistoryLimit *int32 `json:"failedDeploymentsHistoryLimit,omitempty"`

	// Maximum number of retry attempts for failed deployments
	// +optional
	// +kubebuilder:default=3
	// +kubebuilder:validation:Minimum=1
	MaxRetries *int32 `json:"maxRetries,omitempty"`

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
	// ObservedGeneration represents the .metadata.generation that the condition was set based upon.
	// For instance, if .metadata.generation is currently 12, but the .status.conditions[x].observedGeneration is 9, the condition is out of date
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Represents the latest available observations of a function's current state
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Status represents the current state of the function deployment
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=Pending;Deploying;Deployed;Failed
	// +kubebuilder:default=Pending
	Status string `json:"status"`

	// Message provides details about the current Status
	// +optional
	Message string `json:"message,omitempty"`

	// CurrentRevision indicates the version of the Function spec that was successfully deployed
	// +optional
	CurrentRevision string `json:"currentRevision,omitempty"`

	// LastSuccessfulDeployment is the timestamp of the last successful deployment
	// +optional
	LastSuccessfulDeployment *metav1.Time `json:"lastSuccessfulDeployment,omitempty"`

	// Number of successful deployment history to keep
	// +optional
	// +kubebuilder:default=3
	// +kubebuilder:validation:Minimum=0
	SuccessfulDeploymentsHistoryLimit *int32 `json:"successfulDeploymentsHistoryLimit,omitempty"`

	// Number of failed deployment history to keep
	// +optional
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=0
	FailedDeploymentsHistoryLimit *int32 `json:"failedDeploymentsHistoryLimit,omitempty"`

	// RetryCount tracks the number of deployment retry attempts
	// +optional
	RetryCount int32 `json:"retryCount,omitempty"`

	// Active holds pointers to currently executing deployments
	// +optional
	Active []corev1.ObjectReference `json:"active,omitempty"`

	// History holds references to completed deployments, sorted by completion timestamp
	// +optional
	// +patchStrategy=merge
	History []DeploymentHistory `json:"history,omitempty"`
}

type DeploymentHistory struct {
	// Job name of the completed deployment
	JobName string `json:"jobName"`

	// When the deployment completed
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`

	// Whether the deployment was successful
	Successful bool `json:"successful"`

	// Generation of the Function spec that was deployed
	Generation int64 `json:"generation"`

	// Hash of the Function spec that was deployed
	SpecHash string `json:"specHash"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="NAMESPACE",type="string",JSONPath=".metadata.namespace"
// +kubebuilder:printcolumn:name="STATUS",type="string",JSONPath=".status.status"
// +kubebuilder:printcolumn:name="REVISION",type="string",JSONPath=".status.currentRevision"
// +kubebuilder:printcolumn:name="LAST-DEPLOYED",type="date",JSONPath=".status.lastSuccessfulDeployment"
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
