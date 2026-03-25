package collector

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/thomaschaplin/kmortem/api/v1alpha1"
)

// ConditionHistoryCollector records the current node conditions.
type ConditionHistoryCollector struct {
	client client.Client
}

// NewConditionHistoryCollector creates a new ConditionHistoryCollector.
func NewConditionHistoryCollector(c client.Client) *ConditionHistoryCollector {
	return &ConditionHistoryCollector{client: c}
}

// Collect reads node conditions from the node object, populating
// report.Spec.ConditionHistory. Status values are always "True", "False",
// or "Unknown".
func (c *ConditionHistoryCollector) Collect(_ context.Context, node *corev1.Node, report *v1alpha1.NodeReport) error {
	records := make([]v1alpha1.ConditionRecord, 0, len(node.Status.Conditions))
	for _, cond := range node.Status.Conditions {
		t := cond.LastTransitionTime.DeepCopy()
		records = append(records, v1alpha1.ConditionRecord{
			Type:               string(cond.Type),
			Status:             string(cond.Status),
			Reason:             cond.Reason,
			Message:            cond.Message,
			LastTransitionTime: t,
		})
	}
	report.Spec.ConditionHistory = records
	return nil
}
