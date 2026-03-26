package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	v1alpha1 "github.com/thomaschaplin/kmortem/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var summaryCmd = &cobra.Command{
	Use:   "summary",
	Short: "Show aggregate statistics across NodeReports",
	RunE:  runSummary,
}

func init() {
	summaryCmd.Flags().Duration("since", 0, "limit to reports newer than this duration, e.g. 720h")
	rootCmd.AddCommand(summaryCmd)
}

type workloadKey struct {
	name      string
	kind      v1alpha1.OwnerKind
	namespace string
}

type workloadEntry struct {
	key   workloadKey
	count int
}

func runSummary(cmd *cobra.Command, _ []string) error {
	kubeconfig, kubeContext := clientFlags(cmd)
	c, err := newClient(kubeconfig, kubeContext)
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}

	since, _ := cmd.Flags().GetDuration("since")

	var list v1alpha1.NodeReportList
	if err := c.List(context.Background(), &list, &client.ListOptions{}); err != nil {
		return fmt.Errorf("failed to list NodeReports: %w", err)
	}

	var cutoff time.Time
	if since > 0 {
		cutoff = time.Now().Add(-since)
	}

	total := 0
	byCause := map[v1alpha1.TerminationCause]int{}
	byAZ := map[string]int{}
	workloadCount := map[workloadKey]int{}
	totalOOM := 0
	totalPDB := 0

	// Ordered causes for consistent output.
	orderedCauses := []v1alpha1.TerminationCause{
		v1alpha1.TerminationCauseSpotInterruption,
		v1alpha1.TerminationCauseClusterAutoscalerScaleDown,
		v1alpha1.TerminationCauseManualDrain,
		v1alpha1.TerminationCauseNodeConditionFailure,
		v1alpha1.TerminationCauseUnknown,
	}

	for _, r := range list.Items {
		if since > 0 && !r.CreationTimestamp.Time.After(cutoff) {
			continue
		}
		total++
		byCause[r.Spec.TerminationCause]++

		az := r.Spec.InstanceMetadata.AvailabilityZone
		if az == "" {
			az = "<unknown>"
		}
		byAZ[az]++

		totalOOM += len(r.Spec.OOMKills)
		totalPDB += len(r.Spec.PDBViolations)

		for _, p := range r.Spec.Pods {
			if p.OwnerKind == v1alpha1.OwnerKindNone {
				continue
			}
			k := workloadKey{p.OwnerName, p.OwnerKind, p.Namespace}
			workloadCount[k]++
		}
	}

	out := os.Stdout
	fmt.Fprintf(out, "NodeReport Summary")
	if since > 0 {
		fmt.Fprintf(out, " (last %s)", since)
	}
	fmt.Fprintf(out, "\n%s\n\n", repeat("=", 40))
	fmt.Fprintf(out, "Total Terminations: %d\n\n", total)

	// By cause
	fmt.Fprintf(out, "By Cause:\n")
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	for _, cause := range orderedCauses {
		count := byCause[cause]
		pct := 0.0
		if total > 0 {
			pct = float64(count) / float64(total) * 100
		}
		fmt.Fprintf(w, "  %s\t%d\t(%.1f%%)\n", cause, count, pct)
	}
	w.Flush()

	// Top workloads
	fmt.Fprintf(out, "\nTop Displaced Workloads:\n")
	entries := make([]workloadEntry, 0, len(workloadCount))
	for k, v := range workloadCount {
		entries = append(entries, workloadEntry{k, v})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].count > entries[j].count
	})
	if len(entries) > 10 {
		entries = entries[:10]
	}
	if len(entries) == 0 {
		fmt.Fprintf(out, "  (none)\n")
	} else {
		w2 := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w2, "  OWNER\tKIND\tNAMESPACE\tPODS")
		for _, e := range entries {
			fmt.Fprintf(w2, "  %s\t%s\t%s\t%d\n", e.key.name, e.key.kind, e.key.namespace, e.count)
		}
		w2.Flush()
	}

	fmt.Fprintf(out, "\nOOM Kills:      %d\n", totalOOM)
	fmt.Fprintf(out, "PDB Violations: %d\n", totalPDB)

	// By AZ
	fmt.Fprintf(out, "\nBy Availability Zone:\n")
	azKeys := make([]string, 0, len(byAZ))
	for k := range byAZ {
		azKeys = append(azKeys, k)
	}
	sort.Strings(azKeys)
	if len(azKeys) == 0 {
		fmt.Fprintf(out, "  (none)\n")
	} else {
		w3 := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w3, "  AZ\tCOUNT")
		for _, az := range azKeys {
			fmt.Fprintf(w3, "  %s\t%d\n", az, byAZ[az])
		}
		w3.Flush()
	}

	return nil
}

func repeat(s string, n int) string {
	out := make([]byte, 0, n*len(s))
	for i := 0; i < n; i++ {
		out = append(out, s...)
	}
	return string(out)
}
