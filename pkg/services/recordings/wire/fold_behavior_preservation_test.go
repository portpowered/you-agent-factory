package wire_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/recordings/internal/canonical"
	recordinglifecycle "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/recording_lifecycle"
	recordingswire "github.com/portpowered/infinite-you/pkg/services/recordings/wire"
)

// These preservation witnesses exercise native owners with controlled collaborators.
// Composed stop/export/replay journeys live in the public functional lane.

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

func (ledger *behavioralLedger) StreamGenerationID() string { return "wire-fold-sub-gen" }

func (ledger *behavioralLedger) AddEventRecorder(func(factorydefinitions.FactoryEvent)) {}

func (ledger *behavioralLedger) AddEventTypeRecorder(func(factorydefinitions.FactoryEventType)) {}

func (ledger *behavioralLedger) AppendRecordedEvent(event factorydefinitions.FactoryEvent) {
	event.Context.Sequence = len(ledger.events)
	ledger.events = append(ledger.events, event)
}

type foldSnapshotSource struct {
	recordingswire.RecordingLifecycleOwner
	snapshot recordinglifecycle.Snapshot
}

func (source foldSnapshotSource) Snapshot(id recordings.RecordingID) (recordinglifecycle.Snapshot, error) {
	if id != source.snapshot.Status.RecordingID {
		return recordinglifecycle.Snapshot{}, recordings.ErrMissingRecordingTarget
	}
	return source.snapshot, nil
}
func foldFinalizedSnapshot(t *testing.T, id recordings.RecordingID, count int) foldSnapshotSource {
	t.Helper()
	scope := recordings.CanonicalEventScope{FactorySessionID: "wire-fold-session"}
	now := time.Unix(1_700_000_000, 0).UTC()
	first, err := wireFoldRunRequestEvent("wire-fold-run", 0, scope, now, "wire-fold-gen")
	if err != nil {
		t.Fatal(err)
	}
	events := []recordings.CanonicalEvent{first}
	for i := 1; i < count; i++ {
		events = append(events, wireFoldWorkRequestEvent(fmt.Sprintf("wire-fold-%d", i), recordings.CanonicalEventSequence(i), scope, now.Add(time.Duration(i)*time.Second), "wire-fold-gen"))
	}
	finished := now.Add(300 * time.Second)
	return foldSnapshotSource{snapshot: recordinglifecycle.Snapshot{
		Status: recordings.RecordingStatusFacts{RecordingID: id, Scope: scope, Artifact: "artifact://wire-fold", State: recordings.RecordingFinalized, FinalizedAt: &finished}, Events: events,
	}}
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

func TestCanonicalAppendPreservesEventIdentity(t *testing.T) {
	t.Parallel()

	ledger := &behavioralLedger{}
	root := recordingswire.NewCanonicalLedgerOwner(ledger)

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

}

func TestProjectionOwnerPreservesWorldScopeAndDashboard(t *testing.T) {
	t.Parallel()
	scope := recordings.CanonicalEventScope{FactorySessionID: "wire-fold-session"}
	projection := recordingswire.NewProjectionService()
	event := recordings.CanonicalEvent{ID: "wire-fold-event", Sequence: 0, FactoryTick: 1, Scope: scope,
		Cursor:  recordings.CanonicalEventCursor{StreamGenerationID: "wire-fold-gen", Sequence: 0},
		Kind:    recordings.CanonicalEventKind(factorydefinitions.FactoryEventTypeRunResponse),
		Payload: `{}`, RecordedAt: time.Unix(1_700_000_000, 0).UTC()}
	reconstructed, err := canonical.ReconstructWorldState(projection, recordings.ReconstructWorldStateRequest{
		Scope: scope, Events: []recordings.CanonicalEvent{event}, SelectedTick: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	if reconstructed.WorldState.SchemaVersion == "" || reconstructed.WorldState.Scope != scope {
		t.Fatalf("reconstructed world = %#v, want schema and selected scope", reconstructed.WorldState)
	}
	var state recordings.FactoryWorldState
	if err := json.Unmarshal([]byte(reconstructed.WorldState.Payload), &state); err != nil {
		t.Fatal(err)
	}
	dashboard := projection.SimpleDashboardRenderData(state)
	if dashboard.ActiveExecutionsByDispatchID == nil {
		t.Fatal("dashboard has nil active executions map")
	}
}

func TestCanonicalSubscriptionPreservesCursorOrder(t *testing.T) {
	t.Parallel()

	const generationID = "wire-fold-sub-gen"
	now := time.Unix(1_700_000_000, 0).UTC()
	ledger := &behavioralLedger{}
	root := recordingswire.NewCanonicalLedgerOwner(ledger)
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

func TestWireFoldReconstructsCanonicalDispatchReplayWithWorkLineage(t *testing.T) {
	t.Parallel()

	const workID = "wire-fold-dispatch-work"
	const dispatchID = "wire-fold-dispatch"
	now := time.Unix(1_700_000_100, 0).UTC()
	workIDs := []string{workID}
	events := []factorydefinitions.FactoryEvent{
		{Id: "work-request", Type: factorydefinitions.FactoryEventTypeWorkRequest,
			Context: factorydefinitions.FactoryEventContext{Sequence: 0, Tick: 1, EventTime: now, WorkIDs: &workIDs},
			Payload: json.RawMessage(`{"type":"FACTORY_REQUEST_BATCH","works":[{"workId":"wire-fold-dispatch-work","workTypeName":"task","state":{"name":"ready"},"name":"replay work"}]}`)},
		{Id: "dispatch-request", Type: factorydefinitions.FactoryEventTypeDispatchRequest,
			Context: factorydefinitions.FactoryEventContext{Sequence: 1, Tick: 2, EventTime: now.Add(time.Second), DispatchID: stringPointer(dispatchID), WorkIDs: &workIDs},
			Payload: json.RawMessage(`{"transitionId":"t-process","inputs":[{"workId":"wire-fold-dispatch-work","workTypeId":"task","state":"ready"}]}`)},
		{Id: "dispatch-response", Type: factorydefinitions.FactoryEventTypeDispatchResponse,
			Context: factorydefinitions.FactoryEventContext{Sequence: 2, Tick: 3, EventTime: now.Add(2 * time.Second), DispatchID: stringPointer(dispatchID), WorkIDs: &workIDs},
			Payload: json.RawMessage(`{"transitionId":"t-process","outcome":"ACCEPTED","output":"completed through canonical replay","durationMillis":1000}`)},
	}

	projection := recordingswire.NewProjectionService()
	state, err := projection.ReconstructFactoryWorldState(events, 3)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState() = %v", err)
	}
	if len(state.CompletedDispatches) != 1 {
		t.Fatalf("completed dispatches = %#v, want one canonical dispatch completion", state.CompletedDispatches)
	}
	completion := state.CompletedDispatches[0]
	if completion.DispatchID != dispatchID || completion.Result.Outcome != "ACCEPTED" {
		t.Fatalf("replayed completion = %#v, want accepted dispatch %q", completion, dispatchID)
	}
	if len(completion.WorkItemIDs) != 1 || completion.WorkItemIDs[0] != workID {
		t.Fatalf("replayed completion Work lineage = %#v, want [%q]", completion.WorkItemIDs, workID)
	}
}

func TestReplayOwnerPreservesLoadValidationAndProjection(t *testing.T) {
	t.Parallel()

	source := foldFinalizedSnapshot(t, "wire-fold-replay", 3)
	projection := &foldReplayProjection{}
	root := recordingswire.NewReplayOwner(source, projection, nil, nil)
	recording := finalizedWireFoldReplayRecording(t, root, projection)
	assertWireFoldReplayLoadFailures(t, root, recording)
}

func TestArtifactsOwnerPreservesPortableRoundTrip(t *testing.T) {
	t.Parallel()

	source := foldFinalizedSnapshot(t, "recording-wire-fold-export", 2)
	root := recordingswire.NewArtifactsExportOwner(source, nil)

	built, err := root.BuildPortableArtifact(recordings.BuildPortableArtifactRequest{
		RecordingID: source.snapshot.Status.RecordingID,
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
	if err != nil || summarized.Summary.RecordingID != source.snapshot.Status.RecordingID {
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
	root recordingswire.CanonicalLedgerOwner,
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

type foldReplayProjection struct {
	recordings.ProjectionService
	events []factorydefinitions.FactoryEvent
	tick   int
}

func (projection *foldReplayProjection) ReconstructFactoryWorldState(events []factorydefinitions.FactoryEvent, tick int) (recordings.FactoryWorldState, error) {
	projection.events = append([]factorydefinitions.FactoryEvent(nil), events...)
	projection.tick = tick
	return recordings.FactoryWorldState{Tick: tick}, nil
}
func finalizedWireFoldReplayRecording(t *testing.T, root recordingswire.ReplayOwner, projection *foldReplayProjection) recordings.ReplayRecordingFacts {
	t.Helper()

	loaded, err := root.LoadReplayRecording(recordings.LoadReplayRecordingRequest{
		RecordingID: "wire-fold-replay",
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
	live, err := canonical.ReconstructWorldState(projection, recordings.ReconstructWorldStateRequest{
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
	if len(projection.events) != len(loaded.Recording.Events) || projection.tick != 7 {
		t.Fatalf("projection received %d events at tick %d", len(projection.events), projection.tick)
	}

	return loaded.Recording
}

func assertWireFoldReplayLoadFailures(
	t *testing.T,
	root recordingswire.ReplayOwner,
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
