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

package controller

import (
	"context"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	firebasev1alpha1 "github.com/timam/firebase-controller.git/api/v1alpha1"
)

// FunctionReconciler reconciles a Function object
type FunctionReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=firebase.timam.dev,resources=functions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=firebase.timam.dev,resources=functions/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=firebase.timam.dev,resources=functions/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Function object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.0/pkg/reconcile
func (r *FunctionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	function := &firebasev1alpha1.Function{}
	if err := r.Get(ctx, req.NamespacedName, function); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if function.Status.Status == "" {
		if err := r.updateStatus(ctx, function, firebasev1alpha1.FunctionStatusPending, "Function created"); err != nil {
			log.Error(err, "Failed to update initial status")
			return ctrl.Result{}, err
		}
	}

	if function.Status.Status == firebasev1alpha1.FunctionStatusPending {
		// First create the deployment pod
		if err := r.createDeploymentPod(ctx, function); err != nil {
			log.Error(err, "Failed to create deployment pod")
			return ctrl.Result{}, err
		}

		// Then update the status to Deploying
		if err := r.updateStatus(ctx, function, firebasev1alpha1.FunctionStatusDeploying, "Starting deployment"); err != nil {
			log.Error(err, "Failed to update deploying status")
			return ctrl.Result{}, err
		}

		return ctrl.Result{RequeueAfter: time.Second * 10}, nil
	}

	if function.Status.Status == firebasev1alpha1.FunctionStatusDeploying {
		if r.isDeploymentSuccessful(ctx, function) {
			if err := r.updateStatus(ctx, function, firebasev1alpha1.FunctionStatusDeployed, "Deployment successful"); err != nil {
				return ctrl.Result{}, err
			}
		} else if r.isDeploymentFailed(ctx, function) {
			if err := r.updateStatus(ctx, function, firebasev1alpha1.FunctionStatusFailed, "Deployment failed"); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{RequeueAfter: time.Second * 10}, nil
	}

	return ctrl.Result{}, nil
}

// updateStatus updates the Function's status and message
func (r *FunctionReconciler) updateStatus(ctx context.Context, function *firebasev1alpha1.Function, status string, message string) error {
	function.Status.Status = status
	function.Status.Message = message
	return r.Status().Update(ctx, function)
}

// isDeploymentSuccessful checks if the Firebase function deployment succeeded
func (r *FunctionReconciler) isDeploymentSuccessful(ctx context.Context, function *firebasev1alpha1.Function) bool {
	// TODO: Implement actual deployment status check
	// This will involve checking the Firebase deployment status
	return false
}

// isDeploymentFailed checks if the Firebase function deployment failed
func (r *FunctionReconciler) isDeploymentFailed(ctx context.Context, function *firebasev1alpha1.Function) bool {
	// TODO: Implement actual deployment failure check
	// This will involve checking for timeout or error conditions
	return false
}

// createDeploymentPod creates a Pod to run the Firebase deployment
func (r *FunctionReconciler) createDeploymentPod(ctx context.Context, function *firebasev1alpha1.Function) error {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      function.Name + "-firebase-deployer",
			Namespace: function.Namespace,
			Labels: map[string]string{
				"app":      "firebase-deployer",
				"function": function.Name,
			},
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{
				{
					Name:            "firebase-deployer",
					Image:           function.Spec.Source.Container.Image,
					ImagePullPolicy: corev1.PullPolicy(function.Spec.Source.Container.ImagePullPolicy),
				},
			},
		},
	}

	if err := ctrl.SetControllerReference(function, pod, r.Scheme); err != nil {
		return err
	}

	return r.Create(ctx, pod)
}

// SetupWithManager sets up the controller with the Manager.
func (r *FunctionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&firebasev1alpha1.Function{}).
		Complete(r)
}
