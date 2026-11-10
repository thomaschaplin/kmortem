<img src="assets/logo.png" alt="kmortem logo" width="256" height="256" />

# kmortem

> **Nodes die. Evidence shouldn't.**

kmortem is a Kubernetes operator that writes a post-mortem for every node that goes away. When a node starts to terminate, it records what was running on it, why it went, and what was affected, and saves that as a `NodeReport` you can query with `kubectl` long after the node is gone.

## Why

When a node is terminated by a spot interruption, an autoscaler scale-down or a drain, the evidence goes with it. The pods are rescheduled, the events age out, and by the time someone asks "what was running on that node, and why did it die?", there's nothing left to look at.

kmortem captures that evidence while it still exists, so the question has an answer.

## Quick start

```bash
helm install kmortem oci://ghcr.io/thomaschaplin/kmortem/charts/kmortem \
  --namespace kmortem --create-namespace
```

Then, after a node has gone:

```
$ kubectl get nodereports
NAME                                                        NODE             CAUSE                       PHASE      AGE
ip-10-0-1-42-a1b2c3d4-1234-5678-abcd-ef0123456789          ip-10-0-1-42     SpotInterruption            Complete   2d
ip-10-0-2-18-b2c3d4e5-2345-6789-bcde-f01234567890          ip-10-0-2-18     ManualDrain                 Complete   5d
ip-10-0-3-7-c3d4e5f6-3456-789a-cdef-012345678901           ip-10-0-3-7      ClusterAutoscalerScaleDown  Complete   12h
```

Report names include the node's UID, so a replacement node that reuses the same hostname gets its own report.

<details>
<summary>Example report (<code>kubectl get nr &lt;name&gt; -o yaml</code>)</summary>

```yaml
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

</details>

## What's in a report

| Section | What it records |
|---------|-----------------|
| **Termination** | When it was detected, the likely cause, and who started it |
| **Instance** | EC2 instance ID, instance type, availability zone, spot or on-demand, region |
| **Node** | Kernel, OS image, container runtime, kubelet version, CPU and memory capacity, when the node was created, labels and taints |
| **Pods** | Every pod on the node: owning workload, start and end time, exit reason (Evicted, Completed, OOMKilled, Error, Unknown), phase, requests and limits, restart count, containers and images |
| **OOM kills** | Containers killed for exceeding their memory limit, with the limit and the time |
| **Conditions** | Ready, MemoryPressure, DiskPressure and PIDPressure, with when each last changed |
| **PDB violations** | PodDisruptionBudgets covering pods on the node, and how many disruptions they allowed |

Instance details come from node labels and `spec.providerID`, so kmortem doesn't need to call the EC2 metadata service.

## How it works

### Spotting a terminating node

kmortem watches every Node and treats it as terminating when any of these is true:

- It has the `node.kubernetes.io/unschedulable` taint (it was cordoned or is being drained)
- Its `Ready` condition is `False` or `Unknown`
- It has the `cluster-autoscaler.kubernetes.io/scale-down` annotation
- It has a deletion timestamp

Karpenter nodes that are still starting up are ignored. A new node can briefly look unschedulable or not ready while it registers, and without this check every scale-up would produce a false report.

### Working out why

| Signal | Cause |
|--------|-------|
| Karpenter's `karpenter.sh/disrupted` taint on a spot node | `SpotInterruption` |
| `cluster-autoscaler.kubernetes.io/scale-down` annotation | `ClusterAutoscalerScaleDown` |
| `MemoryPressure`, `DiskPressure` or `PIDPressure` is `True` | `NodeConditionFailure` |
| `node.kubernetes.io/unschedulable` taint and nothing else | `ManualDrain` |
| None of the above | `Unknown` |

Rules are checked from top to bottom, and the first match wins. `initiatedBy` is `cluster-autoscaler` when the autoscaler annotation is present. Otherwise it's the field manager that last changed the node's taints, such as `karpenter` or `kubectl`.

### Collecting the evidence

The controller creates the `NodeReport` straight away and collects the evidence in the background, so a slow collection never holds up other nodes.

Collectors run in two groups at the same time:

- **Hot:** data that's only accurate while the node is still up: the pod inventory and OOM kills
- **Warm:** data the API server keeps after the node is gone: condition history and PDB violations

Collection has a 90-second limit, which fits inside the 2-minute AWS spot interruption notice. If a collector fails or runs out of time, the report is still completed with whatever was gathered, and the failure is recorded as a `HotCollectionPartialFailure` or `WarmCollectionPartialFailure` condition on its status. A partial report beats no report.

### Lifecycle

| Phase | Meaning |
|-------|---------|
| `Collecting` | Evidence is being gathered |
| `Complete` | Collection has finished, and the retention period has started |

Reports are deleted automatically once they're older than the retention period.

## Configuration

| Helm value | Default | Description |
|------------|---------|-------------|
| `retentionTTL` | `720h` | How long reports are kept (30 days). Any Go duration, e.g. `48h`. An invalid value falls back to the default. Sets `KMORTEM_RETENTION_TTL`. |
| `namespace.name` | `kmortem` | Namespace the operator runs in |
| `namespace.create` | `false` | Create the namespace as part of the chart. Usually you'd use `--create-namespace` instead. |
| `installCRDs` | `true` | Install the `NodeReport` CRD. Set to `false` if you manage CRDs separately, e.g. with GitOps. |
| `rbac.create` | `true` | Create the ClusterRole and ClusterRoleBinding |
| `serviceAccount.create` | `true` | Create a ServiceAccount |
| `serviceAccount.name` | `""` | ServiceAccount name. Defaults to `<release>-manager`. |
| `image.repository` | `ghcr.io/thomaschaplin/kmortem` | Image repository |
| `image.tag` | `""` | Image tag. Defaults to the chart's `appVersion`. |
| `image.pullPolicy` | `IfNotPresent` | Image pull policy |
| `image.pullSecrets` | `[]` | Image pull secret names |
| `replicaCount` | `1` | Number of operator replicas |
| `leaderElection.enabled` | `true` | Enable leader election |
| `leaderElection.namespace` | `""` | Namespace for the leader election lease. Defaults to `namespace.name`. |
| `resources` | 10m / 64Mi requests, 500m / 128Mi limits | Operator resource requests and limits |

For example:

```bash
helm upgrade --install kmortem oci://ghcr.io/thomaschaplin/kmortem/charts/kmortem \
  --namespace kmortem --create-namespace \
  --set retentionTTL=48h
```

## Limitations

- **Instance details are AWS-only.** On other clouds, the instance section is left mostly empty. The rest of the report still works.
- **kmortem has to see the node before it's gone.** If a node is deleted before the operator notices any of the signals above (for example, while the operator itself is restarting), no report is written.
- **Run the operator away from spot capacity.** If the node running kmortem is the one being interrupted, it can't report on itself.
- **OOM kills come from container status,** so only the most recent termination of each container is recorded.

## Development

```bash
make build                                  # format, vet and compile to bin/kmortem
make test                                   # format, vet and run the tests with the race detector
make lint                                   # golangci-lint
make manifests                              # regenerate the CRD and RBAC, and sync them into the Helm chart
make docker-build IMG=my-registry/kmortem:dev
```

CI checks formatting, runs `go vet` and the tests, and checks that the Helm chart's CRD and RBAC match the generated manifests.

## CRD reference

- **API group:** `kmortem.io`
- **Version:** `v1alpha1`
- **Kind:** `NodeReport`
- **Short name:** `nr`
- **Scope:** Cluster

## How this was built

kmortem was built with AI coding agents. The agents wrote the code. I decided what it should do and checked that it did.

That meant choosing which evidence is worth keeping, what has to be captured inside a 2-minute spot window, and that a partial report is better than none. It also meant catching mistakes the agent didn't. One example: Karpenter nodes look exactly like terminating nodes while they're starting up, and every scale-up was producing a false report until that was fixed.

AI makes the building fast. Knowing when it's wrong is still the job.

## License

[MIT](LICENSE)
