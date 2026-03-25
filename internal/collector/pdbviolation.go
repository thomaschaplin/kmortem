package collector

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/thomaschaplin/kmortem/api/v1alpha1"
)

// PDBViolationCollector checks whether any PodDisruptionBudgets are blocking or
// were violated during the drain of the node.
type PDBViolationCollector struct {
	client client.Client
}

// NewPDBViolationCollector creates a new PDBViolationCollector.
func NewPDBViolationCollector(c client.Client) *PDBViolationCollector {
	return &PDBViolationCollector{client: c}
}

// Collect lists all PDBs and identifies those whose selectors match pods on
// the terminating node, populating report.Spec.PDBViolations.
func (c *PDBViolationCollector) Collect(ctx context.Context, node *corev1.Node, report *v1alpha1.NodeReport) error {
	// List all pods on the node.
	var podList corev1.PodList
	if err := c.client.List(ctx, &podList, client.MatchingFields{"spec.nodeName": node.Name}); err != nil {
		return fmt.Errorf("pdbviolation: list pods on node %s: %w", node.Name, err)
	}

	if len(podList.Items) == 0 {
		return nil
	}

	// Build a set of (namespace, pod) pairs for fast lookup.
	type nsName struct{ ns, name string }
	podSet := make(map[nsName]corev1.Pod, len(podList.Items))
	for _, p := range podList.Items {
		podSet[nsName{p.Namespace, p.Name}] = p
	}

	// List all PDBs across all namespaces.
	var pdbList policyv1.PodDisruptionBudgetList
	if err := c.client.List(ctx, &pdbList); err != nil {
		return fmt.Errorf("pdbviolation: list PodDisruptionBudgets: %w", err)
	}

	var violations []v1alpha1.PDBViolationRecord

	for _, pdb := range pdbList.Items {
		sel, err := metav1.LabelSelectorAsSelector(pdb.Spec.Selector)
		if err != nil {
			continue
		}

		// Check if any pod on the node matches this PDB's selector.
		for _, pod := range podList.Items {
			if pod.Namespace != pdb.Namespace {
				continue
			}
			if !sel.Matches(labels.Set(pod.Labels)) {
				continue
			}

			// The PDB selector matches a pod on this node.
			reason := describePDBViolation(&pdb)
			violations = append(violations, v1alpha1.PDBViolationRecord{
				PDBName:   pdb.Name,
				Namespace: pdb.Namespace,
				Reason:    reason,
			})
			break // one violation record per PDB is enough
		}
	}

	report.Spec.PDBViolations = violations
	return nil
}

// describePDBViolation returns a human-readable reason string for a PDB match.
func describePDBViolation(pdb *policyv1.PodDisruptionBudget) string {
	if pdb.Status.DisruptionsAllowed == 0 {
		return fmt.Sprintf("PDB %s/%s allows 0 disruptions (currentHealthy=%d, desiredHealthy=%d)",
			pdb.Namespace, pdb.Name,
			pdb.Status.CurrentHealthy,
			pdb.Status.DesiredHealthy,
		)
	}
	return fmt.Sprintf("PDB %s/%s matched pods on node (disruptionsAllowed=%d)",
		pdb.Namespace, pdb.Name,
		pdb.Status.DisruptionsAllowed,
	)
}
