package controller

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/log"

	firebasev1alpha1 "github.com/timam/firebase-controller.git/api/v1alpha1"
)

// FunctionReconciler reconciles a Function object
type FunctionReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	K8sClient     *kubernetes.Clientset
	EventRecorder record.EventRecorder
}

// +kubebuilder:rbac:groups=firebase.timam.dev,resources=functions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=firebase.timam.dev,resources=functions/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=firebase.timam.dev,resources=functions/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=pods/log,verbs=get
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch;update
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete

func (r *FunctionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	function := &firebasev1alpha1.Function{}
	if err := r.Get(ctx, req.NamespacedName, function); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Initialize status if empty
	if function.Status.Status == "" {
		if err := r.updateStatus(ctx, function, firebasev1alpha1.FunctionStatusPending, "Function created"); err != nil {
			log.Error(err, "Failed to update initial status")
			return ctrl.Result{}, err
		}
		r.EventRecorder.Event(function, corev1.EventTypeNormal, "Created", "Function resource created")
		return ctrl.Result{RequeueAfter: time.Second}, nil
	}

	// Clean up old jobs
	if err := r.cleanupOldJobs(ctx, function); err != nil {
		log.Error(err, "Failed to cleanup old jobs")
		// Continue even if cleanup fails
	}

	// Handle spec changes for deployed functions
	if function.Status.Status == firebasev1alpha1.FunctionStatusDeployed &&
		function.Status.ObservedGeneration != function.Generation {
		log.Info("Function spec changed, triggering new deployment")
		return r.handleSpecChange(ctx, function)
	}

	// Handle states
	switch function.Status.Status {
	case firebasev1alpha1.FunctionStatusPending:
		return r.handlePendingState(ctx, function)
	case firebasev1alpha1.FunctionStatusDeploying:
		return r.handleDeployingState(ctx, function)
	case firebasev1alpha1.FunctionStatusFailed:
		return r.handleFailedState(ctx, function)
	}

	return ctrl.Result{}, nil
}

func (r *FunctionReconciler) handleDeployingState(ctx context.Context, function *firebasev1alpha1.Function) (ctrl.Result, error) {
	// Check active jobs
	for _, ref := range function.Status.Active {
		job := &batchv1.Job{}
		if err := r.Get(ctx, client.ObjectKey{Namespace: ref.Namespace, Name: ref.Name}, job); err != nil {
			if errors.IsNotFound(err) {
				function.Status.Active = removeJobRef(function.Status.Active, ref)
				if err := r.Status().Update(ctx, function); err != nil {
					return ctrl.Result{}, err
				}
				continue
			}
			return ctrl.Result{}, err
		}

		// Check for timeout
		if job.Status.StartTime != nil && job.Status.CompletionTime == nil {
			if time.Since(job.Status.StartTime.Time) > 5*time.Minute {
				if err := r.Delete(ctx, job); err != nil && !errors.IsNotFound(err) {
					return ctrl.Result{}, err
				}
				if err := r.updateStatus(ctx, function, firebasev1alpha1.FunctionStatusFailed,
					"Deployment failed: timeout after 5 minutes"); err != nil {
					return ctrl.Result{}, err
				}
				r.EventRecorder.Event(function, corev1.EventTypeWarning, "DeploymentTimeout",
					"Deployment job timed out after 5 minutes")
				return ctrl.Result{RequeueAfter: time.Second * 30}, nil
			}
		}

		// Check if job failed
		if job.Status.Failed > 0 {
			errorMessage := "Deployment job failed"
			if logs, err := r.getJobLogs(ctx, job); err == nil && logs != "" {
				errorMessage = fmt.Sprintf("Deployment failed: %s", logs)
			}
			if err := r.updateStatus(ctx, function, firebasev1alpha1.FunctionStatusFailed, errorMessage); err != nil {
				return ctrl.Result{}, err
			}
			r.EventRecorder.Event(function, corev1.EventTypeWarning, "DeploymentFailed", errorMessage)
			return ctrl.Result{RequeueAfter: time.Second * 30}, nil
		}

		// Check for success
		if job.Status.Succeeded > 0 && job.Status.CompletionTime != nil {
			now := metav1.Now()
			function.Status.LastSuccessfulDeployment = &now
			function.Status.ObservedGeneration = function.Generation
			function.Status.CurrentRevision = fmt.Sprintf("%d-%s",
				function.Generation,
				job.Labels["firebase.timam.dev/spec-hash"])
			function.Status.RetryCount = 0

			if err := r.updateStatus(ctx, function, firebasev1alpha1.FunctionStatusDeployed,
				"Deployment completed successfully"); err != nil {
				return ctrl.Result{}, err
			}
			r.EventRecorder.Event(function, corev1.EventTypeNormal, "Deployed",
				"Firebase function deployed successfully")
			return ctrl.Result{}, nil
		}

		// Still running
		return ctrl.Result{RequeueAfter: time.Second * 10}, nil
	}

	// No active jobs found
	if err := r.updateStatus(ctx, function, firebasev1alpha1.FunctionStatusPending,
		"No active deployment jobs found"); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: time.Second * 10}, nil
}

func (r *FunctionReconciler) handleSpecChange(ctx context.Context, function *firebasev1alpha1.Function) (ctrl.Result, error) {
	if err := r.createDeploymentJob(ctx, function); err != nil {
		log.FromContext(ctx).Error(err, "Failed to create deployment job")
		return ctrl.Result{}, err
	}

	if err := r.updateStatus(ctx, function, firebasev1alpha1.FunctionStatusDeploying,
		"Redeploying due to spec change"); err != nil {
		return ctrl.Result{}, err
	}

	r.EventRecorder.Event(function, corev1.EventTypeNormal, "SpecChanged",
		"Detected change in function spec, triggering new deployment")
	return ctrl.Result{RequeueAfter: time.Second * 10}, nil
}

func (r *FunctionReconciler) handlePendingState(ctx context.Context, function *firebasev1alpha1.Function) (ctrl.Result, error) {
	if len(function.Status.Active) > 0 {
		log.FromContext(ctx).Info("Active jobs already exist, skipping new job creation")
		return ctrl.Result{RequeueAfter: time.Second * 10}, nil
	}

	if err := r.createDeploymentJob(ctx, function); err != nil {
		log.FromContext(ctx).Error(err, "Failed to create deployment job")
		r.EventRecorder.Event(function, corev1.EventTypeWarning, "Failed",
			"Failed to create deployment job")

		if updateErr := r.updateStatus(ctx, function, firebasev1alpha1.FunctionStatusFailed,
			err.Error()); updateErr != nil {
			log.FromContext(ctx).Error(updateErr, "Failed to update status after job creation failure")
		}
		return ctrl.Result{}, err
	}

	if err := r.updateStatus(ctx, function, firebasev1alpha1.FunctionStatusDeploying,
		"Starting deployment"); err != nil {
		return ctrl.Result{}, err
	}

	r.EventRecorder.Event(function, corev1.EventTypeNormal, "Created", "Deployment job created")
	return ctrl.Result{RequeueAfter: time.Second * 10}, nil
}

func (r *FunctionReconciler) handleFailedState(ctx context.Context, function *firebasev1alpha1.Function) (ctrl.Result, error) {
	maxRetries := int32(3) // default value
	if function.Spec.MaxRetries != nil {
		maxRetries = *function.Spec.MaxRetries
	}

	if function.Status.RetryCount >= maxRetries {
		err := r.updateStatus(ctx, function, firebasev1alpha1.FunctionStatusFailed,
			fmt.Sprintf("Deployment failed permanently after %d retries", maxRetries))
		return ctrl.Result{}, err
	}

	function.Status.RetryCount++
	if err := r.updateStatus(ctx, function, firebasev1alpha1.FunctionStatusPending,
		fmt.Sprintf("Retrying deployment (attempt %d/%d)", function.Status.RetryCount, maxRetries)); err != nil {
		return ctrl.Result{}, err
	}

	r.EventRecorder.Event(function, corev1.EventTypeNormal, "Retrying",
		fmt.Sprintf("Retrying deployment (attempt %d/%d)", function.Status.RetryCount, maxRetries))

	return ctrl.Result{RequeueAfter: time.Second * 30}, nil
}

func (r *FunctionReconciler) updateStatus(ctx context.Context, function *firebasev1alpha1.Function, status string, message string) error {
	function.Status.Status = status
	function.Status.Message = message
	return r.Status().Update(ctx, function)
}

func (r *FunctionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	config, err := config.GetConfig()
	if err != nil {
		return err
	}

	r.K8sClient, err = kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}

	r.EventRecorder = mgr.GetEventRecorderFor("firebase-controller")

	return ctrl.NewControllerManagedBy(mgr).
		For(&firebasev1alpha1.Function{}).
		Complete(r)
}

func (r *FunctionReconciler) createDeploymentJob(ctx context.Context, function *firebasev1alpha1.Function) error {
	specBytes, _ := json.Marshal(function.Spec)
	hash := sha256.Sum256(specBytes)
	specHash := fmt.Sprintf("%x", hash[:8])

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("%s-", function.Name),
			Namespace:    function.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":        "firebase-function-deployer",
				"app.kubernetes.io/instance":    function.Name,
				"app.kubernetes.io/component":   "deployer",
				"app.kubernetes.io/part-of":     "firebase-controller",
				"firebase.timam.dev/function":   function.Name,
				"firebase.timam.dev/generation": fmt.Sprintf("%d", function.Generation),
				"firebase.timam.dev/spec-hash":  specHash,
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            pointer.Int32(0),
			TTLSecondsAfterFinished: pointer.Int32(3600),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app.kubernetes.io/name":     "firebase-function-deployer",
						"app.kubernetes.io/instance": function.Name,
					},
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Volumes: []corev1.Volume{
						{
							Name: "google-credentials",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: function.Spec.Project.Auth.ServiceAccountKey.SecretName,
								},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name:            "firebase-deployer",
							Image:           function.Spec.Source.Container.Image,
							ImagePullPolicy: corev1.PullPolicy(function.Spec.Source.Container.ImagePullPolicy),
							Env: []corev1.EnvVar{
								{
									Name:  "GOOGLE_APPLICATION_CREDENTIALS",
									Value: function.Spec.Project.Auth.ServiceAccountKey.MountPath + "/firebase-credentials.json",
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "google-credentials",
									MountPath: function.Spec.Project.Auth.ServiceAccountKey.MountPath,
									ReadOnly:  true,
								},
							},
						},
					},
				},
			},
		},
	}

	if err := ctrl.SetControllerReference(function, job, r.Scheme); err != nil {
		return err
	}

	if err := r.Create(ctx, job); err != nil {
		return err
	}

	function.Status.Active = append(function.Status.Active, corev1.ObjectReference{
		Kind:      "Job",
		Namespace: job.Namespace,
		Name:      job.Name,
		UID:       job.UID,
	})

	return r.Status().Update(ctx, function)
}

func (r *FunctionReconciler) cleanupOldJobs(ctx context.Context, function *firebasev1alpha1.Function) error {
	log := log.FromContext(ctx)

	successLimit := int32(3)
	failedLimit := int32(1)
	if function.Status.SuccessfulDeploymentsHistoryLimit != nil {
		successLimit = *function.Status.SuccessfulDeploymentsHistoryLimit
	}
	if function.Status.FailedDeploymentsHistoryLimit != nil {
		failedLimit = *function.Status.FailedDeploymentsHistoryLimit
	}

	jobList := &batchv1.JobList{}
	if err := r.List(ctx, jobList,
		client.InNamespace(function.Namespace),
		client.MatchingLabels{"firebase.timam.dev/function": function.Name}); err != nil {
		return err
	}

	var successful, failed []batchv1.Job
	for _, job := range jobList.Items {
		if job.Status.CompletionTime == nil {
			continue
		}

		if job.Status.Succeeded > 0 {
			successful = append(successful, job)
		} else {
			failed = append(failed, job)
		}
	}

	sort.Slice(successful, func(i, j int) bool {
		return successful[i].Status.CompletionTime.After(successful[j].Status.CompletionTime.Time)
	})
	sort.Slice(failed, func(i, j int) bool {
		return failed[i].Status.CompletionTime.After(failed[j].Status.CompletionTime.Time)
	})

	function.Status.History = nil
	for _, job := range successful {
		function.Status.History = append(function.Status.History, firebasev1alpha1.DeploymentHistory{
			JobName:        job.Name,
			CompletionTime: job.Status.CompletionTime,
			Successful:     true,
			Generation:     getGenerationFromJob(&job),
			SpecHash:       job.Labels["firebase.timam.dev/spec-hash"],
		})
	}
	for _, job := range failed {
		function.Status.History = append(function.Status.History, firebasev1alpha1.DeploymentHistory{
			JobName:        job.Name,
			CompletionTime: job.Status.CompletionTime,
			Successful:     false,
			Generation:     getGenerationFromJob(&job),
			SpecHash:       job.Labels["firebase.timam.dev/spec-hash"],
		})
	}

	for i := int32(len(successful)); i > successLimit; i-- {
		job := successful[i-1]
		if err := r.Delete(ctx, &job); err != nil {
			log.Error(err, "Failed to delete old successful job", "job", job.Name)
			continue
		}
	}

	for i := int32(len(failed)); i > failedLimit; i-- {
		job := failed[i-1]
		if err := r.Delete(ctx, &job); err != nil {
			log.Error(err, "Failed to delete old failed job", "job", job.Name)
			continue
		}
	}

	return r.Status().Update(ctx, function)
}

func removeJobRef(refs []corev1.ObjectReference, target corev1.ObjectReference) []corev1.ObjectReference {
	var result []corev1.ObjectReference
	for _, ref := range refs {
		if ref.Name != target.Name {
			result = append(result, ref)
		}
	}
	return result
}

func getGenerationFromJob(job *batchv1.Job) int64 {
	gen := job.Labels["firebase.timam.dev/generation"]
	if gen == "" {
		return 0
	}
	generation, _ := strconv.ParseInt(gen, 10, 64)
	return generation
}

func (r *FunctionReconciler) getJobLogs(ctx context.Context, job *batchv1.Job) (string, error) {
	podList := &corev1.PodList{}
	if err := r.List(ctx, podList,
		client.InNamespace(job.Namespace),
		client.MatchingLabels(job.Spec.Template.Labels)); err != nil {
		return "", err
	}

	if len(podList.Items) == 0 {
		return "", fmt.Errorf("no pods found for job %s", job.Name)
	}

	pod := podList.Items[0]
	req := r.K8sClient.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, &corev1.PodLogOptions{})
	logs, err := req.Do(ctx).Raw()
	if err != nil {
		return "", err
	}

	return string(logs), nil
}
