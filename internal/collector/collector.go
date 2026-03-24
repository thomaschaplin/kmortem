// Package collector provides the Collector interface and implementations for
// gathering forensic evidence from a terminating Kubernetes node.
package collector

import (
	"context"

	corev1 "k8s.io/api/core/v1"

	"github.com/thomaschaplin/kmortem/api/v1alpha1"
)

// Collector is implemented by each evidence-gathering component.
// Collect is called with the terminating node and the NodeReport to populate.
// Implementations must be safe to call concurrently.
// A non-nil error indicates a partial or complete collection failure; the
// NodeReport may still contain partial data.
type Collector interface {
	Collect(ctx context.Context, node *corev1.Node, report *v1alpha1.NodeReport) error
}
