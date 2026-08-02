package wire_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingswire "github.com/portpowered/infinite-you/pkg/services/recordings/wire"
)

// Fold-behavior preservation tests construct Recordings exclusively through
// recordings/wire and exercise append, subscription order, replay, projection
// queries, and portable artifact export through the published Service root after
// the internal composed-root relocation.

type behavioralLedger struct {
	events []factorydefinitions.FactoryEvent
}

func (ledger *behavioralLedger) CanonicalEvents() []factorydefinitions.FactoryEvent {
	out := make([]factorydefinitions.FactoryEvent, len(ledger.events))
	copy(out, ledger.events)
	return out
}

func (ledger *behavioralLedger) Subscribe(
	_ context.Context,
	_ *factorydefinitions.FactoryEventReconnectCursor,
	_ factorydefinitions.FactoryEventReconnectScope,
) (factorydefinitions.FactoryEventStream, error) {
	return factorydefinitions.FactoryEventStream{
		StreamGenerationID: ledger.StreamGenerationID(),
		History:            ledger.CanonicalEvents(),
	}, nil
}

func (ledger *behavioralLedger) StreamGenerationID() string { return "wire-fold-gen" }

func (ledger *behavioralLedger) AddEventRecorder(func(factorydefinitions.FactoryEvent)) {}

func (ledger *behavioralLedger) AddEventTypeRecorder(func(factorydefinitions.FactoryEventType)) {}

func (ledger *behavioralLedger) AppendRecordedEvent(event factorydefinitions.FactoryEvent) {
	event.Context.Sequence = len(ledger.events)
	ledger.events = append(ledger.events, event)
}

func newWireFoldService(t *testing.T, ledger recordings.Ledger) recordings.Service {
	t.Helper()
	service, err := recordingswire.NewServiceWithProjectionAndEffects(
		ledger,
		recordingswire.NewProjectionService(),
		nil,
		func(path string, data []byte) error {
			return os.WriteFile(path, data, 0o644)
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
		t.Fatalf("NewService() = %v", err)
	}
	var root recordings.Service = service
	if root == nil {
		t.Fatal("constructed service is not assignable to recordings.Service")
	}
	return root
}

func wireFoldRunRequestEvent(
	id string,
	sequence recordings.CanonicalEventSequence,
	scope recordings.CanonicalEventScope,
	recordedAt time.Time,
	generationID string,
) (recordings.CanonicalEvent, error) {
	snapshot, err := factorydefinitions.NewFactorySnapshot(map[string]any{
		"id": "wire-fold-factory",
		"workTypes": []map[string]any{
			{
				"name": "task",
				"states": []map[string]string{
					{"name": "ready", "type": "PROCESSING"},
				},
			},
		},
	})
	if err != nil {
		return recordings.CanonicalEvent{}, fmt.Errorf("factory snapshot: %w", err)
	}
	payload, err := json.Marshal(factorydefinitions.RunRequestEventPayload{
		Factory:    snapshot,
		RecordedAt: recordedAt,
	})
	if err != nil {
		return recordings.CanonicalEvent{}, fmt.Errorf("run request payload: %w", err)
	}
	return recordings.CanonicalEvent{
		ID:          recordings.CanonicalEventID(id),
		Kind:        recordings.CanonicalEventKind(factorydefinitions.FactoryEventTypeRunRequest),
		Sequence:    sequence,
		Scope:       scope,
		FactoryTick: 0,
		Cursor: recordings.CanonicalEventCursor{
			StreamGenerationID: generationID,
			Sequence:           sequence,
		},
		RecordedAt: recordedAt,
		Payload:    string(payload),
	}, nil
}

func wireFoldWorkRequestEvent(
	id string,
	sequence recordings.CanonicalEventSequence,
	scope recordings.CanonicalEventScope,
	recordedAt time.Time,
	generationID string,
) recordings.CanonicalEvent {
	return recordings.CanonicalEvent{
		ID:          recordings.CanonicalEventID(id),
		Kind:        recordings.CanonicalEventKind(factorydefinitions.FactoryEventTypeWorkRequest),
		Sequence:    sequence,
		Scope:       scope,
		FactoryTick: 1,
		Cursor: recordings.CanonicalEventCursor{
			StreamGenerationID: generationID,
			Sequence:           sequence,
		},
		RecordedAt: recordedAt,
		Payload:    `{"type":"WORK_REQUEST"}`,
	}
}

func TestWireFoldPreservesAppendAndProjectionQueryThroughPublishedRoot(t *testing.T) {
	t.Parallel()

	ledger := &behavioralLedger{}
	root := newWireFoldService(t, ledger)

	scope := recordings.CanonicalEventScope{FactorySessionID: "wire-fold-session"}
	event := recordings.CanonicalEvent{
		ID:          recordings.CanonicalEventID("wire-fold-event"),
		Sequence:    0,
		FactoryTick: 1,
		Scope:       scope,
		Cursor: recordings.CanonicalEventCursor{
			StreamGenerationID: ledger.StreamGenerationID(),
			Sequence:           0,
		},
		Kind:       recordings.CanonicalEventKind(factorydefinitions.FactoryEventTypeRunResponse),
		Payload:    `{}`,
		RecordedAt: time.Unix(1_700_000_000, 0).UTC(),
	}

	accepted, err := root.Append(recordings.AppendRecordedEventRequest{Event: event})
	if err != nil {
		t.Fatalf("Append() = %v", err)
	}
	if accepted.Event.ID != event.ID {
		t.Fatalf("Append() event ID = %q, want %q", accepted.Event.ID, event.ID)
	}

	reconstructed, err := root.ReconstructWorldState(recordings.ReconstructWorldStateRequest{
		Scope:        scope,
		Events:       []recordings.CanonicalEvent{accepted.Event},
		SelectedTick: 4,
	})
	if err != nil {
		t.Fatalf("ReconstructWorldState() = %v", err)
	}

	dashboard, err := root.QuerySimpleDashboard(recordings.SimpleDashboardQueryRequest{
		WorldState: reconstructed.WorldState,
	})
	if err != nil {
		t.Fatalf("QuerySimpleDashboard() = %v", err)
	}
	if reconstructed.WorldState.SchemaVersion == "" {
		t.Fatalf("ReconstructWorldState() returned empty schema version: %#v", reconstructed.WorldState)
	}
	if reconstructed.WorldState.Scope != scope {
		t.Fatalf("ReconstructWorldState() scope = %#v, want %#v", reconstructed.WorldState.Scope, scope)
	}
	if dashboard.Data.ActiveExecutionsByDispatchID == nil {
		t.Fatalf("QuerySimpleDashboard() returned nil active executions map: %#v", dashboard.Data)
	}
}

func TestWireFoldPreservesSubscriptionCursorOrderThroughPublishedRoot(t *testing.T) {
	t.Parallel()

	const generationID = "wire-fold-sub-gen"
	now := time.Unix(1_700_000_000, 0).UTC()
	ledger := recordingswire.NewRuntimeLedger(nil, func() time.Time { return now }, generationID, nil)
	root := newWireFoldService(t, ledger)
	scope := recordings.CanonicalEventScope{FactorySessionID: "wire-fold-sub-session"}

	const eventCount = 3
	for sequence := 0; sequence < eventCount; sequence++ {
		appendWireFoldEvent(t, root, scope, sequence, now.Add(time.Duration(sequence)*time.Second), generationID)
	}

	subscribed, err := root.SubscribeFrom(context.Background(), recordings.SubscribeRequest{})
	if err != nil {
		t.Fatalf("SubscribeFrom() = %v", err)
	}
	for sequence := 0; sequence < eventCount; sequence++ {
		outcome := subscribed.Subscription.Next(context.Background())
		if outcome.Kind != recordings.SubscriptionEvent {
			t.Fatalf("subscription outcome at %d = %#v, want event", sequence, outcome)
		}
		if outcome.Event.Sequence != recordings.CanonicalEventSequence(sequence) {
			t.Fatalf("subscription sequence at %d = %d, want %d", sequence, outcome.Event.Sequence, sequence)
		}
		if outcome.Event.Cursor.StreamGenerationID != generationID {
			t.Fatalf("subscription cursor generation = %q, want %q", outcome.Event.Cursor.StreamGenerationID, generationID)
		}
	}

	reconnectCursor := recordings.CanonicalEventCursor{
		StreamGenerationID: generationID,
		Sequence:           0,
	}
	reconnected, err := root.SubscribeFrom(context.Background(), recordings.SubscribeRequest{
		Cursor: &reconnectCursor,
		Scope:  scope,
	})
	if err != nil {
		t.Fatalf("SubscribeFrom() reconnect cursor = %v", err)
	}
	for sequence := 1; sequence < eventCount; sequence++ {
		outcome := reconnected.Subscription.Next(context.Background())
		if outcome.Kind != recordings.SubscriptionEvent {
			t.Fatalf("reconnect outcome at %d = %#v, want event", sequence, outcome)
		}
		if outcome.Event.Sequence != recordings.CanonicalEventSequence(sequence) {
			t.Fatalf("reconnect sequence at %d = %d, want %d", sequence, outcome.Event.Sequence, sequence)
		}
	}
}

func TestWireFoldPreservesReplayLoadValidationAndProjectionThroughPublishedRoot(t *testing.T) {
	t.Parallel()

	root := newWireFoldService(t, &behavioralLedger{})
	recording := finalizedWireFoldReplayRecording(t, root)
	assertWireFoldReplayLoadFailures(t, root, recording)
}

func TestWireFoldPreservesPortableArtifactExportThroughPublishedRoot(t *testing.T) {
	t.Parallel()

	root := newWireFoldService(t, &behavioralLedger{})
	scope := recordings.CanonicalEventScope{FactorySessionID: "wire-fold-export-session"}
	artifactPath := filepath.Join(t.TempDir(), "wire-fold-export.json")
	bound, err := root.BindRecording(recordings.BindRecordingRequest{
		RecordingID: "recording-wire-fold-export",
		Artifact:    recordings.RecordingArtifactReference(artifactPath),
		Scope:       scope,
	})
	if err != nil {
		t.Fatalf("BindRecording() = %v", err)
	}
	runRequest, err := wireFoldRunRequestEvent(
		"wire-fold-export-run-request",
		0,
		scope,
		time.Unix(1_700_000_000, 0).UTC(),
		"wire-fold-export-gen",
	)
	if err != nil {
		t.Fatalf("wireFoldRunRequestEvent() = %v", err)
	}
	for index, event := range []recordings.CanonicalEvent{
		runRequest,
		wireFoldWorkRequestEvent("wire-fold-export-event", 1, scope, time.Unix(1_700_000_001, 0).UTC(), "wire-fold-export-gen"),
	} {
		if _, err := root.RecordRecordingEvent(recordings.RecordRecordingEventRequest{
			RecordingID: bound.Status.RecordingID,
			Event:       event,
		}); err != nil {
			t.Fatalf("RecordRecordingEvent[%d]() = %v", index, err)
		}
	}
	if _, err := root.FinishRecording(recordings.FinishRecordingRequest{
		RecordingID: bound.Status.RecordingID,
		FinishedAt:  time.Unix(1_700_000_002, 0).UTC(),
	}); err != nil {
		t.Fatalf("FinishRecording() = %v", err)
	}
	built, err := root.BuildPortableArtifact(recordings.BuildPortableArtifactRequest{
		RecordingID: bound.Status.RecordingID,
	})
	if err != nil {
		t.Fatalf("BuildPortableArtifact() = %v", err)
	}
	encoded, err := root.EncodePortableArtifact(recordings.EncodePortableArtifactRequest{
		Artifact: built.Artifact,
	})
	if err != nil || len(encoded.Payload) == 0 {
		t.Fatalf("EncodePortableArtifact() = (%d bytes, %v)", len(encoded.Payload), err)
	}
	decoded, err := root.DecodePortableArtifact(recordings.DecodePortableArtifactRequest{
		Payload: encoded.Payload,
	})
	if err != nil {
		t.Fatalf("DecodePortableArtifact() = %v", err)
	}
	if _, err := root.ValidatePortableArtifact(recordings.ValidatePortableArtifactRequest{
		Artifact: decoded.Artifact,
	}); err != nil {
		t.Fatalf("ValidatePortableArtifact() = %v", err)
	}
	summarized, err := root.SummarizePortableArtifact(recordings.SummarizePortableArtifactRequest{
		Artifact: decoded.Artifact,
	})
	if err != nil || summarized.Summary.RecordingID != bound.Status.RecordingID {
		t.Fatalf("SummarizePortableArtifact() = (%#v, %v)", summarized, err)
	}
	if _, err := root.DecodePortableArtifact(recordings.DecodePortableArtifactRequest{
		Payload: []byte(`{`),
	}); !errors.Is(err, recordings.ErrInvalidPortableArtifact) {
		t.Fatalf("DecodePortableArtifact() malformed = %v, want ErrInvalidPortableArtifact", err)
	}
}

func appendWireFoldEvent(
	t *testing.T,
	root recordings.Service,
	scope recordings.CanonicalEventScope,
	sequence int,
	recordedAt time.Time,
	generationID string,
) {
	t.Helper()
	result, err := root.Append(recordings.AppendRecordedEventRequest{
		Event: wireFoldWorkRequestEvent(
			fmt.Sprintf("wire-fold-sub-%d", sequence),
			recordings.CanonicalEventSequence(sequence),
			scope,
			recordedAt,
			generationID,
		),
	})
	if err != nil {
		t.Fatalf("Append() sequence %d = %v", sequence, err)
	}
	if result.Event.Sequence != recordings.CanonicalEventSequence(sequence) {
		t.Fatalf("Append() sequence %d = %d, want %d", sequence, result.Event.Sequence, sequence)
	}
}

func finalizedWireFoldReplayRecording(t *testing.T, root recordings.Service) recordings.ReplayRecordingFacts {
	t.Helper()

	scope := recordings.CanonicalEventScope{FactorySessionID: "wire-fold-replay-session"}
	artifactPath := filepath.Join(t.TempDir(), "wire-fold-replay.json")
	bound, err := root.BindRecording(recordings.BindRecordingRequest{
		Artifact: recordings.RecordingArtifactReference(artifactPath),
		Scope:    scope,
	})
	if err != nil {
		t.Fatalf("BindRecording() = %v", err)
	}
	runRequest, err := wireFoldRunRequestEvent(
		"wire-fold-replay-run-request",
		0,
		scope,
		time.Unix(1_700_000_000, 0).UTC(),
		"wire-fold-replay-gen",
	)
	if err != nil {
		t.Fatalf("wireFoldRunRequestEvent() = %v", err)
	}
	for index, event := range []recordings.CanonicalEvent{
		runRequest,
		wireFoldWorkRequestEvent("wire-fold-replay-1", 1, scope, time.Unix(1_700_000_001, 0).UTC(), "wire-fold-replay-gen"),
		wireFoldWorkRequestEvent("wire-fold-replay-2", 2, scope, time.Unix(1_700_000_002, 0).UTC(), "wire-fold-replay-gen"),
	} {
		if _, err := root.RecordRecordingEvent(recordings.RecordRecordingEventRequest{
			RecordingID: bound.Status.RecordingID,
			Event:       event,
		}); err != nil {
			t.Fatalf("RecordRecordingEvent[%d]() = %v", index, err)
		}
	}
	if _, err := root.FinishRecording(recordings.FinishRecordingRequest{
		RecordingID: bound.Status.RecordingID,
		FinishedAt:  time.Unix(1_700_000_300, 0).UTC(),
	}); err != nil {
		t.Fatalf("FinishRecording() = %v", err)
	}
	loaded, err := root.LoadReplayRecording(recordings.LoadReplayRecordingRequest{
		RecordingID: bound.Status.RecordingID,
	})
	if err != nil {
		t.Fatalf("LoadReplayRecording() = %v", err)
	}
	expectedThrough := loaded.Recording.Events[len(loaded.Recording.Events)-1].Cursor
	planned, err := root.CreateReplayPlan(recordings.CreateReplayPlanRequest{
		SchemaVersion:   recordings.ReplayPlanSchemaV1,
		Timing:          recordings.ReplayTimingOrderOnly,
		Recording:       loaded.Recording,
		ExpectedThrough: &expectedThrough,
		SelectedTick:    7,
	})
	if err != nil {
		t.Fatalf("CreateReplayPlan() = %v", err)
	}
	var observed recordings.ObserveReplayResult
	for step := 0; step < len(loaded.Recording.Events); step++ {
		observed, err = root.ObserveReplay(recordings.ObserveReplayRequest{
			Plan: planned.Plan.Handle,
		})
		if err != nil {
			t.Fatalf("ObserveReplay() step %d = %v", step, err)
		}
	}
	if observed.Observation.Kind != recordings.ReplayCompleted {
		t.Fatalf("ObserveReplay() completion = %#v, want COMPLETED", observed.Observation)
	}
	live, err := root.ReconstructWorldState(recordings.ReconstructWorldStateRequest{
		Scope:        loaded.Recording.Scope,
		Events:       loaded.Recording.Events,
		SelectedTick: 7,
	})
	if err != nil {
		t.Fatalf("ReconstructWorldState() = %v", err)
	}
	if observed.Observation.WorldState != live.WorldState {
		t.Fatalf("replay world = %#v, live world = %#v", observed.Observation.WorldState, live.WorldState)
	}
	dashboard, err := root.QuerySimpleDashboard(recordings.SimpleDashboardQueryRequest{
		WorldState: live.WorldState,
	})
	if err != nil {
		t.Fatalf("QuerySimpleDashboard() = %v", err)
	}
	if dashboard.Data.ActiveExecutionsByDispatchID == nil {
		t.Fatalf("QuerySimpleDashboard() returned nil active executions map: %#v", dashboard.Data)
	}
	return loaded.Recording
}

func assertWireFoldReplayLoadFailures(
	t *testing.T,
	root recordings.Service,
	recording recordings.ReplayRecordingFacts,
) {
	t.Helper()
	if _, err := root.LoadReplayRecording(recordings.LoadReplayRecordingRequest{
		RecordingID: "missing-wire-fold-replay",
	}); !errors.Is(err, recordings.ErrReplayRecordingNotFound) {
		t.Fatalf("LoadReplayRecording() missing = %v, want ErrReplayRecordingNotFound", err)
	}
	if _, err := root.ObserveReplay(recordings.ObserveReplayRequest{
		Plan: "missing-wire-fold-replay-plan",
	}); !errors.Is(err, recordings.ErrReplayPlanNotFound) {
		t.Fatalf("ObserveReplay() missing plan = %v, want ErrReplayPlanNotFound", err)
	}
	corrupt := recording
	corrupt.Events = append([]recordings.CanonicalEvent(nil), recording.Events...)
	corrupt.Events[1].Sequence = 9
	if _, err := root.CreateReplayPlan(recordings.CreateReplayPlanRequest{
		SchemaVersion: recordings.ReplayPlanSchemaV1,
		Timing:        recordings.ReplayTimingOrderOnly,
		Recording:     corrupt,
	}); !errors.Is(err, recordings.ErrCorruptReplayInput) {
		t.Fatalf("CreateReplayPlan() corrupt = %v, want ErrCorruptReplayInput", err)
	}
}
