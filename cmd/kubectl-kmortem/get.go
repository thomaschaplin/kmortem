package main

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	v1alpha1 "github.com/thomaschaplin/kmortem/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var getCmd = &cobra.Command{
	Use:   "get",
	Short: "List NodeReports",
	RunE:  runGet,
}

func init() {
	getCmd.Flags().String("cause", "", "filter by termination cause: spot, drain, autoscaler, failure, unknown")
	getCmd.Flags().Duration("since", 0, "filter to reports newer than this duration, e.g. 24h")
	getCmd.Flags().String("az", "", "filter by availability zone")
	getCmd.Flags().String("lifecycle", "", "filter by instance lifecycle: spot, on-demand")
	getCmd.Flags().String("phase", "", "filter by phase: collecting, complete")
	rootCmd.AddCommand(getCmd)
}

func runGet(cmd *cobra.Command, _ []string) error {
	kubeconfig, kubeContext := clientFlags(cmd)
	c, err := newClient(kubeconfig, kubeContext)
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}

	cause, _ := cmd.Flags().GetString("cause")
	since, _ := cmd.Flags().GetDuration("since")
	az, _ := cmd.Flags().GetString("az")
	lifecycle, _ := cmd.Flags().GetString("lifecycle")
	phase, _ := cmd.Flags().GetString("phase")

	// Validate cause alias up front.
	var wantCause v1alpha1.TerminationCause
	if cause != "" {
		var ok bool
		wantCause, ok = causeAliases[cause]
		if !ok {
			return fmt.Errorf("unknown cause %q: must be one of spot, drain, autoscaler, failure, unknown", cause)
		}
	}

	// Validate phase.
	var wantPhase v1alpha1.StatusPhase
	if phase != "" {
		switch phase {
		case "collecting":
			wantPhase = v1alpha1.StatusPhaseCollecting
		case "complete":
			wantPhase = v1alpha1.StatusPhaseComplete
		default:
			return fmt.Errorf("unknown phase %q: must be collecting or complete", phase)
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

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tNODE\tCAUSE\tLIFECYCLE\tPODS\tOOM\tPHASE\tAGE")

	for _, r := range list.Items {
		if cause != "" && r.Spec.TerminationCause != wantCause {
			continue
		}
		if since > 0 && !r.CreationTimestamp.Time.After(cutoff) {
			continue
		}
		if az != "" && r.Spec.InstanceMetadata.AvailabilityZone != az {
			continue
		}
		if lifecycle != "" && string(r.Spec.InstanceMetadata.Lifecycle) != lifecycle {
			continue
		}
		if phase != "" && r.Status.Phase != wantPhase {
			continue
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t%d\t%s\t%s\n",
			r.Name,
			r.Spec.NodeName,
			r.Spec.TerminationCause,
			r.Spec.InstanceMetadata.Lifecycle,
			len(r.Spec.Pods),
			len(r.Spec.OOMKills),
			r.Status.Phase,
			formatAge(r.CreationTimestamp.Time),
		)
	}

	return w.Flush()
}
