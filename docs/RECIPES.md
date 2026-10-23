# JSON report recipes

These examples use `jq` to query draincheck's JSON report. Run them from the repository root against `testdata/report/json-v1.golden.json` to reproduce the outputs shown below. The examples use the documented v1 fields and do not depend on the order of JSON object fields.

## List resources with `BLOCKED` findings

```sh
jq -r '.findings[] | select(.severity == "BLOCKED") | .resource' testdata/report/json-v1.golden.json | sort
```

Output:

```text
Pod/billing/payments-api-1
PodDisruptionBudget/billing/payments-api-pdb
```

## Count findings by rule ID

```sh
jq -r '[.findings[].rule_id] | sort | group_by(.)[] | "(.[0])\t(length)"' testdata/report/json-v1.golden.json
```

Output:

```text
MISSING_RESOURCE_REQUESTS	1
PDB_DISRUPTION_BLOCKED	2
PDB_OVERLAPPING_SELECTORS	1
POD_OWNERSHIP	1
UNKNOWN_CONSTRAINT	1
```

## List nodes touched by `PDB_DISRUPTION_BLOCKED`

The evidence can contain a top-level `nodes` array or, for operator-managed budgets, node arrays nested under `pdbs`.

```sh
jq -r '
  .findings[]
  | select(.rule_id == "PDB_DISRUPTION_BLOCKED")
  | .evidence
  | if . == null then empty
    else (.nodes[]?, .pdbs[]?.nodes[]?)
    end
' testdata/report/json-v1.golden.json | sort -u
```

Output:

```text
worker-02
worker-03
```

## List pods in a DaemonSet `POD_OWNERSHIP` finding

```sh
jq -r '
  .findings[]
  | select(.rule_id == "POD_OWNERSHIP" and .evidence.category == "DaemonSet")
  | .evidence.pods[].name
' testdata/report/json-v1.golden.json | sort
```

Output:

```text
agent-x1
agent-x2
```

## List all `UNKNOWN` findings

```sh
jq -r '.findings[] | select(.severity == "UNKNOWN") | [.rule_id, .resource] | @tsv' testdata/report/json-v1.golden.json | sort
```

Output:

```text
PDB_DISRUPTION_BLOCKED	Namespace/storage-system
UNKNOWN_CONSTRAINT	Node/worker-02
```

## Count findings by severity

```sh
jq -r '[.findings[].severity] | sort | group_by(.)[] | "(.[0])\t(length)"' testdata/report/json-v1.golden.json
```

Output:

```text
BLOCKED	2
RISK	2
UNKNOWN	2
```

## Fail a shell step when any `UNKNOWN` finding exists

```sh
jq -e '[.findings[] | select(.severity == "UNKNOWN")] | length == 0' testdata/report/json-v1.golden.json
```

Output:

```text
false
```

This command exits with status `1` for the supplied fixture because it contains `UNKNOWN` findings. When none exist, it outputs `true` and exits with status `0`.

**Pipeline gating:** `jq` inspects the report, but draincheck's exit code is what gates the pipeline. Use `--fail-on` to choose which findings cause a non-zero result; see [Exit codes](USAGE.md#exit-codes).
