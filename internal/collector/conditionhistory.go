package collector

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/thomaschaplin/kmortem/api/v1alpha1"
)

// ConditionHistoryCollector records the current node conditions and enriches
// them with transition history sourced from Kubernetes Events.
type ConditionHistoryCollector struct {
	client client.Client
}

// NewConditionHistoryCollector creates a new ConditionHistoryCollector.
func NewConditionHistoryCollector(c client.Client) *ConditionHistoryCollector {
	return &ConditionHistoryCollector{client: c}
}

// Collect reads node conditions and related events, populating
// report.Spec.ConditionHistory.
func (c *ConditionHistoryCollector) Collect(ctx context.Context, node *corev1.Node, report *v1alpha1.NodeReport) error {
	// Primary source: live node conditions from the node object itself.
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

	// Supplement with events related to this node for richer history.
	var eventList corev1.EventList
	if err := c.client.List(ctx, &eventList,
		client.InNamespace("default"),
		client.MatchingFields{"involvedObject.name": node.Name, "involvedObject.kind": "Node"},
	); err != nil {
		// Non-fatal: we already have the conditions from the node object.
		// Record partial failure via the existing data.
		report.Spec.ConditionHistory = records
		return fmt.Errorf("conditionhistory: list events for node %s: %w (conditions captured)", node.Name, err)
	}

	// Add event-derived condition records for any condition-change events not
	// already captured. Events are supplementary — duplicates are acceptable
	// since they provide different timestamps/messages.
	for _, ev := range eventList.Items {
		if ev.Reason == "" {
			continue
		}
		t := ev.LastTimestamp.DeepCopy()
		records = append(records, v1alpha1.ConditionRecord{
			Type:               ev.Reason,
			Status:             string(ev.Type),
			Reason:             ev.Reason,
			Message:            ev.Message,
			LastTransitionTime: t,
		})
	}

	report.Spec.ConditionHistory = records
	return nil
}
