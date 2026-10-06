package engine

import (
	"errors"
	"fmt"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

type snapshotRejection struct{}

func (snapshotRejection) Error() string                          { return "snapshot size limit" }
func (snapshotRejection) NonFatalPetriMutationPersistenceError() {}
func (snapshotRejection) SnapshotSizeLimitDiagnostics() (string, int, int) {
	return "session-safe", 65 << 20, 64 << 20
}

type persistenceTestLogger struct {
	logging.NoopLogger
	fields map[string]any
}

func (logger *persistenceTestLogger) Error(_ string, fields ...any) {
	logger.fields = make(map[string]any)
	for index := 0; index+1 < len(fields); index += 2 {
		logger.fields[fields[index].(string)] = fields[index+1]
	}
}

func TestEnginePersistenceRejectionContinuesCompletedDispatchesWithSafeDiagnostics(t *testing.T) {
	t.Parallel()
	logger := &persistenceTestLogger{}
	calls := 0
	engine := &FactoryEngine{logger: logger, recordPetriMutations: func([]interfaces.TokenMutationRecord) error {
		calls++
		if calls == 1 {
			return fmt.Errorf("record: %w", snapshotRejection{})
		}
		return nil
	}}
	completed := []interfaces.CompletedDispatch{
		{DispatchID: "first", OutputMutations: []interfaces.TokenMutationRecord{{TokenID: "a"}}},
		{DispatchID: "second", OutputMutations: []interfaces.TokenMutationRecord{{TokenID: "b"}}},
	}
	if err := engine.recordCompletedPetriMutations(completed); err != nil || calls != 2 {
		t.Fatalf("continued dispatch processing = %d calls, %v", calls, err)
	}
	for key, want := range map[string]any{"code": "durable_session_snapshot_size_limit", "session_id": "session-safe", "observed_bytes": 65 << 20, "max_bytes": 64 << 20, "persistence_degraded": true} {
		if logger.fields[key] != want {
			t.Fatalf("diagnostic %s = %v, want %v", key, logger.fields[key], want)
		}
	}
	fault := errors.New("writer unavailable")
	calls = 0
	engine.recordPetriMutations = func([]interfaces.TokenMutationRecord) error { calls++; return fault }
	if err := engine.recordCompletedPetriMutations(completed); !errors.Is(err, fault) || calls != 1 {
		t.Fatalf("ordinary writer fault = %v; calls=%d", err, calls)
	}
}
