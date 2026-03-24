package controller

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/thomaschaplin/kmortem/api/v1alpha1"
	"github.com/thomaschaplin/kmortem/internal/archiver"
	"github.com/thomaschaplin/kmortem/internal/config"
)

const (
	// maxArchiveRetries before giving up and deleting anyway.
	maxArchiveRetries = 5
	// archiveRetryBackoffBase is the base for exponential backoff.
	archiveRetryBackoffBase = 30 * time.Second
	// collectingRequeueAfter is how often to check a Collecting NodeReport.
	collectingRequeueAfter = 5 * time.Second
)

// NodeReportReconciler manages the lifecycle of NodeReport objects:
// TTL expiry, S3 archival, and deletion.
type NodeReportReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	Config   *config.Config
	Archiver archiver.Archiver
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
	case v1alpha1.StatusPhaseCollecting:
		// Requeue to check if collection has finished.
		return ctrl.Result{RequeueAfter: collectingRequeueAfter}, nil

	case v1alpha1.StatusPhaseComplete:
		return r.reconcileComplete(ctx, &report)

	case v1alpha1.StatusPhasePendingDeletion:
		return r.reconcileArchive(ctx, &report)

	case v1alpha1.StatusPhaseArchiveFailed:
		return r.reconcileArchiveFailed(ctx, &report)

	case v1alpha1.StatusPhaseArchived:
		return r.deleteReport(ctx, &report)

	default:
		if report.Status.Phase == "" {
			// Freshly created but status not yet set — requeue.
			return ctrl.Result{RequeueAfter: collectingRequeueAfter}, nil
		}
		logger.Info("NodeReport in unexpected phase, ignoring", "phase", report.Status.Phase)
		return ctrl.Result{}, nil
	}
}

// reconcileComplete handles a NodeReport in the Complete phase.
// It checks whether the TTL has expired and acts accordingly.
func (r *NodeReportReconciler) reconcileComplete(ctx context.Context, report *v1alpha1.NodeReport) (ctrl.Result, error) {
	ttl := r.Config.Retention.TTL
	expiry := report.CreationTimestamp.Add(ttl)
	requeueAfter := time.Until(expiry)

	if requeueAfter > 0 {
		// TTL not yet reached — requeue precisely at expiry.
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}

	// TTL expired.
	if r.Config.Archival.Enabled {
		return r.transitionTo(ctx, report, v1alpha1.StatusPhasePendingDeletion, "TTL expired, archival pending")
	}
	// No archival configured — delete immediately.
	return r.deleteReport(ctx, report)
}

// reconcileArchive attempts to write the NodeReport to S3.
func (r *NodeReportReconciler) reconcileArchive(ctx context.Context, report *v1alpha1.NodeReport) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if r.Archiver == nil {
		// Archival not configured; proceed to delete.
		return r.deleteReport(ctx, report)
	}

	if err := r.Archiver.Archive(ctx, report); err != nil {
		logger.Error(err, "S3 archival failed", "nodereport", report.Name)

		report.Status.ArchiveFailureCount++
		backoff := archiveBackoff(report.Status.ArchiveFailureCount)

		_, updateErr := r.transitionTo(ctx, report, v1alpha1.StatusPhaseArchiveFailed,
			fmt.Sprintf("S3 write failed: %v (attempt %d)", err, report.Status.ArchiveFailureCount))
		if updateErr != nil {
			return ctrl.Result{}, updateErr
		}
		return ctrl.Result{RequeueAfter: backoff}, nil
	}

	// Archive succeeded.
	return r.transitionTo(ctx, report, v1alpha1.StatusPhaseArchived, "Successfully archived to S3")
}

// reconcileArchiveFailed retries archival with exponential backoff, or deletes
// the report if max retries have been exceeded.
func (r *NodeReportReconciler) reconcileArchiveFailed(ctx context.Context, report *v1alpha1.NodeReport) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if report.Status.ArchiveFailureCount >= maxArchiveRetries {
		logger.Info("max archive retries exceeded, deleting NodeReport without archiving",
			"nodereport", report.Name,
			"attempts", report.Status.ArchiveFailureCount,
		)
		r.Recorder.Event(report, corev1.EventTypeWarning, "ArchiveMaxRetriesExceeded",
			fmt.Sprintf("S3 archival failed after %d attempts; deleting report", report.Status.ArchiveFailureCount))
		return r.deleteReport(ctx, report)
	}

	// Retry archival.
	return r.reconcileArchive(ctx, report)
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
	report.Status.Conditions = append(report.Status.Conditions, v1alpha1.NodeReportStatusCondition{
		Type:               string(phase),
		Status:             "True",
		Reason:             string(phase),
		Message:            message,
		LastTransitionTime: &now,
	})

	if err := r.Status().Update(ctx, report); err != nil {
		return ctrl.Result{}, fmt.Errorf("update NodeReport %s status to %s: %w", report.Name, phase, err)
	}
	return ctrl.Result{}, nil
}

// archiveBackoff returns the backoff duration for the nth archive attempt.
func archiveBackoff(attempt int) time.Duration {
	if attempt <= 0 {
		return archiveRetryBackoffBase
	}
	d := archiveRetryBackoffBase
	for i := 1; i < attempt; i++ {
		d *= 2
	}
	maxBackoff := 10 * time.Minute
	if d > maxBackoff {
		return maxBackoff
	}
	return d
}

// SetupWithManager registers the NodeReportReconciler with the controller manager.
func (r *NodeReportReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.NodeReport{}).
		Complete(r)
}
