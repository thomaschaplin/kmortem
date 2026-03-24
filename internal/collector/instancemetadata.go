package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/thomaschaplin/kmortem/api/v1alpha1"
)

const (
	imdsTokenTTL     = "21600"
	imdsTokenPath    = "/latest/api/token"
	imdsMetaPath     = "/latest/meta-data"
	imdsSpotPath     = "/latest/meta-data/spot/termination-time"
	imdsInstanceID   = "/latest/meta-data/instance-id"
	imdsInstanceType = "/latest/meta-data/instance-type"
	imdsAZ           = "/latest/meta-data/placement/availability-zone"
	imdsRegion       = "/latest/meta-data/placement/region"
	imdsLifecycle    = "/latest/meta-data/instance-life-cycle"
	imdsTimeout      = 5 * time.Second
)

// InstanceMetadataCollector fetches AWS EC2 instance metadata from the IMDS
// endpoint on the node's internal IP address.
type InstanceMetadataCollector struct {
	httpClient *http.Client
}

// NewInstanceMetadataCollector creates a new InstanceMetadataCollector.
func NewInstanceMetadataCollector() *InstanceMetadataCollector {
	return &InstanceMetadataCollector{
		httpClient: &http.Client{Timeout: imdsTimeout},
	}
}

// Collect fetches instance metadata and populates report.Spec.InstanceMetadata.
func (c *InstanceMetadataCollector) Collect(ctx context.Context, node *corev1.Node, report *v1alpha1.NodeReport) error {
	nodeIP := nodeInternalIP(node)
	if nodeIP == "" {
		return fmt.Errorf("instancemetadata: no internal IP found for node %s", node.Name)
	}

	base := fmt.Sprintf("http://%s", nodeIP)

	// Attempt IMDSv2 token first; fall back to IMDSv1 if unavailable.
	token, _ := c.fetchIMDSv2Token(ctx, base)

	var errs []string

	instanceID, err := c.fetchIMDS(ctx, base+imdsInstanceID, token)
	if err != nil {
		errs = append(errs, err.Error())
	}

	instanceType, err := c.fetchIMDS(ctx, base+imdsInstanceType, token)
	if err != nil {
		errs = append(errs, err.Error())
	}

	az, err := c.fetchIMDS(ctx, base+imdsAZ, token)
	if err != nil {
		errs = append(errs, err.Error())
	}

	region, err := c.fetchIMDS(ctx, base+imdsRegion, token)
	if err != nil {
		errs = append(errs, err.Error())
	}

	lifecycleRaw, err := c.fetchIMDS(ctx, base+imdsLifecycle, token)
	if err != nil {
		errs = append(errs, err.Error())
	}

	lifecycle := v1alpha1.LifecycleOnDemand
	if strings.TrimSpace(lifecycleRaw) == "spot" {
		lifecycle = v1alpha1.LifecycleSpot
	}

	report.Spec.InstanceMetadata = v1alpha1.InstanceMetadata{
		InstanceID:       strings.TrimSpace(instanceID),
		InstanceType:     strings.TrimSpace(instanceType),
		AvailabilityZone: strings.TrimSpace(az),
		Region:           strings.TrimSpace(region),
		Lifecycle:        lifecycle,
	}

	if len(errs) > 0 {
		return fmt.Errorf("instancemetadata: partial failure: %s", strings.Join(errs, "; "))
	}
	return nil
}

// fetchIMDSv2Token obtains an IMDSv2 session token.
func (c *InstanceMetadataCollector) fetchIMDSv2Token(ctx context.Context, base string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, base+imdsTokenPath, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("X-aws-ec2-metadata-token-ttl-seconds", imdsTokenTTL)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// fetchIMDS performs a GET against an IMDS endpoint, optionally with an IMDSv2 token.
func (c *InstanceMetadataCollector) fetchIMDS(ctx context.Context, url, token string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	if token != "" {
		req.Header.Set("X-aws-ec2-metadata-token", token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("GET %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", nil
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET %s: unexpected status %d", url, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// SpotInterruptionNotice is the JSON body of the IMDS spot termination endpoint.
type SpotInterruptionNotice struct {
	Time string `json:"time"`
}

// CheckSpotInterruption returns true if the node's IMDS is reporting a spot
// termination notice. It is safe to call this before creating a NodeReport.
func CheckSpotInterruption(ctx context.Context, node *corev1.Node) bool {
	nodeIP := nodeInternalIP(node)
	if nodeIP == "" {
		return false
	}

	client := &http.Client{Timeout: imdsTimeout}
	url := fmt.Sprintf("http://%s%s", nodeIP, imdsSpotPath)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}

	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		var notice SpotInterruptionNotice
		if err := json.NewDecoder(resp.Body).Decode(&notice); err == nil && notice.Time != "" {
			return true
		}
		// Non-JSON 200 also counts
		return true
	}
	return false
}

// nodeInternalIP returns the InternalIP address of a node.
func nodeInternalIP(node *corev1.Node) string {
	for _, addr := range node.Status.Addresses {
		if addr.Type == corev1.NodeInternalIP {
			return addr.Address
		}
	}
	return ""
}
