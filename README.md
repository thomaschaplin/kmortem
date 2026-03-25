# kmortem

> **Nodes die. Evidence shouldn't.**

kmortem is a Kubernetes operator that watches for node termination events and preserves a forensic record of everything that happened — what ran, why it died, and what the blast radius was — as a `NodeReport` custom resource. Records are queryable via `kubectl` long after the node is gone.

## How it works

kmortem runs two controllers:

**NodeReconciler** watches all Node objects. When it detects termination signals (cordoned taint, failed Ready condition, cluster-autoscaler annotation, Karpenter disruption taint, or deletion timestamp), it creates a `NodeReport` and immediately dispatches a background goroutine to collect evidence. The reconciler itself returns instantly — it never blocks on collection.

**NodeReportReconciler** manages the lifecycle of `NodeReport` objects: checking collection progress, enforcing TTL expiry, optionally archiving to S3, and finally deleting the object.

Evidence is gathered by four collectors split into two parallel groups:

- **Hot** (time-critical, while the node is still reachable): pod inventory, OOMKill events
- **Warm** (API-server backed, survives node removal): condition history, PodDisruptionBudget violations

Instance metadata (EC2 instance ID, type, AZ, lifecycle, region) is resolved synchronously from node labels and `spec.providerID` — no IMDS call required.

Both groups run simultaneously with a 90-second total timeout, designed to fit inside the AWS spot interruption 2-minute window. Partial failures are tolerated — a partial report is better than no report.

## Usage

```
$ kubectl get nodereports
NAME                    NODE             CAUSE                    PHASE      AGE
ip-10-0-1-42            ip-10-0-1-42     SpotInterruption         Complete   2d
ip-10-0-2-18            ip-10-0-2-18     ManualDrain              Complete   5d
ip-10-0-3-7             ip-10-0-3-7      ClusterAutoscalerScaleDown Complete  12h
```

```
$ kubectl get nr ip-10-0-1-42 -o yaml
apiVersion: kmortem.io/v1alpha1
kind: NodeReport
metadata:
  name: ip-10-0-1-42
  creationTimestamp: "2026-03-23T12:00:00Z"
spec:
  nodeName: ip-10-0-1-42
  nodeUID: abc-123-def-456
  terminationTime: "2026-03-23T12:00:01Z"
  terminationCause: SpotInterruption
  initiatedBy: karpenter
  instanceMetadata:
    instanceID: i-0abc123def456
    instanceType: m5.xlarge
    availabilityZone: eu-west-1a
    lifecycle: spot
    region: eu-west-1
  nodeMetadata:
    kernelVersion: 5.10.0-1234
    osImage: Amazon Linux 2
    containerRuntime: containerd://1.7.0
    kubeletVersion: v1.29.3
    allocatableCPU: "3920m"
    allocatableMemory: 14Gi
    capacityCPU: "4"
    capacityMemory: 16Gi
    createdAt: "2026-03-01T09:00:00Z"
    taints:
      - key: node.kubernetes.io/unschedulable
        effect: NoSchedule
  pods:
    - name: payments-7d8f9b-xkz2p
      namespace: production
      ownerKind: Deployment
      ownerName: payments
      startTime: "2026-03-23T10:00:00Z"
      exitReason: Evicted
      phase: Failed
      cpuRequest: "250m"
      memoryRequest: "512Mi"
      cpuLimit: "500m"
      memoryLimit: "1Gi"
      restartCount: 2
      containers:
        - name: api
          image: payments:v1.2.3
    - name: worker-abc
      namespace: jobs
      ownerKind: Job
      ownerName: nightly-report
      exitReason: Completed
      phase: Succeeded
  oomKills:
    - podName: ml-inference-6c7d8e-9f0a
      namespace: ml
      containerName: model-server
      memoryLimit: 4Gi
      timestamp: "2026-03-23T11:58:42Z"
  pdbViolations:
    - pdbName: payments-pdb
      namespace: production
      reason: "PDB production/payments-pdb allows 0 disruptions (currentHealthy=1, desiredHealthy=2)"
status:
  phase: Complete
  collectionCompletedAt: "2026-03-23T12:01:02Z"
  message: Collection complete
```

## Termination causes

| Signal | Classified as |
|--------|--------------|
| Karpenter `karpenter.sh/disruption` taint on a spot node | `SpotInterruption` |
| `cluster-autoscaler.kubernetes.io/scale-down` annotation | `ClusterAutoscalerScaleDown` |
| `MemoryPressure`, `DiskPressure`, or `PIDPressure` condition = True | `NodeConditionFailure` |
| `node.kubernetes.io/unschedulable` taint (no other signals) | `ManualDrain` |
| None of the above | `Unknown` |

`initiatedBy` is also populated: `cluster-autoscaler` when the CA annotation is present, otherwise the Kubernetes field manager that last updated `spec.taints`.

## Evidence gathered

- **Instance metadata** — EC2 instance ID, type, availability zone, lifecycle (spot/on-demand), region; resolved from node labels and `spec.providerID`
- **Pod inventory** — every pod that was scheduled on the node: namespace, owner workload (Deployment/StatefulSet/DaemonSet/Job/CronJob), start time, end time, exit reason (Evicted/Completed/OOMKilled/Error/Unknown), phase, resource requests and limits, restart count, container names and images
- **OOM kills** — any container killed for exceeding its memory limit: pod, container, memory limit, timestamp
- **Condition history** — all node conditions (Ready, MemoryPressure, DiskPressure, PIDPressure) with last transition times, enriched with Kubernetes Events
- **Node metadata** — kernel version, OS image, container runtime, kubelet version, allocatable and capacity CPU/memory, creation time, labels, taints
- **PDB violations** — PodDisruptionBudgets whose selectors matched pods on the node, with disruption counts

## Status phases

| Phase | Meaning |
|-------|---------|
| `Collecting` | Evidence gathering in progress |
| `Complete` | All available data recorded; TTL clock running |

## Configuration

Configuration is loaded from environment variables on the manager pod.

| Variable | Default | Description |
|----------|---------|-------------|
| `KMORTEM_RETENTION_TTL` | `720h` | How long `NodeReport` objects live in the cluster (30 days). Any Go duration string is accepted, e.g. `48h`. |

## Deployment

**Prerequisites:** a running Kubernetes cluster, `kubectl` configured, and cluster-admin permissions.

The operator image is built with a multi-stage Dockerfile using `golang:1.24` for compilation and `gcr.io/distroless/static:nonroot` as the runtime base (runs as UID 65532).

```bash
# 1. Install the CRD
kubectl apply -f config/crd/bases/kmortem.io_nodereports.yaml

# 2. Create the namespace and RBAC
kubectl apply -f config/manager/namespace.yaml
kubectl apply -f config/rbac/

# 3. Deploy the operator
kubectl apply -f config/manager/manager.yaml
```

Or use the Makefile shortcut:

```bash
make deploy
```

## Development

```bash
# Build the binary
make build

# Run tests
make test

# Format and vet
make fmt vet

# Build the container image
make docker-build IMG=my-registry/kmortem:dev

# Push
make docker-push IMG=my-registry/kmortem:dev
```

## CRD reference

- **API group:** `kmortem.io`
- **Version:** `v1alpha1`
- **Kind:** `NodeReport`
- **Short name:** `nr`
- **Scope:** Cluster
