package service

import (
	"errors"
	"testing"
	"time"

	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
)

func TestCombinedServiceCanonicalAppendCanRecordReplayAndExport(t *testing.T) {
	t.Parallel()

	svc := NewService(&stubLedger{}, NewProjectionService())
	scope := recordings.CanonicalEventScope{FactorySessionID: "session-canonical"}
	appended, err := svc.Append(recordings.AppendRecordedEventRequest{
		Event: recordings.CanonicalEvent{
			ID:          "canonical-lifecycle-event",
			FactoryTick: 0,
			Scope:       scope,
			RecordedAt:  time.Unix(1_700_000_000, 0).UTC(),
			Kind:        "FACTORY_STATE_RESPONSE",
			Payload:     `{"state":"RUNNING"}`,
		},
	})
	if err != nil {
		t.Fatalf("Append canonical lifecycle event: %v", err)
	}
	bound, err := svc.BindRecording(recordings.BindRecordingRequest{
		RecordingID: "recording-canonical",
		Artifact:    "artifact:canonical",
		Scope:       scope,
	})
	if err != nil {
		t.Fatalf("BindRecording canonical lifecycle: %v", err)
	}
	assertInvalidCanonicalRecordingEventsDoNotMutate(
		t,
		svc,
		bound.Status,
		appended.Event,
	)
	if _, err := svc.RecordRecordingEvent(recordings.RecordRecordingEventRequest{
		RecordingID: bound.Status.RecordingID,
		Event:       appended.Event,
	}); err != nil {
		t.Fatalf("RecordRecordingEvent appended fact: %v", err)
	}
	if _, err := svc.FinishRecording(recordings.FinishRecordingRequest{
		RecordingID: bound.Status.RecordingID,
		FinishedAt:  time.Unix(1_700_000_001, 0).UTC(),
	}); err != nil {
		t.Fatalf("FinishRecording canonical lifecycle: %v", err)
	}
	loaded, err := svc.LoadReplayRecording(recordings.LoadReplayRecordingRequest{
		RecordingID: bound.Status.RecordingID,
	})
	if err != nil {
		t.Fatalf("LoadReplayRecording appended fact: %v", err)
	}
	plan, err := svc.CreateReplayPlan(recordings.CreateReplayPlanRequest{
		SchemaVersion: recordings.ReplayPlanSchemaV1,
		Timing:        recordings.ReplayTimingOrderOnly,
		Recording:     loaded.Recording,
	})
	if err != nil {
		t.Fatalf("CreateReplayPlan appended fact: %v", err)
	}
	observed, err := svc.ObserveReplay(recordings.ObserveReplayRequest{
		Plan: plan.Plan.Handle,
	})
	if err != nil || observed.Observation.Kind != recordings.ReplayCompleted {
		t.Fatalf("ObserveReplay appended fact = (%#v, %v)", observed, err)
	}
	built, err := svc.BuildPortableArtifact(recordings.BuildPortableArtifactRequest{
		RecordingID: bound.Status.RecordingID,
	})
	if err != nil || len(built.Artifact.Events) != 1 ||
		built.Artifact.Events[0] != appended.Event {
		t.Fatalf("BuildPortableArtifact appended fact = (%#v, %v)", built, err)
	}
}

func assertInvalidCanonicalRecordingEventsDoNotMutate(
	t *testing.T,
	svc recordings.Service,
	status recordings.RecordingStatusFacts,
	valid recordings.CanonicalEvent,
) {
	t.Helper()
	tests := map[string]func(*recordings.CanonicalEvent){
		"missing identity":    func(event *recordings.CanonicalEvent) { event.ID = "" },
		"whitespace identity": func(event *recordings.CanonicalEvent) { event.ID = " " },
		"missing kind":        func(event *recordings.CanonicalEvent) { event.Kind = "" },
		"whitespace kind":     func(event *recordings.CanonicalEvent) { event.Kind = " " },
		"missing timestamp": func(event *recordings.CanonicalEvent) {
			event.RecordedAt = time.Time{}
		},
		"invalid JSON": func(event *recordings.CanonicalEvent) { event.Payload = "{" },
	}
	for name, mutate := range tests {
		event := valid
		mutate(&event)
		if _, err := svc.RecordRecordingEvent(recordings.RecordRecordingEventRequest{
			RecordingID: status.RecordingID,
			Event:       event,
		}); !errors.Is(err, recordings.ErrInvalidRecordingEvent) {
			t.Fatalf("%s error = %v, want ErrInvalidRecordingEvent", name, err)
		}
		got := recordingLifecycleStatus(t, svc, status.RecordingID)
		if got.AcceptedEvents != status.AcceptedEvents || got.LastEvent != nil {
			t.Fatalf("%s mutated recording status: %#v", name, got)
		}
	}
}

func TestCombinedServiceRejectsMalformedCanonicalReplayEvents(t *testing.T) {
	t.Parallel()

	valid := recordings.ReplayRecordingFacts{
		RecordingID: "recording-canonical-replay",
		Events: []recordings.CanonicalEvent{
			replayStateEvent(0, `{"state":"RUNNING"}`),
			replayStateEvent(1, `{"state":"COMPLETED"}`),
		},
	}
	for name, mutate := range malformedReplayRecordingMutations() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			svc := NewService(&stubLedger{}, NewProjectionService())
			corrupt := cloneReplayRecording(valid)
			mutate(&corrupt)
			result, err := svc.CreateReplayPlan(replayPlanRequest(corrupt))
			if !errors.Is(err, recordings.ErrCorruptReplayInput) {
				t.Fatalf("CreateReplayPlan malformed event error = %v, want ErrCorruptReplayInput", err)
			}
			if result != (recordings.CreateReplayPlanResult{}) {
				t.Fatalf("CreateReplayPlan malformed event result = %#v, want no observable plan", result)
			}

			planned, err := svc.CreateReplayPlan(replayPlanRequest(valid))
			if err != nil {
				t.Fatalf("CreateReplayPlan valid event after rejection: %v", err)
			}
			var observed recordings.ObserveReplayResult
			for observation := 0; observation < len(valid.Events); observation++ {
				observed, err = svc.ObserveReplay(recordings.ObserveReplayRequest{
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

func malformedReplayRecordingMutations() map[string]func(*recordings.ReplayRecordingFacts) {
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

func cloneReplayRecording(recording recordings.ReplayRecordingFacts) recordings.ReplayRecordingFacts {
	cloned := recording
	cloned.Events = append([]recordings.CanonicalEvent(nil), recording.Events...)
	return cloned
}

func replayPlanRequest(recording recordings.ReplayRecordingFacts) recordings.CreateReplayPlanRequest {
	return recordings.CreateReplayPlanRequest{
		SchemaVersion: recordings.ReplayPlanSchemaV1,
		Timing:        recordings.ReplayTimingOrderOnly,
		Recording:     recording,
	}
}
