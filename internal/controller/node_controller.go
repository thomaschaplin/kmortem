package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/thomaschaplin/kmortem/api/v1alpha1"
	"github.com/thomaschaplin/kmortem/internal/collector"
)

const (
	// collectionTimeout is the maximum time allowed for the collection pipeline.
	// Chosen to fit comfortably within the 2-minute AWS spot interruption window.
	collectionTimeout = 90 * time.Second

	// nodeUnschedulableTaint is applied when a node is cordoned.
	nodeUnschedulableTaint = "node.kubernetes.io/unschedulable"

	// clusterAutoscalerAnnotation marks nodes queued for scale-down.
	clusterAutoscalerAnnotation = "cluster-autoscaler.kubernetes.io/scale-down-disabled"
	clusterAutoscalerScaleDown  = "cluster-autoscaler.kubernetes.io/scale-down"

	// Karpenter signals.
	karpenterDisruptionTaint   = "karpenter.sh/disruption"
	karpenterCapacityTypeLabel = "karpenter.sh/capacity-type"
)

// NodeReconciler watches Node objects and creates a NodeReport when a node
// is detected as terminating.
//
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=get;list;watch
// +kubebuilder:rbac:groups=policy,resources=poddisruptionbudgets,verbs=get;list;watch
// +kubebuilder:rbac:groups=kmortem.io,resources=nodereports,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=kmortem.io,resources=nodereports/status,verbs=get;update;patch
type NodeReconciler struct {
	client.Client
	// RootCtx is the operator's root context. Collection goroutines must use
	// this context (not the reconcile context, which is cancelled on return).
	RootCtx  context.Context
	Pipeline *collector.Pipeline
	// inFlight guards against launching multiple collection goroutines for the
	// same node. Key: node UID (string), Value: struct{}.
	inFlight sync.Map
}

// Reconcile is called for every Node event. It detects termination signals,
// deduplicates, and dispatches a collection goroutine.
func (r *NodeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var node corev1.Node
	if err := r.Get(ctx, req.NamespacedName, &node); err != nil {
		if errors.IsNotFound(err) {
			// Node is already gone; nothing to do.
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("get node %s: %w", req.Name, err)
	}

	if !isTerminating(&node) {
		return ctrl.Result{}, nil
	}

	nodeUID := string(node.UID)

	// Check if a NodeReport already exists for this node UID to avoid duplicates.
	if exists, err := r.nodeReportExists(ctx, nodeUID); err != nil {
		return ctrl.Result{}, err
	} else if exists {
		return ctrl.Result{}, nil
	}

	// Check if collection is already in flight for this node.
	if _, loaded := r.inFlight.LoadOrStore(nodeUID, struct{}{}); loaded {
		return ctrl.Result{}, nil
	}

	cause := classifyTerminationCause(ctx, &node)
	logger.Info("node termination detected", "node", node.Name, "cause", cause)

	// Create the NodeReport with phase=Collecting.
	report, err := r.createNodeReport(ctx, &node, cause)
	if err != nil {
		r.inFlight.Delete(nodeUID)
		return ctrl.Result{}, fmt.Errorf("create NodeReport for node %s: %w", node.Name, err)
	}

	// Spawn the collection goroutine using the operator's root context so it
	// outlives this reconcile call.
	collectionCtx, cancel := context.WithTimeout(r.RootCtx, collectionTimeout)

	go func() {
		defer cancel()
		defer r.inFlight.Delete(nodeUID)
		r.Pipeline.Run(collectionCtx, &node, report)
	}()

	return ctrl.Result{}, nil
}

// isTerminating returns true if the node is showing termination signals.
func isTerminating(node *corev1.Node) bool {
	// Signal 1: unschedulable taint (node cordoned / drain initiated).
	for _, taint := range node.Spec.Taints {
		if taint.Key == nodeUnschedulableTaint {
			return true
		}
	}

	// Signal 2: Ready condition flipped to False or Unknown.
	for _, cond := range node.Status.Conditions {
		if cond.Type == corev1.NodeReady {
			if cond.Status == corev1.ConditionFalse || cond.Status == corev1.ConditionUnknown {
				return true
			}
		}
	}

	// Signal 3: cluster-autoscaler scale-down annotation.
	if _, ok := node.Annotations[clusterAutoscalerScaleDown]; ok {
		return true
	}

	// Signal 4: DeletionTimestamp set (last resort — race-condition risk).
	if node.DeletionTimestamp != nil {
		return true
	}

	return false
}

// classifyTerminationCause uses available signals to determine why the node
// is being terminated.
func classifyTerminationCause(_ context.Context, node *corev1.Node) v1alpha1.TerminationCause {
	// Karpenter spot interruption: disruption taint present on a spot node.
	if isKarpenterDisrupting(node) && isSpotNode(node) {
		return v1alpha1.TerminationCauseSpotInterruption
	}

	// Cluster autoscaler annotation.
	if _, ok := node.Annotations[clusterAutoscalerScaleDown]; ok {
		return v1alpha1.TerminationCauseClusterAutoscalerScaleDown
	}

	// Node condition failures (MemoryPressure, DiskPressure, PIDPressure).
	for _, cond := range node.Status.Conditions {
		switch cond.Type {
		case corev1.NodeMemoryPressure, corev1.NodeDiskPressure, corev1.NodePIDPressure:
			if cond.Status == corev1.ConditionTrue {
				return v1alpha1.TerminationCauseNodeConditionFailure
			}
		}
	}

	// Manual drain: unschedulable taint without other signals.
	for _, taint := range node.Spec.Taints {
		if taint.Key == nodeUnschedulableTaint {
			return v1alpha1.TerminationCauseManualDrain
		}
	}

	return v1alpha1.TerminationCauseUnknown
}

// isKarpenterDisrupting returns true if Karpenter has set its disruption taint,
// indicating it is actively draining the node.
func isKarpenterDisrupting(node *corev1.Node) bool {
	for _, taint := range node.Spec.Taints {
		if taint.Key == karpenterDisruptionTaint && taint.Effect == corev1.TaintEffectNoSchedule {
			return true
		}
	}
	return false
}

// isSpotNode returns true if the node is a spot instance, based on Karpenter
// or EKS capacity type labels.
func isSpotNode(node *corev1.Node) bool {
	labels := node.Labels
	if labels[karpenterCapacityTypeLabel] == "spot" {
		return true
	}
	if strings.ToUpper(labels["eks.amazonaws.com/capacityType"]) == "SPOT" {
		return true
	}
	return false
}

// nodeReportExists returns true if a NodeReport already exists with the given
// node UID in spec.nodeUID.
func (r *NodeReconciler) nodeReportExists(ctx context.Context, nodeUID string) (bool, error) {
	var list v1alpha1.NodeReportList
	if err := r.List(ctx, &list, client.MatchingFields{"spec.nodeUID": nodeUID}); err != nil {
		return false, fmt.Errorf("list NodeReports: %w", err)
	}
	return len(list.Items) > 0, nil
}

// createNodeReport creates a new NodeReport object for the terminating node.
func (r *NodeReconciler) createNodeReport(ctx context.Context, node *corev1.Node, cause v1alpha1.TerminationCause) (*v1alpha1.NodeReport, error) {
	now := metav1.Now()

	report := &v1alpha1.NodeReport{
		ObjectMeta: metav1.ObjectMeta{
			Name: node.Name,
		},
		Spec: v1alpha1.NodeReportSpec{
			NodeName:         node.Name,
			NodeUID:          string(node.UID),
			TerminationTime:  &now,
			TerminationCause: cause,
			InitiatedBy:      resolveInitiatedBy(node),
		},
		Status: v1alpha1.NodeReportStatus{
			Phase:   v1alpha1.StatusPhaseCollecting,
			Message: "Evidence collection in progress",
		},
	}

	if err := r.Create(ctx, report); err != nil {
		if errors.IsAlreadyExists(err) {
			// Fetch the existing report to continue collection against it.
			if getErr := r.Get(ctx, types.NamespacedName{Name: node.Name}, report); getErr != nil {
				return nil, getErr
			}
			return report, nil
		}
		return nil, err
	}

	return report, nil
}

// resolveInitiatedBy inspects the node's managed fields to determine which
// field manager last set spec.taints, indicating who triggered the drain.
func resolveInitiatedBy(node *corev1.Node) string {
	// Cluster-autoscaler is identified by annotation.
	if _, ok := node.Annotations[clusterAutoscalerScaleDown]; ok {
		return "cluster-autoscaler"
	}

	// For everything else, find which field manager owns spec.taints.
	for _, mf := range node.ManagedFields {
		if mf.FieldsV1 == nil {
			continue
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(mf.FieldsV1.Raw, &fields); err != nil {
			continue
		}
		specRaw, ok := fields["f:spec"]
		if !ok {
			continue
		}
		var spec map[string]json.RawMessage
		if err := json.Unmarshal(specRaw, &spec); err != nil {
			continue
		}
		if _, hasTaints := spec["f:taints"]; hasTaints {
			return mf.Manager
		}
	}
	return ""
}

// SetupWithManager registers the NodeReconciler with the controller manager.
func (r *NodeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Node{}).
		Complete(r)
}
