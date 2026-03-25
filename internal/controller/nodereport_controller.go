package controller

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/thomaschaplin/kmortem/api/v1alpha1"
	"github.com/thomaschaplin/kmortem/internal/config"
)

const (
	// collectingRequeueAfter is how often to check a Collecting NodeReport.
	collectingRequeueAfter = 5 * time.Second
)

// NodeReportReconciler manages the lifecycle of NodeReport objects:
// TTL expiry and deletion.
type NodeReportReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	Config   *config.Config
}

// Reconcile handles NodeReport lifecycle transitions.
func (r *NodeReportReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var report v1alpha1.NodeReport
	if err := r.Get(ctx, req.NamespacedName, &report); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("get NodeReport %s: %w", req.Name, err)
	}

	switch report.Status.Phase {
	case v1alpha1.StatusPhaseCollecting, "":
		// Requeue to check if collection has finished.
		return ctrl.Result{RequeueAfter: collectingRequeueAfter}, nil

	case v1alpha1.StatusPhaseComplete:
		return r.reconcileComplete(ctx, &report)

	default:
		logger.Info("NodeReport in unexpected phase, ignoring", "phase", report.Status.Phase)
		return ctrl.Result{}, nil
	}
}

// reconcileComplete handles a NodeReport in the Complete phase.
// It deletes the report once the TTL has expired.
func (r *NodeReportReconciler) reconcileComplete(ctx context.Context, report *v1alpha1.NodeReport) (ctrl.Result, error) {
	ttl := r.Config.Retention.TTL
	expiry := report.CreationTimestamp.Add(ttl)
	requeueAfter := time.Until(expiry)

	if requeueAfter > 0 {
		// TTL not yet reached — requeue precisely at expiry.
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}

	return r.deleteReport(ctx, report)
}

// deleteReport deletes the NodeReport object from the cluster.
func (r *NodeReportReconciler) deleteReport(ctx context.Context, report *v1alpha1.NodeReport) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("deleting NodeReport", "nodereport", report.Name)

	if err := r.Delete(ctx, report); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("delete NodeReport %s: %w", report.Name, err)
	}
	return ctrl.Result{}, nil
}

// transitionTo updates the NodeReport status to the given phase.
func (r *NodeReportReconciler) transitionTo(ctx context.Context, report *v1alpha1.NodeReport, phase v1alpha1.StatusPhase, message string) (ctrl.Result, error) {
	report.Status.Phase = phase
	report.Status.Message = message

	now := metav1.Now()
	newCondition := v1alpha1.NodeReportStatusCondition{
		Type:               string(phase),
		Status:             "True",
		Reason:             string(phase),
		Message:            message,
		LastTransitionTime: &now,
	}
	upserted := false
	for i, c := range report.Status.Conditions {
		if c.Type == string(phase) {
			report.Status.Conditions[i] = newCondition
			upserted = true
			break
		}
	}
	if !upserted {
		report.Status.Conditions = append(report.Status.Conditions, newCondition)
	}

	if err := r.Status().Update(ctx, report); err != nil {
		return ctrl.Result{}, fmt.Errorf("update NodeReport %s status to %s: %w", report.Name, phase, err)
	}
	return ctrl.Result{}, nil
}

// SetupWithManager registers the NodeReportReconciler with the controller manager.
func (r *NodeReportReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.NodeReport{}).
		Complete(r)
}
