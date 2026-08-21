package service_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	projectionquerywire "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/projection_query/wire"
	recordinglifecycle "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/recording_lifecycle"
	replayservice "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/replay/internal/service"
)

func TestReplayLoadPropagatesLifecycleFailures(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("lifecycle unavailable")
	lifecycle := snapshotLifecycleStub{err: sentinel}
	replay := replayservice.New(lifecycle, projectionquerywire.NewService(), nil, nil)

	if _, err := replay.LoadReplayRecording(recordings.LoadReplayRecordingRequest{
		RecordingID: "recording-lifecycle-failure",
	}); !errors.Is(err, sentinel) {
		t.Fatalf("neutral load error = %v, want lifecycle error", err)
	}
	if _, err := replay.LoadReplayRecordingForResume(recordings.LoadReplayRecordingForResumeRequest{
		RecordingID: "recording-lifecycle-failure",
	}); !errors.Is(err, sentinel) {
		t.Fatalf("resume load error = %v, want lifecycle error", err)
	}
}

func TestReplayResumeRejectsMissingArtifactSource(t *testing.T) {
	t.Parallel()

	lifecycle := snapshotLifecycleStub{
		snapshot: recordinglifecycle.Snapshot{
			Status: recordings.RecordingStatusFacts{
				RecordingID: "recording-missing-source",
				Artifact:    "  ",
			},
		},
	}
	replay := replayservice.New(
		lifecycle,
		projectionquerywire.NewService(),
		func(string) ([]byte, error) {
			t.Fatal("resume source should not be read when the artifact is missing")
			return nil, nil
		},
		func([]byte) (*factorydefinitions.FactorySnapshot, error) {
			t.Fatal("Factory snapshot should not be decoded when the artifact is missing")
			return nil, nil
		},
	)
	if _, err := replay.LoadReplayRecordingForResume(recordings.LoadReplayRecordingForResumeRequest{
		RecordingID: "recording-missing-source",
	}); err == nil || !strings.Contains(err.Error(), "source is missing") {
		t.Fatalf("missing source error = %v, want explicit source error", err)
	}
}

func TestReplayResumeRejectsCanonicalScopeMismatchAndUnsupportedKind(t *testing.T) {
	t.Parallel()

	_, lifecycle, recordingID, scope := newReplayHarness(t)
	decodeFactorySnapshot := func(data []byte) (*factorydefinitions.FactorySnapshot, error) {
		return factorydefinitions.NewFactorySnapshot(json.RawMessage(data))
	}

	t.Run("scope mismatch", func(t *testing.T) {
		replay := replayservice.New(
			lifecycle,
			projectionquerywire.NewService(),
			func(string) ([]byte, error) {
				return resumeEventStream(t, recordings.CanonicalEventScope{
					FactorySessionID: "foreign-session",
				}), nil
			},
			decodeFactorySnapshot,
		)
		if _, err := replay.LoadReplayRecordingForResume(recordings.LoadReplayRecordingForResumeRequest{
			RecordingID: recordingID,
		}); !errors.Is(err, recordings.ErrCorruptReplayInput) {
			t.Fatalf("scope mismatch error = %v, want ErrCorruptReplayInput", err)
		}
	})

	t.Run("unsupported kind", func(t *testing.T) {
		stream := strings.Replace(
			string(resumeEventStream(t, scope)),
			`"type":"WORK_REQUEST"`,
			`"type":"UNSUPPORTED_REPLAY_KIND"`,
			1,
		)
		replay := replayservice.New(
			lifecycle,
			projectionquerywire.NewService(),
			func(string) ([]byte, error) { return []byte(stream), nil },
			decodeFactorySnapshot,
		)
		if _, err := replay.LoadReplayRecordingForResume(recordings.LoadReplayRecordingForResumeRequest{
			RecordingID: recordingID,
		}); !errors.Is(err, recordings.ErrCorruptReplayInput) {
			t.Fatalf("unsupported kind error = %v, want ErrCorruptReplayInput", err)
		}
	})
}

func TestReplayObservePropagatesProjectionFailures(t *testing.T) {
	t.Parallel()

	_, lifecycle, recordingID, scope := newReplayHarness(t)
	snapshot, err := lifecycle.Snapshot(recordingID)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	replay := replayservice.New(lifecycle, failingProjectionStub{
		err: errors.New("projection unavailable"),
	}, nil, nil)
	planned, err := replay.CreateReplayPlan(recordings.CreateReplayPlanRequest{
		SchemaVersion: recordings.ReplayPlanSchemaV1,
		Timing:        recordings.ReplayTimingOrderOnly,
		Recording: recordings.ReplayRecordingFacts{
			RecordingID: recordingID,
			Scope:       scope,
			Events:      snapshot.Events,
		},
	})
	if err != nil {
		t.Fatalf("CreateReplayPlan: %v", err)
	}
	if _, err := replay.ObserveReplay(recordings.ObserveReplayRequest{
		Plan: planned.Plan.Handle,
	}); err == nil || !strings.Contains(err.Error(), "projection unavailable") {
		t.Fatalf("ObserveReplay error = %v, want projection error", err)
	}
}

type snapshotLifecycleStub struct {
	recordinglifecycle.Service
	snapshot recordinglifecycle.Snapshot
	err      error
}

func (stub snapshotLifecycleStub) Snapshot(recordings.RecordingID) (recordinglifecycle.Snapshot, error) {
	return stub.snapshot, stub.err
}

type failingProjectionStub struct {
	recordings.ProjectionService
	err error
}

func (stub failingProjectionStub) ReconstructFactoryWorldState(
	[]factorydefinitions.FactoryEvent,
	int,
) (recordings.FactoryWorldState, error) {
	return recordings.FactoryWorldState{}, stub.err
}

var _ recordinglifecycle.Service = snapshotLifecycleStub{}
var _ recordings.ProjectionService = failingProjectionStub{}
