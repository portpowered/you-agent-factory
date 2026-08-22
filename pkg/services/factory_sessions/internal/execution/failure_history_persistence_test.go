package factorysessionexecution

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/runtimepersist"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestPersistedTokenFailureHistoryRetainsHeadTailAndReloads(t *testing.T) {
	const historySize = defaultPersistedTokenFailureLogCapacity + 8
	history := failureHistoryForRetryCount(historySize)
	state := runtimeSessionState{
		petriMutations: []interfaces.TokenMutationRecord{failureMutation(history, 1)},
	}

	snapshot := persistedSnapshotFromRuntimeState(state)
	if len(state.petriMutations[0].Token.History.FailureLog) != historySize {
		t.Fatalf("live failure log length = %d, want %d", len(state.petriMutations[0].Token.History.FailureLog), historySize)
	}
	if got := state.petriMutations[0].Token.History.FailureLogDroppedCount; got != 0 {
		t.Fatalf("live dropped failure count = %d, want 0", got)
	}

	got := snapshot.Records[0].PetriMutation.Token.History
	if got.FailureLogDroppedCount != 8 {
		t.Fatalf("persisted dropped failure count = %d, want 8", got.FailureLogDroppedCount)
	}
	if len(got.FailureLog) != defaultPersistedTokenFailureLogCapacity {
		t.Fatalf("persisted failure log length = %d, want %d", len(got.FailureLog), defaultPersistedTokenFailureLogCapacity)
	}
	for index := range got.FailureLog {
		wantIndex := index
		if index >= defaultPersistedTokenFailureLogCapacity/2 {
			wantIndex = historySize - (defaultPersistedTokenFailureLogCapacity - defaultPersistedTokenFailureLogCapacity/2) + index - defaultPersistedTokenFailureLogCapacity/2
		}
		want := history.FailureLog[wantIndex]
		if got.FailureLog[index] != want {
			t.Fatalf("persisted failure log[%d] = %#v, want %#v", index, got.FailureLog[index], want)
		}
	}
	if got.LastError != history.LastError {
		t.Fatalf("persisted LastError = %q, want %q", got.LastError, history.LastError)
	}

	encoded, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		t.Fatalf("marshal bounded snapshot: %v", err)
	}
	var reloaded PersistedRuntimeSessionState
	if err := json.Unmarshal(encoded, &reloaded); err != nil {
		t.Fatalf("unmarshal bounded snapshot: %v", err)
	}
	reloadedHistory := reloaded.Records[0].PetriMutation.Token.History
	if reloadedHistory.FailureLogDroppedCount != got.FailureLogDroppedCount ||
		reloadedHistory.LastError != got.LastError ||
		len(reloadedHistory.FailureLog) != len(got.FailureLog) {
		t.Fatalf("reloaded failure history = %#v, want %#v", reloadedHistory, got)
	}
}

func TestPersistedTokenFailureHistoryWithinCapacityIsUnchanged(t *testing.T) {
	history := failureHistoryForRetryCount(defaultPersistedTokenFailureLogCapacity)
	state := runtimeSessionState{
		petriMutations: []interfaces.TokenMutationRecord{failureMutation(history, 1)},
	}

	snapshot := persistedSnapshotFromRuntimeState(state)
	got := snapshot.Records[0].PetriMutation.Token.History
	if len(got.FailureLog) != len(history.FailureLog) {
		t.Fatalf("failure log length = %d, want %d", len(got.FailureLog), len(history.FailureLog))
	}
	for index := range history.FailureLog {
		if got.FailureLog[index] != history.FailureLog[index] {
			t.Fatalf("failure log[%d] changed: got %#v, want %#v", index, got.FailureLog[index], history.FailureLog[index])
		}
	}
	if got.FailureLogDroppedCount != history.FailureLogDroppedCount || got.LastError != history.LastError {
		t.Fatalf("history metadata changed: got %#v, want %#v", got, history)
	}
}

func TestDurablePetriFailureHistorySnapshotGrowthIsBounded(t *testing.T) {
	retryCounts := []int{10, 100, 1000}
	baselineBytes := make(map[int]int, len(retryCounts))
	boundedBytes := make(map[int]int, len(retryCounts))

	for _, retryCount := range retryCounts {
		t.Run(fmt.Sprintf("N=%d", retryCount), func(t *testing.T) {
			store := &failureHistorySnapshotStore{}
			mutations := make([]interfaces.TokenMutationRecord, retryCount)
			for retry := 1; retry <= retryCount; retry++ {
				mutations[retry-1] = failureMutation(failureHistoryForRetryCount(retry), retry)
			}
			state := runtimeSessionState{
				session: SessionReadResult{
					SessionID: "~default",
					Status:    LifecycleStatusRunning,
				},
				petriMutations: mutations,
			}
			service := &JavaScriptRuntimeService{
				clock:       failureHistoryClock{},
				persistence: store,
			}
			if err := service.persistSessionSnapshot(state); err != nil {
				t.Fatalf("persist retry sequence: %v", err)
			}

			live := cloneRuntimeSessionState(&state)
			last := live.petriMutations[len(live.petriMutations)-1].Token.History
			if len(last.FailureLog) != retryCount {
				t.Fatalf("live final failure log length = %d, want %d", len(last.FailureLog), retryCount)
			}
			if last.FailureLogDroppedCount != 0 {
				t.Fatalf("live final dropped failure count = %d, want 0", last.FailureLogDroppedCount)
			}

			unbounded := persistedSnapshotFromRuntimeStateWithFailureLogCapacity(live, 0)
			before, err := json.MarshalIndent(unbounded, "", "  ")
			if err != nil {
				t.Fatalf("marshal unbounded baseline: %v", err)
			}
			baselineBytes[retryCount] = len(before)
			boundedBytes[retryCount] = len(store.payload)
			t.Logf("N=%d before_bytes=%d after_bytes=%d", retryCount, len(before), len(store.payload))
		})
	}

	if boundedBytes[10] != baselineBytes[10] {
		t.Fatalf("N=10 changed despite fitting capacity: before=%d after=%d", baselineBytes[10], boundedBytes[10])
	}
	for _, retryCount := range []int{100, 1000} {
		if boundedBytes[retryCount] >= baselineBytes[retryCount] {
			t.Fatalf("N=%d bounded snapshot = %d, want less than unbounded baseline %d", retryCount, boundedBytes[retryCount], baselineBytes[retryCount])
		}
	}
	if boundedBytes[1000] > boundedBytes[100]*12 {
		t.Fatalf("bounded snapshots grew superlinearly: N=100=%d, N=1000=%d", boundedBytes[100], boundedBytes[1000])
	}
}

type failureHistorySnapshotStore struct {
	payload []byte
}

func (s *failureHistorySnapshotStore) Save(_ string, encoded []byte) error {
	s.payload = append([]byte(nil), encoded...)
	return nil
}

func (s *failureHistorySnapshotStore) Load(string) ([]byte, error) {
	if len(s.payload) == 0 {
		return nil, os.ErrNotExist
	}
	return append([]byte(nil), s.payload...), nil
}

type failureHistoryClock struct{}

func (failureHistoryClock) Now() time.Time {
	return time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
}

func failureMutation(history workerexecution.History, retry int) interfaces.TokenMutationRecord {
	return interfaces.TokenMutationRecord{
		DispatchID:   fmt.Sprintf("dispatch-%04d", retry),
		TransitionID: "retry",
		Outcome:      workerexecution.OutcomeFailed,
		Type:         interfaces.MutationMove,
		TokenID:      fmt.Sprintf("token-%04d", retry),
		FromPlace:    "task:running",
		ToPlace:      "task:failed",
		Reason:       "worker failed",
		Token: &workerexecution.Token{
			ID:    fmt.Sprintf("token-%04d", retry),
			State: "failed",
			Color: workerexecution.Color{
				WorkID:     "work-1",
				WorkTypeID: "task",
				DataType:   workerexecution.DataTypeWork,
			},
			History: history,
		},
	}
}

func failureHistoryForRetryCount(count int) workerexecution.History {
	log := make([]workerexecution.Failure, count)
	for index := range log {
		log[index] = workerexecution.Failure{
			TransitionID: "retry",
			Timestamp:    time.Date(2026, 8, 22, 12, 0, index, 0, time.UTC),
			Error:        fmt.Sprintf("failure-%04d", index+1),
			Attempt:      index + 1,
		}
	}
	lastError := ""
	if len(log) > 0 {
		lastError = log[len(log)-1].Error
	}
	return workerexecution.History{
		TotalVisits:         map[string]int{"retry": count},
		ConsecutiveFailures: map[string]int{"retry": count},
		PlaceVisits:         map[string]int{"task:failed": count},
		LastError:           lastError,
		FailureLog:          log,
	}
}

var _ runtimepersist.Store = (*failureHistorySnapshotStore)(nil)
