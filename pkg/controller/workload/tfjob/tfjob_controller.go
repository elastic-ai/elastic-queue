/*
Copyright 2022 The ElasticAI authors.

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

package tfjob

import (
	"context"
	"fmt"
	tfcommonv1 "github.com/kubeflow/common/pkg/apis/common/v1"
	tfv1 "github.com/kubeflow/training-operator/pkg/apis/kubeflow.org/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	kueue "sigs.k8s.io/kueue/apis/kueue/v1alpha1"
	kueueconstants "sigs.k8s.io/kueue/pkg/constants"
	"sigs.k8s.io/kueue/pkg/workload"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	utilpriority "sigs.k8s.io/kueue/pkg/util/priority"
)

var (
	ownerKey = "tfjob.metadata.controller"
)

// TFJobReconciler reconciles a TFJob object
type TFJobReconciler struct {
	client                     client.Client
	scheme                     *runtime.Scheme
	record                     record.EventRecorder
	manageJobsWithoutQueueName bool
}

type options struct {
	manageJobsWithoutQueueName bool
}

// Option configures the reconciler.
type Option func(*options)

// WithManageJobsWithoutQueueName indicates if the controller should reconcile
// jobs that don't set the queue name annotation.
func WithManageJobsWithoutQueueName(f bool) Option {
	return func(o *options) {
		o.manageJobsWithoutQueueName = f
	}
}

var defaultOptions = options{}

func NewReconciler(
	scheme *runtime.Scheme,
	client client.Client,
	record record.EventRecorder,
	opts ...Option) *TFJobReconciler {

	options := defaultOptions
	for _, opt := range opts {
		opt(&options)
	}

	return &TFJobReconciler{
		scheme:                     scheme,
		client:                     client,
		record:                     record,
		manageJobsWithoutQueueName: options.manageJobsWithoutQueueName,
	}
}

// SetupWithManager sets up the controller with the Manager. It indexes workloads
// based on the owning jobs.
func (r *TFJobReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&tfv1.TFJob{}).
		Owns(&kueue.Workload{}).
		Complete(r)
}

func SetupIndexes(indexer client.FieldIndexer) error {
	return indexer.IndexField(context.Background(), &kueue.Workload{}, ownerKey, func(o client.Object) []string {
		// grab the Workload object, extract the owner...
		wl := o.(*kueue.Workload)
		owner := metav1.GetControllerOf(wl)
		if owner == nil {
			return nil
		}
		// ...make sure it's a Job...
		if owner.APIVersion != "kubeflow.org/v1" || owner.Kind != "TFJob" {
			return nil
		}
		// ...and if so, return it
		return []string{owner.Name}
	})
}

//+kubebuilder:rbac:groups=kubeflow.org,resources=tfjobs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=kubeflow.org,resources=tfjobs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=kubeflow.org,resources=tfjobs/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the TFJob object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.0/pkg/reconcile
func (r *TFJobReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var job tfv1.TFJob
	if err := r.client.Get(ctx, req.NamespacedName, &job); err != nil {
		// we'll ignore not-found errors, since there is nothing to do.
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	log := ctrl.LoggerFrom(ctx).WithValues("tfjob", klog.KObj(&job))
	ctx = ctrl.LoggerInto(ctx, log)

	if queueName(&job) == "" && !r.manageJobsWithoutQueueName {
		log.V(3).Info(fmt.Sprintf("%s annotation is not set, ignoring the tfjob", kueueconstants.QueueAnnotation))
		return ctrl.Result{}, nil
	}

	log.V(2).Info("Reconciling Job")

	var childWorkloads kueue.WorkloadList
	if err := r.client.List(ctx, &childWorkloads, client.InNamespace(req.Namespace),
		client.MatchingFields{ownerKey: req.Name}); err != nil {
		log.Error(err, "Unable to list child workloads")
		return ctrl.Result{}, err
	}

	// 1. make sure there is only a single existing instance of the workload
	wl, err := r.ensureAtMostOneWorkload(ctx, &job, childWorkloads)
	if err != nil {
		log.Error(err, "Getting existing workloads")
		return ctrl.Result{}, err
	}

	jobFinishedCond, jobFinished := jobFinishedCondition(&job)
	// 2. create new workload if none exists
	if wl == nil {
		// Nothing to do if the job is finished
		if jobFinished {
			return ctrl.Result{}, nil
		}
		err := r.handleJobWithNoWorkload(ctx, &job)
		if err != nil {
			log.Error(err, "Handling job with no workload")
		}
		return ctrl.Result{}, err
	}

	// 3. handle a finished job
	if jobFinished {
		added := false
		wl.Status.Conditions, added = appendFinishedConditionIfNotExists(wl.Status.Conditions, jobFinishedCond)
		if !added {
			return ctrl.Result{}, nil
		}
		err := r.client.Status().Update(ctx, wl)
		if err != nil {
			log.Error(err, "Updating workload status")
		}
		return ctrl.Result{}, err
	}

	// 4. Handle a not finished job
	if jobSuspended(&job) {
		// 4.1 start the job if the workload has been admitted, and the job is still suspended
		if wl.Spec.Admission != nil {
			log.V(2).Info("Job admitted, unsuspending")
			err := r.startJob(ctx, wl, &job)
			if err != nil {
				log.Error(err, "Unsuspending job")
			}
			return ctrl.Result{}, err
		}

		// 4.2 update queue name if changed.
		q := queueName(&job)
		if wl.Spec.QueueName != q {
			log.V(2).Info("Job changed queues, updating workload")
			wl.Spec.QueueName = q
			err := r.client.Update(ctx, wl)
			if err != nil {
				log.Error(err, "Updating workload queue")
			}
			return ctrl.Result{}, err
		}
		log.V(3).Info("Job is suspended and workload not yet admitted by a clusterQueue, nothing to do")
		return ctrl.Result{}, nil
	}

	if wl.Spec.Admission == nil {
		// 4.3 the job must be suspended if the workload is not yet admitted.
		log.V(2).Info("Running job is not admitted by a cluster queue, suspending")
		err := r.stopJob(ctx, wl, &job, "Not admitted by cluster queue")
		if err != nil {
			log.Error(err, "Suspending job with non admitted workload")
		}
		return ctrl.Result{}, err
	}

	// 4.4 workload is admitted and job is running, nothing to do.
	log.V(3).Info("Job running with admitted workload, nothing to do")
	return ctrl.Result{}, nil
}

func queueName(job *tfv1.TFJob) string {
	return job.Annotations[kueueconstants.QueueAnnotation]
}

// ensureAtmostoneworkload finds a matching workload and deletes redundant ones.
func (r *TFJobReconciler) ensureAtMostOneWorkload(ctx context.Context, job *tfv1.TFJob, workloads kueue.WorkloadList) (*kueue.Workload, error) {
	log := ctrl.LoggerFrom(ctx)

	// Find a matching workload first if there is one.
	var toDelete []*kueue.Workload
	var match *kueue.Workload
	for i := range workloads.Items {
		w := &workloads.Items[i]
		owner := metav1.GetControllerOf(w)
		// Indexes don't work in unit tests, so we explicitly check for the
		// owner here.
		if owner.Name != job.Name {
			continue
		}
		if match == nil && jobAndWorkloadEqual(job, w) {
			match = w
		} else {
			toDelete = append(toDelete, w)
		}
	}

	// If there is no matching workload and the job is running, suspend it.
	if match == nil && !jobSuspended(job) {
		log.V(2).Info("job with no matching workload, suspending")
		var w *kueue.Workload
		if len(workloads.Items) == 1 {
			// The job may have been modified and hence the existing workload
			// doesn't match the job anymore. All bets are off if there are more
			// than one workload...
			w = &workloads.Items[0]
		}
		if err := r.stopJob(ctx, w, job, "No matching Workload"); err != nil {
			log.Error(err, "stopping job")
		}
	}

	// Delete duplicate workload instances.
	existedWls := 0
	for i := range toDelete {
		err := r.client.Delete(ctx, toDelete[i])
		if err == nil || !apierrors.IsNotFound(err) {
			existedWls++
		}
		if err != nil && !apierrors.IsNotFound(err) {
			log.Error(err, "Failed to delete workload")
		}
		if err == nil {
			r.record.Eventf(job, corev1.EventTypeNormal, "DeletedWorkload",
				"Deleted not matching Workload: %v", workload.Key(toDelete[i]))
		}
	}

	if existedWls != 0 {
		if match == nil {
			return nil, fmt.Errorf("no matching workload was found, tried deleting %d existing workload(s)", existedWls)
		}
		return nil, fmt.Errorf("only one workload should exist, found %d", len(workloads.Items))
	}

	return match, nil
}

func jobAndWorkloadEqual(job *tfv1.TFJob, wl *kueue.Workload) bool {
	for _, podSet := range wl.Spec.PodSets {
		if replicaSpec, ok := job.Spec.TFReplicaSpecs[tfcommonv1.ReplicaType(podSet.Name)]; !ok && *replicaSpec.Replicas != podSet.Count {
			return false
		} else {
			// nodeSelector may change, hence we are not checking checking for
			// equality of the whole job.Spec.Template.Spec.
			if !equality.Semantic.DeepEqual(replicaSpec.Template.Spec.InitContainers, podSet.Spec.InitContainers) {
				return false
			}
			if !equality.Semantic.DeepEqual(replicaSpec.Template.Spec.Containers, podSet.Spec.Containers) {
				return false
			}
		}
	}

	return true
}

func jobFinishedCondition(j *tfv1.TFJob) (tfcommonv1.JobConditionType, bool) {
	for _, c := range j.Status.Conditions {
		if (c.Type == tfcommonv1.JobSucceeded || c.Type == tfcommonv1.JobFailed) && c.Status == corev1.ConditionTrue {
			return c.Type, true
		}
	}
	return "", false
}

func (r *TFJobReconciler) handleJobWithNoWorkload(ctx context.Context, job *tfv1.TFJob) error {
	log := ctrl.LoggerFrom(ctx)
	for k, v := range job.Status.ReplicaStatuses {
		// Wait until there are no active pods.
		if v.Active != 0 {
			log.V(2).Info("Job is suspended but still has active pods, waiting", "ReplicaType", k)
			return nil
		}
	}

	// Create the corresponding workload.
	wl, err := ConstructWorkloadFor(ctx, r.client, job, r.scheme)
	if err != nil {
		return err
	}
	if err = r.client.Create(ctx, wl); err != nil {
		return err
	}

	r.record.Eventf(job, corev1.EventTypeNormal, "CreatedWorkload",
		"Created Workload: %v", workload.Key(wl))
	return nil
}

func ConstructWorkloadFor(ctx context.Context, client client.Client,
	job *tfv1.TFJob, scheme *runtime.Scheme) (*kueue.Workload, error) {
	podsets := make([]kueue.PodSet, 0)
	for k, v := range job.Spec.TFReplicaSpecs {
		podsets = append(podsets, kueue.PodSet{
			Name:  string(k),
			Spec:  v.Template.Spec,
			Count: *v.Replicas,
		})
	}
	w := &kueue.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name:      job.Name,
			Namespace: job.Namespace,
		},
		Spec: kueue.WorkloadSpec{
			PodSets:   podsets,
			QueueName: queueName(job),
		},
	}

	priorityClass := ""
	if len(podsets) > 0 {
		// TODO
		priorityClass = podsets[0].Spec.PriorityClassName
	}
	// Populate priority from priority class.
	priorityClassName, p, err := utilpriority.GetPriorityFromPriorityClass(
		ctx, client, priorityClass)
	if err != nil {
		return nil, err
	}
	w.Spec.Priority = &p
	w.Spec.PriorityClassName = priorityClassName

	if err := ctrl.SetControllerReference(job, w, scheme); err != nil {
		return nil, err
	}

	return w, nil
}

// stopJob sends updates to suspend the job, reset the startTime so we can update the scheduling directives
// later when unsuspending and resets the nodeSelector to its previous state based on what is available in
// the workload (which should include the original affinities that the job had).
func (r *TFJobReconciler) stopJob(ctx context.Context, w *kueue.Workload,
	job *tfv1.TFJob, eventMsg string) error {
	job.Spec.Suspend = pointer.Bool(true)
	if err := r.client.Update(ctx, job); err != nil {
		return err
	}
	r.record.Eventf(job, corev1.EventTypeNormal, "Stopped", eventMsg)

	// Reset start time so we can update the scheduling directives later when unsuspending.
	if job.Status.StartTime != nil {
		job.Status.StartTime = nil
		if err := r.client.Status().Update(ctx, job); err != nil {
			return err
		}
	}

	for _, v := range job.Spec.TFReplicaSpecs {
		if w != nil && !equality.Semantic.DeepEqual(v.Template.Spec.NodeSelector,
			w.Spec.PodSets[0].Spec.NodeSelector) {
			v.Template.Spec.NodeSelector = map[string]string{}
			for k, v1 := range w.Spec.PodSets[0].Spec.NodeSelector {
				v.Template.Spec.NodeSelector[k] = v1
			}
			return r.client.Update(ctx, job)
		}
	}

	return nil
}

func (r *TFJobReconciler) startJob(ctx context.Context, w *kueue.Workload, job *tfv1.TFJob) error {
	log := ctrl.LoggerFrom(ctx)

	if len(w.Spec.PodSets) != 1 {
		return fmt.Errorf("one podset must exist, found %d", len(w.Spec.PodSets))
	}
	nodeSelector, err := r.getNodeSelectors(ctx, w)
	if err != nil {
		return err
	}
	if len(nodeSelector) != 0 {
		for _, v := range job.Spec.TFReplicaSpecs {
			if v.Template.Spec.NodeSelector == nil {
				v.Template.Spec.NodeSelector = nodeSelector
			} else {
				for k, v1 := range nodeSelector {
					v.Template.Spec.NodeSelector[k] = v1
				}
			}
		}
	} else {
		log.V(3).Info("no nodeSelectors to inject")
	}

	job.Spec.Suspend = pointer.Bool(false)
	if err := r.client.Update(ctx, job); err != nil {
		return err
	}

	r.record.Eventf(job, corev1.EventTypeNormal, "Started",
		"Admitted by clusterQueue %v", w.Spec.Admission.ClusterQueue)
	return nil
}

func (r *TFJobReconciler) getNodeSelectors(ctx context.Context, w *kueue.Workload) (map[string]string, error) {
	if len(w.Spec.Admission.PodSetFlavors[0].Flavors) == 0 {
		return nil, nil
	}

	processedFlvs := sets.NewString()
	nodeSelector := map[string]string{}
	for _, flvName := range w.Spec.Admission.PodSetFlavors[0].Flavors {
		if processedFlvs.Has(flvName) {
			continue
		}
		// Lookup the ResourceFlavors to fetch the node affinity labels to apply on the job.
		flv := kueue.ResourceFlavor{}
		if err := r.client.Get(ctx, types.NamespacedName{Name: flvName}, &flv); err != nil {
			return nil, err
		}
		for k, v := range flv.Labels {
			nodeSelector[k] = v
		}
		processedFlvs.Insert(flvName)
	}
	return nodeSelector, nil
}

func appendFinishedConditionIfNotExists(conds []kueue.WorkloadCondition, jobStatus tfcommonv1.JobConditionType) ([]kueue.WorkloadCondition, bool) {
	for i, c := range conds {
		if c.Type == kueue.WorkloadFinished {
			if c.Status == corev1.ConditionTrue {
				return conds, false
			}
			conds = append(conds[:i], conds[i+1:]...)
			break
		}
	}
	message := "Job finished successfully"
	if jobStatus == tfcommonv1.JobFailed {
		message = "Job failed"
	}
	now := metav1.Now()
	conds = append(conds, kueue.WorkloadCondition{
		Type:               kueue.WorkloadFinished,
		Status:             corev1.ConditionTrue,
		LastProbeTime:      now,
		LastTransitionTime: now,
		Reason:             "JobFinished",
		Message:            message,
	})
	return conds, true
}
func jobSuspended(j *tfv1.TFJob) bool {
	return j.Spec.Suspend != nil && *j.Spec.Suspend
}
