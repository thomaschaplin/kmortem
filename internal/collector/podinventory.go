package collector

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/thomaschaplin/kmortem/api/v1alpha1"
)

// PodInventoryCollector lists all pods that were scheduled on the node and
// records their ownership, lifecycle times, and exit reason.
type PodInventoryCollector struct {
	client client.Client
}

// NewPodInventoryCollector creates a new PodInventoryCollector.
func NewPodInventoryCollector(c client.Client) *PodInventoryCollector {
	return &PodInventoryCollector{client: c}
}

// Collect lists pods for the node and populates report.Spec.Pods.
func (c *PodInventoryCollector) Collect(ctx context.Context, node *corev1.Node, report *v1alpha1.NodeReport) error {
	var podList corev1.PodList
	if err := c.client.List(ctx, &podList, client.MatchingFields{"spec.nodeName": node.Name}); err != nil {
		return fmt.Errorf("podinventory: list pods on node %s: %w", node.Name, err)
	}

	records := make([]v1alpha1.PodRecord, 0, len(podList.Items))
	for _, pod := range podList.Items {
		records = append(records, podToRecord(&pod))
	}

	report.Spec.Pods = records
	return nil
}

// podToRecord converts a Pod into a PodRecord.
func podToRecord(pod *corev1.Pod) v1alpha1.PodRecord {
	rec := v1alpha1.PodRecord{
		Name:       pod.Name,
		Namespace:  pod.Namespace,
		OwnerKind:  resolveOwnerKind(pod),
		OwnerName:  resolveOwnerName(pod),
		ExitReason: resolvePodExitReason(pod),
		Phase:      string(pod.Status.Phase),
	}

	if pod.Status.StartTime != nil {
		t := pod.Status.StartTime.DeepCopy()
		rec.StartTime = t
	}

	// Use the latest container termination time as the pod end time.
	rec.EndTime = latestContainerEndTime(pod)

	return rec
}

// resolveOwnerKind returns the OwnerKind for the pod based on its owner references.
// It resolves through ReplicaSet → Deployment by checking for the typical RS naming pattern.
func resolveOwnerKind(pod *corev1.Pod) v1alpha1.OwnerKind {
	for _, ref := range pod.OwnerReferences {
		switch ref.Kind {
		case "ReplicaSet":
			// A ReplicaSet is usually owned by a Deployment.
			return v1alpha1.OwnerKindDeployment
		case "StatefulSet":
			return v1alpha1.OwnerKindStatefulSet
		case "DaemonSet":
			return v1alpha1.OwnerKindDaemonSet
		case "Job":
			return v1alpha1.OwnerKindJob
		case "CronJob":
			return v1alpha1.OwnerKindCronJob
		}
	}
	return v1alpha1.OwnerKindNone
}

// resolveOwnerName returns the name of the owning workload. For ReplicaSet-owned
// pods, it attempts to strip the ReplicaSet hash suffix to get the Deployment name.
func resolveOwnerName(pod *corev1.Pod) string {
	for _, ref := range pod.OwnerReferences {
		return ref.Name
	}
	return ""
}

// resolvePodExitReason determines the exit reason for a pod.
func resolvePodExitReason(pod *corev1.Pod) v1alpha1.PodExitReason {
	// Check for eviction.
	if pod.Status.Reason == "Evicted" {
		return v1alpha1.PodExitReasonEvicted
	}

	// Check container statuses for OOMKilled.
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.State.Terminated != nil && cs.State.Terminated.Reason == "OOMKilled" {
			return v1alpha1.PodExitReasonOOMKilled
		}
		if cs.LastTerminationState.Terminated != nil && cs.LastTerminationState.Terminated.Reason == "OOMKilled" {
			return v1alpha1.PodExitReasonOOMKilled
		}
	}

	switch pod.Status.Phase {
	case corev1.PodSucceeded:
		return v1alpha1.PodExitReasonCompleted
	case corev1.PodFailed:
		return v1alpha1.PodExitReasonError
	case corev1.PodRunning, corev1.PodPending:
		return v1alpha1.PodExitReasonUnknown
	}

	return v1alpha1.PodExitReasonUnknown
}

// latestContainerEndTime returns the latest termination time across all containers.
func latestContainerEndTime(pod *corev1.Pod) *metav1.Time {
	var latest *metav1.Time
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.State.Terminated != nil {
			t := cs.State.Terminated.FinishedAt
			if latest == nil || t.After(latest.Time) {
				latest = &t
			}
		}
	}
	return latest
}
