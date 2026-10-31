package analyzer

import (
	"fmt"

	"github.com/Veinar/draincheck/internal/findings"
)

// PDBDisruptionBlockedID is the rule ID reported by PDBDisruptionBlocked.
const PDBDisruptionBlockedID = "PDB_DISRUPTION_BLOCKED"

// PDBDisruptionBlocked reports PodDisruptionBudgets whose status allows zero
// disruptions while they select pods on target nodes, so the eviction API will
// refuse to evict those pods.
//
// One BLOCKED finding is emitted per budget, pointing at the budget, however
// many pods it selects: ten replicas behind one budget are one finding. The
// selected pods and their nodes are in the typed evidence and none is dropped;
// a pod selected by two budgets is listed under each. Pods that kubectl drain
// never evicts are skipped (DaemonSet pods and static pods, see
// podDrain.consultsBudget), as are pods in a terminal phase (Succeeded, Failed),
// because eviction does not consult budgets for them. A budget left with no
// pod gives no finding. A budget with an invalid selector is skipped: it
// cannot be matched reliably.
//
// Pods that bypass every budget (Pending, terminating, see podDrain.bypassesBudget) are
// skipped like finished ones. A pod that is not Ready is skipped too when the
// eviction API lets it past the budget (see evictedPastBudget); it is listed in
// the evidence apart from the pods the budget refuses, and a budget left with
// no refusable pod gives no finding. A budget whose unhealthyPodEvictionPolicy
// is a value draincheck does not know is UNKNOWN, not guessed.
//
// A budget that selects a pod another budget selects too (see overlappedPDBs) is
// skipped: the disruption controller chooses one of them arbitrarily, so its
// numbers are not reliable, and PDBOverlappingSelectors reports the pod, which the
// eviction API refuses whatever the budgets say.
//
// A budget whose status is stale (see PDBStaleStatus) is skipped too: its
// numbers cannot be trusted, so PDBStaleStatus reports it as UNKNOWN instead.
// So is a budget whose disruption controller failed to compute it (see
// PDBSyncFailed): the 0 it records is not a real budget.
//
// A budget that can never allow a disruption (see unsatisfiableBudget) is
// skipped: PDBUnsatisfiable reports it, so no budget is reported twice. This
// rule keeps the budgets at 0 that may clear by themselves.
//
// A budget that selects a pod controlled by a custom resource (an operator, see
// operatorOwned) is not BLOCKED but UNKNOWN: whether the drain stops depends on
// operator state draincheck cannot see. Those budgets are grouped per operator
// and namespace, see operatorManagedFindings. Budgets whose pods are controlled
// by Deployments, StatefulSets, Jobs and the like stay BLOCKED.
type PDBDisruptionBlocked struct{}

// ID implements Rule.
func (PDBDisruptionBlocked) ID() string { return PDBDisruptionBlockedID }

// Evaluate implements Rule.
func (PDBDisruptionBlocked) Evaluate(in Input) []findings.Finding {
	var out []findings.Finding
	var operatorManaged []pdbSelection
	for _, sel := range selectedPDBs(in) {
		pdb := sel.pdb
		if sel.overlapped || pdb.Status.DisruptionsAllowed != 0 || pdbStale(pdb) {
			continue
		}
		if _, failed := pdbSyncFailure(pdb); failed {
			continue
		}
		if unsatisfiableBudget(sel) {
			continue // reported once, by PDB_UNSATISFIABLE
		}
		if !sel.policyKnown {
			out = append(out, unknownPolicyFinding(sel))
			continue
		}
		if len(sel.operators) > 0 {
			operatorManaged = append(operatorManaged, sel)
			continue
		}
		description := fmt.Sprintf(
			"PodDisruptionBudget %s/%s allows 0 disruptions (%d healthy, %d desired), so eviction of its %s on %s will be refused. kubectl drain retries a refused eviction until --timeout, which is infinite by default, so it waits and does not complete.",
			pdb.Namespace, pdb.Name, pdb.Status.CurrentHealthy, pdb.Status.DesiredHealthy,
			plural(len(sel.pods), "pod"), plural(len(sel.nodes), "target node"))
		if sel.unready > 0 {
			description += " The budget itself is unhealthy, so with IfHealthyBudget it refuses the pods that are not Ready as well."
		}
		out = append(out, findings.Finding{
			RuleID:      PDBDisruptionBlockedID,
			Severity:    findings.Blocked,
			Resource:    sel.resource(),
			Evidence:    sel.evidence(),
			Description: description,
			Remediation: "Confirm with `kubectl get pdb -n <namespace> <name>` (ALLOWED DISRUPTIONS is 0) and `kubectl describe pdb -n <namespace> <name>` (selector, current and desired). Then, safest first: bring the selected pods to Ready, by adding replicas or fixing the ones that are not, until the budget allows a disruption; or, if the budget is stricter than the workload needs, relax minAvailable/maxUnavailable. Do this before draining: the drain only waits.",
		})
	}
	return append(out, operatorManagedFindings(operatorManaged)...)
}

// unknownPolicyFinding is the UNKNOWN finding for a budget at 0 whose
// unhealthyPodEvictionPolicy draincheck does not know: which of its pods
// eviction refuses depends on that policy, so nothing is guessed.
func unknownPolicyFinding(sel pdbSelection) findings.Finding {
	pdb := sel.pdb
	return findings.Finding{
		RuleID:   PDBDisruptionBlockedID,
		Severity: findings.Unknown,
		Resource: sel.resource(),
		Evidence: sel.evidence(),
		Description: fmt.Sprintf(
			"PodDisruptionBudget %s/%s allows 0 disruptions (%d healthy, %d desired), but its unhealthyPodEvictionPolicy %q is not one draincheck knows, so it cannot tell whether eviction of its %s on %s will be refused.",
			pdb.Namespace, pdb.Name, pdb.Status.CurrentHealthy, pdb.Status.DesiredHealthy,
			configuredPolicy(pdb), plural(len(sel.pods), "pod"), plural(len(sel.nodes), "target node")),
		Remediation: "Confirm with `kubectl get pdb -n <namespace> <name> -o yaml` and read spec.unhealthyPodEvictionPolicy. If the value is not AlwaysAllow or IfHealthyBudget, look up what it does in the Kubernetes documentation for the version of the cluster, then run draincheck again. Until then, treat the budget as one that may refuse the eviction.",
	}
}
