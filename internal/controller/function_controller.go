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
	"bytes"
	"context"
	"io"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
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
	Scheme    *runtime.Scheme
	K8sClient *kubernetes.Clientset
}

// +kubebuilder:rbac:groups=firebase.timam.dev,resources=functions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=firebase.timam.dev,resources=functions/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=firebase.timam.dev,resources=functions/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=pods/log,verbs=get

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
			// Get the pod
			pod := &corev1.Pod{}
			podName := function.Name + "-firebase-deployer"
			if err := r.Get(ctx, client.ObjectKey{Namespace: function.Namespace, Name: podName}, pod); err == nil {
				// Get pod logs for error message
				failureReason := r.getPodLogs(ctx, pod)
				// Update status with failure reason
				if err := r.updateStatus(ctx, function, firebasev1alpha1.FunctionStatusFailed, failureReason); err != nil {
					return ctrl.Result{}, err
				}
				// Cleanup the failed pod
				if err := r.cleanupDeploymentPod(ctx, function); err != nil {
					log.Error(err, "Failed to cleanup deployment pod")
				}
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
	pod := &corev1.Pod{}
	podName := function.Name + "-firebase-deployer"
	err := r.Get(ctx, client.ObjectKey{Namespace: function.Namespace, Name: podName}, pod)
	if err != nil {
		return false
	}

	// Check if pod is in failed state
	if pod.Status.Phase == corev1.PodFailed {
		return true
	}

	// Check for container errors
	for _, containerStatus := range pod.Status.ContainerStatuses {
		if containerStatus.State.Terminated != nil && containerStatus.State.Terminated.ExitCode != 0 {
			return true
		}
	}

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

// getPodLogs retrieves logs from the deployment pod
func (r *FunctionReconciler) getPodLogs(ctx context.Context, pod *corev1.Pod) string {
	if r.K8sClient == nil {
		// Initialize Kubernetes client if not already done
		config, err := config.GetConfig()
		if err != nil {
			return "Failed to get kubernetes config"
		}

		r.K8sClient, err = kubernetes.NewForConfig(config)
		if err != nil {
			return "Failed to create kubernetes client"
		}
	}

	podLogOpts := corev1.PodLogOptions{}
	req := r.K8sClient.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, &podLogOpts)
	podLogs, err := req.Stream(ctx)
	if err != nil {
		return "Failed to get pod logs: " + err.Error()
	}
	defer podLogs.Close()

	buf := new(bytes.Buffer)
	_, err = io.Copy(buf, podLogs)
	if err != nil {
		return "Failed to read pod logs: " + err.Error()
	}
	return buf.String()
}

// cleanupDeploymentPod deletes the deployment pod
func (r *FunctionReconciler) cleanupDeploymentPod(ctx context.Context, function *firebasev1alpha1.Function) error {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      function.Name + "-firebase-deployer",
			Namespace: function.Namespace,
		},
	}
	return r.Delete(ctx, pod)
}

// SetupWithManager sets up the controller with the Manager.
func (r *FunctionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Initialize Kubernetes client
	config, err := config.GetConfig()
	if err != nil {
		return err
	}

	r.K8sClient, err = kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&firebasev1alpha1.Function{}).
		Complete(r)
}
