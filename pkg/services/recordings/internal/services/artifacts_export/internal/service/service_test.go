package service_test

import (
	"errors"
	"testing"
	"time"

	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordinglifecycle "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/recording_lifecycle"
	artifactsexportservice "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/artifacts_export/internal/service"
)

type snapshotSourceFake struct {
	snapshot recordinglifecycle.Snapshot
	err      error
}

func (fake snapshotSourceFake) Snapshot(recordings.RecordingID) (recordinglifecycle.Snapshot, error) {
	if fake.err != nil {
		return recordinglifecycle.Snapshot{}, fake.err
	}
	return fake.snapshot, nil
}

func TestBuildPortableArtifactRejectsActiveRecording(t *testing.T) {
	t.Parallel()

	service := artifactsexportservice.New(snapshotSourceFake{
		snapshot: recordinglifecycle.Snapshot{
			Status: recordings.RecordingStatusFacts{
				RecordingID: "recording-1",
				State:       recordings.RecordingActive,
			},
		},
	})
	_, err := service.BuildPortableArtifact(recordings.BuildPortableArtifactRequest{
		RecordingID: "recording-1",
	})
	if !errors.Is(err, recordings.ErrPortableArtifactUnavailable) {
		t.Fatalf("BuildPortableArtifact active = %v, want ErrPortableArtifactUnavailable", err)
	}
}

func TestBuildPortableArtifactProducesValidatedArtifact(t *testing.T) {
	t.Parallel()

	finalizedAt := time.Unix(1_700_000_000, 0).UTC()
	scope := recordings.CanonicalEventScope{FactorySessionID: "session-1"}
	event := recordings.CanonicalEvent{
		ID: "event-1", Kind: "WORK_REQUEST",
		Sequence: 0,
		Scope:    scope,
		Cursor: recordings.CanonicalEventCursor{
			StreamGenerationID: "generation-1",
			Sequence:           0,
		},
		RecordedAt: time.Unix(1_700_000_000, 0).UTC(),
		Payload:    "{}",
	}
	service := artifactsexportservice.New(snapshotSourceFake{
		snapshot: recordinglifecycle.Snapshot{
			Status: recordings.RecordingStatusFacts{
				RecordingID: "recording-1",
				Artifact:    "artifact:recording-1",
				Scope:       scope,
				State:       recordings.RecordingFinalized,
				FinalizedAt: &finalizedAt,
			},
			Events: []recordings.CanonicalEvent{event},
		},
	})
	built, err := service.BuildPortableArtifact(recordings.BuildPortableArtifactRequest{
		RecordingID: "recording-1",
	})
	if err != nil {
		t.Fatalf("BuildPortableArtifact: %v", err)
	}
	if _, err := service.ValidatePortableArtifact(recordings.ValidatePortableArtifactRequest{
		Artifact: built.Artifact,
	}); err != nil {
		t.Fatalf("ValidatePortableArtifact: %v", err)
	}
	encoded, err := service.EncodePortableArtifact(recordings.EncodePortableArtifactRequest{
		Artifact: built.Artifact,
	})
	if err != nil || len(encoded.Payload) == 0 {
		t.Fatalf("EncodePortableArtifact = (%d bytes, %v)", len(encoded.Payload), err)
	}
	decoded, err := service.DecodePortableArtifact(recordings.DecodePortableArtifactRequest{
		Payload: encoded.Payload,
	})
	if err != nil {
		t.Fatalf("DecodePortableArtifact: %v", err)
	}
	if decoded.Artifact.Summary.RecordingID != "recording-1" {
		t.Fatalf("decoded summary = %#v", decoded.Artifact.Summary)
	}
}
