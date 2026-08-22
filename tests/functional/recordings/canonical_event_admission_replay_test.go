package recordings_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingswire "github.com/portpowered/infinite-you/pkg/services/recordings/wire"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

// TestCanonicalRecordingScopeAdmissionPreservesFactsThroughReplay exercises
// the published Recordings root through its public wire. The scope operation
// owns both canonical ledger admission and recording association, so replay
// can be checked against the exact detached facts returned by admission.
func TestCanonicalRecordingScopeAdmissionPreservesFactsThroughReplay(t *testing.T) {
	t.Parallel()

	const generationID = "functional-recordings-admission-generation"
	recordedAt := time.Date(2026, time.August, 22, 10, 0, 0, 0, time.UTC)
	ledger := recordingswire.NewRuntimeLedger(
		nil,
		func() time.Time { return recordedAt },
		generationID,
		nil,
	)
	service := newFunctionalRecordingsService(t, ledger)
	scope := recordings.CanonicalEventScope{FactorySessionID: "functional-recordings-session"}
	invalid := functionalNonEmittableEvent(scope, generationID, recordedAt)
	assertFunctionalAppendRejectsInvalidKind(t, service, ledger, invalid)

	ctx := context.Background()
	started := beginFunctionalRecordingScope(t, service, scope)
	accepted := appendFunctionalRecordingEvents(t, service, started.Scope, scope, generationID, recordedAt)
	if got := len(ledger.CanonicalEvents()); got != len(accepted) {
		t.Fatalf("ledger events after accepted scope append = %d, want %d", got, len(accepted))
	}
	finalizeFunctionalRecordingScope(t, service, ctx, started.Scope, recordedAt.Add(3*time.Second), len(accepted))
	loaded := loadFunctionalReplay(t, service, ctx, started.Scope)
	assertFunctionalReplayFacts(t, loaded.Recording, accepted, scope, invalid)
	assertFunctionalReplayProgress(t, service, ctx, started.Scope, len(accepted), accepted)
}

func functionalNonEmittableEvent(
	scope recordings.CanonicalEventScope,
	generationID string,
	recordedAt time.Time,
) recordings.CanonicalEvent {
	return functionalCanonicalEvent(
		"functional-recordings-invalid",
		0,
		scope,
		generationID,
		recordings.CanonicalEventKind(factorydefinitions.FactoryEventTypeJavaScriptCheckpointRef),
		`{}`,
		recordedAt,
	)
}

func assertFunctionalAppendRejectsInvalidKind(
	t *testing.T,
	service recordings.Service,
	ledger recordings.RuntimeEventLedger,
	invalid recordings.CanonicalEvent,
) {
	t.Helper()
	_, err := service.Append(recordings.AppendRecordedEventRequest{Event: invalid})
	if !errors.Is(err, recordings.ErrInvalidAppendEvent) {
		t.Fatalf("Append(non-emittable kind) error = %v, want ErrInvalidAppendEvent", err)
	}
	if got := len(ledger.CanonicalEvents()); got != 0 {
		t.Fatalf("ledger events after rejected append = %d, want 0", got)
	}
}

func beginFunctionalRecordingScope(
	t *testing.T,
	service recordings.Service,
	scope recordings.CanonicalEventScope,
) recordings.BeginRecordingScopeResult {
	t.Helper()
	started, err := service.BeginRecordingScope(context.Background(), recordings.BeginRecordingScopeRequest{
		Enabled:     true,
		RecordingID: "functional-recordings-admission",
		Scope:       scope,
		Target: recordings.RecordingTargetRequest{
			Artifact: recordings.RecordingArtifactReference(filepath.Join(t.TempDir(), "recording.json")),
		},
	})
	if err != nil {
		t.Fatalf("BeginRecordingScope() error = %v", err)
	}
	if started.Scope.IsZero() || started.Status.State != recordings.RecordingActive {
		t.Fatalf("BeginRecordingScope() = %#v, want active opaque scope", started)
	}
	return started
}

func appendFunctionalRecordingEvents(
	t *testing.T,
	service recordings.Service,
	scopeRef recordings.RecordingScopeRef,
	scope recordings.CanonicalEventScope,
	generationID string,
	recordedAt time.Time,
) []recordings.CanonicalEvent {
	t.Helper()
	events := functionalAdmissionEvents(t, scope, generationID, recordedAt)
	accepted := make([]recordings.CanonicalEvent, 0, len(events))
	for index, event := range events {
		result, err := service.AppendRecordingScopeEvent(context.Background(), recordings.AppendRecordingScopeEventRequest{
			Scope: scopeRef,
			Event: event,
		})
		if err != nil {
			t.Fatalf("AppendRecordingScopeEvent[%d] error = %v", index, err)
		}
		accepted = append(accepted, result.Event)
	}
	return accepted
}

func functionalAdmissionEvents(
	t *testing.T,
	scope recordings.CanonicalEventScope,
	generationID string,
	recordedAt time.Time,
) []recordings.CanonicalEvent {
	t.Helper()
	return []recordings.CanonicalEvent{
		functionalCanonicalEvent("functional-recordings-run-request", 0, scope, generationID,
			recordings.CanonicalEventKind(factorydefinitions.FactoryEventTypeRunRequest),
			functionalRunRequestPayload(t, recordedAt), recordedAt),
		functionalCanonicalEvent("functional-recordings-work-request", 1, scope, generationID,
			recordings.CanonicalEventKind(factorydefinitions.FactoryEventTypeWorkRequest),
			functionalWorkRequestPayload(t), recordedAt.Add(time.Second)),
		functionalCanonicalEvent("functional-recordings-run-response", 2, scope, generationID,
			recordings.CanonicalEventKind(factorydefinitions.FactoryEventTypeRunResponse),
			`{}`, recordedAt.Add(2*time.Second)),
	}
}

func finalizeFunctionalRecordingScope(
	t *testing.T,
	service recordings.Service,
	ctx context.Context,
	scope recordings.RecordingScopeRef,
	finishedAt time.Time,
	wantEvents int,
) {
	t.Helper()
	finalized, err := service.FinalizeRecordingScope(ctx, recordings.FinalizeRecordingScopeRequest{
		Scope: scope, FinishedAt: finishedAt,
	})
	if err != nil {
		t.Fatalf("FinalizeRecordingScope() error = %v", err)
	}
	if finalized.Status.State != recordings.RecordingFinalized || finalized.Status.AcceptedEvents != wantEvents {
		t.Fatalf("FinalizeRecordingScope() = %#v, want finalized recording with %d events", finalized, wantEvents)
	}
}

func loadFunctionalReplay(
	t *testing.T,
	service recordings.Service,
	ctx context.Context,
	scope recordings.RecordingScopeRef,
) recordings.LoadReplayRecordingScopeResult {
	t.Helper()
	loaded, err := service.LoadReplayRecordingScope(ctx, recordings.LoadReplayRecordingScopeRequest{Scope: scope})
	if err != nil {
		t.Fatalf("LoadReplayRecordingScope() error = %v", err)
	}
	return loaded
}

func assertFunctionalReplayFacts(
	t *testing.T,
	recording recordings.ReplayRecordingFacts,
	accepted []recordings.CanonicalEvent,
	scope recordings.CanonicalEventScope,
	invalid recordings.CanonicalEvent,
) {
	t.Helper()
	if recording.Scope != scope {
		t.Fatalf("replay scope = %#v, want %#v", recording.Scope, scope)
	}
	if !reflect.DeepEqual(recording.Events, accepted) {
		t.Fatalf("replayed events = %#v, want admitted events %#v", recording.Events, accepted)
	}
	for _, event := range recording.Events {
		if event.ID == invalid.ID || event.Kind == invalid.Kind {
			t.Fatalf("replay contains rejected event: %#v", event)
		}
	}
}

func assertFunctionalReplayProgress(
	t *testing.T,
	service recordings.Service,
	ctx context.Context,
	scope recordings.RecordingScopeRef,
	wantEvents int,
	accepted []recordings.CanonicalEvent,
) {
	t.Helper()
	planned, err := service.CreateReplayPlanScope(ctx, recordings.CreateReplayPlanScopeRequest{
		Scope: scope, SchemaVersion: recordings.ReplayPlanSchemaV1,
		Timing: recordings.ReplayTimingOrderOnly, SelectedTick: 2,
	})
	if err != nil {
		t.Fatalf("CreateReplayPlanScope() error = %v", err)
	}
	var observed recordings.ObserveReplayScopeResult
	for index := 0; index < wantEvents; index++ {
		observed, err = service.ObserveReplayScope(ctx, recordings.ObserveReplayScopeRequest{
			Scope: scope, Plan: planned.Plan.Handle,
		})
		if err != nil {
			t.Fatalf("ObserveReplayScope[%d] error = %v", index, err)
		}
		if observed.Observation.ProcessedEvents != index+1 {
			t.Fatalf("ObserveReplayScope[%d] processed events = %d, want %d", index, observed.Observation.ProcessedEvents, index+1)
		}
	}
	if observed.Observation.Kind != recordings.ReplayCompleted ||
		observed.Observation.TotalEvents != wantEvents || observed.Observation.Through == nil ||
		*observed.Observation.Through != accepted[wantEvents-1].Cursor {
		t.Fatalf("replay completion = %#v, want completed ordered prefix", observed.Observation)
	}
}

func newFunctionalRecordingsService(
	t *testing.T,
	ledger recordings.RuntimeEventLedger,
) recordings.Service {
	t.Helper()

	service, err := recordingswire.NewServiceWithProjectionAndEffects(
		ledger,
		recordingswire.NewProjectionService(),
		nil,
		func(path string, payload []byte) error {
			return os.WriteFile(path, payload, 0o600)
		},
		os.MkdirAll,
		func(dir, pattern string) (recordings.RecordingTemporaryFile, error) {
			return os.CreateTemp(dir, pattern)
		},
		os.Remove,
		os.Rename,
		os.ReadFile,
	)
	if err != nil {
		t.Fatalf("NewServiceWithProjectionAndEffects() error = %v", err)
	}
	return service
}

func functionalCanonicalEvent(
	id string,
	sequence recordings.CanonicalEventSequence,
	scope recordings.CanonicalEventScope,
	generationID string,
	kind recordings.CanonicalEventKind,
	payload string,
	recordedAt time.Time,
) recordings.CanonicalEvent {
	return recordings.CanonicalEvent{
		ID:          recordings.CanonicalEventID(id),
		Sequence:    sequence,
		FactoryTick: int(sequence),
		Scope:       scope,
		Cursor: recordings.CanonicalEventCursor{
			StreamGenerationID: generationID,
			Sequence:           sequence,
		},
		RecordedAt: recordedAt,
		Kind:       kind,
		Payload:    payload,
	}
}

func functionalRunRequestPayload(t *testing.T, recordedAt time.Time) string {
	t.Helper()

	snapshot, err := factorydefinitions.NewFactorySnapshot(map[string]any{
		"id": "functional-recordings-admission-factory",
		"workTypes": []map[string]any{{
			"name":   "task",
			"states": []map[string]string{{"name": "ready", "type": "PROCESSING"}},
		}},
	})
	if err != nil {
		t.Fatalf("NewFactorySnapshot() error = %v", err)
	}
	payload, err := json.Marshal(factorydefinitions.RunRequestEventPayload{
		Factory:    snapshot,
		RecordedAt: recordedAt,
	})
	if err != nil {
		t.Fatalf("marshal run request payload: %v", err)
	}
	return string(payload)
}

func functionalWorkRequestPayload(t *testing.T) string {
	t.Helper()

	payload, err := json.Marshal(work.WorkRequestEventPayload{
		Type: work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.WorkRequestEventWork{{
			Name:       "admitted-work",
			WorkID:     "functional-work-1",
			RequestID:  "functional-request-1",
			WorkTypeID: "task",
		}},
	})
	if err != nil {
		t.Fatalf("marshal work request payload: %v", err)
	}
	return string(payload)
}
