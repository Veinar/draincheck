package findings

// PDBUnsatisfiableEvidence is the evidence for a PodDisruptionBudget that can
// never allow a disruption: status.desiredHealthy is not below
// status.expectedPods. There is one finding per budget, however many pods it
// selects.
type PDBUnsatisfiableEvidence struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	// ExpectedPods and DesiredHealthy are the status numbers as reported; the
	// budget is unsatisfiable because DesiredHealthy >= ExpectedPods.
	ExpectedPods   int32 `json:"expected_pods"`
	DesiredHealthy int32 `json:"desired_healthy"`
	// MinAvailable and MaxUnavailable are spec.minAvailable and
	// spec.maxUnavailable as configured (a number or a percentage); the one
	// that is not set is omitted.
	MinAvailable   string `json:"min_available,omitempty"`
	MaxUnavailable string `json:"max_unavailable,omitempty"`
	// Nodes are the target nodes the selected pods run on, sorted, without
	// duplicates.
	Nodes []string `json:"nodes"`
	// Pods are the pods on the target nodes that the budget selects and
	// kubectl drain would evict, sorted by namespace, name and node.
	Pods []PDBPod `json:"pods"`
}

func (PDBUnsatisfiableEvidence) isEvidence() {}
