<img src="assets/logo.png" alt="logo" width="256" height="256" />

# kmortem

> **Nodes die. Evidence shouldn't.**

kmortem is a Kubernetes operator that watches for node termination events and preserves a forensic record of everything that happened — what ran, why it died, and what the blast radius was — as a `NodeReport` custom resource. Records are queryable via `kubectl` long after the node is gone.

## How it works

kmortem runs two controllers:

**NodeReconciler** watches all Node objects. When it detects termination signals (cordoned taint, failed Ready condition, cluster-autoscaler annotation, Karpenter disruption taint, or deletion timestamp), it creates a `NodeReport` and immediately dispatches a background goroutine to collect evidence. The reconciler itself returns instantly — it never blocks on collection.

**NodeReportReconciler** manages the lifecycle of `NodeReport` objects: checking collection progress, enforcing TTL expiry, and deleting the object once the TTL elapses.

Evidence is gathered by four collectors split into two parallel groups:

- **Hot** (time-critical, while the node is still reachable): pod inventory, OOMKill events
- **Warm** (API-server backed, survives node removal): condition history, PodDisruptionBudget violations

Instance metadata (EC2 instance ID, type, AZ, lifecycle, region) is resolved synchronously from node labels and `spec.providerID` — no IMDS call required.

Both groups run simultaneously with a 90-second total timeout, designed to fit inside the AWS spot interruption 2-minute window. Partial failures are tolerated — a partial report is better than no report.

## Usage

NodeReport names embed the node UID to ensure uniqueness across node replacements — nodes recycled with the same hostname produce distinct records.

```
$ kubectl get nodereports
NAME                                                        NODE             CAUSE                       PHASE      AGE
ip-10-0-1-42-a1b2c3d4-1234-5678-abcd-ef0123456789          ip-10-0-1-42     SpotInterruption            Complete   2d
ip-10-0-2-18-b2c3d4e5-2345-6789-bcde-f01234567890          ip-10-0-2-18     ManualDrain                 Complete   5d
ip-10-0-3-7-c3d4e5f6-3456-789a-cdef-012345678901           ip-10-0-3-7      ClusterAutoscalerScaleDown  Complete   12h
```

```
$ kubectl get nr ip-10-0-1-42-a1b2c3d4-1234-5678-abcd-ef0123456789 -o yaml
apiVersion: kmortem.io/v1alpha1
kind: NodeReport
metadata:
  name: ip-10-0-1-42-a1b2c3d4-1234-5678-abcd-ef0123456789
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
| Karpenter `karpenter.sh/disrupted` taint on a spot node | `SpotInterruption` |
| `cluster-autoscaler.kubernetes.io/scale-down` annotation | `ClusterAutoscalerScaleDown` |
| `MemoryPressure`, `DiskPressure`, or `PIDPressure` condition = True | `NodeConditionFailure` |
| `node.kubernetes.io/unschedulable` taint (no other signals) | `ManualDrain` |
| None of the above | `Unknown` |

`initiatedBy` is also populated: `cluster-autoscaler` when the CA annotation is present, otherwise the Kubernetes field manager that last updated `spec.taints`.

## Evidence gathered

- **Instance metadata** — EC2 instance ID, type, availability zone, lifecycle (spot/on-demand), region; resolved from node labels and `spec.providerID`
- **Pod inventory** — every pod that was scheduled on the node: namespace, owner workload (Deployment/StatefulSet/DaemonSet/Job/CronJob), start time, end time, exit reason (Evicted/Completed/OOMKilled/Error/Unknown), phase, resource requests and limits, restart count, container names and images
- **OOM kills** — any container killed for exceeding its memory limit: pod, container, memory limit, timestamp
- **Condition history** — all node conditions (Ready, MemoryPressure, DiskPressure, PIDPressure) with last transition times; status values are always `True`, `False`, or `Unknown`
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

**Prerequisites:** a running Kubernetes cluster and `kubectl` configured with cluster-admin permissions.

The operator is distributed as a Helm chart. The chart installs the CRD, RBAC, namespace, and Deployment in one step.

```bash
helm install kmortem oci://ghcr.io/thomaschaplin/kmortem/charts/kmortem
```

To customise the installation (image tag, retention TTL, resource limits, etc.) pass values at install time:

```bash
helm install kmortem oci://ghcr.io/thomaschaplin/kmortem/charts/kmortem \
  --set retentionTTL=48h \
  --set resources.limits.memory=256Mi
```

To upgrade:

```bash
helm upgrade kmortem oci://ghcr.io/thomaschaplin/kmortem/charts/kmortem
```

### Helm values

| Value | Default | Description |
|-------|---------|-------------|
| `image.repository` | `ghcr.io/thomaschaplin/kmortem` | Container image repository |
| `image.tag` | `""` (uses `appVersion`) | Image tag override |
| `image.pullPolicy` | `IfNotPresent` | Image pull policy |
| `image.pullSecrets` | `[]` | Image pull secret names (e.g. for private registries) |
| `replicaCount` | `1` | Number of operator replicas |
| `installCRDs` | `true` | Install the NodeReport CRD; set `false` if managing CRDs externally |
| `namespace.create` | `false` | Create the release namespace as a chart resource; `helm install --create-namespace` is used by default |
| `serviceAccount.create` | `true` | Create a ServiceAccount |
| `serviceAccount.name` | `""` (auto-generated) | ServiceAccount name override |
| `retentionTTL` | `720h` | NodeReport retention duration |
| `leaderElection.enabled` | `true` | Enable leader election |
| `leaderElection.namespace` | `""` (uses release namespace) | Namespace for leader election lease |
| `resources.limits.cpu` | `500m` | CPU limit |
| `resources.limits.memory` | `128Mi` | Memory limit |
| `resources.requests.cpu` | `10m` | CPU request |
| `resources.requests.memory` | `64Mi` | Memory request |

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

## Transparency

Yes, every line of this was vibe-coded. Not a single character was typed by a human with intent. And you know what? It works, it solves a real problem, and it shipped in an afternoon instead of a sprint.

We live in an era where the bottleneck isn't writing code — it's having good ideas and the judgement to know when something is worth building. So instead of spending a week hand-crafting the perfect operator, this one was described, reasoned about, and iterated on in plain English.

The nodes still die. The evidence still doesn't. Ship fast, solve problems. Now go build something else.
