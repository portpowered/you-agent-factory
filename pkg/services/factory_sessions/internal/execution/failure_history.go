package factorysessionexecution

import (
	"fmt"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

const (
	// defaultPersistedTokenFailureLogCapacity bounds the failure records
	// retained in each token copy written to a durable Factory Session
	// snapshot. The live Factory Runtime history remains unbounded; this is a
	// serialization policy.
	defaultPersistedTokenFailureLogCapacity = 32

	// defaultPersistedSnapshotMaxBytes bounds one encoded durable Factory
	// Session snapshot before the persistence writer is invoked. The limit is
	// deliberately a byte count because JSON encoding and filesystem writes are
	// byte-oriented operations.
	defaultPersistedSnapshotMaxBytes = 64 << 20
)

// SnapshotSizeLimitError reports a durable snapshot rejected before any
// persistence writer is called. It intentionally identifies only the target,
// measured size, and configured bound; snapshot content is never included.
type SnapshotSizeLimitError struct {
	Path        string
	ActualBytes int
	MaxBytes    int
}

func (e *SnapshotSizeLimitError) Error() string {
	if e == nil {
		return "durable session snapshot exceeds its configured byte limit"
	}
	return fmt.Sprintf(
		"durable session snapshot %q is %d bytes; configured maximum is %d bytes",
		e.Path,
		e.ActualBytes,
		e.MaxBytes,
	)
}

// NonFatalPetriMutationPersistenceError marks a deterministic size rejection
// as diagnosable runtime backpressure. The Factory Runtime can keep its
// process loop alive while direct Factory Session callers still receive the
// actionable error. Ordinary writer failures do not implement this marker
// and retain their existing fatal propagation behavior.
func (*SnapshotSizeLimitError) NonFatalPetriMutationPersistenceError() {}

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
