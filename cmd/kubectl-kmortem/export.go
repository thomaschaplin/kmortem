package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	v1alpha1 "github.com/thomaschaplin/kmortem/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export NodeReports as JSON, JSONL, or CSV",
	RunE:  runExport,
}

func init() {
	exportCmd.Flags().String("format", "json", "output format: json, jsonl, csv")
	exportCmd.Flags().Duration("since", 0, "filter to reports newer than this duration, e.g. 24h")
	exportCmd.Flags().String("cause", "", "filter by termination cause: spot, drain, autoscaler, failure, unknown")
	rootCmd.AddCommand(exportCmd)
}

func runExport(cmd *cobra.Command, _ []string) error {
	kubeconfig, kubeContext := clientFlags(cmd)
	c, err := newClient(kubeconfig, kubeContext)
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}

	format, _ := cmd.Flags().GetString("format")
	since, _ := cmd.Flags().GetDuration("since")
	cause, _ := cmd.Flags().GetString("cause")

	var wantCause v1alpha1.TerminationCause
	if cause != "" {
		var ok bool
		wantCause, ok = causeAliases[cause]
		if !ok {
			return fmt.Errorf("unknown cause %q: must be one of spot, drain, autoscaler, failure, unknown", cause)
		}
	}

	var list v1alpha1.NodeReportList
	if err := c.List(context.Background(), &list, &client.ListOptions{}); err != nil {
		return fmt.Errorf("failed to list NodeReports: %w", err)
	}

	var cutoff time.Time
	if since > 0 {
		cutoff = time.Now().Add(-since)
	}

	filtered := list.Items[:0]
	for _, r := range list.Items {
		if cause != "" && r.Spec.TerminationCause != wantCause {
			continue
		}
		if since > 0 && !r.CreationTimestamp.Time.After(cutoff) {
			continue
		}
		filtered = append(filtered, r)
	}

	switch format {
	case "json":
		return exportJSON(filtered)
	case "jsonl":
		return exportJSONL(filtered)
	case "csv":
		return exportCSV(filtered)
	default:
		return fmt.Errorf("unsupported format %q: must be json, jsonl, or csv", format)
	}
}

func exportJSON(reports []v1alpha1.NodeReport) error {
	b, err := json.MarshalIndent(reports, "", "  ")
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(b)
	if err != nil {
		return err
	}
	fmt.Println()
	return nil
}

func exportJSONL(reports []v1alpha1.NodeReport) error {
	for _, r := range reports {
		b, err := json.Marshal(r)
		if err != nil {
			return err
		}
		fmt.Println(string(b))
	}
	return nil
}

func exportCSV(reports []v1alpha1.NodeReport) error {
	w := csv.NewWriter(os.Stdout)
	header := []string{
		"report_name", "node", "cause", "az", "lifecycle",
		"pod_namespace", "pod_name", "owner_kind", "owner_name",
		"exit_reason", "cpu_request", "memory_request", "restart_count",
	}
	if err := w.Write(header); err != nil {
		return err
	}

	for _, r := range reports {
		base := []string{
			r.Name,
			r.Spec.NodeName,
			string(r.Spec.TerminationCause),
			r.Spec.InstanceMetadata.AvailabilityZone,
			string(r.Spec.InstanceMetadata.Lifecycle),
		}
		if len(r.Spec.Pods) == 0 {
			row := append(base, "", "", "", "", "", "", "", "")
			if err := w.Write(row); err != nil {
				return err
			}
			continue
		}
		for _, p := range r.Spec.Pods {
			row := append(append([]string{}, base...), //nolint:gocritic
				p.Namespace,
				p.Name,
				string(p.OwnerKind),
				p.OwnerName,
				string(p.ExitReason),
				p.CPURequest,
				p.MemoryRequest,
				fmt.Sprintf("%d", p.RestartCount),
			)
			if err := w.Write(row); err != nil {
				return err
			}
		}
	}

	w.Flush()
	return w.Error()
}
