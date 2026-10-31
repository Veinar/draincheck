package tests

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/Veinar/draincheck/internal/analyzer"
	"github.com/Veinar/draincheck/internal/findings"
	"github.com/Veinar/draincheck/internal/inventory"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
)

// runFixture loads testdata/regression/<name> into a fake clientset, collects
// a snapshot the way the CLI does and runs every default rule against all
// nodes. The integration tests live in their own package, so this cannot reuse
// the helpers of the unit tests.
func runFixture(t *testing.T, name string) []findings.Finding {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "testdata", "regression", name))
	if err != nil {
		t.Fatal(err)
	}

	var objs []runtime.Object
	docs := utilyaml.NewYAMLReader(bufio.NewReader(bytes.NewReader(data)))
	for {
		doc, err := docs.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(bytes.TrimSpace(doc)) == 0 {
			continue
		}
		obj, _, err := scheme.Codecs.UniversalDeserializer().Decode(doc, nil, nil)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if pod, ok := obj.(*corev1.Pod); ok && pod.Status.Phase == "" && len(pod.Status.Conditions) == 0 {
			// A pod with no status is a healthy one: the eviction handler treats a
			// pod with no Ready condition as not ready, which no running pod is.
			pod.Status.Phase = corev1.PodRunning
			pod.Status.Conditions = []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}
		}
		objs = append(objs, obj)
	}

	snap, err := inventory.NewClientsetCollector(fake.NewClientset(objs...), nil).Collect(context.Background())
	if err != nil {
		t.Fatalf("%s: collect: %v", name, err)
	}
	targets, err := inventory.ResolveTargetNodes(snap, nil, true)
	if err != nil {
		t.Fatalf("%s: targets: %v", name, err)
	}
	return analyzer.Run(nil, analyzer.Input{Snapshot: snap, Targets: targets}, analyzer.DefaultRules()...)
}

// podsCovered counts the pods a finding's evidence lists, so a regrouped
// finding can be checked against the pods it replaced. It returns -1 for
// evidence that does not list pods.
func podsCovered(e findings.Evidence) int {
	switch ev := e.(type) {
	case findings.MissingRequestsEvidence:
		return len(ev.Pods)
	case findings.OwnershipEvidence:
		return len(ev.Pods)
	case findings.PDBEvidence:
		return len(ev.Pods)
	case findings.PDBUnsatisfiableEvidence:
		return len(ev.Pods)
	case findings.PDBStaleStatusEvidence:
		return len(ev.Pods)
	case findings.PDBBatchEvidence:
		return len(ev.Pods)
	case findings.OperatorManagedPDBsEvidence:
		n := 0
		for _, pdb := range ev.PDBs {
			n += len(pdb.Pods)
		}
		return n
	}
	return -1
}

// TestNoiseRegression locks in the grouped result for a cluster shaped like the
// run that motivated the noise work (95 findings, 4 BLOCKED, 91 RISK). The
// fixture is synthetic: 59 pods, where each cause is one finding, the four
// operator budgets are one UNKNOWN finding, and only the two real problems are
// BLOCKED. If a rule starts reporting per pod again, or a budget it should not
// trust turns BLOCKED, this test fails.
func TestNoiseRegression(t *testing.T) {
	got := runFixture(t, "noise.yaml")

	// In the order findings.Sort gives: namespace, kind, name, then rule.
	want := []struct {
		rule     string
		severity findings.Severity
		kind     string
		ns, name string
		pods     int
	}{
		{"PDB_DISRUPTION_BLOCKED", findings.Unknown, "Namespace", "", "storage-system", 4},
		{"POD_OWNERSHIP", findings.Risk, "Node", "", "worker-01", 2},
		{"NO_REMAINING_NODES", findings.Risk, "Node", "", "worker-01,worker-02,worker-03,worker-04,worker-05,worker-06", -1},
		{"POD_OWNERSHIP", findings.Risk, "Node", "", "worker-02", 2},
		{"POD_OWNERSHIP", findings.Risk, "Node", "", "worker-03", 2},
		{"MISSING_RESOURCE_REQUESTS", findings.Risk, "Pod", "default", "debug-shell", 1},
		{"POD_OWNERSHIP", findings.Blocked, "Pod", "default", "debug-shell", 1},
		{"POD_OWNERSHIP", findings.Blocked, "DaemonSet", "kube-system", "kube-proxy", 6},
		{"POD_OWNERSHIP", findings.Blocked, "DaemonSet", "monitoring", "node-exporter", 6},
		{"MISSING_RESOURCE_REQUESTS", findings.Risk, "Deployment", "shop", "api", 10},
		{"MISSING_RESOURCE_REQUESTS", findings.Risk, "Deployment", "shop", "web", 12},
		{"MISSING_RESOURCE_REQUESTS", findings.Risk, "Deployment", "shop", "worker", 8},
		{"PDB_OVERLAPPING_SELECTORS", findings.Blocked, "Pod", "shop", "cache-0", -1},
		{"PDB_BATCH_EXCEEDS_BUDGET", findings.Risk, "PodDisruptionBudget", "shop", "api-pdb", 10},
		{"PDB_UNSATISFIABLE", findings.Blocked, "PodDisruptionBudget", "shop", "db-pdb", 3},
		{"PDB_STALE_STATUS", findings.Unknown, "PodDisruptionBudget", "shop", "queue-pdb", 2},
		{"PDB_BATCH_EXCEEDS_BUDGET", findings.Risk, "PodDisruptionBudget", "shop", "web-pdb", 12},
	}

	if len(got) != len(want) {
		t.Errorf("got %d findings, want %d", len(got), len(want))
		for _, f := range got {
			t.Logf("  %s %s %s %s/%s", f.RuleID, f.Severity, f.Resource.Kind, f.Resource.Namespace, f.Resource.Name)
		}
		return
	}
	for i, w := range want {
		f := got[i]
		if f.RuleID != w.rule || f.Severity != w.severity ||
			f.Resource.Kind != w.kind || f.Resource.Namespace != w.ns || f.Resource.Name != w.name {
			t.Errorf("finding %d = %s %s %s %s/%s, want %s %s %s %s/%s", i,
				f.RuleID, f.Severity, f.Resource.Kind, f.Resource.Namespace, f.Resource.Name,
				w.rule, w.severity, w.kind, w.ns, w.name)
			continue
		}
		// Grouping must not drop pods: the evidence still lists each one.
		if w.pods >= 0 {
			if n := podsCovered(f.Evidence); n != w.pods {
				t.Errorf("%s %s/%s lists %d pods, want %d", f.RuleID, f.Resource.Namespace, f.Resource.Name, n, w.pods)
			}
		}
	}
}

// TestNoiseRegressionSeverities checks the totals an operator reads first: no
// BLOCKED finding comes from an operator or a stale budget.
func TestNoiseRegressionSeverities(t *testing.T) {
	counts := map[findings.Severity]int{}
	for _, f := range runFixture(t, "noise.yaml") {
		counts[f.Severity]++
	}
	want := map[findings.Severity]int{findings.Blocked: 5, findings.Risk: 10, findings.Unknown: 2}
	for sev, n := range want {
		if counts[sev] != n {
			t.Errorf("%s findings = %d, want %d", sev, counts[sev], n)
		}
	}
}
