package collector

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/thomaschaplin/kmortem/api/v1alpha1"
)

// OOMKillCollector scans container statuses on the node for OOMKilled events.
type OOMKillCollector struct {
	client client.Client
}

// NewOOMKillCollector creates a new OOMKillCollector.
func NewOOMKillCollector(c client.Client) *OOMKillCollector {
	return &OOMKillCollector{client: c}
}

// Collect scans pods on the node for OOMKill events and populates report.Spec.OOMKills.
func (c *OOMKillCollector) Collect(ctx context.Context, node *corev1.Node, report *v1alpha1.NodeReport) error {
	var podList corev1.PodList
	if err := c.client.List(ctx, &podList, client.MatchingFields{"spec.nodeName": node.Name}); err != nil {
		return fmt.Errorf("oomkill: list pods on node %s: %w", node.Name, err)
	}

	var records []v1alpha1.OOMKillRecord

	for _, pod := range podList.Items {
		for _, cs := range pod.Status.ContainerStatuses {
			if rec := oomKillRecord(&pod, &cs); rec != nil {
				records = append(records, *rec)
			}
		}
		for _, cs := range pod.Status.InitContainerStatuses {
			if rec := oomKillRecord(&pod, &cs); rec != nil {
				records = append(records, *rec)
			}
		}
	}

	report.Spec.OOMKills = records
	return nil
}

// oomKillRecord checks a container status and returns an OOMKillRecord if the
// container was OOMKilled, checking both current and last termination state.
func oomKillRecord(pod *corev1.Pod, cs *corev1.ContainerStatus) *v1alpha1.OOMKillRecord {
	terminated := cs.State.Terminated
	if terminated == nil {
		terminated = cs.LastTerminationState.Terminated
	}
	if terminated == nil || terminated.Reason != "OOMKilled" {
		return nil
	}

	limit := memoryLimitForContainer(pod, cs.Name)
	ts := terminated.FinishedAt

	return &v1alpha1.OOMKillRecord{
		PodName:       pod.Name,
		Namespace:     pod.Namespace,
		ContainerName: cs.Name,
		MemoryLimit:   limit,
		Timestamp:     timestampOrNil(ts),
	}
}

// memoryLimitForContainer looks up the memory limit for a named container in
// the pod spec. Returns empty string if no limit is set.
func memoryLimitForContainer(pod *corev1.Pod, containerName string) string {
	containers := append(pod.Spec.Containers, pod.Spec.InitContainers...)
	for _, c := range containers {
		if c.Name == containerName {
			if limit, ok := c.Resources.Limits[corev1.ResourceMemory]; ok {
				return formatQuantity(limit)
			}
			return ""
		}
	}
	return ""
}

func formatQuantity(q resource.Quantity) string {
	return q.String()
}

func timestampOrNil(t metav1.Time) *metav1.Time {
	if t.IsZero() {
		return nil
	}
	return t.DeepCopy()
}
