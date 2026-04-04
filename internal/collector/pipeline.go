package collector

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/thomaschaplin/kmortem/api/v1alpha1"
	kmortemconfig "github.com/thomaschaplin/kmortem/internal/config"
)

// Pipeline orchestrates the hot and warm collector groups in parallel.
// Hot collectors run against data that may disappear when the node dies.
// Warm collectors run against API server data that persists after node removal.
// Both groups start simultaneously.
type Pipeline struct {
	PodInventory     *PodInventoryCollector
	OOMKill          *OOMKillCollector
	ConditionHistory *ConditionHistoryCollector
	PDBViolation     *PDBViolationCollector

	client client.Client
}

// NewPipeline creates a Pipeline wiring all collectors.
func NewPipeline(c client.Client, _ *kmortemconfig.Config) *Pipeline {
	return &Pipeline{
		PodInventory:     NewPodInventoryCollector(c),
		OOMKill:          NewOOMKillCollector(c),
		ConditionHistory: NewConditionHistoryCollector(c),
		PDBViolation:     NewPDBViolationCollector(c),
		client:           c,
	}
}

// Run executes all collectors and finalises the NodeReport status.
// It is safe to call from a goroutine; the caller is responsible for
// ensuring ctx has an appropriate timeout.
func (p *Pipeline) Run(ctx context.Context, node *corev1.Node, report *v1alpha1.NodeReport) {
	// Populate metadata synchronously from the node object — no API calls needed.
	populateNodeMetadata(node, report)
	populateInstanceMetadata(node, report)

	var (
		mu       sync.Mutex
		hotErrs  []string
		warmErrs []string
	)

	appendErr := func(target *[]string, label string, err error) {
		if err != nil {
			mu.Lock()
			*target = append(*target, fmt.Sprintf("%s: %v", label, err))
			mu.Unlock()
		}
	}

	// Hot group: time-critical collectors — pod status is only accurate while
	// the kubelet is still reporting.
	hotGroup, hotCtx := errgroup.WithContext(ctx)
	hotGroup.Go(func() error {
		err := p.PodInventory.Collect(hotCtx, node, report)
		appendErr(&hotErrs, "podinventory", err)
		return nil
	})
	hotGroup.Go(func() error {
		err := p.OOMKill.Collect(hotCtx, node, report)
		appendErr(&hotErrs, "oomkill", err)
		return nil
	})

	// Warm group: API-server-backed collectors — data survives node removal.
	warmGroup, warmCtx := errgroup.WithContext(ctx)
	warmGroup.Go(func() error {
		err := p.ConditionHistory.Collect(warmCtx, node, report)
		appendErr(&warmErrs, "conditionhistory", err)
		return nil
	})
	warmGroup.Go(func() error {
		err := p.PDBViolation.Collect(warmCtx, node, report)
		appendErr(&warmErrs, "pdbviolation", err)
		return nil
	})

	// Both groups run in parallel; wait for both to finish.
	_ = hotGroup.Wait()
	_ = warmGroup.Wait()

	p.finalise(ctx, report, hotErrs, warmErrs)
}

// finalise sets the NodeReport status to Complete and records any collection
// failures as status conditions.
func (p *Pipeline) finalise(ctx context.Context, report *v1alpha1.NodeReport, hotErrs, warmErrs []string) {
	now := metav1.Now()
	report.Status.CollectionCompletedAt = &now
	report.Status.Phase = v1alpha1.StatusPhaseComplete
	report.Status.Message = "Collection complete"

	var conditions []v1alpha1.NodeReportStatusCondition

	if len(hotErrs) > 0 {
		conditions = append(conditions, v1alpha1.NodeReportStatusCondition{
			Type:               "HotCollectionPartialFailure",
			Status:             "True",
			Reason:             "CollectorError",
			Message:            strings.Join(hotErrs, "; "),
			LastTransitionTime: &now,
		})
	}

	if len(warmErrs) > 0 {
		conditions = append(conditions, v1alpha1.NodeReportStatusCondition{
			Type:               "WarmCollectionPartialFailure",
			Status:             "True",
			Reason:             "CollectorError",
			Message:            strings.Join(warmErrs, "; "),
			LastTransitionTime: &now,
		})
	}

	// Persist the collected spec data (pods, node metadata, etc.) to the API server.
	// Save the status we intend to write — client.Update refreshes report from
	// the server response, which has an empty status subresource and would
	// overwrite the fields we just set.
	if err := p.client.Update(ctx, report); err != nil {
		_ = err
	}

	// Create per-pod PodReport objects from the collected pod inventory.
	podReportErrs := p.createPodReports(ctx, report)
	if len(podReportErrs) > 0 {
		conditions = append(conditions, v1alpha1.NodeReportStatusCondition{
			Type:               "PodReportCreationPartialFailure",
			Status:             "True",
			Reason:             "PodReportCreationError",
			Message:            strings.Join(podReportErrs, "; "),
			LastTransitionTime: &now,
		})
	}

	report.Status.Conditions = conditions

	// Restore the status and persist it via the status subresource.
	status := report.Status
	report.Status = status
	if err := p.client.Status().Update(ctx, report); err != nil {
		_ = err
	}
}

// createPodReports creates a namespace-scoped PodReport for each pod in the
// NodeReport. Data is derived directly from the already-collected report spec —
// no additional API calls to the kubelet are needed.
func (p *Pipeline) createPodReports(ctx context.Context, report *v1alpha1.NodeReport) []string {
	type podKey struct{ ns, name string }

	// Build an OOM index keyed by namespace/podName for O(1) lookup.
	oomIndex := make(map[podKey][]v1alpha1.OOMKillRecord, len(report.Spec.OOMKills))
	for _, o := range report.Spec.OOMKills {
		k := podKey{o.Namespace, o.PodName}
		oomIndex[k] = append(oomIndex[k], o)
	}

	var errs []string
	for _, pod := range report.Spec.Pods {
		// Skip deduplication check if there's no UID to key on.
		if pod.PodUID != "" {
			var existing v1alpha1.PodReportList
			if err := p.client.List(ctx, &existing,
				client.InNamespace(pod.Namespace),
				client.MatchingFields{"spec.podUID": pod.PodUID},
			); err == nil && len(existing.Items) > 0 {
				continue
			}
		}

		k := podKey{pod.Namespace, pod.Name}
		pr := &v1alpha1.PodReport{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pod.Name,
				Namespace: pod.Namespace,
			},
			Spec: v1alpha1.PodReportSpec{
				NodeReportName:   report.Name,
				NodeReportUID:    string(report.UID),
				NodeName:         report.Spec.NodeName,
				TerminationCause: report.Spec.TerminationCause,
				PodName:          pod.Name,
				PodUID:           pod.PodUID,
				Namespace:        pod.Namespace,
				OwnerKind:        pod.OwnerKind,
				OwnerName:        pod.OwnerName,
				StartTime:        pod.StartTime,
				EndTime:          pod.EndTime,
				ExitReason:       pod.ExitReason,
				Phase:            pod.Phase,
				CPURequest:       pod.CPURequest,
				MemoryRequest:    pod.MemoryRequest,
				CPULimit:         pod.CPULimit,
				MemoryLimit:      pod.MemoryLimit,
				RestartCount:     pod.RestartCount,
				Containers:       pod.Containers,
				OOMKills:         oomIndex[k],
			},
		}

		if err := p.client.Create(ctx, pr); err != nil {
			if !k8serrors.IsAlreadyExists(err) {
				errs = append(errs, fmt.Sprintf("podreport %s/%s: %v", pod.Namespace, pod.Name, err))
			}
			continue
		}

		pr.Status = v1alpha1.PodReportStatus{
			Phase:   v1alpha1.StatusPhaseComplete,
			Message: "Created from NodeReport " + report.Name,
		}
		if err := p.client.Status().Update(ctx, pr); err != nil {
			errs = append(errs, fmt.Sprintf("podreport status %s/%s: %v", pod.Namespace, pod.Name, err))
		}
	}
	return errs
}

// populateInstanceMetadata reads AWS instance metadata from node labels and
// spec.providerID — no API calls required.
func populateInstanceMetadata(node *corev1.Node, report *v1alpha1.NodeReport) {
	collector := NewInstanceMetadataCollector()
	_ = collector.Collect(context.Background(), node, report)
}

// populateNodeMetadata copies static node system info into the report.
func populateNodeMetadata(node *corev1.Node, report *v1alpha1.NodeReport) {
	ni := node.Status.NodeInfo
	createdAt := node.CreationTimestamp.DeepCopy()
	report.Spec.NodeMetadata = v1alpha1.NodeMetadata{
		KernelVersion:    ni.KernelVersion,
		OSImage:          ni.OSImage,
		ContainerRuntime: ni.ContainerRuntimeVersion,
		KubeletVersion:   ni.KubeletVersion,
		CreatedAt:        createdAt,
		Labels:           node.Labels,
	}

	if cpu, ok := node.Status.Allocatable[corev1.ResourceCPU]; ok {
		report.Spec.NodeMetadata.AllocatableCPU = cpu.String()
	}
	if mem, ok := node.Status.Allocatable[corev1.ResourceMemory]; ok {
		report.Spec.NodeMetadata.AllocatableMemory = mem.String()
	}
	if cpu, ok := node.Status.Capacity[corev1.ResourceCPU]; ok {
		report.Spec.NodeMetadata.CapacityCPU = cpu.String()
	}
	if mem, ok := node.Status.Capacity[corev1.ResourceMemory]; ok {
		report.Spec.NodeMetadata.CapacityMemory = mem.String()
	}

	taints := make([]v1alpha1.TaintRecord, 0, len(node.Spec.Taints))
	for _, t := range node.Spec.Taints {
		taints = append(taints, v1alpha1.TaintRecord{
			Key:    t.Key,
			Value:  t.Value,
			Effect: string(t.Effect),
		})
	}
	report.Spec.NodeMetadata.Taints = taints
}
