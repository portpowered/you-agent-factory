package factorysessionexecution

import (
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

// defaultPersistedTokenFailureLogCapacity bounds the failure records retained
// in each token copy written to a durable Factory Session snapshot. The live
// Factory Runtime history remains unbounded; this is a serialization policy.
const defaultPersistedTokenFailureLogCapacity = 32

// compactPersistedTokenFailureLogs applies the durable snapshot retention
// policy without changing the live runtime state from which the snapshot was
// built. The retained records are an ordered oldest head followed by a newest
// tail, and the omitted count is carried in the persisted History value.
func compactPersistedTokenFailureLogs(
	snapshot *PersistedRuntimeSessionState,
	failureLogCapacity int,
) {
	if snapshot == nil || failureLogCapacity <= 0 {
		return
	}
	for index := range snapshot.Records {
		compactPersistedTokenFailureLog(
			snapshot.Records[index].PetriMutation,
			failureLogCapacity,
		)
	}
}

func compactPersistedTokenFailureLog(
	mutation *interfaces.TokenMutationRecord,
	failureLogCapacity int,
) {
	if mutation == nil || mutation.Token == nil {
		return
	}
	history := mutation.Token.History
	if len(history.FailureLog) <= failureLogCapacity {
		return
	}

	headCount := failureLogCapacity / 2
	tailCount := failureLogCapacity - headCount
	dropped := len(history.FailureLog) - failureLogCapacity
	retained := make([]workerexecution.Failure, 0, failureLogCapacity)
	retained = append(retained, history.FailureLog[:headCount]...)
	retained = append(retained, history.FailureLog[len(history.FailureLog)-tailCount:]...)
	history.FailureLog = retained
	history.FailureLogDroppedCount += dropped
	mutation.Token.History = history
}
