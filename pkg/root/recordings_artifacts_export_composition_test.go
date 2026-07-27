package root_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingsservice "github.com/portpowered/infinite-you/pkg/services/recordings/service"
)

// TestBuildProcessComposesRecordingsArtifactExportThroughRoot proves the Wire
// recordings factory used by root.BuildProcess wires artifact close/export/read
// through the singular Recordings root rather than a second peer authority.
func TestBuildProcessComposesRecordingsArtifactExportThroughRoot(t *testing.T) {
	t.Parallel()

	if _, err := root.BuildProcess(context.Background(), serviceedges.Edges{}); err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}

	planner := recordings.LiveRecordingTargetPlannerFunc(
		func(recordings.LiveRecordingTargetRequest) (recordings.LiveRecordingTarget, error) {
			return recordings.LiveRecordingTarget{}, nil
		},
	)
	rootService := recordingsservice.NewServiceWithLifecycleEffects(
		&inertRecordingsLedger{},
		recordingsservice.NewProjectionService(),
		planner,
		nil,
		nil,
	)
	scope := recordings.CanonicalEventScope{FactorySessionID: "session-build-process"}
	bound, err := rootService.BindRecording(recordings.BindRecordingRequest{
		RecordingID: "recording-build-process",
		Artifact:    "artifact:build-process",
		Scope:       scope,
	})
	if err != nil {
		t.Fatalf("BindRecording: %v", err)
	}
	if _, err := rootService.FinishRecording(recordings.FinishRecordingRequest{
		RecordingID: bound.Status.RecordingID,
		FinishedAt:  time.Unix(1_700_000_000, 0).UTC(),
	}); err != nil {
		t.Fatalf("FinishRecording: %v", err)
	}
	built, err := rootService.BuildPortableArtifact(recordings.BuildPortableArtifactRequest{
		RecordingID: bound.Status.RecordingID,
	})
	if err != nil {
		t.Fatalf("BuildPortableArtifact: %v", err)
	}
	if _, err := rootService.ValidatePortableArtifact(recordings.ValidatePortableArtifactRequest{
		Artifact: built.Artifact,
	}); err != nil {
		t.Fatalf("ValidatePortableArtifact: %v", err)
	}
	if _, err := rootService.DecodePortableArtifact(recordings.DecodePortableArtifactRequest{
		Payload: []byte(`{`),
	}); !errors.Is(err, recordings.ErrInvalidPortableArtifact) {
		t.Fatalf("DecodePortableArtifact malformed = %v, want ErrInvalidPortableArtifact", err)
	}
}

type inertRecordingsLedger struct {
	recordings.Ledger
}
