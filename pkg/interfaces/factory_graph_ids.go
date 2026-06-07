package interfaces

import "strings"

// CanonicalFactoryGraphEntityID resolves the durable public identifier used by
// the graph contract, preferring explicit ids while preserving legacy name-keyed
// compatibility for factories that have not materialized ids yet.
func CanonicalFactoryGraphEntityID(explicitID, fallbackName string) string {
	normalizedID := strings.TrimSpace(explicitID)
	if normalizedID != "" {
		return normalizedID
	}
	return fallbackName
}

func CanonicalFactoryGraphResourceID(resource ResourceConfig) string {
	return CanonicalFactoryGraphEntityID(resource.ID, resource.Name)
}

func CanonicalFactoryGraphWorkerID(worker WorkerConfig) string {
	return CanonicalFactoryGraphEntityID(worker.ID, worker.Name)
}

func CanonicalFactoryGraphWorkTypeID(workType WorkTypeConfig) string {
	return CanonicalFactoryGraphEntityID(workType.ID, workType.Name)
}

func CanonicalFactoryGraphWorkstationID(workstation FactoryWorkstationConfig) string {
	return CanonicalFactoryGraphEntityID(workstation.ID, workstation.Name)
}

func CanonicalFactoryGraphWorkStateID(workType WorkTypeConfig, state StateConfig) string {
	return CanonicalFactoryGraphEntityID(workType.ID, workType.Name) +
		":" +
		CanonicalFactoryGraphEntityID(state.ID, state.Name)
}

func CanonicalFactoryGraphNodeID(kind, subjectID string) string {
	return kind + ":" + subjectID
}

func CanonicalFactoryGraphEdgeID(kind, sourceNodeID, targetNodeID string) string {
	return kind + ":" + sourceNodeID + "->" + targetNodeID
}
