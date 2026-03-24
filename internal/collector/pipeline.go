package collector

import (
	"context"
	"fmt"
	"strings"
	"sync"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"golang.org/x/sync/errgroup"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/thomaschaplin/kmortem/api/v1alpha1"
	kmortemconfig "github.com/thomaschaplin/kmortem/internal/config"
)

// Pipeline orchestrates the hot and warm collector groups in parallel.
// Hot collectors run against data that may disappear when the node dies.
// Warm collectors run against API server data that persists after node removal.
// Both groups start simultaneously.
type Pipeline struct {
	InstanceMetadata *InstanceMetadataCollector
	PodInventory     *PodInventoryCollector
	OOMKill          *OOMKillCollector
	ConditionHistory *ConditionHistoryCollector
	PDBViolation     *PDBViolationCollector

	client client.Client
}

// NewPipeline creates a Pipeline wiring all collectors.
func NewPipeline(c client.Client, _ *kmortemconfig.Config) *Pipeline {
	return &Pipeline{
		InstanceMetadata: NewInstanceMetadataCollector(),
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
	// Capture node metadata directly from the node object — always available.
	populateNodeMetadata(node, report)

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

	// Hot group: time-critical collectors — node IP may become unreachable soon.
	hotGroup, hotCtx := errgroup.WithContext(ctx)
	hotGroup.Go(func() error {
		err := p.InstanceMetadata.Collect(hotCtx, node, report)
		appendErr(&hotErrs, "instancemetadata", err)
		return nil // never propagate — partial results are acceptable
	})
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

	report.Status.Conditions = conditions

	// Persist the completed status back to the API server.
	if err := p.client.Status().Update(ctx, report); err != nil {
		// Best-effort — the report data is still in the object even if the
		// status write fails; the NodeReportReconciler will fix it on next requeue.
		_ = err
	}
}

// populateNodeMetadata copies static node system info into the report.
func populateNodeMetadata(node *corev1.Node, report *v1alpha1.NodeReport) {
	ni := node.Status.NodeInfo
	report.Spec.NodeMetadata = v1alpha1.NodeMetadata{
		KernelVersion:    ni.KernelVersion,
		OSImage:          ni.OSImage,
		ContainerRuntime: ni.ContainerRuntimeVersion,
		KubeletVersion:   ni.KubeletVersion,
	}

	if cpu, ok := node.Status.Allocatable[corev1.ResourceCPU]; ok {
		report.Spec.NodeMetadata.AllocatableCPU = cpu.String()
	}
	if mem, ok := node.Status.Allocatable[corev1.ResourceMemory]; ok {
		report.Spec.NodeMetadata.AllocatableMemory = mem.String()
	}
}
