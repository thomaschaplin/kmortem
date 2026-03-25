# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
make build          # fmt + vet + compile to bin/kmortem
make test           # fmt + vet + go test -v -race -count=1 ./...
make fmt            # go fmt ./...
make vet            # go vet ./...
make lint           # golangci-lint run ./... (requires golangci-lint in PATH)
make manifests      # regenerate CRD and RBAC YAML via controller-gen
make generate       # regenerate DeepCopy methods via controller-gen
make deploy         # kubectl apply all manifests to cluster
make docker-build IMG=registry/kmortem:tag
```

Run a single test package:
```bash
go test -v -race ./internal/collector/...
```

## Architecture

kmortem is a controller-runtime operator with two controllers and four collectors.

### Controllers (`internal/controller/`)

**NodeReconciler** (`node_controller.go`) — watches all `Node` objects. When `isTerminating()` detects a termination signal (unschedulable taint, Ready=False/Unknown, cluster-autoscaler annotation, or DeletionTimestamp), it deduplicates via `sync.Map` (in-flight) and a `spec.nodeUID` field index (existing NodeReports), creates a `NodeReport` with `phase=Collecting`, then fires a background goroutine with a 90s timeout to run the collection pipeline. The reconcile call returns immediately.

**NodeReportReconciler** (`nodereport_controller.go`) — watches `NodeReport` objects. Re-queues `Collecting` reports every 5s; once `Complete`, enforces TTL expiry (`KMORTEM_RETENTION_TTL`, default 720h) by deleting the object.

### Collection Pipeline (`internal/collector/`)

`Pipeline.Run()` fires two `errgroup` goroutines simultaneously:

- **Hot group** (time-critical, kubelet must be alive): `PodInventoryCollector`, `OOMKillCollector`
- **Warm group** (API-server-backed, survives node removal): `ConditionHistoryCollector`, `PDBViolationCollector`

Node and instance metadata are populated synchronously before either group starts — no API calls needed, reads directly from `node.Status.NodeInfo`, labels, and `spec.providerID`.

Partial failures are tolerated: errors are collected per-group and written as `HotCollectionPartialFailure` / `WarmCollectionPartialFailure` status conditions. The report is always finalised to `Complete`.

The `Collector` interface (`collector.go`) is the extension point: `Collect(ctx, node, report) error`.

### API Types (`api/v1alpha1/`)

`NodeReport` is a cluster-scoped CRD (short name `nr`). All forensic data lives in `spec` (immutable evidence); collection status lives in `status` (subresource). Key enums: `TerminationCause`, `StatusPhase`, `PodExitReason`, `OwnerKind`, `Lifecycle`.

### Entry Point (`cmd/main.go`)

Registers schemes (core, policy/v1, kmortem v1alpha1), sets up two field indexers (`spec.nodeName` on Pods, `spec.nodeUID` on NodeReports), wires both controllers, and starts the manager with leader election in the `kmortem` namespace.

### Configuration (`internal/config/`)

Loaded from env at startup via `config.LoadFromEnv()`. Currently only `KMORTEM_RETENTION_TTL`.

### Deployment manifests (`config/`)

- `config/crd/bases/` — generated CRD YAML
- `config/rbac/` — ServiceAccount, ClusterRole, ClusterRoleBinding
- `config/manager/` — namespace and Deployment manifests
