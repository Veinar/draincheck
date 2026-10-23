# draincheck glossary

Short definitions of the Kubernetes and draincheck terms used in the draincheck documentation. Each Kubernetes term links to the official documentation.

| Term | Meaning |
| --- | --- |
| [`BLOCKED`](#blocked) | A finding level. A known constraint definitively prevents or rejects the planned operation under the observed state. |
| [`RISK`](#risk) | A finding level. A configuration or workload condition could cause disruption, downtime or scheduling friction. |
| [`UNKNOWN`](#unknown) | A finding level. The tool cannot establish the outcome reliably due to unsupported constraints, custom webhooks or dynamic races. |
| [cordon](#cordon) | Marking a node unschedulable so the scheduler does not place new pods on it. |
| [DaemonSet](#daemonset) | A workload that runs one pod on every node that matches it. |
| [disruptionsAllowed](#disruptionsallowed) | A PodDisruptionBudget status field: the number of voluntary disruptions currently allowed. |
| [drain](#drain) | Evicting the pods of a node so the node can be taken down for maintenance. |
| [eviction](#eviction) | The API-driven, voluntary removal of a pod, respecting PodDisruptionBudgets. |
| [maxUnavailable](#maxunavailable) | A PodDisruptionBudget field: how many pods may be unavailable at the same time. |
| [minAvailable](#minavailable) | A PodDisruptionBudget field: how many pods must stay available during voluntary disruptions. |
| [owner reference](#owner-reference) | The metadata link from a pod to the controller that owns it. |
| [PodDisruptionBudget](#poddisruptionbudget) | A policy object that limits how many pods of a set may be down at once during voluntary disruptions. |
| [static pod](#static-pod) | A pod defined by a file on the node and managed by the kubelet directly, not by the API server. |

## Terms

### BLOCKED

A finding level. A known constraint definitively prevents or rejects the planned operation under the observed state. In drain terms: the drain will not complete from the observed state without outside action (a change to the cluster, the workload or the budget), and waiting does not fix it. There is no `SAFE` level in draincheck. See [SPEC.md](SPEC.md#3-design-philosophy-no-false-confidence).

### RISK

A finding level. A configuration or workload condition could cause disruption, downtime or scheduling friction. In drain terms: the drain proceeds, possibly in steps or after waiting, but something is lost, degraded or delayed. There is no `SAFE` level in draincheck. See [SPEC.md](SPEC.md#3-design-philosophy-no-false-confidence).

### UNKNOWN

A finding level. The tool cannot establish the outcome reliably due to unsupported constraints, custom webhooks or dynamic races. draincheck reports `UNKNOWN` instead of guessing. There is no `SAFE` level in draincheck. See [SPEC.md](SPEC.md#3-design-philosophy-no-false-confidence).

### cordon

Marking a node unschedulable so the scheduler does not place new pods on it. Cordoning does not evict running pods; `kubectl drain` cordons and then evicts. See [Safely Drain a Node](https://kubernetes.io/docs/tasks/administer-cluster/safely-drain-node/).

### DaemonSet

A workload resource that runs one pod on every node that matches it, typically for infrastructure such as networking or storage agents. `kubectl drain` never evicts DaemonSet pods and refuses a drain without `--ignore-daemonsets`. See [DaemonSet](https://kubernetes.io/docs/concepts/workloads/controllers/daemonset/).

### disruptionsAllowed

A PodDisruptionBudget status field: the number of voluntary disruptions currently allowed. At `0` the eviction API refuses evictions of pods the budget covers. draincheck reports a budget at `0` as `BLOCKED` and a batch larger than the budget as `RISK`. See [PodDisruptionBudget status](https://kubernetes.io/docs/tasks/run-application/configure-pdb/).

### drain

Evicting the pods of a node so the node can be taken down for maintenance, after cordoning it. See [Safely Drain a Node](https://kubernetes.io/docs/tasks/administer-cluster/safely-drain-node/).

### eviction

The API-driven, voluntary removal of a pod, which the API server checks against PodDisruptionBudgets before it proceeds. `kubectl drain` evicts pods one by one. See [Evictions](https://kubernetes.io/docs/concepts/scheduling-eviction/).

### maxUnavailable

A PodDisruptionBudget field: how many pods of a set may be unavailable at the same time during voluntary disruptions. It can be a number or a percentage. See [PodDisruptionBudget](https://kubernetes.io/docs/concepts/scheduling-eviction/).

### minAvailable

A PodDisruptionBudget field: how many pods of a set must stay available during voluntary disruptions. It can be a number or a percentage. See [PodDisruptionBudget](https://kubernetes.io/docs/concepts/scheduling-eviction/).

### owner reference

The metadata link from a pod to the controller that owns it, such as a ReplicaSet, DaemonSet or StatefulSet. draincheck uses owner references to group pods by workload and to find DaemonSet pods, static pods and unmanaged naked pods. See [Object Metadata](https://kubernetes.io/docs/concepts/overview/working-with-objects/owners-dependents/).

### PodDisruptionBudget

A policy object that limits how many pods of a set may be down at once during voluntary disruptions such as drains and evictions. See [PodDisruptionBudget](https://kubernetes.io/docs/tasks/run-application/configure-pdb/) and [Voluntary and Involuntary Disruptions](https://kubernetes.io/docs/concepts/scheduling-eviction/).

### static pod

A pod defined by a file on the node and managed by the kubelet directly, not by the API server; its mirror pod appears in the API only for visibility. The kubelet restarts it whenever it fails, so it cannot be evicted or rescheduled and stops only when the node goes down. See [Static Pods](https://kubernetes.io/docs/concepts/cluster-administration/static-pod/).
