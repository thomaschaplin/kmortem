package collector

import (
	"context"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/thomaschaplin/kmortem/api/v1alpha1"
)

// InstanceMetadataCollector reads AWS EC2 instance metadata from the node
// object — labels and spec.providerID — without contacting the IMDS endpoint.
type InstanceMetadataCollector struct{}

// NewInstanceMetadataCollector creates a new InstanceMetadataCollector.
func NewInstanceMetadataCollector() *InstanceMetadataCollector {
	return &InstanceMetadataCollector{}
}

// Collect reads instance metadata from node labels and spec.providerID,
// populating report.Spec.InstanceMetadata.
func (c *InstanceMetadataCollector) Collect(_ context.Context, node *corev1.Node, report *v1alpha1.NodeReport) error {
	labels := node.Labels

	report.Spec.InstanceMetadata = v1alpha1.InstanceMetadata{
		InstanceID:       parseInstanceID(node.Spec.ProviderID),
		InstanceType:     firstLabel(labels, "node.kubernetes.io/instance-type", "beta.kubernetes.io/instance-type"),
		AvailabilityZone: firstLabel(labels, "topology.kubernetes.io/zone", "failure-domain.beta.kubernetes.io/zone"),
		Region:           firstLabel(labels, "topology.kubernetes.io/region", "failure-domain.beta.kubernetes.io/region"),
		Lifecycle:        resolveLifecycle(labels),
	}
	return nil
}

// parseInstanceID extracts the EC2 instance ID from a providerID string.
// AWS providerID format: aws:///us-east-1a/i-0abc123def456
func parseInstanceID(providerID string) string {
	parts := strings.Split(providerID, "/")
	if len(parts) == 0 {
		return ""
	}
	last := parts[len(parts)-1]
	if strings.HasPrefix(last, "i-") {
		return last
	}
	return ""
}

// resolveLifecycle determines spot vs on-demand from node labels.
// Checks Karpenter (karpenter.sh/capacity-type), EKS
// (eks.amazonaws.com/capacityType), and generic (node.kubernetes.io/lifecycle)
// labels in that order.
func resolveLifecycle(labels map[string]string) v1alpha1.Lifecycle {
	if lc := labels["karpenter.sh/capacity-type"]; lc != "" {
		switch strings.ToLower(lc) {
		case "spot":
			return v1alpha1.LifecycleSpot
		case "on-demand":
			return v1alpha1.LifecycleOnDemand
		}
	}
	if cap := labels["eks.amazonaws.com/capacityType"]; cap != "" {
		switch strings.ToUpper(cap) {
		case "SPOT":
			return v1alpha1.LifecycleSpot
		case "ON_DEMAND":
			return v1alpha1.LifecycleOnDemand
		}
	}
	if lc := labels["node.kubernetes.io/lifecycle"]; lc != "" {
		switch strings.ToLower(lc) {
		case "spot":
			return v1alpha1.LifecycleSpot
		case "normal", "on-demand":
			return v1alpha1.LifecycleOnDemand
		}
	}
	return v1alpha1.LifecycleUnknown
}

// firstLabel returns the value of the first key that exists in labels.
func firstLabel(labels map[string]string, keys ...string) string {
	for _, k := range keys {
		if v, ok := labels[k]; ok && v != "" {
			return v
		}
	}
	return ""
}
