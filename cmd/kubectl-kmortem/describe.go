package main

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
	v1alpha1 "github.com/thomaschaplin/kmortem/api/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
)

var describeCmd = &cobra.Command{
	Use:   "describe <name>",
	Short: "Show full detail for a NodeReport",
	Args:  cobra.ExactArgs(1),
	RunE:  runDescribe,
}

func init() {
	rootCmd.AddCommand(describeCmd)
}

func runDescribe(cmd *cobra.Command, args []string) error {
	kubeconfig, kubeContext := clientFlags(cmd)
	c, err := newClient(kubeconfig, kubeContext)
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}

	var r v1alpha1.NodeReport
	if err := c.Get(context.Background(), types.NamespacedName{Name: args[0]}, &r); err != nil {
		return fmt.Errorf("NodeReport %q not found: %w", args[0], err)
	}

	out := os.Stdout

	// Header
	fmt.Fprintf(out, "=== NodeReport: %s ===\n\n", r.Name)
	fmt.Fprintf(out, "Node:       %s\n", r.Spec.NodeName)
	fmt.Fprintf(out, "Cause:      %s\n", r.Spec.TerminationCause)
	fmt.Fprintf(out, "Phase:      %s\n", r.Status.Phase)
	if r.Spec.TerminationTime != nil {
		fmt.Fprintf(out, "Detected:   %s\n", r.Spec.TerminationTime.UTC().Format("2006-01-02 15:04:05 UTC"))
	}
	if r.Status.CollectionCompletedAt != nil {
		fmt.Fprintf(out, "Collected:  %s\n", r.Status.CollectionCompletedAt.UTC().Format("2006-01-02 15:04:05 UTC"))
	}
	if r.Spec.InitiatedBy != "" {
		fmt.Fprintf(out, "InitiatedBy: %s\n", r.Spec.InitiatedBy)
	}

	// Instance
	im := r.Spec.InstanceMetadata
	fmt.Fprintf(out, "\n--- Instance ---\n")
	fmt.Fprintf(out, "ID:         %s\n", orDash(im.InstanceID))
	fmt.Fprintf(out, "Type:       %s\n", orDash(im.InstanceType))
	fmt.Fprintf(out, "AZ:         %s\n", orDash(im.AvailabilityZone))
	fmt.Fprintf(out, "Lifecycle:  %s\n", orDash(string(im.Lifecycle)))
	fmt.Fprintf(out, "Region:     %s\n", orDash(im.Region))

	// Node
	nm := r.Spec.NodeMetadata
	fmt.Fprintf(out, "\n--- Node ---\n")
	fmt.Fprintf(out, "OS:           %s\n", orDash(nm.OSImage))
	fmt.Fprintf(out, "Kernel:       %s\n", orDash(nm.KernelVersion))
	fmt.Fprintf(out, "Runtime:      %s\n", orDash(nm.ContainerRuntime))
	fmt.Fprintf(out, "Kubelet:      %s\n", orDash(nm.KubeletVersion))
	fmt.Fprintf(out, "Alloc CPU:    %s\n", orDash(nm.AllocatableCPU))
	fmt.Fprintf(out, "Alloc Memory: %s\n", orDash(nm.AllocatableMemory))

	// Conditions
	if len(r.Spec.ConditionHistory) > 0 {
		fmt.Fprintf(out, "\n--- Conditions ---\n")
		w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "TYPE\tSTATUS\tREASON\tMESSAGE\tLAST TRANSITION")
		for _, cond := range r.Spec.ConditionHistory {
			lastT := "-"
			if cond.LastTransitionTime != nil {
				lastT = cond.LastTransitionTime.UTC().Format("2006-01-02 15:04:05")
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				cond.Type, cond.Status, orDash(cond.Reason), truncate(cond.Message, 60), lastT)
		}
		w.Flush()
	}

	// Pods
	fmt.Fprintf(out, "\n--- Pods (%d) ---\n", len(r.Spec.Pods))
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAMESPACE\tPOD\tOWNER\tKIND\tCPU-REQ\tMEM-REQ\tEXIT\tRESTARTS")
	for _, p := range r.Spec.Pods {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%d\n",
			p.Namespace, p.Name,
			orDash(p.OwnerName), p.OwnerKind,
			orDash(p.CPURequest), orDash(p.MemoryRequest),
			p.ExitReason, p.RestartCount,
		)
	}
	w.Flush()

	// OOM Kills
	if len(r.Spec.OOMKills) > 0 {
		fmt.Fprintf(out, "\n--- OOM Kills (%d) ---\n", len(r.Spec.OOMKills))
		w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "POD\tNAMESPACE\tCONTAINER\tMEM-LIMIT\tTIME")
		for _, o := range r.Spec.OOMKills {
			ts := "-"
			if o.Timestamp != nil {
				ts = o.Timestamp.UTC().Format("2006-01-02 15:04:05")
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				o.PodName, o.Namespace, o.ContainerName, orDash(o.MemoryLimit), ts)
		}
		w.Flush()
	}

	// PDB Violations
	if len(r.Spec.PDBViolations) > 0 {
		fmt.Fprintf(out, "\n--- PDB Violations (%d) ---\n", len(r.Spec.PDBViolations))
		w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "PDB\tNAMESPACE\tREASON")
		for _, p := range r.Spec.PDBViolations {
			fmt.Fprintf(w, "%s\t%s\t%s\n", p.PDBName, p.Namespace, p.Reason)
		}
		w.Flush()
	}

	return nil
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
