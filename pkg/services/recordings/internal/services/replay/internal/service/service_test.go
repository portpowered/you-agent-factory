package service_test

import (
	"errors"
	"testing"
	"time"

	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/recordings/internal/canonical"
	projectionquerywire "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/projection_query/wire"
	recordinglifecycle "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/recording_lifecycle"
	recordinglifecyclewire "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/recording_lifecycle/wire"
	replayservice "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/replay/internal/service"
)

func TestLoadReplayRecordingReturnsDetachedFinalizedFacts(t *testing.T) {
	t.Parallel()

	replay, lifecycle, recordingID, scope := newReplayHarness(t)
	finalizeReplayRecording(t, lifecycle, recordingID)

	loaded, err := replay.LoadReplayRecording(recordings.LoadReplayRecordingRequest{
		RecordingID: recordingID,
	})
	if err != nil {
		t.Fatalf("LoadReplayRecording: %v", err)
	}
	if loaded.Recording.RecordingID != recordingID {
		t.Fatalf("recording id = %q, want %q", loaded.Recording.RecordingID, recordingID)
	}
	if loaded.Recording.Scope != scope {
		t.Fatalf("scope = %#v, want %#v", loaded.Recording.Scope, scope)
	}
	if len(loaded.Recording.Events) != 2 {
		t.Fatalf("events = %d, want 2", len(loaded.Recording.Events))
	}

	loaded.Recording.Events[0].Kind = "MUTATED"
	reloaded, err := replay.LoadReplayRecording(recordings.LoadReplayRecordingRequest{
		RecordingID: recordingID,
	})
	if err != nil {
		t.Fatalf("LoadReplayRecording reload: %v", err)
	}
	if reloaded.Recording.Events[0].Kind != "FACTORY_STATE_RESPONSE" {
		t.Fatalf("reload event kind = %q, want original retained facts", reloaded.Recording.Events[0].Kind)
	}
}

func TestLoadReplayRecordingTypedFailures(t *testing.T) {
	t.Parallel()

	replay, lifecycle, recordingID, _ := newReplayHarness(t)

	if _, err := replay.LoadReplayRecording(recordings.LoadReplayRecordingRequest{
		RecordingID: "missing-recording",
	}); !errors.Is(err, recordings.ErrReplayRecordingNotFound) {
		t.Fatalf("missing recording error = %v, want ErrReplayRecordingNotFound", err)
	}
	if _, err := replay.LoadReplayRecording(recordings.LoadReplayRecordingRequest{
		RecordingID: recordingID,
	}); !errors.Is(err, recordings.ErrReplayRecordingNotFinalized) {
		t.Fatalf("active recording error = %v, want ErrReplayRecordingNotFinalized", err)
	}

	finalizeReplayRecording(t, lifecycle, recordingID)
	if _, err := replay.LoadReplayRecording(recordings.LoadReplayRecordingRequest{
		RecordingID: recordingID,
	}); err != nil {
		t.Fatalf("finalized recording load: %v", err)
	}
}

func TestCreateReplayPlanAcceptsOrderOnlyPlan(t *testing.T) {
	t.Parallel()

	replay, lifecycle, recordingID, _ := newReplayHarness(t)
	finalizeReplayRecording(t, lifecycle, recordingID)
	loaded, err := replay.LoadReplayRecording(recordings.LoadReplayRecordingRequest{
		RecordingID: recordingID,
	})
	if err != nil {
		t.Fatalf("LoadReplayRecording: %v", err)
	}

	planned, err := replay.CreateReplayPlan(recordings.CreateReplayPlanRequest{
		SchemaVersion: recordings.ReplayPlanSchemaV1,
		Timing:        recordings.ReplayTimingOrderOnly,
		Recording:     loaded.Recording,
	})
	if err != nil {
		t.Fatalf("CreateReplayPlan: %v", err)
	}
	if planned.Plan.Handle == "" {
		t.Fatal("plan handle is empty, want opaque retained handle")
	}
	if planned.Plan.TotalEvents != len(loaded.Recording.Events) {
		t.Fatalf("total events = %d, want %d", planned.Plan.TotalEvents, len(loaded.Recording.Events))
	}
	if planned.Plan.SchemaVersion != recordings.ReplayPlanSchemaV1 ||
		planned.Plan.Timing != recordings.ReplayTimingOrderOnly {
		t.Fatalf("plan facts = %#v, want order-only v1 facts", planned.Plan)
	}
}

func TestCreateReplayPlanRejectsUnsupportedOptions(t *testing.T) {
	t.Parallel()

	replay, lifecycle, recordingID, _ := newReplayHarness(t)
	finalizeReplayRecording(t, lifecycle, recordingID)
	loaded, err := replay.LoadReplayRecording(recordings.LoadReplayRecordingRequest{
		RecordingID: recordingID,
	})
	if err != nil {
		t.Fatalf("LoadReplayRecording: %v", err)
	}
	recording := loaded.Recording

	unsupported := []recordings.CreateReplayPlanRequest{
		{
			SchemaVersion: "recordings.replay-plan.v2",
			Timing:        recordings.ReplayTimingOrderOnly,
			Recording:     recording,
		},
		{
			SchemaVersion: recordings.ReplayPlanSchemaV1,
			Timing:        recordings.ReplayTimingMode("REAL_TIME"),
			Recording:     recording,
		},
		{
			SchemaVersion: recordings.ReplayPlanSchemaV1,
			Timing:        recordings.ReplayTimingOrderOnly,
			Recording:     recording,
			SelectedTick:  -1,
		},
	}
	for index, request := range unsupported {
		result, err := replay.CreateReplayPlan(request)
		if !errors.Is(err, recordings.ErrUnsupportedReplayPlan) {
			t.Fatalf("unsupported[%d] error = %v, want ErrUnsupportedReplayPlan", index, err)
		}
		if result != (recordings.CreateReplayPlanResult{}) {
			t.Fatalf("unsupported[%d] result = %#v, want no observable plan", index, result)
		}
	}

	planned, err := replay.CreateReplayPlan(recordings.CreateReplayPlanRequest{
		SchemaVersion: recordings.ReplayPlanSchemaV1,
		Timing:        recordings.ReplayTimingOrderOnly,
		Recording:     recording,
	})
	if err != nil {
		t.Fatalf("CreateReplayPlan after unsupported rejection: %v", err)
	}
	observed, err := replay.ObserveReplay(recordings.ObserveReplayRequest{
		Plan: planned.Plan.Handle,
	})
	if err != nil || observed.Observation.TotalEvents != len(recording.Events) {
		t.Fatalf("ObserveReplay after unsupported rejection = (%#v, %v)", observed, err)
	}
}

func TestObserveReplayProgressAndCompletion(t *testing.T) {
	t.Parallel()

	replay, lifecycle, recordingID, scope := newReplayHarness(t)
	finalizeReplayRecording(t, lifecycle, recordingID)
	loaded, err := replay.LoadReplayRecording(recordings.LoadReplayRecordingRequest{
		RecordingID: recordingID,
	})
	if err != nil {
		t.Fatalf("LoadReplayRecording: %v", err)
	}
	expectedThrough := loaded.Recording.Events[len(loaded.Recording.Events)-1].Cursor
	selectedTick := 7
	planned, err := replay.CreateReplayPlan(recordings.CreateReplayPlanRequest{
		SchemaVersion:   recordings.ReplayPlanSchemaV1,
		Timing:          recordings.ReplayTimingOrderOnly,
		Recording:       loaded.Recording,
		ExpectedThrough: &expectedThrough,
		SelectedTick:    selectedTick,
	})
	if err != nil {
		t.Fatalf("CreateReplayPlan: %v", err)
	}

	progress, err := replay.ObserveReplay(recordings.ObserveReplayRequest{
		Plan: planned.Plan.Handle,
	})
	if err != nil {
		t.Fatalf("ObserveReplay progress: %v", err)
	}
	if progress.Observation.Kind != recordings.ReplayProgress {
		t.Fatalf("progress kind = %q, want PROGRESS", progress.Observation.Kind)
	}
	if progress.Observation.ProcessedEvents != 1 ||
		progress.Observation.TotalEvents != len(loaded.Recording.Events) {
		t.Fatalf("progress counts = (%d, %d), want (1, %d)",
			progress.Observation.ProcessedEvents,
			progress.Observation.TotalEvents,
			len(loaded.Recording.Events),
		)
	}
	if progress.Observation.Through == nil ||
		*progress.Observation.Through != loaded.Recording.Events[0].Cursor {
		t.Fatalf("progress through = %#v, want %#v", progress.Observation.Through, loaded.Recording.Events[0].Cursor)
	}

	completed, err := replay.ObserveReplay(recordings.ObserveReplayRequest{
		Plan: planned.Plan.Handle,
	})
	if err != nil {
		t.Fatalf("ObserveReplay completion: %v", err)
	}
	if completed.Observation.Kind != recordings.ReplayCompleted {
		t.Fatalf("completion kind = %q, want COMPLETED", completed.Observation.Kind)
	}
	if completed.Observation.ProcessedEvents != len(loaded.Recording.Events) {
		t.Fatalf("completion processed = %d, want %d",
			completed.Observation.ProcessedEvents, len(loaded.Recording.Events))
	}
	if completed.Observation.Through == nil ||
		*completed.Observation.Through != expectedThrough {
		t.Fatalf("completion through = %#v, want %#v", completed.Observation.Through, expectedThrough)
	}
	if completed.Observation.WorldState.Scope != scope ||
		completed.Observation.WorldState.SelectedTick != selectedTick {
		t.Fatalf("completion world scope/tick = (%#v, %d), want (%#v, %d)",
			completed.Observation.WorldState.Scope, completed.Observation.WorldState.SelectedTick,
			scope, selectedTick)
	}
}

func TestObserveReplayEquivalentWorldState(t *testing.T) {
	t.Parallel()

	replay, lifecycle, recordingID, scope := newReplayHarness(t)
	projection := projectionquerywire.NewService()
	finalizeReplayRecording(t, lifecycle, recordingID)
	loaded, err := replay.LoadReplayRecording(recordings.LoadReplayRecordingRequest{
		RecordingID: recordingID,
	})
	if err != nil {
		t.Fatalf("LoadReplayRecording: %v", err)
	}
	selectedTick := 7
	planned, err := replay.CreateReplayPlan(recordings.CreateReplayPlanRequest{
		SchemaVersion: recordings.ReplayPlanSchemaV1,
		Timing:        recordings.ReplayTimingOrderOnly,
		Recording:     loaded.Recording,
		SelectedTick:  selectedTick,
	})
	if err != nil {
		t.Fatalf("CreateReplayPlan: %v", err)
	}
	var completed recordings.ObserveReplayResult
	for step := 0; step < len(loaded.Recording.Events); step++ {
		completed, err = replay.ObserveReplay(recordings.ObserveReplayRequest{
			Plan: planned.Plan.Handle,
		})
		if err != nil {
			t.Fatalf("ObserveReplay step %d: %v", step, err)
		}
	}
	reconstructed, err := canonical.ReconstructWorldState(projection, recordings.ReconstructWorldStateRequest{
		Scope:        scope,
		Events:       loaded.Recording.Events,
		SelectedTick: selectedTick,
	})
	if err != nil {
		t.Fatalf("ReconstructWorldState: %v", err)
	}
	if completed.Observation.WorldState != reconstructed.WorldState {
		t.Fatalf("replay world = %#v, reconstructed = %#v",
			completed.Observation.WorldState, reconstructed.WorldState)
	}
	if completed.Observation.WorldState.Through != reconstructed.WorldState.Through {
		t.Fatalf("replay through = %#v, reconstructed through = %#v",
			completed.Observation.WorldState.Through, reconstructed.WorldState.Through)
	}
}

func TestObserveReplayRepeatableObservations(t *testing.T) {
	t.Parallel()

	replay, lifecycle, recordingID, _ := newReplayHarness(t)
	finalizeReplayRecording(t, lifecycle, recordingID)
	loaded, err := replay.LoadReplayRecording(recordings.LoadReplayRecordingRequest{
		RecordingID: recordingID,
	})
	if err != nil {
		t.Fatalf("LoadReplayRecording: %v", err)
	}
	planRequest := recordings.CreateReplayPlanRequest{
		SchemaVersion: recordings.ReplayPlanSchemaV1,
		Timing:        recordings.ReplayTimingOrderOnly,
		Recording:     loaded.Recording,
		SelectedTick:  4,
	}
	firstPlan, err := replay.CreateReplayPlan(planRequest)
	if err != nil {
		t.Fatalf("CreateReplayPlan first: %v", err)
	}
	secondPlan, err := replay.CreateReplayPlan(planRequest)
	if err != nil {
		t.Fatalf("CreateReplayPlan second: %v", err)
	}

	observePlan := func(handle recordings.ReplayPlanHandle) recordings.ReplayObservation {
		var observed recordings.ObserveReplayResult
		for step := 0; step < len(loaded.Recording.Events); step++ {
			observed, err = replay.ObserveReplay(recordings.ObserveReplayRequest{Plan: handle})
			if err != nil {
				t.Fatalf("ObserveReplay step %d: %v", step, err)
			}
		}
		return observed.Observation
	}
	first := observePlan(firstPlan.Plan.Handle)
	second := observePlan(secondPlan.Plan.Handle)
	throughMismatch := first.Through == nil ||
		second.Through == nil ||
		*first.Through != *second.Through
	if first.Kind != second.Kind ||
		first.ProcessedEvents != second.ProcessedEvents ||
		first.TotalEvents != second.TotalEvents ||
		first.WorldState != second.WorldState ||
		throughMismatch {
		t.Fatalf("repeat observations differ:\nfirst=%#v\nsecond=%#v", first, second)
	}
}

func TestObserveReplayDivergenceOnExpectedThroughMismatch(t *testing.T) {
	t.Parallel()

	replay, lifecycle, recordingID, _ := newReplayHarness(t)
	finalizeReplayRecording(t, lifecycle, recordingID)
	loaded, err := replay.LoadReplayRecording(recordings.LoadReplayRecordingRequest{
		RecordingID: recordingID,
	})
	if err != nil {
		t.Fatalf("LoadReplayRecording: %v", err)
	}
	expected := loaded.Recording.Events[len(loaded.Recording.Events)-1].Cursor
	expected.Sequence++
	planned, err := replay.CreateReplayPlan(recordings.CreateReplayPlanRequest{
		SchemaVersion:   recordings.ReplayPlanSchemaV1,
		Timing:          recordings.ReplayTimingOrderOnly,
		Recording:       loaded.Recording,
		ExpectedThrough: &expected,
		SelectedTick:    7,
	})
	if err != nil {
		t.Fatalf("CreateReplayPlan: %v", err)
	}
	if _, err := replay.ObserveReplay(recordings.ObserveReplayRequest{
		Plan: planned.Plan.Handle,
	}); err != nil {
		t.Fatalf("ObserveReplay progress before divergence: %v", err)
	}
	diverged, err := replay.ObserveReplay(recordings.ObserveReplayRequest{
		Plan: planned.Plan.Handle,
	})
	if err != nil {
		t.Fatalf("ObserveReplay divergence: %v", err)
	}
	if diverged.Observation.Kind != recordings.ReplayDiverged {
		t.Fatalf("divergence kind = %q, want DIVERGED", diverged.Observation.Kind)
	}
	if diverged.Observation.Divergence == nil {
		t.Fatal("divergence facts missing")
	}
	if diverged.Observation.Divergence.Expected != expected {
		t.Fatalf("divergence expected = %#v, want %#v",
			diverged.Observation.Divergence.Expected, expected)
	}
	observedCursor := loaded.Recording.Events[len(loaded.Recording.Events)-1].Cursor
	if diverged.Observation.Divergence.Observed != observedCursor {
		t.Fatalf("divergence observed = %#v, want %#v",
			diverged.Observation.Divergence.Observed, observedCursor)
	}
}

func TestCreateReplayPlanDetachesCallerFacts(t *testing.T) {
	t.Parallel()

	replay, lifecycle, recordingID, _ := newReplayHarness(t)
	finalizeReplayRecording(t, lifecycle, recordingID)
	loaded, err := replay.LoadReplayRecording(recordings.LoadReplayRecordingRequest{
		RecordingID: recordingID,
	})
	if err != nil {
		t.Fatalf("LoadReplayRecording: %v", err)
	}

	planned, err := replay.CreateReplayPlan(recordings.CreateReplayPlanRequest{
		SchemaVersion: recordings.ReplayPlanSchemaV1,
		Timing:        recordings.ReplayTimingOrderOnly,
		Recording:     loaded.Recording,
	})
	if err != nil {
		t.Fatalf("CreateReplayPlan: %v", err)
	}

	loaded.Recording.Events = append(loaded.Recording.Events, replayStateEvent(99, `{"state":"INJECTED"}`))
	planned.Plan.TotalEvents = 0

	var observed recordings.ObserveReplayResult
	for step := 0; step < len(loaded.Recording.Events); step++ {
		observed, err = replay.ObserveReplay(recordings.ObserveReplayRequest{
			Plan: planned.Plan.Handle,
		})
		if err != nil {
			t.Fatalf("ObserveReplay step %d: %v", step, err)
		}
	}
	if observed.Observation.TotalEvents != 2 {
		t.Fatalf("retained total events = %d, want 2", observed.Observation.TotalEvents)
	}
	if observed.Observation.Kind != recordings.ReplayCompleted {
		t.Fatalf("observation kind = %q, want COMPLETED", observed.Observation.Kind)
	}
}

func newReplayHarness(t *testing.T) (
	*replayservice.Service,
	recordinglifecycle.Service,
	recordings.RecordingID,
	recordings.CanonicalEventScope,
) {
	t.Helper()

	planner := recordings.LiveRecordingTargetPlannerFunc(
		func(recordings.LiveRecordingTargetRequest) (recordings.LiveRecordingTarget, error) {
			return recordings.LiveRecordingTarget{}, nil
		},
	)
	lifecycle := recordinglifecyclewire.NewService(planner, nil, nil)
	projection := projectionquerywire.NewService()
	replay := replayservice.New(lifecycle, projection)
	scope := recordings.CanonicalEventScope{FactorySessionID: "session-replay-load-plan"}
	bound, err := lifecycle.BindRecording(recordings.BindRecordingRequest{
		RecordingID: "recording-replay-load-plan",
		Artifact:    "artifact:replay-load-plan",
		Scope:       scope,
	})
	if err != nil {
		t.Fatalf("BindRecording: %v", err)
	}
	for index, event := range []recordings.CanonicalEvent{
		replayStateEvent(0, `{"state":"RUNNING"}`),
		replayStateEvent(1, `{"state":"COMPLETED"}`),
	} {
		scoped := event
		scoped.Scope = scope
		if _, err := lifecycle.RecordRecordingEvent(recordings.RecordRecordingEventRequest{
			RecordingID: bound.Status.RecordingID,
			Event:       scoped,
		}); err != nil {
			t.Fatalf("RecordRecordingEvent[%d]: %v", index, err)
		}
	}
	return replay, lifecycle, bound.Status.RecordingID, scope
}

func finalizeReplayRecording(
	t *testing.T,
	lifecycle recordinglifecycle.Service,
	recordingID recordings.RecordingID,
) {
	t.Helper()

	if _, err := lifecycle.FinishRecording(recordings.FinishRecordingRequest{
		RecordingID: recordingID,
		FinishedAt:  time.Unix(1_700_000_100, 0).UTC(),
	}); err != nil {
		t.Fatalf("FinishRecording: %v", err)
	}
}

func replayStateEvent(sequence recordings.CanonicalEventSequence, payload string) recordings.CanonicalEvent {
	return recordings.CanonicalEvent{
		ID:       recordings.CanonicalEventID("state-" + string(rune('0'+sequence))),
		Sequence: sequence,
		Cursor: recordings.CanonicalEventCursor{
			StreamGenerationID: "generation-replay-load-plan",
			Sequence:           sequence,
		},
		RecordedAt: time.Unix(1_700_000_000+int64(sequence), 0).UTC(),
		Kind:       "FACTORY_STATE_RESPONSE",
		Payload:    payload,
	}
}
