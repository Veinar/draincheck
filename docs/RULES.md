# draincheck rules

Design rules for analyzer rules. Every rule that can raise `BLOCKED` or `RISK` must say up front how it avoids crying wolf. A false `BLOCKED` costs more trust than a missed `RISK`, so this is a design requirement, not polish.

What each rule reports is described in [USAGE.md](USAGE.md#rules). This file records how each rule keeps its noise down.

## Noise budget

Pick the severity from the definitions in [SPEC.md](SPEC.md#31-what-blocked-and-risk-mean-for-a-drain-that-retries): `BLOCKED` only when waiting cannot fix the problem, `RISK` when the drain proceeds but something is lost, degraded or delayed.

Every rule answers the eight criteria below, in its entry in this file and in the pull request that adds or changes it.

| # | Criterion | Requirement |
| - | --------- | ----------- |
| 1 | False-positive case | The realistic situation in which the rule fires although the drain would succeed. |
| 2 | Downgrade | How that case is detected and what severity it gets instead. `UNKNOWN` when state is stale or a constraint cannot be evaluated; never keep `BLOCKED` on doubt. |
| 3 | Mirror kubectl | A rule must not tell the operator to fix something `kubectl drain` handles on its own. For pods kubectl skips (DaemonSet-managed with `--ignore-daemonsets`, mirror pods, terminal pods), a rule whose rationale is eviction or rescheduling emits nothing, and its entry says why. Information that is still true (a DaemonSet needs the flag, static pods go down with the node) is reported once, grouped, not per pod. |
| 4 | Grouping | How many findings a large workload produces (10 replicas behind one PDB) and whether findings are grouped, with per-pod detail kept in typed evidence. One finding per cause, never one per pod. |
| 5 | Estimates | A rule based on an estimate (capacity, soft preferences) never emits `BLOCKED`, shows the numbers it used and states that it is an estimate. |
| 6 | Evidence | The finding carries enough typed evidence to verify it in seconds. |
| 7 | Fixture | `testdata/` includes the false-positive case and a test proving the downgrade. |
| 8 | Visibility | Nothing is dropped silently. Every fact a rule used to report per pod stays reachable, in grouped typed evidence (JSON) or in verbose text. Reducing noise changes how a fact is presented, not whether it exists. |

### Template

Copy this into the entry of a new rule.

```markdown
### `RULE_ID`

Severity: `BLOCKED` | `RISK` | `UNKNOWN`.

| Criterion | Answer |
| --- | --- |
| False-positive case | |
| Downgrade | |
| Mirror kubectl | |
| Grouping | |
| Estimates | |
| Evidence | |
| Fixture | |
| Visibility | |
```

Mark anything not implemented yet as *planned* and link the issue. Document only what the code does.

## Advice standard

A finding must give the operator more than `kubectl drain` would: not only what is wrong, but how to confirm it in seconds and what to do next. The `description` and the `remediation` of a finding carry five things between them.

| # | What | Where | Requirement |
| - | ---- | ----- | ----------- |
| 1 | The fact | `description` | The numbers and names, taken from the evidence: `allows 0 disruptions (3 healthy, 3 desired)`, not `is too strict`. |
| 2 | Why it matters for this drain | `description` | In terms of the definitions in [SPEC.md](SPEC.md#31-what-blocked-and-risk-mean-for-a-drain-that-retries): the drain waits (`BLOCKED`), completes with something lost or delayed (`RISK`), or the outcome is not known (`UNKNOWN`). |
| 3 | What `kubectl drain` itself does with it | `description` | Only what has been checked against kubectl (see SPEC.md and *Mirror kubectl* in the entries). Where draincheck cannot establish it, the description says so. |
| 4 | A read-only command that confirms it | `remediation` | One or more commands in backticks, starting with `kubectl get`, `kubectl describe` or `kubectl auth can-i`. A command never writes: none contains `delete`, `patch`, `apply`, `create`, `cordon`, `drain`, `evict`, `scale` or `edit`. |
| 5 | The action, safest first | `remediation` | In order, after the command. An action that writes is described in words, never as a command. |

Further requirements:

- **`UNKNOWN` says what draincheck could not establish and what to look at to decide.** The description names the missing fact (a stale status, an operator, a policy value); the command reads exactly that.
- **An estimate says it is an estimate and shows its inputs.** A count that ignores taints or capacity says so in the same sentence.
- **Placeholders.** A command names the object with a placeholder in angle brackets (`<namespace>`, `<name>`, `<pod>`, `<node>`, `<kind>`, `<pdb>`, `<owner>`), taken from the `resource` and the evidence of the finding. The text stays the same for every finding of a rule, so the compact text report still prints it once.
- **Plain commands.** A command is one `kubectl` invocation with no pipe, no shell and no `xargs`. To read a field, use `-o yaml` or `-o jsonpath=` and name the field in words.
- **Permissions.** A verify command can need a permission that draincheck's own role does not have (`describe`, or `get` on a custom resource). It runs as the operator, not as draincheck.

`TestAdviceContract` (`internal/analyzer/advice_contract_test.go`) enforces the fact (1) and the read-only command (4) for every finding of every default rule, and fails for a rule that has no case in its table. It also rejects a command that is not one plain `kubectl get`, `describe` or `auth can-i` invocation (no pipe, `;`, `&` or `$(`) and a placeholder not listed above. `TestAdviceTableListsEveryDefaultRule` fails for a rule that has no row below. A structured `verify` field in the JSON report is *planned* too (#130). Until then the commands are in the `remediation` text.

### Advice per rule

Each row is the advice of one rule; `<...>` are the placeholders described above. `kubectl behaviour` says only what is established: SPEC.md section 3.1, or checked with kubectl as the entry of the rule says.

| Rule | Fact | What `kubectl drain` does | Verify | Action |
| ---- | ---- | ------------------------- | ------ | ------ |
| `PDB_DISRUPTION_BLOCKED` | The budget allows 0 disruptions (healthy and desired counts) and selects N pods on M target nodes. | Retries a refused eviction until `--timeout`, infinite by default: it waits and does not complete. | `kubectl get pdb -n <namespace> <name>`, `kubectl describe pdb -n <namespace> <name>` | Bring the selected pods to Ready until the budget allows a disruption; or relax `minAvailable` / `maxUnavailable` if the budget is stricter than the workload needs. `UNKNOWN` (policy not known): read `spec.unhealthyPodEvictionPolicy` with `kubectl get pdb -n <namespace> <name> -o yaml`, look up the value for the cluster version. `UNKNOWN` (operator-managed): `kubectl get pdb -n <namespace>`, `kubectl get pdb -n <namespace> <name> -o yaml`, then the operator's drain documentation; do not relax `minAvailable`. |
| `PDB_UNSATISFIABLE` | The budget can never allow a disruption: `desiredHealthy` is not below `expectedPods` (`minAvailable` at or above the replica count, or `maxUnavailable: 0`), and it selects N pods on M target nodes. | Retries a refused eviction until `--timeout`, infinite by default: it waits and does not complete, and nothing will change that. | `kubectl get pdb -n <namespace> <name>`, `kubectl get pdb -n <namespace> <name> -o yaml` | Scale the workload above `minAvailable` and wait for Ready; or relax `minAvailable` / `maxUnavailable` for the drain and restore it afterwards. `maxUnavailable: 0` is often deliberate: change it only if you own the decision. |
| `PDB_OVERLAPPING_SELECTORS` | A pod is selected by N budgets in one namespace; the numbers each budget reports are listed and flagged as not reliable. | The eviction API refuses the pod whatever the budgets allow. `kubectl drain` reports `This pod has more than one PodDisruptionBudget, which the eviction subresource does not support.` for that pod at once and does not retry it. | `kubectl get pod -n <namespace> <pod> --show-labels`, `kubectl describe pdb -n <namespace> <pdb>` | Narrow the selectors so each pod has one budget, or merge the budgets. |
| `PDB_BATCH_EXCEEDS_BUDGET` | The batch evicts N pods of a budget that allows M at once. | Works through the nodes one after another and waits for each replacement; the drain proceeds. | `kubectl get pdb -n <namespace> <name>` | Expect a slower drain; to speed it up add replicas or relax the budget, or drain the nodes one after another. |
| `PDB_STALE_STATUS` | `status.observedGeneration` differs from `metadata.generation`; the numbers are out of date. | Not established: it depends on what the eviction API makes of the stale status. | `kubectl get pdb -n <namespace> <name> -o jsonpath='{.metadata.generation} {.status.observedGeneration}'` | Wait and run draincheck again; if it stays stale, check kube-controller-manager. |
| `PDB_SYNC_FAILED` | The disruption controller could not compute the budget (reason and message in the finding). | Not established: the recorded 0 is not a real budget. | `kubectl get pdb -n <namespace> <name> -o jsonpath='{.status.conditions}'` | Fix what the message names (a pod owner without a scale subresource, a selector it cannot resolve), run draincheck again. |
| `NODE_NOT_SCHEDULABLE` | N of the M nodes that stay up are cordoned or not Ready; the moved pods can land on at most K nodes. An upper bound: taints and capacity are not checked. | Only evicts; completes whatever the remaining nodes look like. | `kubectl get nodes`, `kubectl describe node <node>` | Repair the not Ready nodes or uncordon the cordoned ones; otherwise count only the other nodes as room. |
| `NO_REMAINING_NODES` | The drain targets N of M nodes and no node is left that takes new pods (cordons and readiness only). | Only evicts; completes, and the pods stay `Pending`. | `kubectl get nodes`, `kubectl get pods -A --field-selector=status.phase=Pending` | Add a node, or repair or uncordon one, or drain fewer nodes at once. |
| `WORKLOAD_UNTOLERATED_TAINTS` | The workload has N pods on the targets and none can land on any of the M nodes left, because of taints; other pods of the drain can. | Only evicts; completes, and the workload's pods stay `Pending`. | `kubectl describe node <node>`, `kubectl get pod -n <namespace> <pod> -o jsonpath='{.spec.tolerations}'` | Add or uncordon a node the workload tolerates, or drain fewer nodes; give the workload a toleration only if that node is meant to run it. |
| `SINGLE_REPLICA_WORKLOAD` | The Deployment or StatefulSet has 1 replica and it runs on a target node. | Does not warn about the downtime. | `kubectl get <kind> -n <namespace> <name> -o jsonpath='{.spec.replicas}'`, `kubectl get pdb -n <namespace>` | Run at least 2 replicas, with a budget; or accept the downtime. |
| `ALL_REPLICAS_ON_TARGETS` | The Deployment or StatefulSet has 2 or more replicas and every Ready pod runs on a target node, with no budget that makes the drain wait. | Evicts the pods node by node; the workload is down until a replacement is Ready elsewhere, and it does not warn. | `kubectl get pods -n <namespace> -o wide`, `kubectl get pdb -n <namespace>` | Drain the nodes one at a time and wait for the replacements; or spread the replicas to nodes outside the batch first; or add a PodDisruptionBudget. |
| `POD_OWNERSHIP` | DaemonSet pods, static pods or a pod with no controller are on a target node. `BLOCKED` for a DaemonSet pod without `--ignore-daemonsets` and for a pod with no controller without `--force`; `RISK` for a pod with no controller with `--force` and for static pods. | DaemonSet: refused without `--ignore-daemonsets`, never evicted. Static: skipped. No controller: refused without `--force`, not recreated. | DaemonSet: `kubectl get daemonset -n <namespace> <name>`. Static: `kubectl get pods -A --field-selector spec.nodeName=<node>`, `kubectl get pod -n <namespace> <pod> -o jsonpath='{.metadata.annotations}'`. No controller: `kubectl get pod -n <namespace> <name> -o jsonpath='{.metadata.ownerReferences}'` | DaemonSet: drain with `--ignore-daemonsets`. Static: plan for the node outage. No controller: put it under a controller, or accept the loss and use `--force`. |
| `POD_LOCAL_STORAGE` | The workload has N pods with `emptyDir` volumes (named in the evidence) on M target nodes. `BLOCKED` without `--delete-emptydir-data`, `RISK` with it. | Without the flag: refuses with `cannot delete Pods with local storage` and stops before it evicts anything. With it: deletes the pods, and the data in the volumes is lost. | `kubectl get pod -n <namespace> <pod> -o jsonpath='{.spec.volumes}'` | Move data that must survive to a PersistentVolumeClaim; or, for a cache or scratch data, drain with `--delete-emptydir-data` and accept the loss. |
| `MISSING_RESOURCE_REQUESTS` | N of the workload's M pods on the targets have containers with no CPU or memory request. | Evicts the pods whatever they request. | `kubectl get pod -n <namespace> <pod> -o jsonpath='{.spec.containers[*].resources.requests}'` | Set requests on every container (or at pod level) in the pod template. |

## Pods a drain acts on

One function, `classifyPod` in `internal/analyzer/pod_drain.go`, says what a drain with the given flags does with a pod on a target node. Every rule asks it; no rule reads the phase, the deletion timestamp or the mirror annotation on its own (a test fails if one does). The same pod is therefore in every finding it belongs to, or in none.

| State of the pod | Fate | Counted as a pod to move (`NODE_NOT_SCHEDULABLE`, `NO_REMAINING_NODES`) | Budget consulted (PDB rules) | Other rules |
| --- | --- | --- | --- | --- |
| Running or Unknown phase, controlled by a Deployment, StatefulSet, Job or other controller | evicted, recreated by its controller | yes | yes | `SINGLE_REPLICA_WORKLOAD`, `ALL_REPLICAS_ON_TARGETS` (when Ready), `MISSING_RESOURCE_REQUESTS`, `POD_LOCAL_STORAGE` (with an `emptyDir`) |
| Pending, with a controller | evicted, recreated by its controller | yes | no (eviction deletes it without a budget) | `SINGLE_REPLICA_WORKLOAD`, `MISSING_RESOURCE_REQUESTS`, `POD_LOCAL_STORAGE` (`BLOCKED` only: it has no data to lose) |
| No controller (an owner reference that is not the controller counts as none) | evicted, not recreated | no | yes, unless Pending | `POD_OWNERSHIP`, `MISSING_RESOURCE_REQUESTS` (as its own workload), `POD_LOCAL_STORAGE` (with an `emptyDir`) |
| DaemonSet-controlled | left alone by `kubectl drain` | no | no | `POD_OWNERSHIP` |
| Static (mirror annotation), even with a DaemonSet or Node owner reference | left alone by `kubectl drain` | no | no | `POD_OWNERSHIP` |
| Succeeded or Failed (any owner, also DaemonSet and static) | finished | no | no | none |
| Already terminating (deletion timestamp set, not finished) | already leaving | no | no | `POD_OWNERSHIP`; `POD_LOCAL_STORAGE` (`BLOCKED` only, with an `emptyDir`) |
| Not matched by `--pod-selector`, in any of the states above | not selected: `kubectl drain` does not list it | no | no | none (checked with kubectl v1.36.1: only the matching pods are named, and a selector that matches nothing drains cleanly) |
| Any of the states above, with `--disable-eviction` | unchanged: the fate and the class are the same | unchanged | no, for every state (`kubectl drain` deletes the pods, so the eviction API and its budgets are not called) | unchanged |

Order of precedence: not selected, then finished, then already terminating, then left alone, then evicted. For `POD_OWNERSHIP` the class decides, not the fate: a terminating DaemonSet, static or controller-less pod is still reported, because `kubectl drain` applies its DaemonSet, static and unreplicated checks to it as well. `POD_LOCAL_STORAGE` is the same for the refusal, because kubectl refuses a terminating pod with an `emptyDir` too, and skips the `RISK`, because the drain is not what removes the pod. For every other rule a terminating pod is skipped: its controller has already created the replacement, so the drain does not take anything down.

## Entries

### `PDB_DISRUPTION_BLOCKED`

Severity: `BLOCKED`; `UNKNOWN` for budgets managed by an operator.

| Criterion | Answer |
| --- | --- |
| False-positive case | `status.disruptionsAllowed` is read as is. A PDB whose status is not yet reconciled (`observedGeneration` differs from `generation`) reports a stale number; it is not reported here, `PDB_STALE_STATUS` reports it as `UNKNOWN` instead. A PDB that an operator creates and removes itself can show 0 allowed disruptions and still not stop the drain, because the operator may remove it when the node is cordoned. Such a PDB is recognised by its pods (one is controlled by a custom resource) and reported as `UNKNOWN`. A PDB that selects only DaemonSet or static pods gives no finding, because kubectl never evicts those pods (see *Mirror kubectl*). |
| Downgrade | Pods that bypass every budget (phase `Succeeded`, `Failed` or `Pending`, or already terminating) and pods kubectl never evicts (below) are skipped, and a PDB with an invalid selector is skipped. A pod that is not Ready is skipped too when the eviction API lets it past the budget (`unhealthyPodEvictionPolicy`, below); it is listed in the evidence as `evictable_unhealthy_pods`, apart from `pods`, and a PDB left with no pod it can refuse gives no finding. A PDB whose `unhealthyPodEvictionPolicy` is a value draincheck does not know is `UNKNOWN`, not guessed. A PDB left with no pod gives no finding. A PDB whose status is stale (`observedGeneration` differs from `generation`) is skipped and becomes `UNKNOWN` through `PDB_STALE_STATUS`. A PDB that selects a pod on the target nodes together with another PDB is skipped: its numbers can be out of date (see `PDB_OVERLAPPING_SELECTORS`, *Numbers of overlapping budgets*), and `PDB_OVERLAPPING_SELECTORS` reports the pod, which the eviction API refuses whatever the budgets say. A PDB that selects a pod controlled by a custom resource (an API group other than core, `apps` or `batch`) is `UNKNOWN`, not `BLOCKED`; a PDB over pods of built-in controllers only stays `BLOCKED`. Only the selected pods on the target nodes count. The state of the operator is not evaluated. An optional remediation text per API group never changes the severity; an operator without one gets the general text. |
| Mirror kubectl | Implemented: terminal pods, Pending and terminating pods (the eviction handler deletes them without consulting a budget), DaemonSet-managed pods and static (mirror) pods are skipped, because `kubectl drain` never evicts the last two, so the eviction API never consults a budget for them. DaemonSet and static pods stay visible through `POD_OWNERSHIP`. **Unhealthy pods:** a pod that is not Ready is evicted past its budget with `unhealthyPodEvictionPolicy: AlwaysAllow`, and with `IfHealthyBudget` (the default when unset) while `currentHealthy >= desiredHealthy` and `desiredHealthy > 0`. With an unhealthy budget (`IfHealthyBudget`, `currentHealthy < desiredHealthy`) such a pod is still refused, and the description says the budget itself is unhealthy. The status of a stale or sync-failed budget cannot say which pods are let through, so none is. One corner is not covered: a pod whose DaemonSet has been deleted is still skipped as a DaemonSet pod, because draincheck does not list DaemonSets (see `POD_OWNERSHIP`); kubectl handles such an orphan differently, so a budget on it can be missed. |
| Grouping | `BLOCKED`: one finding per PDB, pointing at the PDB: 10 replicas behind one PDB give one finding. The description states the numbers (healthy, desired) and the pod and node counts once. `UNKNOWN`: one finding per operator (API group and kind) and namespace, pointing at the namespace, with every PDB in the evidence. An operator that keeps a PDB per node gives one finding. A PDB over pods of several custom resources is placed under the first by API group, then kind. |
| Estimates | Not an estimate: the number comes from the PDB status. |
| Evidence | `findings.PDBEvidence`: PDB `namespace` and `name`, `current_healthy`, `desired_healthy`, `disruptions_allowed`, `nodes` (the target nodes the pods run on) and `pods` (each with `namespace`, `name` and `node`). For an operator-managed PDB: `findings.OperatorManagedPDBsEvidence`: `managed_by` (`api_group` and `kind`), `namespace` and `pdbs`, each a `findings.PDBEvidence`. |
| Fixture | `testdata/analyzer/pdb-disruption-blocked.yaml`: a PDB over pods on three nodes, two PDBs on one pod (not reported here, see `PDB_OVERLAPPING_SELECTORS`), and pods that must not be reported (finished, DaemonSet, static), including a PDB that selects only a DaemonSet pod. Also PDBs that allow a disruption and PDBs with an invalid or missing selector. The stale case is in `testdata/analyzer/pdb-stale-status.yaml`. `testdata/analyzer/pdb-operator-managed.yaml` has an operator with a PDB on each of four nodes, the same operator in a second namespace, two operators in one namespace, an operator with a remediation text of its own, budgets over Deployments, StatefulSets, Jobs and ReplicationControllers that stay `BLOCKED`, a PDB over pods of both kinds, an operator PDB that allows a disruption and one that is stale. |
| Visibility | Every selected pod and its node is in `pods` and `nodes` in the JSON report, so grouping hides nothing. Operator-managed PDBs stay visible one by one in `pdbs`. The text report shows one line per finding; `--verbose` shows the full description. |

### `PDB_UNSATISFIABLE`

Severity: `BLOCKED`.

| Criterion | Answer |
| --- | --- |
| False-positive case | `maxUnavailable: 0` is often deliberate: the owner wants no pod ever evicted. The finding is still true (the drain cannot complete) and says the setting may be deliberate; it does not tell the operator to relax it. The numbers come from the budget status, so a status that is out of date is not trusted: it is `PDB_STALE_STATUS`. |
| Downgrade | Never `BLOCKED` when the numbers cannot be trusted or the budget cannot be judged: a stale status, a failed sync, an operator-managed pod, an overlapping budget or an unknown `unhealthyPodEvictionPolicy` stay with the rule that reports that state (`PDB_STALE_STATUS`, `PDB_SYNC_FAILED`, `PDB_DISRUPTION_BLOCKED` as `UNKNOWN`, `PDB_OVERLAPPING_SELECTORS`). A budget with no expected pod in its status is not reported. |
| Mirror kubectl | It selects the same pods as `PDB_DISRUPTION_BLOCKED` (one shared selection): terminal, Pending, terminating, DaemonSet-managed and static pods are not counted, and with `--disable-eviction` no budget is consulted. kubectl refuses the eviction in the same way and retries; what it cannot say is that retrying is pointless. |
| Split from `PDB_DISRUPTION_BLOCKED` | One definition, `unsatisfiableBudget`, decides which rule reports a budget at 0. This rule takes `desiredHealthy >= expectedPods`, a budget that waiting cannot fix. `PDB_DISRUPTION_BLOCKED` keeps a budget at 0 that can clear by itself (a pod that is not Ready, a rollout in progress). No budget gets both. |
| Grouping | One finding per PDB, pointing at the PDB: 10 replicas behind one PDB give one finding. The description states the numbers and the pod and node counts once. |
| Estimates | Not an estimate: the numbers come from the PDB status. |
| Evidence | `findings.PDBUnsatisfiableEvidence`: PDB `namespace` and `name`, `expected_pods`, `desired_healthy`, `min_available` and `max_unavailable` as configured (the unset one is omitted), `nodes` and `pods` (each with `namespace`, `name` and `node`). |
| Fixture | `testdata/analyzer/pdb-unsatisfiable.yaml`: `minAvailable` equal to and above the replicas, `maxUnavailable` 0 and `"0%"`, a budget at 0 that can clear (one pod not Ready), and the states left to other rules: stale, operator-managed, unknown policy, DaemonSet-only and a budget that allows disruptions. |
| Visibility | Every selected pod and its node is in `pods` and `nodes`, so grouping hides nothing. |

### `PDB_OVERLAPPING_SELECTORS`

Severity: `BLOCKED`.

| Criterion | Answer |
| --- | --- |
| False-positive case | The eviction API rejects a pod selected by several PDBs whatever the budgets allow, so for a pod that is evicted the finding is exact. DaemonSet and static pods, which kubectl never evicts, are skipped (see *Mirror kubectl*). The rule reads the selectors from the PDB spec and never its status, so a stale status (see `PDB_STALE_STATUS`) does not affect it. |
| Downgrade | Implemented: pods that bypass every budget (terminal, Pending, terminating), pods kubectl never evicts (below) and PDBs with an invalid selector are skipped. No other downgrade; none is needed for evicted pods. |
| Mirror kubectl | Implemented: terminal pods, DaemonSet-managed pods and static (mirror) pods are skipped, because `kubectl drain` never evicts them, so the eviction API never sees them. For a pod that is evicted, `kubectl drain` gets the refusal as an error and does not retry it, unlike a budget at 0 (checked on a kind cluster, Kubernetes v1.37.0); with `--timeout` the other pods are still waited for. They stay visible through `POD_OWNERSHIP`. A pod whose DaemonSet has been deleted is still skipped, for the reason given under `PDB_DISRUPTION_BLOCKED`. |
| Grouping | One finding per pod, with every matching PDB in evidence. Replicas sharing the same overlap each get a finding. |
| Estimates | Not an estimate. |
| Evidence | `findings.OverlappingPDBsEvidence`: `namespace`, the `pdbs` that select the pod, sorted by name, and `budgets`: the same budgets in the same order, each with `name`, `current_healthy`, `desired_healthy` and `disruptions_allowed` as the status reports them. |
| Numbers of overlapping budgets | They are kept in the evidence and not trusted. Measured on a kind cluster, Kubernetes v1.37.0 (issue #139): with two budgets on the same Ready pods, the disruption controller logs `matches multiple PodDisruptionBudgets. Chose "<name>" arbitrarily.` for each pod change, and a budget that was not chosen kept a number from an earlier sync (`currentHealthy` 0 or 1 while both pods were Ready, `allowed` 0). In 20 fresh overlaps, 19 had at least one such budget (16 of 20 for the second budget, 3 of 20 for the first); every budget never chosen after its pods were Ready was wrong (14 of 14), and picked ones were right in 21 of 26. A wrong budget stayed wrong for 570 s of waiting with nothing touched; deleting a pod, restarting or scaling the Deployment corrected it in 10 of 11 cases, and it made a right budget wrong once. A budget added to a settled Deployment was right at the first reading in 35 of 35, and the 5 untouched ones stayed right; once the pods were replaced it was wrong in 5 of 20 (deleting a pod three times 3 of 5, a rolling update 1 of 5, scaling 2->3->2 1 of 5, a rollout restart 0 of 5), always one of the two budgets, and none of the 5 healed within 7 minutes. That only the chosen budget is updated is the reading of these measurements, not checked in the controller source. So `PDB_DISRUPTION_BLOCKED` and `PDB_BATCH_EXCEEDS_BUDGET` leave out a budget that selects a pod on the target nodes together with another budget; this rule reports the pod, and the description says the numbers can be out of date and prints them. Only overlaps on the target nodes count, so a budget left out is always named by a finding of this rule. `PDB_STALE_STATUS` and `PDB_SYNC_FAILED` still report a budget that is stale or failed, since they describe the object, not its numbers. |
| Fixture | `testdata/analyzer/pdb-overlapping-selectors.yaml`; `testdata/analyzer/pdb-overlapping-numbers.yaml` has the recorded pair (`dup-a` allowing 1, `dup-b` at 0), budgets that stay reported next to it, and an overlap that only reaches a pod off the targets. |
| Visibility | One finding per affected pod; nothing is hidden, and the numbers of every overlapping budget are in the evidence. |

### `PDB_STALE_STATUS`

Severity: `UNKNOWN`.

| Criterion | Answer |
| --- | --- |
| False-positive case | `status.observedGeneration` lags `metadata.generation` for a short time after every edit of a PDB spec, until the disruption controller catches up, so a run in that window reports a PDB that is about to be fine. That is why the answer is `UNKNOWN`, not a verdict: run draincheck again after a short wait. A PDB that stays out of date points to an unhealthy kube-controller-manager. Both fields being `0` counts as reconciled. A budget the controller failed on never gets an `observedGeneration` (seen on kind, Kubernetes v1.37.0, #145), so it always looks stale; waiting would not help it, so it is left to `PDB_SYNC_FAILED`. |
| Downgrade | This rule is the downgrade. A stale PDB that `PDB_DISRUPTION_BLOCKED` would report as `BLOCKED` is `UNKNOWN` here, and so is a stale PDB that claims to allow a disruption, because that number is out of date too. Never `BLOCKED` or `RISK`: nothing is known about the real numbers. A budget with the `SyncFailed` condition is not reported here, `PDB_SYNC_FAILED` reports it and names the cause. |
| Mirror kubectl | It counts the same pods as `PDB_DISRUPTION_BLOCKED` (they share one selection): terminal, DaemonSet-managed and static pods are not counted, so a stale PDB that selects only those gives no finding. A pod whose DaemonSet has been deleted is still skipped, see `PDB_DISRUPTION_BLOCKED`. |
| Grouping | One finding per PDB, pointing at the PDB: 10 replicas behind one stale PDB give one finding. The description states the two generations and the pod and node counts once. |
| Estimates | Not an estimate. The finding says the numbers cannot be trusted. |
| Evidence | `findings.PDBStaleStatusEvidence`: everything in `findings.PDBEvidence` (the numbers as last reported, `nodes` and `pods`) plus `observed_generation` and `generation`. |
| Fixture | `testdata/analyzer/pdb-stale-status.yaml`: a stale PDB that allows 0, a stale PDB that allows 1, settled PDBs (one allows 0 and stays `BLOCKED`), a stale PDB that selects only a DaemonSet pod and one whose pod is not on a target node. |
| Visibility | Everything `PDB_DISRUPTION_BLOCKED` would have listed for the PDB stays in the evidence, so replacing that finding hides nothing. |

### `PDB_SYNC_FAILED`

Severity: `UNKNOWN`.

| Criterion | Answer |
| --- | --- |
| False-positive case | The condition can be set briefly while the controller retries, for example while the owner of the pods is being replaced, so a run in that window reports a budget that is about to be fine. That is why the answer is `UNKNOWN`, not a verdict: run draincheck again after a short wait. A condition that stays points at a pod owner without a scale subresource or a selector the controller cannot resolve. |
| Downgrade | This rule is the downgrade. A failed PDB records 0 allowed disruptions, which `PDB_DISRUPTION_BLOCKED` would report as `BLOCKED`; it is `UNKNOWN` here, and so is a failed PDB whose status claims to allow a disruption. Never `BLOCKED` or `RISK`: nothing is known about the real numbers. A budget that is stale as well is still reported here, not by `PDB_STALE_STATUS`: the controller does not set `status.observedGeneration` for a budget it fails on, so a failed budget is stale in practice (#145), and the advice to wait and run again would never help it. If the controller itself is down, the condition left from the last sync is reported, and the advice to run draincheck again still holds. |
| Mirror kubectl | It counts the same pods as `PDB_DISRUPTION_BLOCKED` (they share one selection): terminal, DaemonSet-managed and static pods are not counted, so a failed PDB that selects only those gives no finding. A pod whose DaemonSet has been deleted is still skipped, see `PDB_DISRUPTION_BLOCKED`. |
| Grouping | One finding per PDB, pointing at the PDB: 10 replicas behind one failed PDB give one finding. The description states the controller error and the pod and node counts once. |
| Estimates | Not an estimate. The finding says the numbers cannot be trusted. |
| Evidence | `findings.PDBSyncFailedEvidence`: everything in `findings.PDBEvidence` (the numbers as last reported, `nodes` and `pods`) plus `reason` and `message` of the condition. |
| Fixture | `testdata/analyzer/pdb-sync-failed.yaml`: a failed PDB that allows 0, one that allows 1, a settled PDB (reason `SufficientPods`), a PDB at 0 with no condition (stays `BLOCKED`), a failed PDB that selects only a DaemonSet pod, one whose pod is not on a target node, one that is stale as well (generation 2, observed 1) and one that never got an `observedGeneration` (generation 1), the case seen on a real cluster. |
| Visibility | Everything `PDB_DISRUPTION_BLOCKED` would have listed for the PDB stays in the evidence, so replacing that finding hides nothing. |

### `PDB_BATCH_EXCEEDS_BUDGET`

Severity: `RISK`.

| Criterion | Answer |
| --- | --- |
| False-positive case | The budget is read as reported and the pods are counted from the snapshot. A budget that allows 1 and covers 3 pods on the batch makes the drain wait for two replacements; the finding says so and does not say the drain fails. Pods that are not Ready and that the eviction API lets past the budget (`unhealthyPodEvictionPolicy`, see `PDB_DISRUPTION_BLOCKED`) do not use the budget up and are not counted. A budget whose policy value draincheck does not know is skipped. |
| Downgrade | Never `BLOCKED`: with at least one disruption allowed, `kubectl drain` retries and proceeds in steps. A budget at 0 is left to `PDB_DISRUPTION_BLOCKED`. A stale budget, a budget whose controller failed, a budget over operator-controlled pods and a budget that overlaps another on the target nodes are skipped (`PDB_STALE_STATUS`, `PDB_SYNC_FAILED`, operator-managed `PDB_DISRUPTION_BLOCKED`, `PDB_OVERLAPPING_SELECTORS`). |
| Mirror kubectl | It counts the same pods as `PDB_DISRUPTION_BLOCKED` (they share one selection): terminal, DaemonSet-managed and static pods are not counted. What kubectl cannot do, and this rule does, is sum the pods of several nodes against one budget. |
| Grouping | One finding per PDB, pointing at the PDB: 10 replicas behind one PDB give one finding. The description states the pod and node counts and the disruptions allowed once. |
| Estimates | Not an estimate: both numbers come from the snapshot. It does not predict how long the wait is. |
| Evidence | `findings.PDBBatchEvidence`: everything in `findings.PDBEvidence` (the numbers, `nodes` and `pods`) plus `pods_to_evict`. |
| Fixture | `testdata/analyzer/pdb-batch-exceeds-budget.yaml`: a budget that fits per node and not as a batch, one exceeded by 3 pods against 2 allowed, one that always fits, one at 0, and stale, sync-failed, operator-controlled and DaemonSet-only budgets that are not reported. |
| Visibility | The finding lists every pod the batch evicts for the budget, so nothing is hidden. |

### `NODE_NOT_SCHEDULABLE`

Severity: `RISK`.

| Criterion | Answer |
| --- | --- |
| False-positive case | A remaining node is cordoned on purpose (kept for a debugging session, or waiting for its own maintenance) and the workloads have room elsewhere. The finding does not say the drain fails; it says the room is smaller than the node count suggests, with the nodes named. It is raised only when the drain moves pods, so a cordoned spare node alone does not cry wolf. |
| Downgrade | Never `BLOCKED`: the drain completes whatever the remaining nodes look like. A node that reports no `Ready` condition is not flagged for readiness, since its absence says nothing. No pod to move means no finding. |
| Mirror kubectl | A target that is already cordoned is not reported: `kubectl drain` cordons the targets itself and prints `already cordoned` (checked with kubectl v1.36.1). With several nodes in one invocation it cordons all of them before it evicts anything from the first, so the pods evicted from one target can never land on another target; that is why only the nodes that are not targets count as room (checked with kubectl v1.36.1). Pods kubectl never evicts (DaemonSet, static), finished pods, pods already terminating (their replacement exists) and pods with no controller need no new home and are not counted (see *Pods a drain acts on*). |
| Grouping | One finding for the run, however many nodes are affected: ten cordoned nodes are one finding, with each node in the evidence. |
| Estimates | Not an estimate: it reads `spec.unschedulable` and the `Ready` condition as reported. It does not say whether the pods fit on the nodes that are left. |
| Evidence | `findings.NodeNotSchedulableEvidence`: `pods_to_move`, `remaining_nodes` and `nodes` (name, cordoned flag, Ready status). |
| Fixture | `testdata/analyzer/node-not-schedulable.yaml`: a cordoned node, a node with `Ready=False`, one with `Ready=Unknown`, a healthy node, a node with no `Ready` condition, a cordoned target, and targets whose pods need no new home. |
| Visibility | When no pod moves, nothing is reported and the node state is not in the output; `kubectl get nodes` shows it. Otherwise every affected node is in the evidence. |

### `NO_REMAINING_NODES`

Severity: `RISK`.

| Criterion | Answer |
| --- | --- |
| False-positive case | Draining everything on purpose: a cluster being retired, or a node pool replaced after new nodes are added by another tool. The finding says the pods that move stay `Pending` until a node takes them, which is true in both cases; it does not say the drain fails. It is raised only when the drain moves pods. |
| Downgrade | Never `BLOCKED`: `kubectl drain` only evicts, so it completes whatever is left. Nothing is reported when no pod needs a new home. A node that reports no `Ready` condition counts as room, since its absence says nothing. |
| Mirror kubectl | The pods counted are the ones `NODE_NOT_SCHEDULABLE` counts: pods kubectl never evicts (DaemonSet, static), finished pods, pods already terminating and pods with no controller need no new home. kubectl cannot see this: it takes one node at a time and does not look at what the other nodes can take. |
| Grouping | One finding for the run, however many nodes are drained. Its resource is the target nodes, sorted and joined by commas. |
| Estimates | Not an estimate: it counts nodes outside the target set that are not cordoned, are `Ready` and carry no `NoSchedule` or `NoExecute` taint that every moving pod fails to tolerate (`PreferNoSchedule` never removes room; matching is the scheduler's own). It fires only when no moving pod can land on any node left. Room for some pods and none for others is `WORKLOAD_UNTOLERATED_TAINTS`. Affinity, selectors and capacity are not looked at, so "some node is left" does not mean the pods fit. |
| Evidence | `findings.NoRemainingNodesEvidence`: `pods_to_move`, `target_nodes`, `total_nodes` and `remaining_schedulable` (always 0 when reported) and, when a taint is involved, `tainted_nodes` (each remaining node a taint rules out, with the `key` and `effect` of the taints). |
| Fixture | `testdata/analyzer/no-remaining-nodes-taints.yaml` (a worker and a tainted control plane); `testdata/analyzer/node-not-schedulable.yaml`, with every node as a target and with all but the cordoned and not Ready nodes as targets; `testdata/regression/noise.yaml` (all six nodes drained). |
| Visibility | When no pod moves, nothing is reported. When the nodes left are all cordoned or not Ready, `NODE_NOT_SCHEDULABLE` is reported as well and names them; this rule states the consequence. |

### `WORKLOAD_UNTOLERATED_TAINTS`

Severity: `RISK`.

| Criterion | Answer |
| --- | --- |
| False-positive case | A workload that is meant to wait for a node it is allowed on (a pool that is scaled up after the drain). The finding says its pods stay `Pending` until a node takes them, which is true; it does not say the drain fails. Affinity and selectors can rule out more nodes than taints do, and are not looked at, so room that taints leave may not fit the pods. |
| Downgrade | Never `BLOCKED`: `kubectl drain` only evicts, so it completes. Nothing is reported when no pod needs a new home. A taint that a pod of the workload tolerates is room; `PreferNoSchedule` never removes room. |
| Mirror kubectl | The pods counted are the ones `NO_REMAINING_NODES` counts (see "Pods a drain acts on"). Taints are judged as the scheduler does. kubectl cannot see this: it does not look at where the evicted pods can go. |
| Grouping | One finding per owning workload whose moving pods all fail to land. The resource is the workload. |
| Estimates | An estimate: it judges cordons, readiness and taints only, never affinity, selectors or capacity, so room that is left may still not fit the pods. |
| Evidence | `findings.WorkloadUntoleratedTaintsEvidence`: `owner_kind`, `owner_name`, `pods` (name and node), `remaining_nodes` and `tainted_nodes` (each node left with the taints the workload does not tolerate). |
| Fixture | `testdata/analyzer/workload-untolerated-taints.yaml` (a worker, a tainted control plane and a gpu node; `db` tolerates nothing). |
| Visibility | Silent when every moving pod is stranded (`NO_REMAINING_NODES` states the whole run) and when no node is left at all. |

### `SINGLE_REPLICA_WORKLOAD`

Severity: `RISK`.

| Criterion | Answer |
| --- | --- |
| False-positive case | A workload that is meant to run one replica and tolerates a restart (a batch-like worker, a dev service). The finding says the workload is unavailable until the new pod is Ready, which is true; it does not say the drain fails. A workload that an HPA scales above 1 under load is reported while it runs one replica, since the HPA is not read. |
| Downgrade | Never `BLOCKED`: the drain completes. A pod whose Deployment or StatefulSet is not in the snapshot is skipped, not guessed. |
| Mirror kubectl | Pods kubectl never evicts (DaemonSet, static), finished pods and pods already terminating give no finding. kubectl does not warn about downtime; this is information it lacks. |
| Grouping | One finding per workload, however many of its pods are on the targets (a rollout can briefly run two). |
| Estimates | Not an estimate: it reads `spec.replicas`, 1 when unset, not the live pod count. |
| Evidence | `findings.SingleReplicaEvidence`: `owner_kind`, `owner_name`, `replicas` and `pods` (`namespace`, `name`, `node`). |
| Fixture | `testdata/analyzer/single-replica-workload.yaml`: single replica, three replicas, unset replicas, a finished pod, a terminating pod, a DaemonSet pod, a static pod, a workload missing from the snapshot, `replicas: 0`, a same-named workload in another namespace, a custom kind also called `Deployment`, a bare ReplicaSet and a rollout with two pods. |
| Visibility | Nothing is dropped silently except pods of unknown workloads, which cannot be judged. Every reported pod is in `pods`. |

### `POD_OWNERSHIP`

Severity: `BLOCKED` or `RISK`, by the drain flags: a DaemonSet pod is `BLOCKED` without `--ignore-daemonsets` and not reported with it; a pod with no controller is `BLOCKED` without `--force` and `RISK` with it; a static pod is always `RISK`.

| Criterion | Answer |
| --- | --- |
| False-positive case | `BLOCKED` is the answer of kubectl's own filters for the flags given (modelled from `filters.go` of kubectl 1.36.0 and 1.37.1), so it is wrong only when the flags given to draincheck differ from the ones given to `kubectl drain`; that is why the flags are accepted. The static pod finding is information, not a fault, because kubectl skips those pods. One corner is not covered: a pod whose DaemonSet has been deleted is still reported as a DaemonSet pod, because draincheck does not list DaemonSets. kubectl treats that orphan differently: without `--force` it fails that node with `cannot delete daemonsets.apps "<name>" not found` and goes on to the next node, and with `--force` it evicts the pod (checked with kubectl v1.36.1). Handling it is *planned*, see *POD_OWNERSHIP must handle orphaned DaemonSet pods (DaemonSet no longer exists)*. |
| Downgrade | Implemented: the flag is the downgrade. `--ignore-daemonsets` removes the DaemonSet findings; `--force` turns the no-controller finding from `BLOCKED` to `RISK`, because kubectl then deletes the pod and nothing recreates it. `--force` does not cover DaemonSet pods, as in kubectl. Static pods are `RISK` whatever the flags are. Only the ownership filters are asked: a pod with no controller that also has an `emptyDir` volume is reported here for its missing controller, while kubectl names the `emptyDir` first; the volume has its own finding, `POD_LOCAL_STORAGE`. A controller of kind `DaemonSet` outside `apps/v1` is not a DaemonSet to kubectl and is not reported. |
| Mirror kubectl | Terminal pods are skipped. A pod that is already terminating is **not** skipped here, unlike in every other rule: `kubectl drain` applies its DaemonSet, static and unreplicated checks to it too (checked with kubectl v1.36.1 on a kind cluster: a pod with no controller that was Terminating and still Running was refused with `cannot delete Pods that declare no controller`; see *Pods a drain acts on*). DaemonSet and static pods are reported on purpose, because the operator needs the `--ignore-daemonsets` flag and must plan for the node outage; the remediation says kubectl skips them. Because that advice is the same for every pod of a kind, it is given once per finding, not once per pod. |
| Grouping | A DaemonSet is one finding, however many nodes it runs on (28 pods on 28 nodes is one finding). Static pods are one finding per node, listing the static pods that stop with it. A pod with no controller is one finding of its own, since each one is lost on drain. Collapsing static pods across nodes is left to the compact text report: the node is the unit of action, because `--nodes` picks nodes. |
| Estimates | Not an estimate. |
| Evidence | `findings.OwnershipEvidence`: `category`, `nodes` and `pods` (each with `namespace`, `name` and `node`), plus `owner_kind` and `owner_name` for a DaemonSet. |
| Fixture | `testdata/analyzer/pod-ownership.yaml` (run with and without the flags in `pod_ownership_test.go`): a DaemonSet on three nodes, two DaemonSets with the same name in different namespaces, static pods on two nodes (one node with pods from two namespaces), naked pods, and managed and finished pods that must not be reported. |
| Visibility | Every pod stays in `pods` in the JSON report, with its namespace and node. The text report shows one line per finding; `--verbose` prints the full description, which names the static pods. |

### `POD_LOCAL_STORAGE`

Severity: `BLOCKED` or `RISK`, by `--delete-emptydir-data`: a pod with an `emptyDir` volume is `BLOCKED` without the flag and `RISK` with it.

| Criterion | Answer |
| --- | --- |
| False-positive case | `BLOCKED` is the answer of kubectl's own local-storage filter for the flags given (modelled from `filters.go` of kubectl 1.36.0 and 1.37.1), so it is wrong only when the flags given to draincheck differ from the ones given to `kubectl drain`. The `RISK` is information, not a fault: the drain completes. Any `emptyDir` counts, also one with `medium: Memory` or a `sizeLimit`, as in kubectl (a `medium: Memory` pod is named in the refusal; checked). draincheck cannot know whether the data in the volume matters; the remediation says to decide that. |
| Downgrade | Implemented: the flag is the downgrade, from `BLOCKED` to `RISK`. kubectl decides in its filter order, and so does the rule: a DaemonSet pod and a static pod are decided before their volumes are looked at, so they are not reported here; a finished pod is removed without checks. A pod that is already terminating is `BLOCKED` without the flag (kubectl refuses it) and not reported with it (the drain is not what removes it). A Pending pod is `BLOCKED` without the flag and not reported with it (no data yet). A pod with no controller is judged as if `--force` were given, because its refusal is `POD_OWNERSHIP`'s; with `--delete-emptydir-data` alone it is `RISK` here, and `BLOCKED` there. |
| Mirror kubectl | Terminal pods, DaemonSet pods and static pods are not reported (kubectl's filters decide them before the volumes; a DaemonSet pod with an `emptyDir` is named under the DaemonSet message only; checked), a pod `--pod-selector` does not match is not touched. The refusal text `cannot delete Pods with local storage (use --delete-emptydir-data to override)` was observed with kubectl v1.36.1 on kind (#29). |
| Grouping | One finding per owning workload (the Deployment, not its ReplicaSet), however many pods and nodes; a pod with no controller is a finding of its own. |
| Estimates | Not an estimate. |
| Evidence | `findings.LocalStorageEvidence`: `owner_kind` and `owner_name` (empty for a pod with no controller), `delete_emptydir_data` (the flag as given), `nodes` and `pods` (each with `namespace`, `name`, `node` and `volumes`, the names of its `emptyDir` volumes, sorted). |
| Fixture | `testdata/analyzer/pod-local-storage.yaml`: two Deployments (one with `medium: Memory`), a StatefulSet with two volumes, a pod with no controller, and pods that are not reported (configMap, hostPath and claim volumes only; finished; DaemonSet; static; on a node that is not a target). `pod_local_storage_test.go` adds the terminating, Pending, no-phase, `--pod-selector`, foreign DaemonSet kind and ordering cases. |
| Visibility | Every pod and volume name is in the JSON report; `--verbose` prints the full description. |

### `MISSING_RESOURCE_REQUESTS`

Severity: `RISK`.

| Criterion | Answer |
| --- | --- |
| False-positive case | Requests are read from the stored pod spec, so values from a LimitRange or an admission webhook count as set. A pod that is not rescheduled is not affected; those are skipped (below). A pod with no controller is reported as its own workload, although nothing recreates it after a drain; the finding only matters if the operator recreates it elsewhere. |
| Downgrade | Estimate-class rule: it is only ever `RISK`. |
| Mirror kubectl | Implemented: terminal pods, pods already terminating (their replacement exists), DaemonSet pods and static (mirror) pods are skipped, because the rule's rationale (the pod may not fit where it is rescheduled) does not apply to pods that are never rescheduled. They stay visible through `POD_OWNERSHIP`. |
| Grouping | Findings are grouped per owning workload (Deployment, StatefulSet, Job and so on), found with `inventory.OwnerResolver`. A pod with no controller counts as its own workload. Ten replicas missing requests give one finding. The description states the count once (for example "2 of 3 pods of Deployment shop/web") and the per-pod detail goes into the evidence. |
| Estimates | Yes. The finding lists the containers and which request each lacks. Init containers are not inspected. |
| Evidence | `findings.MissingRequestsEvidence`: `owner_kind` and `owner_name` (omitted for a pod with no controller), `pods_on_targets` (the workload's pods on the target nodes that kubectl would evict, the denominator of the count) and `pods`. Each entry of `pods` has `name`, `node` and `containers` with `missing_cpu` and `missing_memory`. |
| Fixture | `testdata/analyzer/missing-resource-requests.yaml`: replicas of a Deployment on several nodes, a partly affected Deployment, a Job with a finished pod, DaemonSet and mirror pods. |
| Visibility | Every affected pod, its node and its containers are in `pods` in the JSON report. The text report shows the count; `--verbose` shows the full description. |

## See also

- [USAGE.md](USAGE.md#rules): what each rule reports.
- [SPEC.md](SPEC.md): the specification.
