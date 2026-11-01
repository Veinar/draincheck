# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Each entry should link to the pull request or issue that introduced it. Contributors who opt in are credited with their GitHub handle, e.g. `(#12, @username)`. Add new
entries under `[Unreleased]` in the same pull request as the change (see
[CONTRIBUTING.md](CONTRIBUTING.md)).

## [Unreleased]

### Added

- The report names the `kubectl drain` flags the plan still needs: a `KUBECTL DRAIN FLAGS` section in the text report (for example `kubectl drain will refuse without: --ignore-daemonsets --delete-emptydir-data`) and a new top-level `drain_flags` object in the JSON report (`missing`, `eviction_disabled`). Only flags whose absence makes a finding `BLOCKED` are named, never one you passed; with `--disable-eviction` the report says once that PodDisruptionBudgets are not consulted. `--debug` logs the result as `drain flags` (#30).

#### Analyzer rules

- `PDB_UNSATISFIABLE`: a PodDisruptionBudget that can never allow a disruption (`minAvailable` at or above the replica count, or `maxUnavailable: 0`, which is `desiredHealthy` not below `expectedPods`) is reported as `BLOCKED`, one finding per budget. Waiting cannot fix it; only a change to the budget or the replica count does, and the description says `maxUnavailable: 0` may be deliberate. It replaces `PDB_DISRUPTION_BLOCKED` for these budgets, so the rule ID such a budget reports changes; `PDB_DISRUPTION_BLOCKED` keeps the budgets at 0 that can clear by themselves. The JSON `evidence` has `namespace`, `name`, `expected_pods`, `desired_healthy`, `min_available`, `max_unavailable`, `nodes` and `pods` (#99).
- `ALL_REPLICAS_ON_TARGETS`: a Deployment or StatefulSet with 2 or more desired replicas whose Ready pods all run on the nodes you drain is reported as `RISK`, one finding per workload. Drained together, the nodes can take the whole workload down, because `kubectl drain` does not wait for replacements to be Ready before it drains the next node (whether it reaches zero depends on timing); `kubectl drain` does not warn about it. Only Ready pods count, so a replica still starting on another node does not clear the finding. A workload covered by a PodDisruptionBudget that needs a pod healthy is skipped (kubectl waits on the budget), unless you pass `--disable-eviction`. A workload with one replica stays with `SINGLE_REPLICA_WORKLOAD`, so the two rules never report the same workload. The JSON `evidence` has `owner_kind`, `owner_name`, `replicas`, `ready_pods`, `nodes` and `pods` (#98).

### Fixed

- A kubeconfig that logs in with the `oidc` auth provider no longer fails with `no Auth Provider found for name "oidc"`. `kubectl` accepts such a kubeconfig, so draincheck now does too. Kubeconfigs that use an exec plugin were not affected.
- `NO_REMAINING_NODES` advice no longer says "a taint named above" when the report names no taint (for example when every node is drained). The taint advice now reads "if the description names a taint on a node" (#170).

## [0.1.0] - 2026-10-28

First minor release. draincheck reads a snapshot of the cluster and reports what
a multi-node `kubectl drain` would trip over, before you drain: budgets that
refuse an eviction or that the batch exceeds, pods `kubectl drain` refuses
without a flag, workloads that go down or cannot land anywhere, and missing
requests. It is read-only, it never says a drain is safe, and it takes the same
drain flags as `kubectl drain`, so you can ask the question for the drain you
will run. Reports are text or JSON.

**What a server dry run (`kubectl drain --dry-run=server`) does not show, and
draincheck does**: several nodes against one PodDisruptionBudget, no node left to
take the evicted pods (cordoned, not `Ready` or tainted), a workload that cannot
land anywhere because of taints, and a one-replica workload that goes down while
its pod moves. [docs/COMPARISON.md](docs/COMPARISON.md) compares the two rule
by rule, including where `kubectl drain` is ahead.

### Upgrading from 0.0.1

- **Exit code on clusters with DaemonSets.** At the default `--fail-on blocked`
  a cluster with DaemonSets now exits `1` unless you pass
  `--ignore-daemonsets`, because `kubectl drain` refuses those pods without it.
  The same holds for a pod with no controller (`--force`) and a pod with an
  `emptyDir` volume (`--delete-emptydir-data`). Pass the flags you will give
  `kubectl drain` (#108, #29, #107).
- **JSON `evidence` changed** for `POD_OWNERSHIP`, `MISSING_RESOURCE_REQUESTS`
  and `PDB_DISRUPTION_BLOCKED`: findings are now one per cause instead of one
  per pod, and the per-pod detail moved into lists inside `evidence` (#70,
  #71, #73). Read the entries below before you parse a 0.0.1 report.
- **More read permissions.** draincheck now lists Deployments, StatefulSets,
  Jobs, CronJobs, PersistentVolumeClaims and PersistentVolumes as well. A run
  without `list` on them stops with exit `2`. `examples/rbac/` has a role with
  exactly what is needed (#38, #39).

### Stability

Meant to hold from here on: the exit codes (`0`, `1`, `2`), the flag names, the
severity names `BLOCKED`, `RISK` and `UNKNOWN`, and the top-level fields of the
JSON report (`schema_version`, `generated_at`, `disclaimer`, `findings`).

Can still change before 1.0, with a changelog entry when it does: rule IDs and
which rule reports a given situation, the `evidence` fields of each rule, the
wording of descriptions and remediation, and the layout of the text report.

### Known limits

- An extra positional argument is ignored, not rejected: `draincheck --nodes n1
  n2` checks `n1` only. Name every node in `--nodes`.
- There is no request timeout. A server that never answers keeps the run
  waiting until you stop it with `SIGINT` or `SIGTERM`, which end it with exit
  `2`.
- The snapshot is read with sequential list calls at one moment. The cluster
  can change right after, which is why every report carries its timestamp and a
  disclaimer.
- A PodDisruptionBudget managed by an operator is `UNKNOWN`: draincheck does not
  read operator state, so it cannot tell whether the operator removes the
  budget when a node is cordoned.
- Not checked yet: node affinity and selectors, pod anti-affinity, capacity,
  `hostPort`, `hostPath` and volume placement, and admission webhooks on
  eviction. A clean report does not cover them.
- The kubectl behaviour modelled is that of 1.36.0 and 1.37.1, checked on kind
  with 1.36.1. The rules were run on kind and on one real cluster, so the rate
  of wrong findings on other clusters is not known. Please report one if occured.

### Added

- The `kubectl drain` flags `--ignore-daemonsets`, `--delete-emptydir-data`,
  `--force`, `--pod-selector` and `--disable-eviction`, with kubectl's names,
  defaults and meaning. `--pod-selector` is in effect: a pod the selector does
  not match is not touched by the drain, so no rule reports it, and a selector
  that does not parse ends the run with exit `2`. The other four are accepted
  and shown in the `--debug` settings line; `--ignore-daemonsets` and `--force`
  now change `POD_OWNERSHIP` (below) and `--delete-emptydir-data` changes
  `POD_LOCAL_STORAGE` (below). The pod filters of kubectl 1.36.0 and 1.37.1 are
  modelled in the analyzer for those rules (#107).
- `--disable-eviction` is in effect: `kubectl drain` then deletes the pods
  instead of evicting them, so no PodDisruptionBudget is consulted and every
  `PDB_*` rule reports nothing. Rules that do not read budgets, such as
  `SINGLE_REPLICA_WORKLOAD` and `POD_OWNERSHIP`, are unchanged. With `--debug`,
  an `eviction disabled` line says how many budgets in the cluster were not
  consulted (#109).
- `--debug` flag. It writes one line to stderr for each action draincheck
  takes: the settings, every list call to the API server with its object count
  and duration, how the target nodes were resolved, each rule with its number
  of findings, the finding counts per severity and the exit code. The report on
  stdout is byte-for-byte the same with and without the flag, and the lines
  never contain tokens or kubeconfig contents (#21).

#### Analyzer rules

- `POD_LOCAL_STORAGE`: a pod on a target node with an `emptyDir` volume is
  `BLOCKED` without `--delete-emptydir-data`, because `kubectl drain` refuses it
  and stops before it evicts anything, and `RISK` with the flag, because the
  drain completes and the data in the volume is lost. One finding per owning
  workload (a pod with no controller is its own), with every pod and volume
  name in the JSON `evidence`: `owner_kind`, `owner_name`,
  `delete_emptydir_data`, `nodes` and `pods` (each with `namespace`, `name`,
  `node` and `volumes`). DaemonSet, static and finished pods are not reported,
  as `kubectl drain` decides them first (#29).
- `NO_REMAINING_NODES`: when the drain moves pods and no node is left that
  takes them, because every node is a target or the ones left are cordoned,
  not `Ready` or tainted `NoSchedule` or `NoExecute` where none of the moving
  pods tolerates the taint (a one-worker cluster with a tainted control plane),
  it is reported as `RISK`, one finding for the run. The drain
  completes, but the evicted pods stay `Pending` until a node can take them.
  Nothing is reported when no pod needs a new home, or when one moving pod can
  land on a node left. The JSON `evidence` has
  `pods_to_move`, `target_nodes`, `total_nodes` and `remaining_schedulable`,
  and `tainted_nodes` (each node a taint rules out, with the taint `key` and
  `effect`) when a taint is involved (#43, #132).
- `WORKLOAD_UNTOLERATED_TAINTS`: a workload whose evicted pods cannot land on any node
  left because of `NoSchedule` or `NoExecute` taints that they do not
  tolerate, while pods of other workloads can (for example a Deployment
  without a toleration on a cluster whose only other node is a tainted control
  plane, next to `coredns`, which tolerates it), is reported as `RISK`, one
  finding per workload. The drain completes, but the pods stay `Pending`. The
  JSON `evidence` has `owner_kind`, `owner_name`, `pods`, `remaining_nodes`
  and `tainted_nodes` (#9).
- `NODE_NOT_SCHEDULABLE`: when the drain moves pods, nodes outside the target
  set that are cordoned or not `Ready` are reported as `RISK`, one finding for
  all of them. The drain completes, but the evicted pods have fewer nodes to
  land on than the node count suggests. A target that is already cordoned is
  not reported, and nothing is reported when no pod needs a new home. The JSON
  `evidence` has `pods_to_move`, `remaining_nodes` and the state of each
  affected node (#42).
- `PDB_BATCH_EXCEEDS_BUDGET`: a PodDisruptionBudget that allows some
  disruptions, but fewer than the target nodes together would cause, is
  reported as `RISK`, one finding per budget. `kubectl drain` takes one node at
  a time and sees each fit; draincheck sums the pods of all target nodes that
  share the budget. The drain does not fail, it waits for replacements between
  evictions. A budget at 0 stays `BLOCKED` under `PDB_DISRUPTION_BLOCKED`. The
  JSON `evidence` has the budget fields of `PDB_DISRUPTION_BLOCKED` plus
  `pods_to_evict` (#12).
- `PDB_SYNC_FAILED`: a PodDisruptionBudget whose `DisruptionAllowed` condition
  has the reason `SyncFailed` while it selects pods on the target nodes is
  reported as `UNKNOWN`, one finding per budget. The disruption controller
  could not compute the budget, so the 0 allowed disruptions it records is not
  a real number. The JSON `evidence` has the budget fields of
  `PDB_DISRUPTION_BLOCKED` plus the condition `reason` and `message` (#100).
- `PDB_STALE_STATUS`: a PodDisruptionBudget whose `status.observedGeneration`
  differs from `metadata.generation` while it selects pods on the target nodes
  is reported as `UNKNOWN`, one finding per budget. Such a status describes an
  older spec, so its `disruptionsAllowed` cannot be trusted. The JSON
  `evidence` has the budget fields of `PDB_DISRUPTION_BLOCKED` (the numbers as
  last reported, `nodes` and `pods`) plus `observed_generation` and
  `generation` (#41).
- `SINGLE_REPLICA_WORKLOAD`: a Deployment or StatefulSet that desires one
  replica and has a pod on a target node is reported as `RISK`, one finding per
  workload. The pod is evicted before its replacement is Ready, so the workload
  is down in between, which `kubectl drain` does not warn about. Finished,
  DaemonSet and static pods are skipped. The JSON `evidence` has `owner_kind`,
  `owner_name`, `replicas` and `pods` (#40).

### Changed

- **Exit code on clusters with DaemonSets.** A DaemonSet pod or a pod with no
  controller on a target node now ends the run with exit `1` at the default
  `--fail-on blocked`, as `kubectl drain` refuses such pods. Pass
  `--ignore-daemonsets` and `--force` as you will to `kubectl drain` to get
  the answer for that drain (#108).
- draincheck now also lists Deployments, StatefulSets, Jobs and CronJobs (all
  namespaces), so planned workload rules can see them. No rule uses them yet and
  the report does not change. The account that runs draincheck needs `list` on
  `deployments` and `statefulsets` (`apps`) and `jobs` and `cronjobs`
  (`batch`); a run without that permission stops with an error, it does not
  continue with an empty list (#38).
- draincheck now also lists PersistentVolumeClaims (all namespaces) and
  PersistentVolumes, so the planned volume rules can see them. No rule uses
  them yet and the report does not change. The account that runs draincheck
  needs `list` on `persistentvolumeclaims` and on `persistentvolumes`, which is
  cluster-scoped; a run without that permission stops with an error, it does
  not continue with an empty list (#39).
- `examples/rbac/` holds a read-only `ClusterRole` (`list` on every resource
  draincheck reads, no other verb) and a `ClusterRoleBinding` to adapt, so the
  account that runs draincheck can be set up without writing the rules by hand.

#### Analyzer rules

- `POD_OWNERSHIP` now follows what `kubectl drain` does with the pod under
  the flags you pass. A DaemonSet pod is `BLOCKED` without
  `--ignore-daemonsets` and not reported with it. A pod with no controller is
  `BLOCKED` without `--force` and a `RISK` with it, because kubectl then deletes
  it and nothing recreates it. A static pod stays a `RISK`. Before, every
  finding was a `RISK`. The finding's evidence and grouping do not change, only
  the severity and the wording for a pod with no controller under `--force`
  (#108).
- `unhealthyPodEvictionPolicy` is honoured by the PodDisruptionBudget rules.
  A pod that is not Ready is evicted past its budget with `AlwaysAllow`, and
  with `IfHealthyBudget` (the default) while the budget is healthy. Such a pod
  is no longer counted as blocked: `PDB_DISRUPTION_BLOCKED` gives no finding
  for a budget at 0 whose only pods it lets through, and
  `PDB_BATCH_EXCEEDS_BUDGET` does not count them. With an unhealthy budget the
  pod is still refused and the finding says so. A budget with a policy value
  draincheck does not know is `UNKNOWN`. The JSON `evidence` gains
  `unhealthy_pod_eviction_policy` and `evictable_unhealthy_pods`, both omitted
  when empty (#5).
- `MISSING_RESOURCE_REQUESTS` now reports one finding per owning workload
  (Deployment, StatefulSet, Job or a pod with no controller) instead of one
  per pod, so 10 replicas without requests are one finding. The finding points
  at the workload and states how many of its pods on the target nodes are
  affected. The JSON `evidence` changes: `node` and `containers` move into a
  `pods` list (each pod with its `node` and `containers`), next to
  `owner_kind`, `owner_name` and `pods_on_targets`. No pod is dropped (#73).
- `POD_OWNERSHIP` now groups by cause instead of reporting every pod: one
  finding per DaemonSet, one per node for its static pods, and still one per
  pod with no controller. A DaemonSet on 28 nodes is one finding that says
  `--ignore-daemonsets` is needed, not 28 copies of the same advice. Findings
  point at the `DaemonSet` or the `Node` instead of a pod. The JSON `evidence`
  changes: `node` is replaced by `nodes` and a `pods` list (each pod with its
  `namespace`, `name` and `node`), next to `category` and, for DaemonSets,
  `owner_kind` and `owner_name`. No pod is dropped (#70).
- `PDB_DISRUPTION_BLOCKED` now reports one finding per PodDisruptionBudget
  instead of one per pod, so 10 replicas behind one budget are one finding.
  The finding points at the `PodDisruptionBudget` instead of a pod, and its
  description gives the numbers (healthy, desired) once, with how many pods on
  how many target nodes are affected. The JSON `evidence` gains `nodes` and a
  `pods` list (each pod with its `namespace`, `name` and `node`), next to the
  existing budget fields. No pod is dropped (#71).
- `PDB_DISRUPTION_BLOCKED` no longer reports a PodDisruptionBudget whose
  disruption controller failed to compute it (`PDB_SYNC_FAILED` reports it as
  `UNKNOWN`) (#100).
- `PDB_DISRUPTION_BLOCKED` no longer reports a PodDisruptionBudget whose status
  is stale as `BLOCKED`; `PDB_STALE_STATUS` reports it as `UNKNOWN` instead, so
  a drain is not failed on a number that may be out of date (#41).
- `PDB_DISRUPTION_BLOCKED` reports a PodDisruptionBudget that selects pods
  controlled by a custom resource (an operator) as `UNKNOWN` instead of
  `BLOCKED`, because whether such a budget stops the drain depends on operator
  state draincheck cannot see. The budgets of one operator in one namespace
  are one finding that points at the `Namespace`, so an operator with a budget
  per node gives one finding, not one per node. The JSON `evidence` of that
  finding is `managed_by` (`api_group`, `kind`), `namespace` and a `pdbs` list
  with the usual fields of each budget. Budgets over Deployments, StatefulSets,
  Jobs and the like stay `BLOCKED`. Findings of this kind make the run exit `1`
  only with `--fail-on any` (#72).
- The `remediation` of every finding now names a read-only `kubectl`
  command that confirms it (`kubectl get pdb -n <namespace> <name>` and the
  like, with placeholders for the names in the finding), followed by the action,
  safest first. The `description` of `PDB_DISRUPTION_BLOCKED`,
  `PDB_OVERLAPPING_SELECTORS`, `NO_REMAINING_NODES` and
  `SINGLE_REPLICA_WORKLOAD` also says what
  `kubectl drain` does with the situation. The JSON fields and the severities
  do not change (#128).

### Fixed

- `--debug` now writes a line when the kubeconfig is loaded (the server host
  and port only) and when the client is built, and a `failed` line with the
  error when either step fails, so the debug log of a run with a bad
  kubeconfig or context shows the cause before `finished`. Tokens, certificates
  and kubeconfig contents are never written (#105).

- A run stopped with `SIGTERM` (a CI runner cancelling a job, a container
  stop, `timeout(1)`) now ends like one stopped with `SIGINT`: exit `2` with an
  error message, and the `finished` line with `--debug`. It used to end with
  `143` and no message (#104).

- A closed stdout or stderr pipe no longer ends the run with `SIGPIPE`, which
  a shell reports as `141`, a code outside the exit-code contract. Debug lines
  written to a closed stderr are dropped and the run keeps its exit code; a
  report that cannot be written to a closed stdout ends with `2`. The
  *Closed pipes* limit is gone from `docs/USAGE.md` and `docs/SPEC.md` (#106).

#### Analyzer rules

- `PDB_DISRUPTION_BLOCKED`, `PDB_BATCH_EXCEEDS_BUDGET` and
  `PDB_OVERLAPPING_SELECTORS` no longer count pods in phase `Pending` or pods
  that are already terminating. The eviction API deletes them without
  consulting a budget, so reporting them was a false `BLOCKED` (#5).
- `PDB_DISRUPTION_BLOCKED` and `PDB_OVERLAPPING_SELECTORS` no longer report
  DaemonSet pods and static pods. `kubectl drain` never evicts them, so no
  PodDisruptionBudget can refuse their eviction; a budget that selects only
  such pods was a false `BLOCKED`. They are still reported by `POD_OWNERSHIP`
  (#71).
- `PDB_DISRUPTION_BLOCKED` and `PDB_BATCH_EXCEEDS_BUDGET` no longer report a
  budget that selects a pod on a target node together with another budget.
  When several budgets select a pod, the disruption controller chooses one of
  them arbitrarily each time the pod changes, and a budget it did not choose
  was seen to keep an older number (`0` healthy pods while every pod was
  Ready), so the false `BLOCKED` sat next to the overlap that stops the drain
  anyway. `PDB_OVERLAPPING_SELECTORS` reports the pod, and its `evidence` now
  has `budgets`: each budget's `name`, `current_healthy`, `desired_healthy`
  and `disruptions_allowed` as reported, and the description says they can be
  out of date. `pdbs` is unchanged (#139).
- `PDB_SYNC_FAILED` now reports a budget the disruption controller failed on
  even when it looks stale. The controller never sets `status.observedGeneration`
  for such a budget, so it was always reported as `PDB_STALE_STATUS` ("wait and
  run again"), which never helps, and the cause was lost. It is now one
  `PDB_SYNC_FAILED` finding with the controller's error, for example a pod
  owner without a scale subresource, and `PDB_STALE_STATUS` leaves it alone.
  Severity (`UNKNOWN`) and evidence are unchanged (#145).
- `NODE_NOT_SCHEDULABLE`, `NO_REMAINING_NODES` and `MISSING_RESOURCE_REQUESTS`
  no longer count pods that are already terminating. Their controller has
  already created the replacement, so the drain does not take them down; they
  inflated the number of pods to move and could add a workload to the
  missing-requests report. All rules now share one definition of which pods a
  drain acts on. `POD_OWNERSHIP` still reports a terminating DaemonSet, static
  or controller-less pod, because `kubectl drain` checks those too (#126).

## [0.0.1] - 2026-10-16

First pre-alpha release. It checks PodDisruptionBudgets, pod ownership and
missing resource requests before a multi-node drain, and reports in text or
JSON. Output and rules may still change before 0.1.0.

### Added

- CLI built on Cobra with `--version`, `--nodes`, `--kubeconfig`, `--context`,
  `--output` and `--fail-on` flags. Node names passed to `--nodes` are trimmed
  and empty names are rejected.
- Inventory collector (client-go) producing an immutable snapshot of nodes,
  pods, PodDisruptionBudgets and ReplicaSets, with pod owner resolution and
  validation of target nodes.
- Findings model: severity, resource reference and typed finding, with
  deterministic sorting by namespace, kind and name.
- Analyzer rule interface and runner with deterministic ordering (#2).
- JSON report formatter with `schema_version` `v1` and a golden-file test (#18).
- Human-readable text report formatter, used as the default output (#17).
- Pipeline wiring collector, analyzer and report, with exit codes: `1` when a
  finding reaches the `--fail-on` threshold, `2` when the run cannot complete
  (#20).
- Snapshot timestamp and mandatory disclaimer in every report: the text report
  gains a `Timestamp:` header and a closing disclaimer line, and the JSON
  report gains `generated_at` and `disclaimer` fields. The timestamp is when
  the cluster snapshot was taken, so the same snapshot gives identical output
  (#19).
- Compact text report: findings are grouped by rule and each remediation is
  printed once, long lists are collapsed. `--verbose` prints every finding in
  full (#74).
- Tag-triggered release workflow: signed multi-platform artifacts with SLSA
  provenance (#23).
- Project documentation: specification (`docs/SPEC.md`), `README.md`,
  `AGENTS.md`, `CONTRIBUTING.md`, pull request template, `SECURITY.md` (#24),
  `CODE_OF_CONDUCT.md` and CI documentation (`docs/CI.md`).
- CI: build, quality (vet, gofmt, mod tidy, race tests, golangci-lint) and PR
  validation workflows, with workflow linting via actionlint.
- Security automation: CodeQL, govulncheck, dependency review, OpenSSF
  Scorecard and Dependabot for Go modules and GitHub Actions.
- Apache-2.0 license, `Makefile` (build, test, vet, fmt) and golangci-lint
  configuration.

#### Analyzer rules

- `PDB_DISRUPTION_BLOCKED`: PDB with `disruptionsAllowed == 0` (#3).
- `PDB_OVERLAPPING_SELECTORS`: pod matched by more than one PDB (#4).
- `POD_OWNERSHIP`: DaemonSet, static and naked pods (#6).
- `MISSING_RESOURCE_REQUESTS`: pods without CPU or memory requests (#7).

### Changed

- Bump Go to 1.26 and refresh indirect dependencies.
- Bump `k8s.io/apimachinery` and `k8s.io/client-go` from 0.35.9 to 0.37.1.

[Unreleased]: https://github.com/Veinar/draincheck/compare/v0.1.0...develop
[0.1.0]: https://github.com/Veinar/draincheck/compare/v0.0.1...v0.1.0
[0.0.1]: https://github.com/Veinar/draincheck/releases/tag/v0.0.1
