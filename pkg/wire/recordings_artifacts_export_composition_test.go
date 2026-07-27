package wire

import (
	"errors"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingsservice "github.com/portpowered/infinite-you/pkg/services/recordings/service"
)

type inertRecordingsLedger struct {
	recordings.Ledger
}

// TestInjectBundleComposesRecordingsArtifactExportThroughWireFactory proves the
// Wire recordings factory wires artifact close/export/read through the singular
// Recordings root rather than a second peer authority.
func TestInjectBundleComposesRecordingsArtifactExportThroughWireFactory(t *testing.T) {
	t.Parallel()

	if _, err := InjectBundle(t.Context(), serviceedges.Edges{}); err != nil {
		t.Fatalf("InjectBundle() error = %v", err)
	}

	planner := recordings.LiveRecordingTargetPlannerFunc(
		func(recordings.LiveRecordingTargetRequest) (recordings.LiveRecordingTarget, error) {
			return recordings.LiveRecordingTarget{}, nil
		},
	)
	publication, err := recordingsservice.NewPortableArtifactPublication()
	if err != nil {
		t.Fatalf("NewPortableArtifactPublication: %v", err)
	}
	rootService := recordingsservice.NewServiceWithLifecycleEffects(
		&inertRecordingsLedger{},
		recordingsservice.NewProjectionService(),
		planner,
		nil,
		nil,
		publication,
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
	encoded, err := rootService.EncodePortableArtifact(recordings.EncodePortableArtifactRequest{
		Artifact: built.Artifact,
	})
	if err != nil || len(encoded.Payload) == 0 {
		t.Fatalf("EncodePortableArtifact = (%d bytes, %v)", len(encoded.Payload), err)
	}
	decoded, err := rootService.DecodePortableArtifact(recordings.DecodePortableArtifactRequest{
		Payload: encoded.Payload,
	})
	if err != nil {
		t.Fatalf("DecodePortableArtifact: %v", err)
	}
	if _, err := rootService.ValidatePortableArtifact(recordings.ValidatePortableArtifactRequest{
		Artifact: decoded.Artifact,
	}); err != nil {
		t.Fatalf("ValidatePortableArtifact decoded artifact: %v", err)
	}
	summarized, err := rootService.SummarizePortableArtifact(recordings.SummarizePortableArtifactRequest{
		Artifact: decoded.Artifact,
	})
	if err != nil || summarized.Summary.RecordingID != bound.Status.RecordingID {
		t.Fatalf("SummarizePortableArtifact = (%#v, %v)", summarized, err)
	}
	if _, err := rootService.DecodePortableArtifact(recordings.DecodePortableArtifactRequest{
		Payload: []byte(`{`),
	}); !errors.Is(err, recordings.ErrInvalidPortableArtifact) {
		t.Fatalf("DecodePortableArtifact malformed = %v, want ErrInvalidPortableArtifact", err)
	}
}
