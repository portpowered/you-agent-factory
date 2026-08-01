package wire

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingswire "github.com/portpowered/infinite-you/pkg/services/recordings/wire"
)

type inertRecordingsLedger struct {
	recordings.Ledger
}

// TestInjectBundleComposesRecordingsArtifactExportThroughWireFactory proves the
// Wire recordings factory wires artifact close/export/read through the singular
// Recordings root rather than a second peer authority.
func TestInjectBundleComposesRecordingsArtifactExportThroughWireFactory(t *testing.T) {
	t.Parallel()

	var readCalls atomic.Int32
	edges := serviceedges.Edges{
		RecordingReadFile: func(path string) ([]byte, error) {
			readCalls.Add(1)
			return os.ReadFile(path)
		},
	}
	if _, err := InjectBundle(t.Context(), edges); err != nil {
		t.Fatalf("InjectBundle() error = %v", err)
	}

	factory := provideRecordingsFactory(
		edges,
		provideLiveRecordingTargetPlanner(),
		platformreplay.Local{},
	)
	rootService := factory(
		&inertRecordingsLedger{},
		recordingswire.NewProjectionService(),
	)
	if rootService == nil {
		t.Fatal("provideRecordingsFactory() returned nil service")
	}
	scope := recordings.CanonicalEventScope{FactorySessionID: "session-build-process"}
	artifactPath := filepath.Join(t.TempDir(), "build-process.json")
	bound, err := rootService.BindRecording(recordings.BindRecordingRequest{
		RecordingID: "recording-build-process",
		Artifact:    recordings.RecordingArtifactReference(artifactPath),
		Scope:       scope,
	})
	if err != nil {
		t.Fatalf("BindRecording: %v", err)
	}
	runRequest, err := wireCompositionRunRequestEvent(
		"build-process-run-request",
		0,
		scope,
		time.Unix(1_700_000_000, 0).UTC(),
		"generation-build-process",
	)
	if err != nil {
		t.Fatalf("wireCompositionRunRequestEvent: %v", err)
	}
	if _, err := rootService.RecordRecordingEvent(recordings.RecordRecordingEventRequest{
		RecordingID: bound.Status.RecordingID,
		Event:       runRequest,
	}); err != nil {
		t.Fatalf("RecordRecordingEvent run request: %v", err)
	}
	if _, err := rootService.RecordRecordingEvent(recordings.RecordRecordingEventRequest{
		RecordingID: bound.Status.RecordingID,
		Event: recordings.CanonicalEvent{
			ID:         "artifact-export-event",
			Kind:       "WORK_REQUEST",
			Sequence:   1,
			Scope:      scope,
			RecordedAt: time.Unix(1_700_000_001, 0).UTC(),
			Payload:    "{}",
			Cursor: recordings.CanonicalEventCursor{
				StreamGenerationID: "generation-build-process",
				Sequence:           1,
			},
		},
	}); err != nil {
		t.Fatalf("RecordRecordingEvent: %v", err)
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
	exported, err := rootService.ExportPortableArtifact(context.Background(), recordings.ExportPortableArtifactRequest{
		RecordingID: bound.Status.RecordingID,
	})
	if err != nil {
		t.Fatalf("ExportPortableArtifact: %v", err)
	}
	read, err := rootService.ReadPortableArtifact(context.Background(), recordings.ReadPortableArtifactRequest{
		RecordingID: bound.Status.RecordingID,
		Reference:   exported.Reference,
	})
	if err != nil {
		t.Fatalf("ReadPortableArtifact: %v", err)
	}
	if read.Artifact.Summary.RecordingID != bound.Status.RecordingID {
		t.Fatalf("ReadPortableArtifact recording = %q, want %q", read.Artifact.Summary.RecordingID, bound.Status.RecordingID)
	}
	if got := readCalls.Load(); got == 0 {
		t.Fatal("Recordings read-file edge was not used by portable artifact read")
	}
	if _, err := rootService.DecodePortableArtifact(recordings.DecodePortableArtifactRequest{
		Payload: []byte(`{`),
	}); !errors.Is(err, recordings.ErrInvalidPortableArtifact) {
		t.Fatalf("DecodePortableArtifact malformed = %v, want ErrInvalidPortableArtifact", err)
	}
}
