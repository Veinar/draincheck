# draincheck usage

> **Status: pre-alpha.** The collector, rules and the text and JSON reports are wired to the CLI. Sections marked *planned* describe intended behaviour.

## Synopsis

```sh
draincheck --nodes NODE[,NODE...] [--kubeconfig PATH] [--context NAME] [--output text|json] [--verbose] [--fail-on blocked|risk|any] [--debug]
draincheck --version
```

## Flags

| Flag           | Default                | Description                                                  |
| -------------- | ---------------------- | ------------------------------------------------------------ |
| `--nodes`      | none (required)        | Nodes to drain, comma-separated. `'*'` selects every node.   |
| `--kubeconfig` | standard loading rules | Path to the kubeconfig file.                                 |
| `--context`    | current context        | Kubeconfig context to use.                                   |
| `--output`     | `text`                 | `text` (human-readable) or `json`. |
| `--verbose`    | off                    | Text output only: print every finding in full instead of the compact layout. JSON always carries every finding. |
| `--fail-on`    | `blocked`              | Lowest finding level that makes the run exit `1`: `blocked`, `risk` or `any`. |
| `--debug`      | off                    | Write one line to stderr for each action draincheck takes. The report on stdout does not change. See [Debug output](#debug-output). |
| `--version`    |                        | Print the version and exit.                                  |

`'*'` cannot be combined with node names. Quote it so the shell does not expand it. Surrounding whitespace in names is trimmed; empty names are error.

## Examples

```sh
draincheck --nodes worker-02,worker-04
draincheck --nodes '*' --context staging --output json
draincheck --nodes worker-02 --output json --fail-on risk   # also fail CI on RISK
draincheck --nodes worker-02 --output json
```

## Findings

Each finding has exactly one severity. There is no `SAFE`.

| Severity  | Meaning                                                              |
| --------- | -------------------------------------------------------------------- |
| `BLOCKED` | The drain will not complete from the observed state without outside action; waiting does not fix it. |
| `RISK`    | The drain proceeds, possibly in steps or after waiting, but something is lost, degraded or delayed. |
| `UNKNOWN` | The outcome cannot be established reliably.                          |

`kubectl drain` retries a refused eviction until its timeout, which is why the definitions rest on whether waiting can fix the problem. [SPEC.md](SPEC.md#31-what-blocked-and-risk-mean-for-a-drain-that-retries) has the reasoning and worked examples. Kubernetes and draincheck terms used in this document are defined in the [glossary](GLOSSARY.md).

Findings are sorted by namespace, kind and name.

### Grouping

A finding covers one cause, not one pod. A cause that reaches many pods, such as ten replicas behind one PodDisruptionBudget or a DaemonSet that runs on every node, is one finding. The pods and nodes it covers are listed in its `evidence`, so the JSON report holds every one of them. The text report shows one line per finding.

| Rule | One finding per | `resource` points at |
| --- | --- | --- |
| `PDB_DISRUPTION_BLOCKED` (`BLOCKED`) | PodDisruptionBudget | the PodDisruptionBudget |
| `PDB_DISRUPTION_BLOCKED` (`UNKNOWN`) | operator and namespace | the namespace |
| `PDB_BATCH_EXCEEDS_BUDGET` | PodDisruptionBudget | the PodDisruptionBudget |
| `PDB_STALE_STATUS` | PodDisruptionBudget | the PodDisruptionBudget |
| `PDB_SYNC_FAILED` | PodDisruptionBudget | the PodDisruptionBudget |
| `PDB_OVERLAPPING_SELECTORS` | pod | the pod |
| `NODE_NOT_SCHEDULABLE` | run (all affected nodes together) | the affected node names, joined by commas |
| `NO_REMAINING_NODES` | run | the target node names, joined by commas |
| `SINGLE_REPLICA_WORKLOAD` | Deployment or StatefulSet | the workload |
| `POD_OWNERSHIP` | DaemonSet, node (for its static pods) or pod without a controller | the DaemonSet, node or pod |
| `MISSING_RESOURCE_REQUESTS` | owning workload | the workload; a pod without a controller is its own workload |

An operator that keeps a PodDisruptionBudget for every node is grouped by the operator and the namespace. The operator is the API group and kind of the custom resource that controls the pods. The budgets of one operator in one namespace are one finding, however many nodes they cover, and each budget stays visible in `evidence.pdbs` with its numbers, nodes and pods. The same operator in another namespace, or another operator in the same namespace, is a finding of its own.

## Text output

`--output text` (the default) writes a human-readable report to stdout. Findings are grouped by severity, `BLOCKED` first, then `RISK`, then `UNKNOWN`; sections without findings are omitted. Inside a section, findings are grouped by rule, rules in alphabetical order. A rule block has a header with the finding count, one line per finding and the remediation once. The output is deterministic: the same findings and snapshot time give byte-identical output.

```text
draincheck - preflight maintenance analysis
Timestamp: 2026-10-14T09:30:00Z

=== FINDINGS SUMMARY ===
  BLOCKED: 1
  RISK:    0
  UNKNOWN: 0

=== BLOCKED (1) ===

[PDB_DISRUPTION_BLOCKED] 1 finding
  PodDisruptionBudget/billing/payments-api-pdb: PodDisruptionBudget billing/payments-api-pdb allows 0 disruptions (3 healthy, 3 desired), so eviction of its 2 pods on 2 target nodes will be refused.
  Remediation: Add healthy replicas, or relax minAvailable/maxUnavailable on the PodDisruptionBudget, before draining.

This report describes the observed cluster state at 2026-10-14T09:30:00Z and does not guarantee execution success. Cluster state may drift immediately after analysis.
```

- Findings of a rule that share a remediation print it once. A rule with several distinct remediations (for example `POD_OWNERSHIP` for DaemonSet versus static pods) prints each text once, with the resources it applies to: `Remediation (DaemonSet/a/ds-1, DaemonSet/a/ds-2): ...`.
- Long lists are collapsed after 5 entries and end in `... and M more`. A line at the end of the report says when anything was collapsed. `--verbose` prints every finding.
- `--verbose` prints one block per finding with its full description and remediation:

```text
[POD_OWNERSHIP] DaemonSet/monitoring/agent
  DaemonSet monitoring/agent has 2 pods on 2 target nodes. kubectl drain refuses DaemonSet pods without --ignore-daemonsets and never evicts them.
  Remediation: Drain with --ignore-daemonsets; DaemonSet pods stay until the node itself goes down.
```

- `Timestamp` is when the cluster snapshot was taken (RFC 3339, UTC), not when the report was printed. The closing disclaimer is mandatory and is always the last line.
- With no findings the report says `No findings reported.` That is not a guarantee that the drain will succeed; there is no `SAFE` status.
- The cluster name and the target node list are *planned*.
- The layout is meant for people. Use `--output json` in scripts and CI.

## JSON output

`--output json` writes the document below to stdout. Errors go to stderr, so stdout is always either a complete document or empty.

```json
{
  "schema_version": "v1",
  "generated_at": "2026-10-14T09:30:00Z",
  "disclaimer": "This report describes the observed cluster state at 2026-10-14T09:30:00Z and does not guarantee execution success. Cluster state may drift immediately after analysis.",
  "findings": [
    {
      "rule_id": "PDB_DISRUPTION_BLOCKED",
      "severity": "BLOCKED",
      "resource": "PodDisruptionBudget/billing/payments-api-pdb",
      "evidence": {
        "namespace": "billing",
        "name": "payments-api-pdb",
        "current_healthy": 3,
        "desired_healthy": 3,
        "disruptions_allowed": 0,
        "nodes": ["worker-02", "worker-03"],
        "pods": [
          { "namespace": "billing", "name": "payments-api-0", "node": "worker-02" },
          { "namespace": "billing", "name": "payments-api-1", "node": "worker-03" }
        ]
      },
      "description": "...",
      "remediation": "..."
    }
  ]
}
```

- `schema_version` is `"v1"`. Any change to the document shape is a deliberate, reviewed change, pinned by the golden file `testdata/report/json-v1.golden.json`.
- `findings` is always an array, empty when there are none. It is sorted by namespace, kind, name, rule ID and severity, so the same findings and snapshot time give byte-identical output.
- `resource` is `Kind/namespace/name`, or `Kind/name` for cluster-scoped objects.
- `evidence` is a typed object whose shape depends on `rule_id` (see *Rules*); it is `null` when a rule has none. `PDB_DISRUPTION_BLOCKED` has two shapes: one budget for `BLOCKED`, and the budgets of one operator for `UNKNOWN`.
- `generated_at` is the time the cluster snapshot was taken (RFC 3339, UTC), not the time the document was written, so the same snapshot always gives the same bytes.
- `disclaimer` is the mandatory statement that the report does not guarantee execution success. It names the same timestamp.

## Rules

Rules are implemented in `internal/analyzer` and all of them run on every invocation. How each rule keeps its noise down (false-positive cases, downgrades, grouping) is recorded in [RULES.md](RULES.md).

### `PDB_DISRUPTION_BLOCKED`

Severity: `BLOCKED`; `UNKNOWN` for budgets managed by an operator (below).

Raised for a PodDisruptionBudget with `status.disruptionsAllowed == 0` that selects pods on the target nodes, so the eviction API will refuse to evict them. Findings are grouped by budget: ten replicas behind one PDB are one finding, not ten.

- **Resource:** the PDB (`PodDisruptionBudget/<namespace>/<name>`), one finding per PDB.
- **Description:** states the numbers once (healthy, desired, 0 allowed) and how many pods on how many target nodes are affected.
- **Evidence:** the PDB `namespace` and `name`, `current_healthy`, `desired_healthy` and `disruptions_allowed`, then `nodes` (the target nodes the selected pods run on, sorted) and `pods`, each with its `namespace`, `name` and `node`, sorted by namespace and name. Every selected pod is listed, so grouping hides nothing. A pod selected by two blocked PDBs is listed under both.
- Only pods scheduled on a target node count. Pods in phase `Succeeded`, `Failed` or `Pending` and pods that are already terminating are skipped, since the eviction API deletes them without consulting a budget. DaemonSet-managed pods and static (mirror) pods are skipped, since `kubectl drain` does not evict them; they stay visible through `POD_OWNERSHIP`. A PDB that selects only such pods gives no finding.
- **Unhealthy pods.** A pod that is not Ready is evicted past its budget with `unhealthyPodEvictionPolicy: AlwaysAllow`, and with `IfHealthyBudget` (the default when the field is unset) while `currentHealthy >= desiredHealthy` and `desiredHealthy > 0`. Such a pod is not counted as blocked; the JSON `evidence` lists it as `evictable_unhealthy_pods`, apart from `pods`, and a PDB left with no pod it can refuse gives no finding. With an unhealthy budget (`currentHealthy < desiredHealthy`) under `IfHealthyBudget` the pod is still refused: the finding stays `BLOCKED` and the description says the budget itself is unhealthy. The `evidence` also carries `unhealthy_pod_eviction_policy` when the field is set.
- A PDB whose `unhealthyPodEvictionPolicy` is a value draincheck does not know is reported as `UNKNOWN`, not guessed.
- A PDB with an empty selector (`{}`) selects every pod in its namespace; one with no selector selects none.
- A PDB whose selector is invalid is skipped.
- A PDB whose status is stale is not reported here: `PDB_STALE_STATUS` reports it as `UNKNOWN`, because its numbers cannot be trusted.
- A budget that allows some disruptions but fewer than the batch causes is `PDB_BATCH_EXCEEDS_BUDGET`, not reported here.

**Budgets managed by an operator.** A budget that selects a pod controlled by a custom resource is `UNKNOWN`, not `BLOCKED`. A pod is controlled by a custom resource when the API group of its controller is not the core group, `apps` or `batch`. An operator can create and remove such a budget itself, so whether the drain stalls depends on operator state draincheck cannot see. Pods controlled by a Deployment, StatefulSet, Job or ReplicationController keep their budget `BLOCKED`; a budget over pods of both kinds is `UNKNOWN`. Only selected pods on the target nodes count, so an operator pod on another node does not change the answer.

- **Resource:** the namespace (`Namespace/<name>`), one finding per operator and namespace.
- **Evidence:** `managed_by` (the operator's `api_group` and `kind`), `namespace` and `pdbs`. `pdbs` lists the budgets sorted by name, each with the fields described above: `namespace`, `name`, the three numbers, `nodes` and `pods`. A budget over pods of several custom resources is listed under the first by API group, then kind.
- **Description:** names the operator, says how many budgets it manages in the namespace, and how many pods on how many target nodes they cover.
- **Remediation:** names the operator, says to check its drain documentation and the health of what it protects, and not to relax `minAvailable`, because the operator may recreate the budget. A few operators add a sentence of their own; every other operator gets the general text.
- It is `UNKNOWN`, so it makes the run exit `1` only with `--fail-on any`.

### `PDB_OVERLAPPING_SELECTORS`

Severity: `BLOCKED`.

Raised for a pod on a target node when more than one PodDisruptionBudget in the same namespace selects it. The eviction API does not support a pod covered by several budgets and refuses the eviction, regardless of how many disruptions each budget allows.

- **Resource:** the pod (`Pod/<namespace>/<name>`), one finding per pod.
- **Evidence:** the `namespace` and the `pdbs` that select the pod, sorted by name.
- Only pods scheduled on a target node count. Pods in phase `Succeeded`, `Failed` or `Pending` and pods that are already terminating are skipped, since eviction does not consult budgets for them. So are DaemonSet-managed pods and static (mirror) pods, which `kubectl drain` never evicts. They stay visible through `POD_OWNERSHIP`.
- An empty selector (`{}`) selects every pod in its namespace, so it overlaps with any other budget there; one with no selector selects none.
- A PDB whose selector is invalid is skipped.
- A pod that is both overlapped and covered by a zero-disruption budget is also listed in the `PDB_DISRUPTION_BLOCKED` finding of that budget.

### `PDB_BATCH_EXCEEDS_BUDGET`

Raised as `RISK` for a PodDisruptionBudget with `status.disruptionsAllowed > 0` when the pods it selects on **all** target nodes together are more than it allows at once. `kubectl drain` works through nodes one after another and a server dry run sees each node fit; draincheck adds the pods of every target node that share a budget. With a disruption allowed the first eviction succeeds, its replacement restores the budget once it is Ready, and the drain proceeds in steps: it waits, it does not stall. A budget at 0 is the stall case and stays `BLOCKED` under `PDB_DISRUPTION_BLOCKED`, so one budget is never in both.

- **Resource:** the PDB (`PodDisruptionBudget/<namespace>/<name>`), one finding per PDB.
- **Description:** states how many pods on how many target nodes the PDB selects, how many disruptions it allows at once, and that the drain waits for replacements.
- **Evidence:** the same fields as `PDB_DISRUPTION_BLOCKED` (`nodes` and `pods` included) plus `pods_to_evict`, the number of pods the batch evicts.
- The pods counted are the same as for `PDB_DISRUPTION_BLOCKED`: only pods on a target node, without pods that bypass every budget, DaemonSet-managed and static pods, and without unhealthy pods the eviction API lets past the budget (they do not use it up).
- A stale budget (`PDB_STALE_STATUS`), a budget whose controller failed (`PDB_SYNC_FAILED`) and a budget over pods controlled by an operator are not reported here: their numbers say nothing reliable.
- A budget whose `unhealthyPodEvictionPolicy` is a value draincheck does not know is not reported here.
- It is `RISK`, so it makes the run exit `1` only with `--fail-on risk` or `any`.

### `PDB_STALE_STATUS`

Severity: `UNKNOWN`.

Raised for a PodDisruptionBudget whose `status.observedGeneration` differs from `metadata.generation` while it selects pods on the target nodes. The status of such a budget describes an older spec, so its numbers, `disruptionsAllowed` included, cannot be trusted, and draincheck cannot say whether the drain will be refused. The finding replaces the `PDB_DISRUPTION_BLOCKED` one for the same budget. It is `UNKNOWN`, so it makes the run exit `1` only with `--fail-on any`.

- **Resource:** the PDB (`PodDisruptionBudget/<namespace>/<name>`), one finding per PDB.
- **Description:** states `status.observedGeneration` and `metadata.generation`, and how many pods on how many target nodes are affected.
- **Evidence:** the same fields as `PDB_DISRUPTION_BLOCKED` (the numbers as last reported, `nodes` and `pods`) plus `observed_generation` and `generation`.
- A stale PDB is reported whatever its status allows, since an out-of-date number that allows a disruption cannot be trusted either.
- The pods counted are the same as for `PDB_DISRUPTION_BLOCKED`: only pods on a target node, without finished, DaemonSet-managed and static pods. A stale PDB that selects only such pods gives no finding.
- A PDB whose status is only briefly behind (right after its spec was edited) clears up once the disruption controller catches up; run draincheck again. One that stays stale points to an unhealthy kube-controller-manager.
- Reporting a PDB whose selector matches no pod at all (probably a typo) is *planned*.

### `PDB_SYNC_FAILED`

Severity: `UNKNOWN`.

Raised for a PodDisruptionBudget whose `DisruptionAllowed` condition has the reason `SyncFailed` while it selects pods on the target nodes. The disruption controller could not compute the budget, for example because the owner of the pods has no scale subresource. It then records that no disruption is allowed, which says nothing about the real budget, so draincheck cannot say whether the drain will be refused. The finding replaces the `PDB_DISRUPTION_BLOCKED` one for the same budget. It is `UNKNOWN`, so it makes the run exit `1` only with `--fail-on any`.

- **Resource:** the PDB (`PodDisruptionBudget/<namespace>/<name>`), one finding per PDB.
- **Description:** names the error the controller recorded, and how many pods on how many target nodes are affected.
- **Evidence:** the same fields as `PDB_DISRUPTION_BLOCKED` (the numbers as last reported, `nodes` and `pods`) plus `reason` (`SyncFailed`) and `message` (the condition message, empty when the controller gave none).
- A failed PDB is reported whatever its status allows, since the numbers cannot be trusted either way.
- A PDB that is stale as well is reported only by `PDB_STALE_STATUS`, so one budget is one finding.
- A budget at 0 with the reason `InsufficientPods` is a plain budget at 0 (`PDB_DISRUPTION_BLOCKED`), not a failure.
- The pods counted are the same as for `PDB_DISRUPTION_BLOCKED`: only pods on a target node, without finished, DaemonSet-managed and static pods.

### `NODE_NOT_SCHEDULABLE`

Severity: `RISK`.

Raised when the drain moves pods and some nodes that stay up cannot take them: a node outside the target set that is cordoned (`spec.unschedulable`) or whose `Ready` condition is `False` or `Unknown`. The drain itself completes, but the evicted pods have fewer places to land than the node count suggests. It is `RISK`, so it makes the run exit `1` only with `--fail-on risk` or `any`.

- **Resource:** `Node/<names>`, the affected nodes sorted and joined by commas. One finding covers all of them, since the cause is one.
- **Description:** how many of the remaining nodes cannot take pods and why each cannot (`cordoned`, `not Ready`, or `cordoned and not Ready`), and how many pods move and at most how many nodes they can land on (taints and capacity are not checked, so the real number can be smaller).
- **Evidence:** `pods_to_move`, `remaining_nodes` (every node outside the target set, whatever its state) and `nodes`, each with `name`, `cordoned` and `ready_status` (`True`, `False` or `Unknown`; omitted when the node reports no `Ready` condition).
- It fires only when at least one pod on a target node moves: a pod that `kubectl drain` evicts and a controller recreates. DaemonSet pods, static pods, finished pods and pods with no controller need no new home, so a cordoned spare node does not matter for a drain that moves none.
- A target node that is already cordoned is not reported. `kubectl drain` cordons the targets itself and prints `already cordoned`.
- A node that reports no `Ready` condition is not flagged for readiness; real nodes always report one.
- Taints, resource pressure and capacity are not looked at here. Placement and capacity rules are *planned*.

### `NO_REMAINING_NODES`

Severity: `RISK`.

Raised when the drain moves pods and no node is left that takes them: every node outside the target set is cordoned (`spec.unschedulable`) or has a `Ready` condition of `False` or `Unknown`, or all nodes are targets. The drain itself completes, but the evicted pods are recreated by their controllers and stay `Pending` until a node can take them, so the workloads are down. It is `RISK`, so it makes the run exit `1` only with `--fail-on risk` or `any`.

- **Resource:** `Node/<names>`, the target nodes sorted and joined by commas. One finding for the run.
- **Description:** how many of the nodes are targets, how many pods move and that they stay `Pending`.
- **Evidence:** `pods_to_move`, `target_nodes`, `total_nodes` and `remaining_schedulable` (always `0`).
- It fires only when at least one pod on a target node moves, counted as for `NODE_NOT_SCHEDULABLE`: DaemonSet pods, static pods, finished pods and pods with no controller need no new home.
- When the nodes left exist but are all cordoned or not `Ready`, `NODE_NOT_SCHEDULABLE` is reported too and names them.
- A node that reports no `Ready` condition counts as room.
- Taints, affinity and capacity are not looked at, so a finding-free run does not promise the pods fit. Placement and capacity rules, and a per-workload variant of this rule, are *planned*.

### `SINGLE_REPLICA_WORKLOAD`

Severity: `RISK`.

Raised when a pod on a target node belongs to a Deployment or StatefulSet that desires one replica. The pod is evicted before its replacement is Ready, so the workload is unavailable in between, even with no PodDisruptionBudget. `kubectl drain` does not warn about it. The drain itself completes, so it is `RISK`; it makes the run exit `1` only with `--fail-on risk` or `any`.

- **Resource:** the Deployment or StatefulSet. One finding per workload.
- **Description:** the workload, its replica count and the node its pod runs on.
- **Evidence:** `owner_kind`, `owner_name`, `replicas` and `pods` (each with `namespace`, `name` and `node`, sorted in that order). A rollout in progress can run two pods of a one-replica Deployment; both are listed in one finding.
- The count is the desired one, `spec.replicas` (1 when unset), not the number of live pods.
- Finished pods, DaemonSet pods and static pods are skipped, because `kubectl drain` does not evict them. A pod that is already terminating is skipped too: the drain is not what removes it. A pod whose workload is not in the snapshot is skipped, because its replica count is unknown.
- A HorizontalPodAutoscaler is not looked at: a workload that scales above 1 under load is still reported while it runs one replica.
- Needs `list` on `deployments` and `statefulsets` (see *Cluster access*).

### `POD_OWNERSHIP`

Severity: `RISK`.

Raised for pods on a target node whose ownership gives them special `kubectl drain` behaviour. Findings are grouped by cause, so a DaemonSet that runs on many nodes is one finding, not one per pod. The `category` in the evidence says which kind it is:

| Category | One finding per | What drain does |
| --- | --- | --- |
| `DaemonSet` | DaemonSet, covering all its pods on the target nodes | Refused unless `--ignore-daemonsets`; never evicted. |
| `StaticPod` | Node, covering the API mirrors of kubelet static pods on it (annotation `kubernetes.io/config.mirror`) | Skipped; cannot be rescheduled, stops with the node. |
| `Unmanaged` | Pod with no controller | Refused unless `--force`; not recreated once deleted. |

- **Resource:** `DaemonSet/<namespace>/<name>` for DaemonSet pods, `Node/<name>` for static pods and `Pod/<namespace>/<name>` for a pod with no controller.
- **Evidence:** `category`, `nodes` (the nodes the covered pods run on, sorted) and `pods`, each with its `namespace`, `name` and `node`, sorted by namespace and name. A DaemonSet finding also has `owner_kind` and `owner_name`. Every covered pod is listed, so grouping hides nothing.
- Static pods get one finding per node rather than one for the whole drain, because the node is what you choose with `--nodes`. The description names the static pods; the point is to plan around the node outage, since kubectl skips them.
- It is `RISK`, not `BLOCKED`, because whether the drain stops depends on flags draincheck cannot observe.
- Only pods scheduled on a target node count. Pods in phase `Succeeded` or `Failed` are skipped, since `kubectl drain` removes them without further checks.
- Pods managed by any other controller (Deployment, StatefulSet, Job, ...) are not reported. A pod whose ReplicaSet is missing from the snapshot is treated as managed.
- A pod with a mirror annotation is always `StaticPod`, even though it also has a `Node` owner reference.
- A pod controlled by a DaemonSet that no longer exists is still reported as a DaemonSet pod, since draincheck does not list DaemonSets. `kubectl drain` handles such an orphan differently: it fails with a not-found error, and only `--force` deletes the pod. Handling this case is *planned*.

### `MISSING_RESOURCE_REQUESTS`

Severity: `RISK`.

Raised for a workload with a pod on a target node that has at least one regular container that declares no CPU request or no memory request. The scheduler treats a missing request as zero, so the pod can be placed on a node with no real room for it, and capacity checks cannot account for it.

- **Resource:** the owning workload (for example `Deployment/<namespace>/<name>`, `StatefulSet/...`, `Job/...`), one finding per workload however many replicas are affected. A pod with no controller is its own workload (`Pod/<namespace>/<name>`). A pod owned by a ReplicaSet resolves to the Deployment that owns the ReplicaSet.
- **Description:** states how many of the workload's pods on the target nodes are affected, for example `2 of 3 pods of Deployment shop/web`.
- **Evidence:** `owner_kind` and `owner_name` (omitted for a pod with no controller), `pods_on_targets` (the workload's pods on the target nodes that `kubectl drain` would evict, with or without requests) and `pods`. Each entry of `pods` has the pod `name`, its `node` and its `containers`: each container with a missing request, in spec order, with `missing_cpu` and `missing_memory`. Pods are sorted by name. The pod namespace is the namespace of the resource.
- Only pods scheduled on a target node count, in `pods` and in `pods_on_targets`. Pods in phase `Succeeded` or `Failed`, DaemonSet-managed pods and static (mirror) pods are skipped, since `kubectl drain` does not evict them and they are never rescheduled. They stay visible through `POD_OWNERSHIP`.
- Init containers are not inspected.
- A CPU or memory request set at pod level (`spec.resources.requests`) covers every container for that resource.
- Requests are read from the stored pod spec, so values filled in by a LimitRange or an admission webhook count as set.

## Cluster access

Read-only. Needs `list` on:

- `nodes`
- `pods` (all namespaces)
- `replicasets` (`apps`, all namespaces)
- `poddisruptionbudgets` (`policy`, all namespaces)
- `deployments` and `statefulsets` (`apps`, all namespaces)
- `jobs` and `cronjobs` (`batch`, all namespaces)
- `persistentvolumeclaims` (all namespaces) and `persistentvolumes`

draincheck never cordons, evicts or modifies anything.

[`examples/rbac/`](../examples/rbac) has a `ClusterRole` with exactly these permissions, each with the `list` verb only, and a `ClusterRoleBinding` to adapt. Apply them with `kubectl apply -f examples/rbac/` after replacing the subject. Keep the role and this list in step.

## Debug output

`--debug` makes every action draincheck takes visible. Each one writes a line at level `DEBUG` to **stderr**, in `key=value` form, so a debug log shows what draincheck read from the cluster, what it decided and how the run ended. Use it to find out why a run behaved as it did, and attach it when you report a problem.

stdout is not touched. The text or JSON report is byte-for-byte the same with and without `--debug`, so scripts that read stdout keep working. Without `--debug`, nothing is written to stderr except errors. `--debug` does not change the exit code either; the one exception is stderr piped into a command that quits early, which ends the run with `141`. See [Closed pipes](#closed-pipes).

| Message | Written | Fields |
| --- | --- | --- |
| `settings` | when the run starts | `kubeconfig` (the files that are read), `context`, `nodes`, `all_nodes`, `output`, `verbose`, `fail_on` |
| `list` | after each list call to the API server | `resource`, `count`, `duration` |
| `list failed` | when a list call fails | `resource`, `duration`, `error` |
| `resolve target nodes` | when the target nodes are accepted | `requested`, `all`, `targets` |
| `resolve target nodes failed` | when a node does not exist or the cluster has no nodes | `requested`, `all`, `error` |
| `rule start` and `rule done` | around each rule | `rule`; `findings` on `rule done` |
| `summary` | when all rules are done | `blocked`, `risk`, `unknown`, `fail_on` |
| `write report` | just before the report is written | `format`, `verbose`, `findings` |
| `finished` | when the run ends, also after an error | `exit_code` |

Example, with the path anonymized and the rule lines shortened:

```text
$ draincheck --nodes worker-1 --debug >report.txt
time=2026-10-18T17:29:41.507Z level=DEBUG msg=settings kubeconfig=[/home/user/.kube/config] context=dev nodes=[worker-1] all_nodes=false output=text verbose=false fail_on=blocked
time=2026-10-18T17:29:41.514Z level=DEBUG msg=list resource=nodes count=1 duration=4.807574ms
time=2026-10-18T17:29:41.515Z level=DEBUG msg=list resource=pods count=1 duration=1.629527ms
time=2026-10-18T17:29:41.516Z level=DEBUG msg=list resource=replicasets count=0 duration=433.921µs
time=2026-10-18T17:29:41.516Z level=DEBUG msg=list resource=poddisruptionbudgets count=0 duration=399.377µs
time=2026-10-18T17:29:41.516Z level=DEBUG msg=list resource=deployments count=0 duration=512.004µs
time=2026-10-18T17:29:41.516Z level=DEBUG msg=list resource=statefulsets count=0 duration=401.118µs
time=2026-10-18T17:29:41.516Z level=DEBUG msg=list resource=jobs count=0 duration=389.652µs
time=2026-10-18T17:29:41.516Z level=DEBUG msg=list resource=cronjobs count=0 duration=377.230µs
time=2026-10-18T17:29:41.516Z level=DEBUG msg=list resource=persistentvolumeclaims count=0 duration=402.771µs
time=2026-10-18T17:29:41.516Z level=DEBUG msg=list resource=persistentvolumes count=0 duration=365.490µs
time=2026-10-18T17:29:41.516Z level=DEBUG msg="resolve target nodes" requested=[worker-1] all=false targets=[worker-1]
time=2026-10-18T17:29:41.516Z level=DEBUG msg="rule start" rule=PDB_DISRUPTION_BLOCKED
time=2026-10-18T17:29:41.516Z level=DEBUG msg="rule done" rule=PDB_DISRUPTION_BLOCKED findings=0
...
time=2026-10-18T17:29:41.516Z level=DEBUG msg="rule done" rule=POD_OWNERSHIP findings=1
...
time=2026-10-18T17:29:41.516Z level=DEBUG msg=summary blocked=0 risk=1 unknown=0 fail_on=blocked
time=2026-10-18T17:29:41.516Z level=DEBUG msg="write report" format=text verbose=false findings=1
time=2026-10-18T17:29:41.516Z level=DEBUG msg=finished exit_code=0
```

Debug lines name files, resources, rules and nodes, and give counts. They never contain tokens, client certificates, `Authorization` headers or anything read from the kubeconfig besides its path. The `kubeconfig` field lists the files draincheck reads: the path from `--kubeconfig`, or else the files in `KUBECONFIG`, or else `~/.kube/config`. An empty `context` means the current context of the kubeconfig.

## Exit codes

| Code | Meaning                                                                 |
| ---- | ----------------------------------------------------------------------- |
| `0`  | Analysis completed; no finding at or above the `--fail-on` level.       |
| `1`  | Analysis completed; at least one finding at or above `--fail-on`.       |
| `2`  | draincheck could not complete: bad flags, unreadable kubeconfig, cluster unreachable, unknown node. |

`--fail-on` picks what counts:

| Value     | Exits `1` on                       |
| --------- | ---------------------------------- |
| `blocked` | `BLOCKED` (default)                |
| `risk`    | `BLOCKED` or `RISK`                |
| `any`     | any finding, including `UNKNOWN`   |

`RISK` findings such as `PDB_BATCH_EXCEEDS_BUDGET` make the run exit `1` only with `--fail-on risk` or `any`. `UNKNOWN` findings, which are `PDB_STALE_STATUS`, `PDB_SYNC_FAILED` and the operator-managed budgets of `PDB_DISRUPTION_BLOCKED`, make the run exit `1` only with `--fail-on any`.

The report is written before the exit code is decided, so a run that exits `1` still prints every finding.

`--debug` does not change the exit code. The same run ends with the same code with and without it, and the `finished` debug line shows the code the process exits with.

### Closed pipes

draincheck is a Go program, and Go ends a program with `SIGPIPE` when it writes to stdout or stderr and the other end of the pipe has already been closed. The process then does not exit with `0`, `1` or `2`; a shell reports `141` (128 + 13). This happens in two cases:

- stdout is piped into a command that quits before reading everything, such as `head`. This happens with or without `--debug`.
- stderr is piped into such a command and `--debug` is set. Without `--debug`, stderr is written only when a run fails and exits `2`, so runs that exit `0` or `1` are not affected. With `--debug`, every run writes to stderr, so a run that would exit `0` or `1` can end with `141` instead.

In a CI step that gates on the exit code, write the report and the debug lines to files or to a reader that reads until the end:

```sh
draincheck --nodes worker-1 --debug >report.txt 2>debug.log
```

When the exit code decides whether a pipeline passes, treat `141` as a failed run and not as `1`.

## See also

- [Specification](SPEC.md)
- [Glossary](GLOSSARY.md)
