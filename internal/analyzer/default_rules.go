package analyzer

// DefaultRules returns every rule draincheck runs, in a stable order.
func DefaultRules() []Rule {
	return []Rule{
		PDBDisruptionBlocked{},
		PDBUnsatisfiable{},
		PDBOverlappingSelectors{},
		PDBBatchExceedsBudget{},
		PDBStaleStatus{},
		PDBSyncFailed{},
		NodeNotSchedulable{},
		NoRemainingNodes{},
		WorkloadUntoleratedTaints{},
		SingleReplicaWorkload{},
		AllReplicasOnTargets{},
		PodOwnership{},
		PodLocalStorage{},
		MissingResourceRequests{},
	}
}
