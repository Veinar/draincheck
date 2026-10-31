package analyzer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultRules(t *testing.T) {
	rules := DefaultRules()
	if len(rules) == 0 {
		t.Fatal("no default rules")
	}
	seen := map[string]bool{}
	for _, r := range rules {
		if r.ID() == "" {
			t.Errorf("%T has an empty ID", r)
		}
		if seen[r.ID()] {
			t.Errorf("duplicate rule ID %q", r.ID())
		}
		seen[r.ID()] = true
	}
	for _, id := range []string{"PDB_DISRUPTION_BLOCKED", "PDB_UNSATISFIABLE", "PDB_OVERLAPPING_SELECTORS", "PDB_STALE_STATUS", "PDB_SYNC_FAILED", "PDB_BATCH_EXCEEDS_BUDGET", "NODE_NOT_SCHEDULABLE", "NO_REMAINING_NODES", "WORKLOAD_UNTOLERATED_TAINTS", "SINGLE_REPLICA_WORKLOAD", "ALL_REPLICAS_ON_TARGETS", "POD_OWNERSHIP", "POD_LOCAL_STORAGE", "MISSING_RESOURCE_REQUESTS"} {
		if !seen[id] {
			t.Errorf("rule %q is not registered", id)
		}
	}
}

// docs/RULES.md ("Advice per rule") lists every rule with its fact, what
// kubectl drain does, the command that confirms it and the action. A rule
// without a row has no reviewed advice.
func TestAdviceTableListsEveryDefaultRule(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "docs", "RULES.md"))
	if err != nil {
		t.Fatal(err)
	}
	_, section, found := strings.Cut(string(data), "\n### Advice per rule\n")
	if !found {
		t.Fatal(`docs/RULES.md has no "### Advice per rule" section`)
	}
	if end := strings.Index(section, "\n## "); end >= 0 {
		section = section[:end]
	}
	if end := strings.Index(section, "\n### "); end >= 0 {
		section = section[:end]
	}
	for _, r := range DefaultRules() {
		if !strings.Contains(section, "\n| `"+r.ID()+"` |") {
			t.Errorf("rule %s has no row in the advice table of docs/RULES.md", r.ID())
		}
	}
}
