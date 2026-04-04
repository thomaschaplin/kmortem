package controller

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/thomaschaplin/kmortem/api/v1alpha1"
	"github.com/thomaschaplin/kmortem/internal/config"
)

// PodReportReconciler manages the TTL lifecycle of PodReport objects.
//
// +kubebuilder:rbac:groups=kmortem.io,resources=podreports,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=kmortem.io,resources=podreports/status,verbs=get;update;patch
type PodReportReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Config *config.Config
}

// Reconcile handles PodReport lifecycle: deletes after TTL from creation.
func (r *PodReportReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var pr v1alpha1.PodReport
	if err := r.Get(ctx, req.NamespacedName, &pr); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("get PodReport %s/%s: %w", req.Namespace, req.Name, err)
	}

	// PodReports are always born Complete; only TTL expiry triggers deletion.
	if pr.Status.Phase != v1alpha1.StatusPhaseComplete {
		return ctrl.Result{RequeueAfter: collectingRequeueAfter}, nil
	}

	ttl := r.Config.Retention.TTL
	expiry := pr.CreationTimestamp.Add(ttl)
	requeueAfter := time.Until(expiry)

	if requeueAfter > 0 {
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}

	logger.Info("deleting PodReport", "namespace", pr.Namespace, "podreport", pr.Name)
	if err := r.Delete(ctx, &pr); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("delete PodReport %s/%s: %w", pr.Namespace, pr.Name, err)
	}
	return ctrl.Result{}, nil
}

// SetupWithManager registers the PodReportReconciler with the controller manager.
func (r *PodReportReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.PodReport{}).
		Complete(r)
}
