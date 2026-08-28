package replay_contracts_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestReplayProjectionExposesSelectedHistoricalTick exercises the public
// Recordings projection capability on the root-composed application. The
// narrower RecordingsRootObserver seam is required because Process.Execute and
// the current public API do not expose ReconstructRecordingScope.
func TestReplayProjectionExposesSelectedHistoricalTick(t *testing.T) {
	var recordingsRoot recordings.Service
	process := support.BuildProcess(t, serviceedges.Edges{
		RecordingsRootObserver: func(root recordings.Service) {
			recordingsRoot = root
		},
	})
	support.CleanupProcess(t, process)
	if recordingsRoot == nil {
		t.Fatal("RecordingsRootObserver was not invoked")
	}

	firstRecording := startProjectionRecording(t, recordingsRoot, "projection-recording-a", "projection-session-a")
	secondRecording := startProjectionRecording(t, recordingsRoot, "projection-recording-b", "projection-session-b")

	recordProjectionEvent(t, recordingsRoot, firstRecording.recordingID, projectionEvent(
		"session-a-run", 0, 0, firstRecording.eventScope,
		recordings.FactoryEventTypeRunRequest,
		runRequestPayload(t),
	))
	recordProjectionEvent(t, recordingsRoot, secondRecording.recordingID, projectionEvent(
		"session-b-run", 1, 0, secondRecording.eventScope,
		recordings.FactoryEventTypeRunRequest,
		runRequestPayload(t),
	))
	recordProjectionEvent(t, recordingsRoot, firstRecording.recordingID, projectionEvent(
		"session-a-work", 2, 1, firstRecording.eventScope,
		recordings.FactoryEventTypeWorkRequest,
		workRequestPayload("session-a-work", "session-a-request"),
	))
	recordProjectionEvent(t, recordingsRoot, secondRecording.recordingID, projectionEvent(
		"session-b-work", 3, 1, secondRecording.eventScope,
		recordings.FactoryEventTypeWorkRequest,
		workRequestPayload("session-b-work", "session-b-request"),
	))
	recordProjectionEvent(t, recordingsRoot, firstRecording.recordingID, projectionEvent(
		"session-a-state", 4, 2, firstRecording.eventScope,
		recordings.FactoryEventTypeWorkStateChange,
		recordings.WorkStateChangeEventPayload{
			FromPlaceID:  "task:ready",
			FromState:    "ready",
			Source:       work.WorkStateChangeSourceCLI,
			ToPlaceID:    "task:complete",
			ToState:      "complete",
			WorkID:       "session-a-work",
			WorkTypeName: "task",
		},
	))
	finishProjectionRecording(t, recordingsRoot, firstRecording.recordingID)
	finishProjectionRecording(t, recordingsRoot, secondRecording.recordingID)
	first := openProjectionScope(t, recordingsRoot, firstRecording)

	beforeHistory := readScopeEvents(t, recordingsRoot, first.Scope, 3)
	intermediate := reconstructScopeAtTick(t, recordingsRoot, first.Scope, 1)
	assertProjectedWork(t, intermediate, "session-a-work", "ready")
	if _, found := projectedState(t, intermediate).WorkItemsByID["session-b-work"]; found {
		t.Fatal("intermediate session-a projection included work from session B")
	}
	if changes := projectedState(t, intermediate).WorkStateChangesByWorkID["session-a-work"]; len(changes) != 0 {
		t.Fatalf("intermediate session-a state changes = %#v, want none before tick 2", changes)
	}

	later := reconstructScopeAtTick(t, recordingsRoot, first.Scope, 2)
	assertProjectedWork(t, later, "session-a-work", "complete")
	if changes := projectedState(t, later).WorkStateChangesByWorkID["session-a-work"]; len(changes) != 1 {
		t.Fatalf("later session-a state changes = %#v, want one transition", changes)
	}
	if later.WorldState.Scope != first.Status.EventScope || later.Status.EventScope != first.Status.EventScope {
		t.Fatalf("later projection scope = world %#v/status %#v, want %#v", later.WorldState.Scope, later.Status.EventScope, first.Status.EventScope)
	}

	repeated := reconstructScopeAtTick(t, recordingsRoot, first.Scope, 1)
	if !reflect.DeepEqual(intermediate, repeated) {
		t.Fatalf("repeated selected-tick projection = %#v, want %#v", repeated, intermediate)
	}
	afterHistory := readScopeEvents(t, recordingsRoot, first.Scope, 3)
	if !reflect.DeepEqual(beforeHistory, afterHistory) {
		t.Fatalf("projection changed canonical session history: before %#v, after %#v", beforeHistory, afterHistory)
	}

	if _, err := recordingsRoot.ReconstructRecordingScope(context.Background(), recordings.ReconstructRecordingScopeRequest{
		Scope:        first.Scope,
		SelectedTick: -1,
	}); !errors.Is(err, recordings.ErrInvalidProjectionInput) {
		t.Fatalf("negative selected tick error = %v, want ErrInvalidProjectionInput", err)
	}
}

type projectionRecording struct {
	recordingID recordings.RecordingID
	eventScope  recordings.CanonicalEventScope
}

func startProjectionRecording(
	t *testing.T,
	service recordings.Service,
	recordingID recordings.RecordingID,
	sessionID string,
) projectionRecording {
	t.Helper()
	started, err := service.StartRecording(recordings.StartRecordingRequest{
		Enabled:     true,
		RecordingID: recordingID,
		Scope: recordings.CanonicalEventScope{
			FactorySessionID: sessionID,
		},
		Target: recordings.RecordingTargetRequest{
			Artifact: recordings.RecordingArtifactReference(filepath.Join(t.TempDir(), sessionID+".json")),
		},
	})
	if err != nil {
		t.Fatalf("StartRecording(%s): %v", sessionID, err)
	}
	if started.Status.RecordingID != recordingID || started.Status.Scope.FactorySessionID != sessionID {
		t.Fatalf("StartRecording(%s) = %#v, want an active scoped recording", sessionID, started)
	}
	return projectionRecording{recordingID: recordingID, eventScope: started.Status.Scope}
}

func recordProjectionEvent(
	t *testing.T,
	service recordings.Service,
	recordingID recordings.RecordingID,
	event recordings.CanonicalEvent,
) recordings.CanonicalEvent {
	t.Helper()
	recorded, err := service.RecordRecordingEvent(recordings.RecordRecordingEventRequest{
		RecordingID: recordingID,
		Event:       event,
	})
	if err != nil {
		t.Fatalf("RecordRecordingEvent(%s): %v", event.ID, err)
	}
	if recorded.Status.RecordingID != recordingID || recorded.Status.LastEvent == nil || recorded.Status.LastEvent.Sequence != event.Sequence {
		t.Fatalf("RecordRecordingEvent(%s) status = %#v, want accepted sequence %d", event.ID, recorded.Status, event.Sequence)
	}
	return event
}

func finishProjectionRecording(
	t *testing.T,
	service recordings.Service,
	recordingID recordings.RecordingID,
) {
	t.Helper()
	finished, err := service.FinishRecording(recordings.FinishRecordingRequest{
		RecordingID: recordingID,
		FinishedAt:  time.Date(2026, time.August, 22, 20, 1, 0, 0, time.UTC),
	})
	if err != nil || finished.Status.FinalizedAt == nil {
		t.Fatalf("FinishRecording(%s) = %#v, error %v, want finalized recording", recordingID, finished.Status, err)
	}
}

func openProjectionScope(
	t *testing.T,
	service recordings.Service,
	recording projectionRecording,
) recordings.OpenRecordingScopeResult {
	t.Helper()
	opened, err := service.OpenRecordingScope(context.Background(), recordings.OpenRecordingScopeRequest{
		RecordingID: recording.recordingID,
		Scope:       recording.eventScope,
	})
	if err != nil {
		t.Fatalf("OpenRecordingScope(%s): %v", recording.recordingID, err)
	}
	if opened.Scope.IsZero() || opened.Status.EventScope != recording.eventScope {
		t.Fatalf("OpenRecordingScope(%s) = %#v, want finalized scope", recording.recordingID, opened)
	}
	return opened
}

func reconstructScopeAtTick(
	t *testing.T,
	service recordings.Service,
	scope recordings.RecordingScopeRef,
	selectedTick int,
) recordings.ReconstructRecordingScopeResult {
	t.Helper()
	projected, err := service.ReconstructRecordingScope(context.Background(), recordings.ReconstructRecordingScopeRequest{
		Scope:        scope,
		SelectedTick: selectedTick,
	})
	if err != nil {
		t.Fatalf("ReconstructRecordingScope(tick=%d): %v", selectedTick, err)
	}
	if projected.WorldState.SchemaVersion != recordings.WorldStateViewSchemaV1 || projected.WorldState.SelectedTick != selectedTick {
		t.Fatalf("selected-tick world state = %#v, want schema and tick %d", projected.WorldState, selectedTick)
	}
	return projected
}

func projectedState(
	t *testing.T,
	projected recordings.ReconstructRecordingScopeResult,
) recordings.FactoryWorldState {
	t.Helper()
	var state recordings.FactoryWorldState
	if err := json.Unmarshal([]byte(projected.WorldState.Payload), &state); err != nil {
		t.Fatalf("decode projected world state: %v", err)
	}
	return state
}

func assertProjectedWork(
	t *testing.T,
	projected recordings.ReconstructRecordingScopeResult,
	workID string,
	wantState string,
) {
	t.Helper()
	state := projectedState(t, projected)
	item, ok := state.WorkItemsByID[workID]
	if !ok || item.State != wantState {
		t.Fatalf("projected Work = %#v, want %s in state %q", state.WorkItemsByID, workID, wantState)
	}
}

func readScopeEvents(
	t *testing.T,
	service recordings.Service,
	scope recordings.RecordingScopeRef,
	wantCount int,
) []recordings.CanonicalEvent {
	t.Helper()
	subscribed, err := service.SubscribeRecordingScope(context.Background(), recordings.SubscribeRecordingScopeRequest{
		Scope: scope,
	})
	if err != nil {
		t.Fatalf("SubscribeRecordingScope: %v", err)
	}
	observed := make([]recordings.CanonicalEvent, 0, wantCount)
	for index := 0; index < wantCount; index++ {
		outcome := subscribed.Subscription.Next(context.Background())
		if outcome.Kind != recordings.SubscriptionEvent {
			t.Fatalf("scope subscription event[%d] = %#v, want event", index, outcome)
		}
		observed = append(observed, outcome.Event)
	}
	return observed
}

func projectionEvent(
	id string,
	sequence recordings.CanonicalEventSequence,
	tick int,
	scope recordings.CanonicalEventScope,
	kind factorydefinitions.FactoryEventType,
	payload any,
) recordings.CanonicalEvent {
	encoded, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}
	return recordings.CanonicalEvent{
		ID:          recordings.CanonicalEventID(id),
		Sequence:    sequence,
		FactoryTick: tick,
		Scope:       scope,
		Cursor: recordings.CanonicalEventCursor{
			StreamGenerationID: "functional-recordings",
			Sequence:           sequence,
		},
		RecordedAt: time.Date(2026, time.August, 22, 20, 0, 0, 0, time.UTC).Add(time.Duration(sequence) * time.Second),
		Kind:       recordings.CanonicalEventKind(kind),
		Payload:    string(encoded),
	}
}

func runRequestPayload(t *testing.T) recordings.RunRequestEventPayload {
	t.Helper()
	snapshot, err := factorydefinitions.NewFactorySnapshot(replayContractFactoryConfig())
	if err != nil {
		t.Fatalf("NewFactorySnapshot: %v", err)
	}
	return recordings.RunRequestEventPayload{
		Factory:    snapshot,
		RecordedAt: time.Date(2026, time.August, 22, 20, 0, 0, 0, time.UTC),
	}
}

func workRequestPayload(workID, requestID string) work.WorkRequestEventPayload {
	return work.WorkRequestEventPayload{
		Source: "functional-projection",
		Type:   work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.WorkRequestEventWork{{
			Name:       workID,
			WorkID:     workID,
			RequestID:  requestID,
			WorkTypeID: "task",
			State:      &work.WorkEventState{Name: "ready", Type: "INITIAL"},
			TraceID:    requestID + "-trace",
		}},
	}
}
