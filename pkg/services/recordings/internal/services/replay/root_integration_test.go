package recordingsreplay_test

import (
	"errors"
	"testing"
	"time"

	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingsservice "github.com/portpowered/infinite-you/pkg/services/recordings/service"
)

type unusedLedger struct {
	recordings.Ledger
}

func TestAcceptedRecordingsRootUsesPrivateReplay(t *testing.T) {
	t.Parallel()

	root := recordingsservice.NewService(
		&unusedLedger{},
		recordingsservice.NewProjectionService(),
	)
	recording := recordings.ReplayRecordingFacts{
		RecordingID: "recording-root-replay",
		Events: []recordings.CanonicalEvent{
			rootReplayEvent("root-replay-event-1", 0),
			rootReplayEvent("root-replay-event-2", 1),
		},
	}
	if _, err := root.CreateReplayPlan(recordings.CreateReplayPlanRequest{
		SchemaVersion: recordings.ReplayPlanSchemaV1,
		Timing:        recordings.ReplayTimingOrderOnly,
		Recording: recordings.ReplayRecordingFacts{
			Events: recording.Events,
		},
	}); !errors.Is(err, recordings.ErrCorruptReplayInput) {
		t.Fatalf("CreateReplayPlan missing recording id = %v, want ErrCorruptReplayInput", err)
	}

	bound, err := root.BindRecording(recordings.BindRecordingRequest{
		Artifact: "artifact:root-replay",
	})
	if err != nil {
		t.Fatalf("BindRecording: %v", err)
	}
	for index, event := range recording.Events {
		if _, err := root.RecordRecordingEvent(recordings.RecordRecordingEventRequest{
			RecordingID: bound.Status.RecordingID,
			Event:       event,
		}); err != nil {
			t.Fatalf("RecordRecordingEvent[%d]: %v", index, err)
		}
	}
	if _, err := root.FinishRecording(recordings.FinishRecordingRequest{
		RecordingID: bound.Status.RecordingID,
		FinishedAt:  time.Unix(1_700_000_200, 0).UTC(),
	}); err != nil {
		t.Fatalf("FinishRecording: %v", err)
	}

	loaded, err := root.LoadReplayRecording(recordings.LoadReplayRecordingRequest{
		RecordingID: bound.Status.RecordingID,
	})
	if err != nil {
		t.Fatalf("LoadReplayRecording: %v", err)
	}
	planned, err := root.CreateReplayPlan(recordings.CreateReplayPlanRequest{
		SchemaVersion: recordings.ReplayPlanSchemaV1,
		Timing:        recordings.ReplayTimingOrderOnly,
		Recording:     loaded.Recording,
	})
	if err != nil {
		t.Fatalf("CreateReplayPlan: %v", err)
	}
	if _, err := root.ObserveReplay(recordings.ObserveReplayRequest{
		Plan: "missing",
	}); !errors.Is(err, recordings.ErrReplayPlanNotFound) {
		t.Fatalf("ObserveReplay missing plan = %v, want ErrReplayPlanNotFound", err)
	}
	progress, err := root.ObserveReplay(recordings.ObserveReplayRequest{
		Plan: planned.Plan.Handle,
	})
	if err != nil || progress.Observation.Kind != recordings.ReplayProgress {
		t.Fatalf("ObserveReplay progress = (%#v, %v)", progress, err)
	}
}

func rootReplayEvent(id string, sequence recordings.CanonicalEventSequence) recordings.CanonicalEvent {
	return recordings.CanonicalEvent{
		ID:       recordings.CanonicalEventID(id),
		Kind:     "WORK_REQUEST",
		Sequence: sequence,
		Cursor: recordings.CanonicalEventCursor{
			StreamGenerationID: "generation-root-replay",
			Sequence:           sequence,
		},
		FactoryTick: 1,
		RecordedAt:  time.Unix(1_700_000_000, 0).UTC(),
		Payload:     `{"type":"WORK_REQUEST"}`,
	}
}
