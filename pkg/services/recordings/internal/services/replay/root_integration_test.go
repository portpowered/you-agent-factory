package recordingsreplay_test

import (
	"errors"
	"testing"
	"time"

	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"

	recordinglifecycle "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/recording_lifecycle"
	replaywire "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/replay/wire"
)

func TestReplayLoadsFinalizedFactsAndObservesOrderedProgress(t *testing.T) {
	t.Parallel()

	recording := recordings.ReplayRecordingFacts{
		RecordingID: "recording-root-replay",
		Events: []recordings.CanonicalEvent{
			rootReplayEvent("root-replay-event-1", 0),
			rootReplayEvent("root-replay-event-2", 1),
		},
	}
	finishedAt := time.Unix(1_700_000_200, 0).UTC()
	lifecycle := &snapshotLifecycle{snapshot: recordinglifecycle.Snapshot{
		Status: recordings.RecordingStatusFacts{RecordingID: recording.RecordingID, FinalizedAt: &finishedAt},
		Events: recording.Events,
	}}
	projection := &replayProjection{}
	root := replaywire.NewService(lifecycle, projection, nil, nil)
	if _, err := root.CreateReplayPlan(recordings.CreateReplayPlanRequest{
		SchemaVersion: recordings.ReplayPlanSchemaV1,
		Timing:        recordings.ReplayTimingOrderOnly,
		Recording: recordings.ReplayRecordingFacts{
			Events: recording.Events,
		},
	}); !errors.Is(err, recordings.ErrCorruptReplayInput) {
		t.Fatalf("CreateReplayPlan missing recording id = %v, want ErrCorruptReplayInput", err)
	}

	loaded, err := root.LoadReplayRecording(recordings.LoadReplayRecordingRequest{
		RecordingID: recording.RecordingID,
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
	if progress.Observation.ProcessedEvents != 1 || len(projection.events) != 1 || projection.events[0].Id != string(recording.Events[0].ID) {
		t.Fatalf("first replay observation = %#v, projection prefix = %#v", progress, projection.events)
	}
	completed, err := root.ObserveReplay(recordings.ObserveReplayRequest{Plan: planned.Plan.Handle})
	if err != nil || completed.Observation.Kind != recordings.ReplayCompleted || completed.Observation.ProcessedEvents != 2 || len(projection.events) != 2 {
		t.Fatalf("completed replay = (%#v, %v), projection prefix = %#v", completed, err, projection.events)
	}
	if projection.events[1].Id != string(recording.Events[1].ID) || completed.Observation.Through == nil || *completed.Observation.Through != recording.Events[1].Cursor {
		t.Fatalf("replay lost final event identity or cursor: events=%#v observation=%#v", projection.events, completed.Observation)
	}
	if lifecycle.selected != recording.RecordingID {
		t.Fatalf("lifecycle selection = %q, want %q", lifecycle.selected, recording.RecordingID)
	}
}

func TestReplayLoadsUnfinalizedSnapshotForResume(t *testing.T) {
	t.Parallel()

	recording := recordings.ReplayRecordingFacts{
		RecordingID: "recording-root-resume",
		Events: []recordings.CanonicalEvent{
			rootReplayEvent("root-resume-event-1", 0),
			rootReplayEvent("root-resume-event-2", 1),
		},
	}
	lifecycle := &snapshotLifecycle{snapshot: recordinglifecycle.Snapshot{
		Status: recordings.RecordingStatusFacts{RecordingID: recording.RecordingID},
		Events: recording.Events,
	}}
	root := replaywire.NewService(lifecycle, &replayProjection{}, nil, nil)
	loaded, err := root.LoadReplayRecordingForResume(recordings.LoadReplayRecordingForResumeRequest{
		RecordingID: recording.RecordingID,
	})
	if err != nil {
		t.Fatalf("LoadReplayRecordingForResume: %v", err)
	}
	if loaded.RecoveredEventCount != 2 || len(loaded.Recording.Events) != 2 || loaded.Truncated {
		t.Fatalf("resume result = %#v, want two complete events without truncation", loaded)
	}
	if _, err := root.LoadReplayRecording(recordings.LoadReplayRecordingRequest{
		RecordingID: recording.RecordingID,
	}); !errors.Is(err, recordings.ErrReplayRecordingNotFinalized) {
		t.Fatalf("neutral load error = %v, want ErrReplayRecordingNotFinalized", err)
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

// Embedding only supplies unused interface methods; any unexpected operation
// panics rather than silently constructing a collaborator or accepting a call.
type snapshotLifecycle struct {
	recordinglifecycle.Service
	snapshot recordinglifecycle.Snapshot
	selected recordings.RecordingID
}

func (lifecycle *snapshotLifecycle) Snapshot(id recordings.RecordingID) (recordinglifecycle.Snapshot, error) {
	lifecycle.selected = id
	return lifecycle.snapshot, nil
}

type replayProjection struct {
	recordings.ProjectionService
	events []recordings.FactoryEvent
}

func (projection *replayProjection) ReconstructFactoryWorldState(events []recordings.FactoryEvent, tick int) (recordings.FactoryWorldState, error) {
	projection.events = append([]recordings.FactoryEvent(nil), events...)
	return recordings.FactoryWorldState{Tick: tick}, nil
}
