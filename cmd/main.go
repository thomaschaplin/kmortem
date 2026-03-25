package main

import (
	"context"
	"flag"
	"os"

	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	kmortemv1alpha1 "github.com/thomaschaplin/kmortem/api/v1alpha1"
	"github.com/thomaschaplin/kmortem/internal/collector"
	"github.com/thomaschaplin/kmortem/internal/config"
	"github.com/thomaschaplin/kmortem/internal/controller"
)

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(kmortemv1alpha1.AddToScheme(scheme))
	utilruntime.Must(corev1.AddToScheme(scheme))
	utilruntime.Must(policyv1.AddToScheme(scheme))
}

func main() {
	var (
		metricsAddr      string
		healthProbeAddr  string
		leaderElect      bool
		leaderElectionNS string
	)

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metrics endpoint binds to.")
	flag.StringVar(&healthProbeAddr, "health-probe-bind-address", ":8081", "The address the health probe endpoint binds to.")
	flag.BoolVar(&leaderElect, "leader-elect", true, "Enable leader election to prevent multiple operator replicas acting simultaneously.")
	flag.StringVar(&leaderElectionNS, "leader-election-namespace", "kmortem", "Namespace for leader election.")

	opts := zap.Options{Development: false}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
	setupLog := ctrl.Log.WithName("setup")

	// Root context — tied to OS signals (SIGTERM, SIGINT).
	rootCtx := ctrl.SetupSignalHandler()

	// Load configuration from environment.
	cfg := config.LoadFromEnv()

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
		HealthProbeBindAddress:  healthProbeAddr,
		LeaderElection:          leaderElect,
		LeaderElectionID:        "kmortem.io",
		LeaderElectionNamespace: leaderElectionNS,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Set up field indexers required by the collectors.
	if err := setupIndexers(rootCtx, mgr); err != nil {
		setupLog.Error(err, "unable to set up field indexers")
		os.Exit(1)
	}

	// Build the collection pipeline.
	pipeline := collector.NewPipeline(mgr.GetClient(), cfg)

	// Wire the NodeReconciler.
	if err := (&controller.NodeReconciler{
		Client:   mgr.GetClient(),
		RootCtx:  rootCtx,
		Pipeline: pipeline,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create NodeReconciler")
		os.Exit(1)
	}

	// Wire the NodeReportReconciler.
	if err := (&controller.NodeReportReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("kmortem"),
		Config:   cfg,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create NodeReportReconciler")
		os.Exit(1)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting kmortem operator")
	if err := mgr.Start(rootCtx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

// setupIndexers registers the field indexers required for efficient pod lookups
// by node name and event lookups by involved object.
func setupIndexers(ctx context.Context, mgr ctrl.Manager) error {
	// Index pods by spec.nodeName.
	if err := mgr.GetFieldIndexer().IndexField(ctx, &corev1.Pod{}, "spec.nodeName", func(obj client.Object) []string {
		pod, ok := obj.(*corev1.Pod)
		if !ok {
			return nil
		}
		return []string{pod.Spec.NodeName}
	}); err != nil {
		return err
	}

	// Index NodeReports by spec.nodeUID for efficient deduplication lookups.
	if err := mgr.GetFieldIndexer().IndexField(ctx, &kmortemv1alpha1.NodeReport{}, "spec.nodeUID", func(obj client.Object) []string {
		nr, ok := obj.(*kmortemv1alpha1.NodeReport)
		if !ok {
			return nil
		}
		return []string{nr.Spec.NodeUID}
	}); err != nil {
		return err
	}

	return nil
}
