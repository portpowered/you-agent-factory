package wire

import (
	"errors"
	"strings"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingsservice "github.com/portpowered/infinite-you/pkg/services/recordings/service"
)

type neutralReplayCompositionLedger struct {
	recordings.Ledger
}

// TestInjectBundleComposesRecordingsNeutralReplayThroughWireFactory proves the
// Wire recordings factory wires neutral replay through the singular Recordings
// root rather than a second peer authority.
func TestInjectBundleComposesRecordingsNeutralReplayThroughWireFactory(t *testing.T) {
	t.Parallel()

	if _, err := InjectBundle(t.Context(), serviceedges.Edges{}); err != nil {
		t.Fatalf("InjectBundle() error = %v", err)
	}

	service := recordingsservice.NewService(
		&neutralReplayCompositionLedger{},
		recordingsservice.NewProjectionService(),
	)
	recording := finalizedNeutralReplayRecording(t, service)
	assertNeutralReplayTypedFailures(t, service, recording)
}

func finalizedNeutralReplayRecording(
	t *testing.T,
	service recordings.Service,
) recordings.ReplayRecordingFacts {
	t.Helper()

	scope := recordings.CanonicalEventScope{FactorySessionID: "session-root-neutral-replay"}
	bound, err := service.BindRecording(recordings.BindRecordingRequest{
		Artifact: "artifact:root-neutral-replay",
		Scope:    scope,
	})
	if err != nil {
		t.Fatalf("BindRecording: %v", err)
	}
	for index, event := range []recordings.CanonicalEvent{
		neutralReplayCompositionEvent("root-neutral-replay-1", 0, scope),
		neutralReplayCompositionEvent("root-neutral-replay-2", 1, scope),
	} {
		if _, err := service.RecordRecordingEvent(recordings.RecordRecordingEventRequest{
			RecordingID: bound.Status.RecordingID,
			Event:       event,
		}); err != nil {
			t.Fatalf("RecordRecordingEvent[%d]: %v", index, err)
		}
	}
	if _, err := service.FinishRecording(recordings.FinishRecordingRequest{
		RecordingID: bound.Status.RecordingID,
		FinishedAt:  time.Unix(1_700_000_300, 0).UTC(),
	}); err != nil {
		t.Fatalf("FinishRecording: %v", err)
	}
	loaded, err := service.LoadReplayRecording(recordings.LoadReplayRecordingRequest{
		RecordingID: bound.Status.RecordingID,
	})
	if err != nil {
		t.Fatalf("LoadReplayRecording: %v", err)
	}
	planned, err := service.CreateReplayPlan(recordings.CreateReplayPlanRequest{
		SchemaVersion: recordings.ReplayPlanSchemaV1,
		Timing:        recordings.ReplayTimingOrderOnly,
		Recording:     loaded.Recording,
	})
	if err != nil {
		t.Fatalf("CreateReplayPlan: %v", err)
	}
	var observed recordings.ObserveReplayResult
	for step := 0; step < len(loaded.Recording.Events); step++ {
		observed, err = service.ObserveReplay(recordings.ObserveReplayRequest{
			Plan: planned.Plan.Handle,
		})
		if err != nil {
			t.Fatalf("ObserveReplay step %d: %v", step, err)
		}
	}
	if observed.Observation.Kind != recordings.ReplayCompleted {
		t.Fatalf("ObserveReplay completion = %#v, want COMPLETED", observed.Observation)
	}
	return loaded.Recording
}

func assertNeutralReplayTypedFailures(
	t *testing.T,
	service recordings.Service,
	recording recordings.ReplayRecordingFacts,
) {
	t.Helper()

	if _, err := service.LoadReplayRecording(recordings.LoadReplayRecordingRequest{
		RecordingID: "missing-root-neutral-replay",
	}); !errors.Is(err, recordings.ErrReplayRecordingNotFound) {
		t.Fatalf("LoadReplayRecording missing = %v, want ErrReplayRecordingNotFound", err)
	}
	if _, err := service.ObserveReplay(recordings.ObserveReplayRequest{
		Plan: "missing-root-neutral-replay-plan",
	}); !errors.Is(err, recordings.ErrReplayPlanNotFound) {
		t.Fatalf("ObserveReplay missing plan = %v, want ErrReplayPlanNotFound", err)
	}
	corrupt := recording
	corrupt.Events = append([]recordings.CanonicalEvent(nil), recording.Events...)
	corrupt.Events[1].Sequence = 9
	if _, err := service.CreateReplayPlan(recordings.CreateReplayPlanRequest{
		SchemaVersion: recordings.ReplayPlanSchemaV1,
		Timing:        recordings.ReplayTimingOrderOnly,
		Recording:     corrupt,
	}); !errors.Is(err, recordings.ErrCorruptReplayInput) {
		t.Fatalf("CreateReplayPlan corrupt = %v, want ErrCorruptReplayInput", err)
	}
	for _, err := range []error{
		recordings.ErrReplayRecordingNotFound,
		recordings.ErrReplayPlanNotFound,
		recordings.ErrCorruptReplayInput,
	} {
		assertWireBoundedReplayError(t, err)
	}
}

func assertWireBoundedReplayError(t *testing.T, err error) {
	t.Helper()
	message := err.Error()
	if len(message) > 120 {
		t.Fatalf("error message too long (%d chars): %q", len(message), message)
	}
	for _, leaked := range []string{"/pkg/", "ledger", "internal/services", "decoder"} {
		if strings.Contains(strings.ToLower(message), strings.ToLower(leaked)) {
			t.Fatalf("error leaked %q: %q", leaked, message)
		}
	}
}

func neutralReplayCompositionEvent(
	id string,
	sequence recordings.CanonicalEventSequence,
	scope recordings.CanonicalEventScope,
) recordings.CanonicalEvent {
	return recordings.CanonicalEvent{
		ID:       recordings.CanonicalEventID(id),
		Kind:     "WORK_REQUEST",
		Sequence: sequence,
		Scope:    scope,
		Cursor: recordings.CanonicalEventCursor{
			StreamGenerationID: "generation-root-neutral-replay",
			Sequence:           sequence,
		},
		FactoryTick: 1,
		RecordedAt:  time.Unix(1_700_000_000+int64(sequence), 0).UTC(),
		Payload:     `{"type":"WORK_REQUEST"}`,
	}
}
