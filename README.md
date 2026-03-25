# kmortem

> **Nodes die. Evidence shouldn't.**

kmortem is a Kubernetes operator that watches for node termination events and preserves a forensic record of everything that happened — what ran, why it died, and what the blast radius was — as a `NodeReport` custom resource. Records are queryable via `kubectl` long after the node is gone.

## How it works

kmortem runs two controllers:

**NodeReconciler** watches all Node objects. When it detects termination signals (cordoned taint, failed Ready condition, cluster-autoscaler annotation, or deletion timestamp), it creates a `NodeReport` and immediately dispatches a background goroutine to collect evidence. The reconciler itself returns instantly — it never blocks on collection.

**NodeReportReconciler** manages the lifecycle of `NodeReport` objects: checking collection progress, enforcing TTL expiry, optionally archiving to S3, and finally deleting the object.

Evidence is gathered by five collectors split into two parallel groups:

- **Hot** (time-critical, while the node is still reachable): AWS instance metadata via IMDS, pod inventory, OOMKill events
- **Warm** (API-server backed, survives node removal): condition history, PodDisruptionBudget violations

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
  instanceMetadata:
    instanceID: i-0abc123def456
    instanceType: m5.xlarge
    availabilityZone: eu-west-1a
    lifecycle: spot
    region: eu-west-1
  nodeMetadata:
    kernelVersion: 5.10.0-1234
    osImage: Amazon Linux 2
    kubeletVersion: v1.29.3
    allocatableCPU: "3920m"
    allocatableMemory: 14Gi
  pods:
    - name: payments-7d8f9b-xkz2p
      namespace: production
      ownerKind: Deployment
      ownerName: payments
      exitReason: Evicted
    - name: worker-abc
      namespace: jobs
      ownerKind: Job
      ownerName: nightly-report
      exitReason: Completed
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
| AWS IMDS spot interruption notice | `SpotInterruption` |
| `cluster-autoscaler.kubernetes.io/scale-down` annotation | `ClusterAutoscalerScaleDown` |
| `node.kubernetes.io/unschedulable` taint (no other signals) | `ManualDrain` |
| `MemoryPressure`, `DiskPressure`, or `PIDPressure` condition = True | `NodeConditionFailure` |
| None of the above | `Unknown` |

## Evidence gathered

- **Instance metadata** — EC2 instance ID, type, availability zone, lifecycle (spot/on-demand), region; spot interruption notice if present
- **Pod inventory** — every pod that was scheduled on the node: namespace, owner workload (Deployment/StatefulSet/DaemonSet/Job/CronJob), start time, end time, exit reason (Evicted/Completed/OOMKilled/Error/Unknown), phase
- **OOM kills** — any container killed for exceeding its memory limit: pod, container, memory limit, timestamp
- **Condition history** — all node conditions (Ready, MemoryPressure, DiskPressure, PIDPressure) with last transition times, enriched with Kubernetes Events
- **Node metadata** — kernel version, OS image, container runtime, kubelet version, allocatable CPU and memory
- **PDB violations** — PodDisruptionBudgets whose selectors matched pods on the node, with disruption counts

## Status phases

| Phase | Meaning |
|-------|---------|
| `Collecting` | Evidence gathering in progress |
| `Complete` | All available data recorded; TTL clock running |
| `PendingDeletion` | TTL expired; S3 archival in progress |
| `Archived` | Written to S3 successfully; pending deletion |
| `ArchiveFailed` | S3 write failed; retrying with exponential backoff |

## Configuration

Configuration is loaded from environment variables on the manager pod.

| Variable | Default | Description |
|----------|---------|-------------|
| `KMORTEM_RETENTION_TTL` | `720h` | How long `NodeReport` objects live in the cluster (30 days). Any Go duration string is accepted, e.g. `48h`. |
| `KMORTEM_ARCHIVAL_ENABLED` | `false` | Set to `true` to archive reports to S3 before deletion. |
| `KMORTEM_ARCHIVAL_BUCKET` | — | S3 bucket name. Required when archival is enabled. |
| `KMORTEM_ARCHIVAL_PREFIX` | `kmortem/nodereports/` | S3 key prefix. |
| `KMORTEM_ARCHIVAL_REGION` | — | AWS region. Falls back to the SDK default chain if unset. |

When archival is enabled, each `NodeReport` is serialised to JSON and written to:

```
s3://{bucket}/{prefix}{nodeName}-{nodeUID}.json
```

Archival failures are retried with exponential backoff (base 30s, cap 10m) up to 5 times. After max retries a warning event is emitted and the report is deleted rather than held indefinitely.

## Deployment

**Prerequisites:** a running Kubernetes cluster, `kubectl` configured, and cluster-admin permissions.

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

To configure S3 archival, set the environment variables on the manager Deployment:

```yaml
env:
  - name: KMORTEM_ARCHIVAL_ENABLED
    value: "true"
  - name: KMORTEM_ARCHIVAL_BUCKET
    value: "my-cluster-forensics"
  - name: KMORTEM_ARCHIVAL_REGION
    value: "eu-west-1"
```

The operator requires an IAM role (via IRSA or instance profile) with `s3:PutObject` on the target bucket.

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
