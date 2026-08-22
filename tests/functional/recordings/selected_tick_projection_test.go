package recordings_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingswire "github.com/portpowered/infinite-you/pkg/services/recordings/wire"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

// TestRecordingScopeProjectionReconstructsSelectedTicksInIsolation records
// interleaved histories through the public scope capability and reconstructs
// each history at multiple ticks. The selected scope owns the retained event
// prefix, while the selected tick controls which state-changing facts are
// effective in the detached projection.
func TestRecordingScopeProjectionReconstructsSelectedTicksInIsolation(t *testing.T) {
	t.Parallel()

	const generationID = "functional-recordings-projection-generation"
	recordedAt := time.Date(2026, time.August, 22, 11, 0, 0, 0, time.UTC)
	ledger := recordingswire.NewRuntimeLedger(
		nil,
		func() time.Time { return recordedAt },
		generationID,
		nil,
	)
	service := newFunctionalRecordingsService(t, ledger)
	ctx := context.Background()

	const (
		sessionA = "functional-projection-session-a"
		sessionB = "functional-projection-session-b"
	)
	scopeA := beginFunctionalProjectionScope(t, service, sessionA)
	scopeB := beginFunctionalProjectionScope(t, service, sessionB)

	eventsA := functionalProjectionEvents(t, sessionA, generationID, recordedAt, [4]recordings.CanonicalEventSequence{0, 2, 4, 6})
	eventsB := functionalProjectionEvents(t, sessionB, generationID, recordedAt, [4]recordings.CanonicalEventSequence{1, 3, 5, 7})
	appendFunctionalProjectionEvent(t, service, scopeA.Scope, eventsA[0])
	appendFunctionalProjectionEvent(t, service, scopeB.Scope, eventsB[0])
	appendFunctionalProjectionEvent(t, service, scopeA.Scope, eventsA[1])
	appendFunctionalProjectionEvent(t, service, scopeB.Scope, eventsB[1])
	appendFunctionalProjectionEvent(t, service, scopeA.Scope, eventsA[2])
	appendFunctionalProjectionEvent(t, service, scopeB.Scope, eventsB[2])
	appendFunctionalProjectionEvent(t, service, scopeA.Scope, eventsA[3])
	appendFunctionalProjectionEvent(t, service, scopeB.Scope, eventsB[3])
	finalizeFunctionalProjectionScope(t, service, scopeA.Scope, recordedAt.Add(10*time.Second), len(eventsA))
	finalizeFunctionalProjectionScope(t, service, scopeB.Scope, recordedAt.Add(11*time.Second), len(eventsB))

	canonicalBeforeQueries := ledger.CanonicalEvents()
	intermediate := reconstructFunctionalProjectionScope(t, service, scopeA.Scope, 1)
	assertFunctionalProjectionState(t, intermediate, sessionA, "work-session-a", "ready", false, 1, 0)
	if _, ok := decodeFunctionalProjectionState(t, intermediate.WorldState).WorkItemsByID["work-session-b"]; ok {
		t.Fatal("session A projection included session B work at the intermediate tick")
	}

	transitioned := reconstructFunctionalProjectionScope(t, service, scopeA.Scope, 2)
	assertFunctionalProjectionState(t, transitioned, sessionA, "work-session-a", "processing", false, 2, 1)

	terminal := reconstructFunctionalProjectionScope(t, service, scopeA.Scope, 3)
	assertFunctionalProjectionState(t, terminal, sessionA, "work-session-a", "done", true, 3, 2)
	stateB := reconstructFunctionalProjectionScope(t, service, scopeB.Scope, 3)
	assertFunctionalProjectionState(t, stateB, sessionB, "work-session-b", "done", true, 3, 2)
	if _, ok := decodeFunctionalProjectionState(t, stateB.WorldState).WorkItemsByID["work-session-a"]; ok {
		t.Fatal("session B projection included session A work")
	}

	repeated := reconstructFunctionalProjectionScope(t, service, scopeA.Scope, 3)
	if !reflect.DeepEqual(repeated, terminal) {
		t.Fatalf("repeated detached projection = %#v, want %#v", repeated, terminal)
	}
	if got := ledger.CanonicalEvents(); !reflect.DeepEqual(got, canonicalBeforeQueries) {
		t.Fatalf("canonical history changed during projection queries = %#v, want %#v", got, canonicalBeforeQueries)
	}

	if _, err := service.ReconstructRecordingScope(ctx, recordings.ReconstructRecordingScopeRequest{
		Scope:        scopeA.Scope,
		SelectedTick: -1,
	}); !errors.Is(err, recordings.ErrInvalidProjectionInput) {
		t.Fatalf("negative selected tick error = %v, want ErrInvalidProjectionInput", err)
	}
	if got := ledger.CanonicalEvents(); !reflect.DeepEqual(got, canonicalBeforeQueries) {
		t.Fatalf("canonical history changed after rejected projection = %#v, want %#v", got, canonicalBeforeQueries)
	}
}

func beginFunctionalProjectionScope(
	t *testing.T,
	service recordings.Service,
	sessionID string,
) recordings.BeginRecordingScopeResult {
	t.Helper()
	started, err := service.BeginRecordingScope(context.Background(), recordings.BeginRecordingScopeRequest{
		Enabled:     true,
		RecordingID: recordings.RecordingID("functional-recordings-projection-" + sessionID),
		Scope:       recordings.CanonicalEventScope{FactorySessionID: sessionID},
		Target: recordings.RecordingTargetRequest{
			Artifact: recordings.RecordingArtifactReference(filepath.Join(t.TempDir(), sessionID+".json")),
		},
	})
	if err != nil {
		t.Fatalf("BeginRecordingScope(%s): %v", sessionID, err)
	}
	if started.Scope.IsZero() || started.Status.State != recordings.RecordingActive {
		t.Fatalf("BeginRecordingScope(%s) = %#v, want active opaque scope", sessionID, started)
	}
	return started
}

func appendFunctionalProjectionEvent(
	t *testing.T,
	service recordings.Service,
	scope recordings.RecordingScopeRef,
	event recordings.CanonicalEvent,
) recordings.CanonicalEvent {
	t.Helper()
	appended, err := service.AppendRecordingScopeEvent(context.Background(), recordings.AppendRecordingScopeEventRequest{
		Scope: scope,
		Event: event,
	})
	if err != nil {
		t.Fatalf("AppendRecordingScopeEvent(%q): %v", event.ID, err)
	}
	return appended.Event
}

func finalizeFunctionalProjectionScope(
	t *testing.T,
	service recordings.Service,
	scope recordings.RecordingScopeRef,
	finishedAt time.Time,
	wantEvents int,
) {
	t.Helper()
	finalized, err := service.FinalizeRecordingScope(context.Background(), recordings.FinalizeRecordingScopeRequest{
		Scope:      scope,
		FinishedAt: finishedAt,
	})
	if err != nil {
		t.Fatalf("FinalizeRecordingScope(%q): %v", scope, err)
	}
	if finalized.Status.State != recordings.RecordingFinalized || finalized.Status.AcceptedEvents != wantEvents {
		t.Fatalf("FinalizeRecordingScope(%q) = %#v, want finalized scope with %d events", scope, finalized, wantEvents)
	}
}

func reconstructFunctionalProjectionScope(
	t *testing.T,
	service recordings.Service,
	scope recordings.RecordingScopeRef,
	selectedTick int,
) recordings.ReconstructRecordingScopeResult {
	t.Helper()
	result, err := service.ReconstructRecordingScope(context.Background(), recordings.ReconstructRecordingScopeRequest{
		Scope:        scope,
		SelectedTick: selectedTick,
	})
	if err != nil {
		t.Fatalf("ReconstructRecordingScope(%q, tick %d): %v", scope, selectedTick, err)
	}
	return result
}

func assertFunctionalProjectionState(
	t *testing.T,
	result recordings.ReconstructRecordingScopeResult,
	sessionID string,
	workID string,
	wantState string,
	wantTerminal bool,
	wantTick int,
	wantChanges int,
) {
	t.Helper()
	if result.WorldState.Scope.FactorySessionID != sessionID || result.WorldState.SelectedTick != wantTick {
		t.Fatalf("projection view = %#v, want session %q at tick %d", result.WorldState, sessionID, wantTick)
	}
	state := decodeFunctionalProjectionState(t, result.WorldState)
	item, ok := state.WorkItemsByID[workID]
	if !ok || item.State != wantState {
		t.Fatalf("projected work %q = %#v, want state %q", workID, item, wantState)
	}
	if got := len(state.WorkStateChangesByWorkID[workID]); got != wantChanges {
		t.Fatalf("projected work-state changes = %d, want %d", got, wantChanges)
	}
	if wantTerminal {
		if _, ok := state.TerminalWorkByID[workID]; !ok {
			t.Fatalf("projected work %q missing terminal index", workID)
		}
		if _, ok := state.ActiveWorkItemsByID[workID]; ok {
			t.Fatalf("projected terminal work %q remained active", workID)
		}
		return
	}
	if _, ok := state.ActiveWorkItemsByID[workID]; !ok {
		t.Fatalf("projected non-terminal work %q missing active index", workID)
	}
	if _, ok := state.TerminalWorkByID[workID]; ok {
		t.Fatalf("projected non-terminal work %q appeared in terminal index", workID)
	}
}

func decodeFunctionalProjectionState(t *testing.T, view recordings.WorldStateView) recordings.FactoryWorldState {
	t.Helper()
	if view.SchemaVersion != recordings.WorldStateViewSchemaV1 {
		t.Fatalf("world-state schema = %q, want %q", view.SchemaVersion, recordings.WorldStateViewSchemaV1)
	}
	var state recordings.FactoryWorldState
	if err := json.Unmarshal([]byte(view.Payload), &state); err != nil {
		t.Fatalf("decode detached world-state payload: %v", err)
	}
	return state
}

func functionalProjectionEvents(
	t *testing.T,
	sessionID string,
	generationID string,
	recordedAt time.Time,
	globalSequences [4]recordings.CanonicalEventSequence,
) []recordings.CanonicalEvent {
	t.Helper()
	workID := "work-" + sessionID[len("functional-projection-"):]
	return []recordings.CanonicalEvent{
		functionalProjectionEvent(
			"projection-"+sessionID+"-run",
			globalSequences[0],
			0,
			recordings.CanonicalEventScope{FactorySessionID: sessionID},
			generationID,
			recordings.CanonicalEventKind(factorydefinitions.FactoryEventTypeRunRequest),
			functionalProjectionRunRequestPayload(t, sessionID, recordedAt),
			recordedAt,
		),
		functionalProjectionEvent(
			"projection-"+sessionID+"-work",
			globalSequences[1],
			1,
			recordings.CanonicalEventScope{FactorySessionID: sessionID},
			generationID,
			recordings.CanonicalEventKind(factorydefinitions.FactoryEventTypeWorkRequest),
			functionalProjectionWorkRequestPayload(t, sessionID, workID),
			recordedAt.Add(time.Second),
		),
		functionalProjectionEvent(
			"projection-"+sessionID+"-processing",
			globalSequences[2],
			2,
			recordings.CanonicalEventScope{FactorySessionID: sessionID},
			generationID,
			recordings.CanonicalEventKind(factorydefinitions.FactoryEventTypeWorkStateChange),
			functionalProjectionStateChangePayload(t, workID, "ready", "processing"),
			recordedAt.Add(2*time.Second),
		),
		functionalProjectionEvent(
			"projection-"+sessionID+"-done",
			globalSequences[3],
			3,
			recordings.CanonicalEventScope{FactorySessionID: sessionID},
			generationID,
			recordings.CanonicalEventKind(factorydefinitions.FactoryEventTypeWorkStateChange),
			functionalProjectionStateChangePayload(t, workID, "processing", "done"),
			recordedAt.Add(3*time.Second),
		),
	}
}

func functionalProjectionEvent(
	id string,
	sequence recordings.CanonicalEventSequence,
	factoryTick int,
	scope recordings.CanonicalEventScope,
	generationID string,
	kind recordings.CanonicalEventKind,
	payload string,
	recordedAt time.Time,
) recordings.CanonicalEvent {
	event := functionalCanonicalEvent(
		id,
		sequence,
		scope,
		generationID,
		kind,
		payload,
		recordedAt,
	)
	event.FactoryTick = factoryTick
	return event
}

func functionalProjectionRunRequestPayload(t *testing.T, sessionID string, recordedAt time.Time) string {
	t.Helper()
	snapshot, err := factorydefinitions.NewFactorySnapshot(map[string]any{
		"id": "functional-recordings-projection-factory-" + sessionID,
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "ready", "type": "INITIAL"},
				{"name": "processing", "type": "PROCESSING"},
				{"name": "done", "type": "TERMINAL"},
			},
		}},
	})
	if err != nil {
		t.Fatalf("NewFactorySnapshot(%s): %v", sessionID, err)
	}
	payload, err := json.Marshal(factorydefinitions.RunRequestEventPayload{
		Factory:    snapshot,
		RecordedAt: recordedAt,
	})
	if err != nil {
		t.Fatalf("marshal projection run request(%s): %v", sessionID, err)
	}
	return string(payload)
}

func functionalProjectionWorkRequestPayload(t *testing.T, sessionID, workID string) string {
	t.Helper()
	payload, err := json.Marshal(work.WorkRequestEventPayload{
		Type: work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.WorkRequestEventWork{{
			Name:       "projection-work-" + sessionID,
			WorkID:     workID,
			RequestID:  "request-" + sessionID,
			WorkTypeID: "task",
			State:      &work.WorkEventState{Name: "ready", Type: "INITIAL"},
		}},
	})
	if err != nil {
		t.Fatalf("marshal projection work request(%s): %v", sessionID, err)
	}
	return string(payload)
}

func functionalProjectionStateChangePayload(t *testing.T, workID, fromState, toState string) string {
	t.Helper()
	payload, err := json.Marshal(factorydefinitions.WorkStateChangeEventPayload{
		FromPlaceID:  "task:" + fromState,
		FromState:    fromState,
		Source:       work.WorkStateChangeSourceCLI,
		ToPlaceID:    "task:" + toState,
		ToState:      toState,
		WorkID:       workID,
		WorkTypeName: "task",
	})
	if err != nil {
		t.Fatalf("marshal projection state change(%s -> %s): %v", fromState, toState, err)
	}
	return string(payload)
}
