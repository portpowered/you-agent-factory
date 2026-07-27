package recordings_test

import (
	"errors"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

func TestNeutralReplayRootContract_EquivalentReductionAndProgress(t *testing.T) {
	t.Parallel()

	service := recordings.Service(&peerRootServiceFake{})
	recording := finalizedReplayRecording(t, service)
	loaded, err := service.LoadReplayRecording(recordings.LoadReplayRecordingRequest{
		RecordingID: recording,
	})
	if err != nil {
		t.Fatalf("LoadReplayRecording: %v", err)
	}
	expectedThrough := loaded.Recording.Events[len(loaded.Recording.Events)-1].Cursor
	planned, err := service.CreateReplayPlan(recordings.CreateReplayPlanRequest{
		SchemaVersion:   recordings.ReplayPlanSchemaV1,
		Timing:          recordings.ReplayTimingOrderOnly,
		Recording:       loaded.Recording,
		ExpectedThrough: &expectedThrough,
		SelectedTick:    7,
	})
	if err != nil {
		t.Fatalf("CreateReplayPlan: %v", err)
	}

	progress, err := service.ObserveReplay(recordings.ObserveReplayRequest{Plan: planned.Plan.Handle})
	if err != nil {
		t.Fatalf("ObserveReplay progress: %v", err)
	}
	if progress.Observation.Kind != recordings.ReplayProgress ||
		progress.Observation.ProcessedEvents != 1 {
		t.Fatalf("progress observation = %#v", progress.Observation)
	}
	completed, err := service.ObserveReplay(recordings.ObserveReplayRequest{Plan: planned.Plan.Handle})
	if err != nil {
		t.Fatalf("ObserveReplay completion: %v", err)
	}
	if completed.Observation.Kind != recordings.ReplayCompleted ||
		completed.Observation.ProcessedEvents != 2 {
		t.Fatalf("completion observation = %#v", completed.Observation)
	}

	live, err := service.ReconstructWorldState(recordings.ReconstructWorldStateRequest{
		Scope:        loaded.Recording.Scope,
		Events:       loaded.Recording.Events,
		SelectedTick: 7,
	})
	if err != nil {
		t.Fatalf("ReconstructWorldState live sequence: %v", err)
	}
	if completed.Observation.WorldState != live.WorldState {
		t.Fatalf("replay world = %#v, live world = %#v", completed.Observation.WorldState, live.WorldState)
	}
}

func TestNeutralReplayRootContract_DivergenceAndTypedFailures(t *testing.T) {
	t.Parallel()

	service := recordings.Service(&peerRootServiceFake{})
	assertReplayLoadFailures(t, service)
	recordingID := finalizedReplayRecording(t, service)
	loaded, err := service.LoadReplayRecording(recordings.LoadReplayRecordingRequest{
		RecordingID: recordingID,
	})
	if err != nil {
		t.Fatalf("LoadReplayRecording: %v", err)
	}
	assertReplayPlanFailures(t, service, loaded.Recording)
	assertReplayDivergence(t, service, loaded.Recording)
}

func finalizedReplayRecording(t *testing.T, service recordings.Service) recordings.RecordingID {
	t.Helper()
	scope := recordings.CanonicalEventScope{FactorySessionID: "session-replay"}
	bound, err := service.BindRecording(recordings.BindRecordingRequest{
		Artifact: "artifact:replay",
		Scope:    scope,
	})
	if err != nil {
		t.Fatalf("BindRecording: %v", err)
	}
	for index := 0; index < 2; index++ {
		event := lifecycleEvent(
			recordings.CanonicalEventID("replay-event-"+string(rune('1'+index))),
			recordings.CanonicalEventSequence(index),
			scope,
		)
		if _, err := service.RecordRecordingEvent(recordings.RecordRecordingEventRequest{
			RecordingID: bound.Status.RecordingID,
			Event:       event,
		}); err != nil {
			t.Fatalf("RecordRecordingEvent[%d]: %v", index, err)
		}
	}
	if _, err := service.FinishRecording(recordings.FinishRecordingRequest{
		RecordingID: bound.Status.RecordingID,
		FinishedAt:  time.Unix(1_700_000_100, 0).UTC(),
	}); err != nil {
		t.Fatalf("FinishRecording: %v", err)
	}
	return bound.Status.RecordingID
}

func assertReplayLoadFailures(t *testing.T, service recordings.Service) {
	t.Helper()
	if _, err := service.LoadReplayRecording(recordings.LoadReplayRecordingRequest{
		RecordingID: "missing",
	}); !errors.Is(err, recordings.ErrReplayRecordingNotFound) {
		t.Fatalf("missing recording error = %v, want ErrReplayRecordingNotFound", err)
	}
	active, err := service.BindRecording(recordings.BindRecordingRequest{
		Artifact: "artifact:active",
	})
	if err != nil {
		t.Fatalf("BindRecording active: %v", err)
	}
	if _, err := service.LoadReplayRecording(recordings.LoadReplayRecordingRequest{
		RecordingID: active.Status.RecordingID,
	}); !errors.Is(err, recordings.ErrReplayRecordingNotFinalized) {
		t.Fatalf("active recording error = %v, want ErrReplayRecordingNotFinalized", err)
	}
}

func assertReplayPlanFailures(
	t *testing.T,
	service recordings.Service,
	recording recordings.ReplayRecordingFacts,
) {
	t.Helper()
	if _, err := service.CreateReplayPlan(recordings.CreateReplayPlanRequest{
		SchemaVersion: "unknown",
		Timing:        recordings.ReplayTimingOrderOnly,
		Recording:     recording,
	}); !errors.Is(err, recordings.ErrUnsupportedReplayPlan) {
		t.Fatalf("unsupported plan error = %v, want ErrUnsupportedReplayPlan", err)
	}
	corrupt := recording
	corrupt.Events = append([]recordings.CanonicalEvent(nil), recording.Events...)
	corrupt.Events[1].Sequence = 9
	if _, err := service.CreateReplayPlan(recordings.CreateReplayPlanRequest{
		SchemaVersion: recordings.ReplayPlanSchemaV1,
		Timing:        recordings.ReplayTimingOrderOnly,
		Recording:     corrupt,
	}); !errors.Is(err, recordings.ErrCorruptReplayInput) {
		t.Fatalf("corrupt input error = %v, want ErrCorruptReplayInput", err)
	}
	if _, err := service.ObserveReplay(recordings.ObserveReplayRequest{
		Plan: "missing",
	}); !errors.Is(err, recordings.ErrReplayPlanNotFound) {
		t.Fatalf("missing plan error = %v, want ErrReplayPlanNotFound", err)
	}
}

func TestNeutralReplayRootContract_RejectsMalformedCanonicalEvents(t *testing.T) {
	t.Parallel()

	valid := recordings.ReplayRecordingFacts{
		RecordingID: "recording-canonical-peer",
		Events: []recordings.CanonicalEvent{
			lifecycleEvent("peer-replay-event-1", 0, recordings.CanonicalEventScope{}),
			lifecycleEvent("peer-replay-event-2", 1, recordings.CanonicalEventScope{}),
		},
	}
	for name, mutate := range peerMalformedReplayRecordingMutations() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			service := recordings.Service(&peerRootServiceFake{})
			corrupt := peerCloneReplayRecording(valid)
			mutate(&corrupt)
			result, err := service.CreateReplayPlan(peerReplayPlanRequest(corrupt))
			if !errors.Is(err, recordings.ErrCorruptReplayInput) {
				t.Fatalf("CreateReplayPlan malformed event error = %v, want ErrCorruptReplayInput", err)
			}
			if result != (recordings.CreateReplayPlanResult{}) {
				t.Fatalf("CreateReplayPlan malformed event result = %#v, want no observable plan", result)
			}

			planned, err := service.CreateReplayPlan(peerReplayPlanRequest(valid))
			if err != nil {
				t.Fatalf("CreateReplayPlan valid event after rejection: %v", err)
			}
			var observed recordings.ObserveReplayResult
			for observation := 0; observation < len(valid.Events); observation++ {
				observed, err = service.ObserveReplay(recordings.ObserveReplayRequest{
					Plan: planned.Plan.Handle,
				})
				if err != nil {
					t.Fatalf("ObserveReplay valid event after rejection: %v", err)
				}
			}
			if observed.Observation.Kind != recordings.ReplayCompleted {
				t.Fatalf("ObserveReplay valid event after rejection = %#v, want completed", observed)
			}
		})
	}
}

func peerMalformedReplayRecordingMutations() map[string]func(*recordings.ReplayRecordingFacts) {
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

func peerCloneReplayRecording(recording recordings.ReplayRecordingFacts) recordings.ReplayRecordingFacts {
	cloned := recording
	cloned.Events = append([]recordings.CanonicalEvent(nil), recording.Events...)
	return cloned
}

func peerReplayPlanRequest(recording recordings.ReplayRecordingFacts) recordings.CreateReplayPlanRequest {
	return recordings.CreateReplayPlanRequest{
		SchemaVersion: recordings.ReplayPlanSchemaV1,
		Timing:        recordings.ReplayTimingOrderOnly,
		Recording:     recording,
	}
}

func assertReplayDivergence(
	t *testing.T,
	service recordings.Service,
	recording recordings.ReplayRecordingFacts,
) {
	t.Helper()
	expected := recording.Events[len(recording.Events)-1].Cursor
	expected.Sequence++
	planned, err := service.CreateReplayPlan(recordings.CreateReplayPlanRequest{
		SchemaVersion:   recordings.ReplayPlanSchemaV1,
		Timing:          recordings.ReplayTimingOrderOnly,
		Recording:       recording,
		ExpectedThrough: &expected,
		SelectedTick:    7,
	})
	if err != nil {
		t.Fatalf("CreateReplayPlan divergence: %v", err)
	}
	if _, err := service.ObserveReplay(recordings.ObserveReplayRequest{
		Plan: planned.Plan.Handle,
	}); err != nil {
		t.Fatalf("ObserveReplay divergence progress: %v", err)
	}
	diverged, err := service.ObserveReplay(recordings.ObserveReplayRequest{
		Plan: planned.Plan.Handle,
	})
	if err != nil {
		t.Fatalf("ObserveReplay divergence: %v", err)
	}
	if diverged.Observation.Kind != recordings.ReplayDiverged ||
		diverged.Observation.Divergence == nil ||
		diverged.Observation.Divergence.Expected != expected {
		t.Fatalf("divergence observation = %#v", diverged.Observation)
	}
}
