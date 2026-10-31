package analyzer

import (
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
	"unicode"

	"github.com/Veinar/draincheck/internal/findings"
)

// The advice contract, written down in docs/RULES.md ("Advice standard"): every
// finding of every default rule names the resource, states a fact with a number
// or a name, carries typed evidence, and gives the operator a read-only command
// that confirms it. This test keeps new and changed rules to it.

// adviceCase is one run of a rule against a fixture. Together the cases must
// cover every rule in DefaultRules(), so a new rule cannot skip the contract.
type adviceCase struct {
	rule    string
	fixture string
	targets []string
	// drain holds the kubectl drain flags of the run; the zero value is no flags.
	drain DrainOptions
}

var workers123 = []string{"worker-01", "worker-02", "worker-03"}

var adviceCases = []adviceCase{
	{"PDB_DISRUPTION_BLOCKED", "pdb-disruption-blocked.yaml", workers123, DrainOptions{}},
	{"PDB_UNSATISFIABLE", "pdb-unsatisfiable.yaml", []string{"worker-01", "worker-02"}, DrainOptions{}},
	// A budget policy draincheck does not know gives the UNKNOWN finding.
	{"PDB_DISRUPTION_BLOCKED", "pdb-unhealthy-pods.yaml", workers123, DrainOptions{}},
	// Operator-managed budgets give the UNKNOWN finding per operator.
	{"PDB_DISRUPTION_BLOCKED", "pdb-operator-managed.yaml", []string{"worker-01", "worker-02", "worker-03", "worker-04", "worker-05"}, DrainOptions{}},
	{"PDB_OVERLAPPING_SELECTORS", "pdb-overlapping-selectors.yaml", []string{"worker-01", "worker-02"}, DrainOptions{}},
	{"PDB_BATCH_EXCEEDS_BUDGET", "pdb-batch-exceeds-budget.yaml", []string{"worker-01", "worker-02"}, DrainOptions{}},
	{"PDB_STALE_STATUS", "pdb-stale-status.yaml", []string{"worker-01", "worker-02"}, DrainOptions{}},
	{"PDB_SYNC_FAILED", "pdb-sync-failed.yaml", []string{"worker-01", "worker-02"}, DrainOptions{}},
	{"NODE_NOT_SCHEDULABLE", "node-not-schedulable.yaml", []string{"worker-01"}, DrainOptions{}},
	{"NO_REMAINING_NODES", "node-not-schedulable.yaml", allNodes, DrainOptions{}},
	{"WORKLOAD_UNTOLERATED_TAINTS", "workload-untolerated-taints.yaml", []string{"worker", "soft"}, DrainOptions{}},
	{"SINGLE_REPLICA_WORKLOAD", "single-replica-workload.yaml", []string{"worker-01", "worker-02"}, DrainOptions{}},
	{"ALL_REPLICAS_ON_TARGETS", "all-replicas-on-targets.yaml", []string{"worker-01", "worker-02"}, DrainOptions{}},
	// DaemonSet, static and controller-less pods each have their own advice, and a
	// controller-less pod has another once --force is set.
	{"POD_OWNERSHIP", "pod-ownership.yaml", workers123, DrainOptions{}},
	{"POD_OWNERSHIP", "pod-ownership.yaml", workers123, DrainOptions{Force: true}},
	// emptyDir pods are BLOCKED without --delete-emptydir-data and RISK with it,
	// each with its own advice; the workload and the single pod have one text.
	{"POD_LOCAL_STORAGE", "pod-local-storage.yaml", []string{"worker-01", "worker-02"}, DrainOptions{}},
	{"POD_LOCAL_STORAGE", "pod-local-storage.yaml", []string{"worker-01", "worker-02"}, DrainOptions{DeleteEmptyDirData: true}},
	{"MISSING_RESOURCE_REQUESTS", "missing-resource-requests.yaml", []string{"worker-01", "worker-02"}, DrainOptions{}},
}

// verifyPrefixes are the only commands a remediation may suggest: they read.
var verifyPrefixes = []string{"kubectl get ", "kubectl describe ", "kubectl auth can-i "}

// writeWords must not appear anywhere in a suggested command.
var writeWords = []string{"delete", "patch", "apply", "create", "cordon", "drain", "evict", "scale", "edit"}

// placeholders are the names a command may leave for the operator to fill in
// from the finding (docs/RULES.md, "Advice standard").
var placeholders = []string{"namespace", "name", "pod", "node", "kind", "pdb", "owner"}

// shellMeta joins or redirects commands. A suggested command is one plain
// kubectl invocation, so none of these may appear in it.
var shellMeta = []string{"|", ";", "&", "$(", "\n", ">>"}

// remediationCommands returns the commands in a remediation: the text between
// backticks that mentions kubectl anywhere, in any case, so `sudo kubectl
// delete ...` is found and judged, not skipped. ok is false when the backticks
// do not pair up, so a command cannot hide in an unclosed span.
func remediationCommands(remediation string) (cmds []string, ok bool) {
	parts := strings.Split(remediation, "`")
	if len(parts)%2 == 0 {
		return nil, false
	}
	for i := 1; i < len(parts); i += 2 {
		if strings.Contains(strings.ToLower(parts[i]), "kubectl") {
			cmds = append(cmds, strings.TrimSpace(parts[i]))
		}
	}
	return cmds, true
}

// adviceViolations lists how f breaks the advice contract; empty when it holds.
func adviceViolations(f findings.Finding) []string {
	var v []string
	if f.Resource.Kind == "" || f.Resource.Name == "" {
		v = append(v, "resource is not set")
	}
	if !f.Severity.Valid() {
		v = append(v, fmt.Sprintf("severity %q is not valid", f.Severity))
	}
	if f.Evidence == nil || reflect.ValueOf(f.Evidence).IsZero() {
		v = append(v, "evidence is empty")
	}
	if strings.TrimSpace(f.Description) == "" {
		v = append(v, "description is empty")
	} else if !strings.ContainsFunc(f.Description, unicode.IsDigit) && !strings.Contains(f.Description, f.Resource.Name) {
		v = append(v, "description states neither a number nor the name of the resource")
	}
	if strings.TrimSpace(f.Remediation) == "" {
		return append(v, "remediation is empty")
	}

	cmds, ok := remediationCommands(f.Remediation)
	if !ok {
		return append(v, "remediation has an unclosed backtick")
	}
	if len(cmds) == 0 {
		v = append(v, "remediation names no verify command")
	}
	for _, c := range cmds {
		if !slices.ContainsFunc(verifyPrefixes, func(p string) bool { return strings.HasPrefix(c, p) }) {
			v = append(v, fmt.Sprintf("command %q does not start with kubectl get, describe or auth can-i", c))
		}
		for _, m := range shellMeta {
			if strings.Contains(c, m) {
				v = append(v, fmt.Sprintf("command %q contains %q: one plain kubectl invocation only", c, m))
			}
		}
		for _, ph := range regexp.MustCompile(`<[^<>]*>`).FindAllString(c, -1) {
			if !slices.Contains(placeholders, strings.Trim(ph, "<>")) {
				v = append(v, fmt.Sprintf("command %q has the unknown placeholder %s", c, ph))
			}
		}
		for _, w := range writeWords {
			if strings.Contains(strings.ToLower(c), w) {
				v = append(v, fmt.Sprintf("command %q contains %q", c, w))
			}
		}
	}
	return v
}

// adviceVariants are the kinds of finding the cases must produce, each with
// its own remediation: a rule's severities, the operator-managed form of
// PDB_DISRUPTION_BLOCKED and the three categories of POD_OWNERSHIP. A fixture
// edit that stops producing one fails the test instead of silently shrinking
// what it checks.
var adviceVariants = []string{
	"PDB_DISRUPTION_BLOCKED/BLOCKED/findings.PDBEvidence",
	"PDB_DISRUPTION_BLOCKED/UNKNOWN/findings.PDBEvidence",
	"PDB_DISRUPTION_BLOCKED/UNKNOWN/findings.OperatorManagedPDBsEvidence",
	"PDB_UNSATISFIABLE/BLOCKED/findings.PDBUnsatisfiableEvidence",
	"PDB_OVERLAPPING_SELECTORS/BLOCKED/findings.OverlappingPDBsEvidence",
	"PDB_BATCH_EXCEEDS_BUDGET/RISK/findings.PDBBatchEvidence",
	"PDB_STALE_STATUS/UNKNOWN/findings.PDBStaleStatusEvidence",
	"PDB_SYNC_FAILED/UNKNOWN/findings.PDBSyncFailedEvidence",
	"NODE_NOT_SCHEDULABLE/RISK/findings.NodeNotSchedulableEvidence",
	"NO_REMAINING_NODES/RISK/findings.NoRemainingNodesEvidence",
	"WORKLOAD_UNTOLERATED_TAINTS/RISK/findings.WorkloadUntoleratedTaintsEvidence",
	"SINGLE_REPLICA_WORKLOAD/RISK/findings.SingleReplicaEvidence",
	"ALL_REPLICAS_ON_TARGETS/RISK/findings.AllReplicasEvidence",
	"POD_OWNERSHIP/BLOCKED/DaemonSet",
	"POD_OWNERSHIP/RISK/StaticPod",
	"POD_OWNERSHIP/BLOCKED/Unmanaged",
	"POD_OWNERSHIP/RISK/Unmanaged",
	"POD_LOCAL_STORAGE/BLOCKED/findings.LocalStorageEvidence",
	"POD_LOCAL_STORAGE/RISK/findings.LocalStorageEvidence",
	"MISSING_RESOURCE_REQUESTS/RISK/findings.MissingRequestsEvidence",
}

// variantOf names the kind of finding f is, as listed in adviceVariants.
func variantOf(f findings.Finding) string {
	kind := fmt.Sprintf("%T", f.Evidence)
	if ev, ok := f.Evidence.(findings.OwnershipEvidence); ok {
		kind = string(ev.Category)
	}
	return fmt.Sprintf("%s/%s/%s", f.RuleID, f.Severity, kind)
}

func TestAdviceContract(t *testing.T) {
	rules := map[string]Rule{}
	for _, r := range DefaultRules() {
		rules[r.ID()] = r
	}

	covered := map[string]bool{}
	produced := map[string]bool{}
	for _, c := range adviceCases {
		covered[c.rule] = true
		rule, ok := rules[c.rule]
		if !ok {
			t.Errorf("case %s/%s: rule is not in DefaultRules()", c.rule, c.fixture)
			continue
		}
		t.Run(c.rule+"/"+c.fixture, func(t *testing.T) {
			got := Run(nil, Input{Snapshot: snapshotFromFixture(t, c.fixture), Targets: c.targets, Drain: c.drain}, rule)
			if len(got) == 0 {
				t.Fatal("the fixture gives no finding, so the contract is not tested")
			}
			for _, f := range got {
				produced[variantOf(f)] = true
				if f.RuleID != c.rule {
					t.Errorf("%s: reported under rule %q", f.Resource, f.RuleID)
				}
				for _, msg := range adviceViolations(f) {
					t.Errorf("%s %s: %s", f.Severity, f.Resource, msg)
				}
			}
		})
	}

	for _, v := range adviceVariants {
		if !produced[v] {
			t.Errorf("no case produces %s: a fixture or a rule changed, restore the case", v)
		}
	}
	for id := range rules {
		if !covered[id] {
			t.Errorf("rule %s has no case in adviceCases: add its fixture to advice_contract_test.go", id)
		}
	}
}

// Both halves of the contract must fail when broken, or the test above proves
// nothing.
func TestAdviceViolations(t *testing.T) {
	good := findings.Finding{
		RuleID:      "R",
		Severity:    findings.Blocked,
		Resource:    findings.ResourceRef{Kind: "PodDisruptionBudget", Namespace: "shop", Name: "web-pdb"},
		Evidence:    findings.PDBEvidence{Namespace: "shop", Name: "web-pdb"},
		Description: "PodDisruptionBudget shop/web-pdb allows 0 disruptions.",
		Remediation: "Confirm with `kubectl get pdb -n <namespace> <name>`. Then add replicas.",
	}
	if v := adviceViolations(good); len(v) != 0 {
		t.Fatalf("a finding that meets the contract has violations: %v", v)
	}

	with := func(edit func(*findings.Finding)) findings.Finding {
		f := good
		edit(&f)
		return f
	}
	tests := []struct {
		name string
		f    findings.Finding
		want string
	}{
		{"no verify command", with(func(f *findings.Finding) { f.Remediation = "Add replicas." }), "names no verify command"},
		{"empty remediation", with(func(f *findings.Finding) { f.Remediation = "" }), "remediation is empty"},
		{"command outside the read-only set", with(func(f *findings.Finding) { f.Remediation = "Run `kubectl top pods`." }), "does not start with"},
		{"unclosed backtick", with(func(f *findings.Finding) { f.Remediation = "Run `kubectl get pdb" }), "unclosed backtick"},
		{"delete", with(func(f *findings.Finding) { f.Remediation = "`kubectl get pdb x` then `kubectl delete pdb x`" }), `contains "delete"`},
		{"get that writes by another word", with(func(f *findings.Finding) {
			f.Remediation = "`kubectl get pods --field-selector=x -o name | xargs kubectl drain`"
		}), `contains "drain"`},
		{"edit in a get", with(func(f *findings.Finding) { f.Remediation = "`kubectl get pdb x --edit`" }), `contains "edit"`},
		{"command behind another program", with(func(f *findings.Finding) { f.Remediation = "`sudo kubectl get pdb x`" }), "does not start with"},
		{"upper case kubectl", with(func(f *findings.Finding) { f.Remediation = "`KUBECTL delete pdb x`" }), "does not start with"},
		{"pipe", with(func(f *findings.Finding) { f.Remediation = "`kubectl get pods -o name | sh`" }), `contains "|"`},
		{"second command after a semicolon", with(func(f *findings.Finding) { f.Remediation = "`kubectl get pods; rm x`" }), `contains ";"`},
		{"command substitution", with(func(f *findings.Finding) { f.Remediation = "`kubectl get pods $(id)`" }), `contains "$("`},
		{"placeholder typo", with(func(f *findings.Finding) { f.Remediation = "`kubectl get pdb -n <namepsace> <name>`" }), "unknown placeholder <namepsace>"},
		{"empty placeholder", with(func(f *findings.Finding) { f.Remediation = "`kubectl get pdb -n <> x`" }), "unknown placeholder <>"},
		{"no evidence", with(func(f *findings.Finding) { f.Evidence = nil }), "evidence is empty"},
		{"zero evidence", with(func(f *findings.Finding) { f.Evidence = findings.PDBEvidence{} }), "evidence is empty"},
		{"invalid severity", with(func(f *findings.Finding) { f.Severity = "SAFE" }), "not valid"},
		{"no resource", with(func(f *findings.Finding) { f.Resource = findings.ResourceRef{} }), "resource is not set"},
		{"description without a number or name", with(func(f *findings.Finding) { f.Description = "A budget is a problem." }), "neither a number nor the name"},
		{"empty description", with(func(f *findings.Finding) { f.Description = "" }), "description is empty"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := adviceViolations(tt.f)
			if !slices.ContainsFunc(v, func(s string) bool { return strings.Contains(s, tt.want) }) {
				t.Errorf("violations %v do not include %q", v, tt.want)
			}
		})
	}
}

func TestRemediationCommands(t *testing.T) {
	cmds, ok := remediationCommands("Run `kubectl get pdb -n a b` and `--force`, then `kubectl describe node x`.")
	if !ok || !slices.Equal(cmds, []string{"kubectl get pdb -n a b", "kubectl describe node x"}) {
		t.Errorf("got %v, %v", cmds, ok)
	}
	if cmds, ok := remediationCommands("kubectl drain skips them."); !ok || len(cmds) != 0 {
		t.Errorf("prose outside backticks is not a command: got %v, %v", cmds, ok)
	}
}
