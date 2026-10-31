package analyzer

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Veinar/draincheck/internal/findings"
)

func unsatisfiableFinding(ns, name string, expected, desired int32, minAvail, maxUnavail string, nodes []string, pods ...findings.PDBPod) findings.Finding {
	return findings.Finding{
		RuleID:   "PDB_UNSATISFIABLE",
		Severity: findings.Blocked,
		Resource: findings.ResourceRef{Kind: "PodDisruptionBudget", Namespace: ns, Name: name},
		Evidence: findings.PDBUnsatisfiableEvidence{
			Namespace: ns, Name: name, ExpectedPods: expected, DesiredHealthy: desired,
			MinAvailable: minAvail, MaxUnavailable: maxUnavail, Nodes: nodes, Pods: pods,
		},
	}
}

func budgetNames(fs []findings.Finding) []string {
	var out []string
	for _, f := range fs {
		out = append(out, f.Resource.Namespace+"/"+f.Resource.Name)
	}
	slices.Sort(out)
	return out
}

func TestPDBUnsatisfiable(t *testing.T) {
	s := snapshotFromFixture(t, "pdb-unsatisfiable.yaml")
	w1 := []string{"worker-01"}

	a := unsatisfiableFinding("a", "min-eq", 2, 2, "2", "", w1, pdbPod("a", "a-0", "worker-01"), pdbPod("a", "a-1", "worker-01"))
	c := unsatisfiableFinding("c", "min-above", 2, 5, "5", "", w1, pdbPod("c", "c-0", "worker-01"), pdbPod("c", "c-1", "worker-01"))
	i := unsatisfiableFinding("i", "pct", 2, 2, "", "0%", w1, pdbPod("i", "i-0", "worker-01"), pdbPod("i", "i-1", "worker-01"))

	tests := []struct {
		name    string
		targets []string
		drain   DrainOptions
		want    []findings.Finding
	}{
		{
			name:    "worker-01: minAvailable at or above the replicas, maxUnavailable 0 and 0%",
			targets: w1,
			want: []findings.Finding{a, c, i,
				unsatisfiableFinding("b", "max-zero", 3, 3, "", "0", w1, pdbPod("b", "b-0", "worker-01"), pdbPod("b", "b-1", "worker-01"))},
		},
		{
			name:    "both nodes: the budget lists the pod on worker-02 too",
			targets: []string{"worker-01", "worker-02"},
			want: []findings.Finding{
				a, c, i,
				unsatisfiableFinding("b", "max-zero", 3, 3, "", "0", []string{"worker-01", "worker-02"},
					pdbPod("b", "b-0", "worker-01"), pdbPod("b", "b-1", "worker-01"), pdbPod("b", "b-2", "worker-02")),
			},
		},
		{
			name:    "--disable-eviction: no budget is consulted",
			targets: w1,
			drain:   DrainOptions{DisableEviction: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PDBUnsatisfiable{}.Evaluate(Input{Snapshot: s, Targets: tt.targets, Drain: tt.drain})
			findings.Sort(got)
			want := slices.Clone(tt.want)
			findings.Sort(want)
			if !reflect.DeepEqual(got, withText(want, got)) {
				t.Fatalf("findings mismatch\n got: %+v\nwant: %+v", got, want)
			}
		})
	}
}

// A budget gets one finding: PDB_UNSATISFIABLE takes the budgets that can never
// allow a disruption, PDB_DISRUPTION_BLOCKED keeps the ones at 0 that may clear
// and the ones it cannot judge (operator-managed, unknown policy).
func TestPDBUnsatisfiableAndDisruptionBlockedNeverShareABudget(t *testing.T) {
	s := snapshotFromFixture(t, "pdb-unsatisfiable.yaml")
	in := Input{Snapshot: s, Targets: []string{"worker-01", "worker-02"}}
	unsat := budgetNames(PDBUnsatisfiable{}.Evaluate(in))
	blocked := budgetNames(PDBDisruptionBlocked{}.Evaluate(in))

	if want := []string{"a/min-eq", "b/max-zero", "c/min-above", "i/pct"}; !slices.Equal(unsat, want) {
		t.Errorf("PDB_UNSATISFIABLE reported %v, want %v", unsat, want)
	}
	if want := []string{"/f", "d/transient", "g/odd-policy"}; !slices.Equal(blocked, want) {
		t.Errorf("PDB_DISRUPTION_BLOCKED reported %v, want %v", blocked, want)
	}
	for _, n := range unsat {
		if slices.Contains(blocked, n) {
			t.Errorf("%s is reported by both rules", n)
		}
	}
}

func TestPDBUnsatisfiableKeepsOperatorAndUnknownPolicyUnknown(t *testing.T) {
	s := snapshotFromFixture(t, "pdb-unsatisfiable.yaml")
	for _, f := range (PDBDisruptionBlocked{}).Evaluate(Input{Snapshot: s, Targets: []string{"worker-01"}}) {
		switch f.Resource.Namespace + "/" + f.Resource.Name {
		case "/f", "g/odd-policy":
			if f.Severity != findings.Unknown {
				t.Errorf("%s: severity %v, want UNKNOWN", f.Resource, f.Severity)
			}
		case "d/transient":
			if f.Severity != findings.Blocked {
				t.Errorf("%s: severity %v, want BLOCKED", f.Resource, f.Severity)
			}
		}
	}
}

func TestPDBUnsatisfiableText(t *testing.T) {
	s := snapshotFromFixture(t, "pdb-unsatisfiable.yaml")
	got := map[string]findings.Finding{}
	for _, f := range (PDBUnsatisfiable{}).Evaluate(Input{Snapshot: s, Targets: []string{"worker-01"}}) {
		got[f.Resource.Namespace] = f
	}
	tests := []struct {
		ns   string
		want []string
	}{
		{"a", []string{"PodDisruptionBudget a/min-eq", "can never allow a disruption", "minAvailable 2", "2 expected pods", "until --timeout"}},
		{"b", []string{"maxUnavailable is 0", "deliberate"}},
		{"c", []string{"minAvailable 5", "2 expected pods"}},
	}
	for _, tt := range tests {
		f, ok := got[tt.ns]
		if !ok {
			t.Fatalf("no finding for namespace %s", tt.ns)
		}
		for _, w := range tt.want {
			if !strings.Contains(f.Description, w) {
				t.Errorf("%s: description %q lacks %q", tt.ns, f.Description, w)
			}
		}
	}
	if strings.Contains(got["a"].Description, "deliberate") {
		t.Errorf("a minAvailable budget is not described as deliberate: %q", got["a"].Description)
	}
	if !strings.Contains(got["a"].Remediation, "kubectl get pdb -n <namespace> <name>") {
		t.Errorf("remediation %q has no read-only command to confirm", got["a"].Remediation)
	}
}

func TestPDBUnsatisfiableNoSnapshot(t *testing.T) {
	if got := (PDBUnsatisfiable{}).Evaluate(Input{Targets: []string{"worker-01"}}); len(got) != 0 {
		t.Errorf("no snapshot gave %+v", got)
	}
}
