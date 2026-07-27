package service_test

import (
	"errors"
	"strings"
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

func TestCreateReplayPlanRejectsCorruptCanonicalEvents(t *testing.T) {
	t.Parallel()

	valid := recordings.ReplayRecordingFacts{
		RecordingID: "recording-replay-corrupt",
		Scope:       recordings.CanonicalEventScope{FactorySessionID: "session-replay-corrupt"},
		Events: []recordings.CanonicalEvent{
			scopedReplayStateEvent(0, `{"state":"RUNNING"}`, recordings.CanonicalEventScope{FactorySessionID: "session-replay-corrupt"}),
			scopedReplayStateEvent(1, `{"state":"COMPLETED"}`, recordings.CanonicalEventScope{FactorySessionID: "session-replay-corrupt"}),
		},
	}
	for name, mutate := range privateMalformedReplayRecordingMutations() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			replay, _, _, _ := newReplayHarness(t)
			corrupt := clonePrivateReplayRecording(valid)
			mutate(&corrupt)
			result, err := replay.CreateReplayPlan(recordings.CreateReplayPlanRequest{
				SchemaVersion: recordings.ReplayPlanSchemaV1,
				Timing:        recordings.ReplayTimingOrderOnly,
				Recording:     corrupt,
			})
			assertBoundedReplayError(t, err, recordings.ErrCorruptReplayInput)
			if result != (recordings.CreateReplayPlanResult{}) {
				t.Fatalf("CreateReplayPlan corrupt result = %#v, want no observable plan", result)
			}

			planned, err := replay.CreateReplayPlan(recordings.CreateReplayPlanRequest{
				SchemaVersion: recordings.ReplayPlanSchemaV1,
				Timing:        recordings.ReplayTimingOrderOnly,
				Recording:     valid,
			})
			if err != nil {
				t.Fatalf("CreateReplayPlan valid after rejection: %v", err)
			}
			var observed recordings.ObserveReplayResult
			for step := 0; step < len(valid.Events); step++ {
				observed, err = replay.ObserveReplay(recordings.ObserveReplayRequest{
					Plan: planned.Plan.Handle,
				})
				if err != nil {
					t.Fatalf("ObserveReplay valid after rejection step %d: %v", step, err)
				}
			}
			if observed.Observation.Kind != recordings.ReplayCompleted {
				t.Fatalf("ObserveReplay valid after rejection = %#v, want COMPLETED", observed)
			}
		})
	}
}

func TestCreateReplayPlanRejectsEmptyRecordingIdentity(t *testing.T) {
	t.Parallel()

	replay, _, _, _ := newReplayHarness(t)
	recording := recordings.ReplayRecordingFacts{
		RecordingID: "   ",
		Events: []recordings.CanonicalEvent{
			replayStateEvent(0, `{"state":"RUNNING"}`),
		},
	}
	result, err := replay.CreateReplayPlan(recordings.CreateReplayPlanRequest{
		SchemaVersion: recordings.ReplayPlanSchemaV1,
		Timing:        recordings.ReplayTimingOrderOnly,
		Recording:     recording,
	})
	assertBoundedReplayError(t, err, recordings.ErrCorruptReplayInput)
	if result != (recordings.CreateReplayPlanResult{}) {
		t.Fatalf("CreateReplayPlan empty identity result = %#v, want no observable plan", result)
	}
}

func TestObserveReplayReturnsReplayPlanNotFound(t *testing.T) {
	t.Parallel()

	replay, _, _, _ := newReplayHarness(t)
	if _, err := replay.ObserveReplay(recordings.ObserveReplayRequest{
		Plan: "missing-replay-plan",
	}); !errors.Is(err, recordings.ErrReplayPlanNotFound) {
		t.Fatalf("ObserveReplay missing plan = %v, want ErrReplayPlanNotFound", err)
	}
	assertBoundedReplayError(t, recordings.ErrReplayPlanNotFound, recordings.ErrReplayPlanNotFound)
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
	return scopedReplayStateEvent(sequence, payload, recordings.CanonicalEventScope{FactorySessionID: "session-replay-load-plan"})
}

func scopedReplayStateEvent(
	sequence recordings.CanonicalEventSequence,
	payload string,
	scope recordings.CanonicalEventScope,
) recordings.CanonicalEvent {
	return recordings.CanonicalEvent{
		ID:       recordings.CanonicalEventID("state-" + string(rune('0'+sequence))),
		Sequence: sequence,
		Scope:    scope,
		Cursor: recordings.CanonicalEventCursor{
			StreamGenerationID: "generation-replay-load-plan",
			Sequence:           sequence,
		},
		RecordedAt: time.Unix(1_700_000_000+int64(sequence), 0).UTC(),
		Kind:       "FACTORY_STATE_RESPONSE",
		Payload:    payload,
	}
}

func privateMalformedReplayRecordingMutations() map[string]func(*recordings.ReplayRecordingFacts) {
	return map[string]func(*recordings.ReplayRecordingFacts){
		"missing identity": func(recording *recordings.ReplayRecordingFacts) {
			recording.Events[0].ID = ""
		},
		"whitespace identity": func(recording *recordings.ReplayRecordingFacts) {
			recording.Events[0].ID = "   "
		},
		"missing kind": func(recording *recordings.ReplayRecordingFacts) {
			recording.Events[0].Kind = ""
		},
		"whitespace kind": func(recording *recordings.ReplayRecordingFacts) {
			recording.Events[0].Kind = "   "
		},
		"zero timestamp": func(recording *recordings.ReplayRecordingFacts) {
			recording.Events[0].RecordedAt = time.Time{}
		},
		"whitespace scope": func(recording *recordings.ReplayRecordingFacts) {
			recording.Scope.FactorySessionID = "   "
			recording.Events[0].Scope = recording.Scope
			recording.Events[1].Scope = recording.Scope
		},
		"invalid JSON": func(recording *recordings.ReplayRecordingFacts) {
			recording.Events[0].Payload = `{"incomplete":`
		},
		"missing cursor generation": func(recording *recordings.ReplayRecordingFacts) {
			recording.Events[0].Cursor.StreamGenerationID = ""
		},
		"cursor sequence mismatch": func(recording *recordings.ReplayRecordingFacts) {
			recording.Events[0].Cursor.Sequence++
		},
		"negative sequence": func(recording *recordings.ReplayRecordingFacts) {
			recording.Events[0].Sequence = -1
			recording.Events[0].Cursor.Sequence = -1
		},
		"generation mismatch": func(recording *recordings.ReplayRecordingFacts) {
			recording.Events[1].Cursor.StreamGenerationID = "other-generation"
		},
		"out of order": func(recording *recordings.ReplayRecordingFacts) {
			recording.Events[1].Sequence = recording.Events[0].Sequence
			recording.Events[1].Cursor.Sequence = recording.Events[1].Sequence
		},
	}
}

func clonePrivateReplayRecording(recording recordings.ReplayRecordingFacts) recordings.ReplayRecordingFacts {
	cloned := recording
	cloned.Events = append([]recordings.CanonicalEvent(nil), recording.Events...)
	return cloned
}

func assertBoundedReplayError(t *testing.T, err error, want error) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
	message := err.Error()
	if len(message) > 120 {
		t.Fatalf("error message too long (%d chars): %q", len(message), message)
	}
	for _, leaked := range []string{
		"/pkg/",
		"ledger",
		"internal/services",
		"decoder",
		"runtime engine",
	} {
		if strings.Contains(strings.ToLower(message), strings.ToLower(leaked)) {
			t.Fatalf("error leaked %q: %q", leaked, message)
		}
	}
}
