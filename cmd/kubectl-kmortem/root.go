package main

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	v1alpha1 "github.com/thomaschaplin/kmortem/api/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var rootCmd = &cobra.Command{
	Use:           "kubectl-kmortem",
	Short:         "Inspect NodeReport forensic data from the kmortem operator",
	SilenceUsage:  true,
	SilenceErrors: true,
}

// causeAliases maps human-friendly CLI values to TerminationCause constants.
var causeAliases = map[string]v1alpha1.TerminationCause{
	"spot":       v1alpha1.TerminationCauseSpotInterruption,
	"drain":      v1alpha1.TerminationCauseManualDrain,
	"autoscaler": v1alpha1.TerminationCauseClusterAutoscalerScaleDown,
	"failure":    v1alpha1.TerminationCauseNodeConditionFailure,
	"unknown":    v1alpha1.TerminationCauseUnknown,
}

func init() {
	rootCmd.PersistentFlags().String("kubeconfig", "", "path to kubeconfig file (defaults to KUBECONFIG env / in-cluster)")
	rootCmd.PersistentFlags().String("context", "", "kubeconfig context to use")
}

func newClient(kubeconfig, kubeContext string) (client.Client, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	if kubeconfig != "" {
		rules.ExplicitPath = kubeconfig
	}
	overrides := &clientcmd.ConfigOverrides{}
	if kubeContext != "" {
		overrides.CurrentContext = kubeContext
	}
	restCfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, overrides).ClientConfig()
	if err != nil {
		return nil, err
	}
	scheme := runtime.NewScheme()
	utilruntime.Must(v1alpha1.AddToScheme(scheme))
	return client.New(restCfg, client.Options{Scheme: scheme})
}

func formatAge(t time.Time) string {
	d := time.Since(t)
	if d >= 24*time.Hour {
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
	if d >= time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dm", int(d.Minutes()))
}

func clientFlags(cmd *cobra.Command) (kubeconfig, kubeContext string) {
	kubeconfig, _ = cmd.Root().PersistentFlags().GetString("kubeconfig")
	kubeContext, _ = cmd.Root().PersistentFlags().GetString("context")
	return
}
