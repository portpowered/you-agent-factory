package artifactsexport_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingsinternal "github.com/portpowered/infinite-you/pkg/services/recordings/internal"
)

type unusedLedger struct {
	recordings.Ledger
}

func TestAcceptedRecordingsRootUsesPrivateArtifactsExport(t *testing.T) {
	t.Parallel()

	root := recordingsinternal.NewService(
		&unusedLedger{},
		recordingsinternal.NewProjectionService(),
	)
	scope := recordings.CanonicalEventScope{FactorySessionID: "session-export-root"}
	bound, err := root.BindRecording(recordings.BindRecordingRequest{
		RecordingID: "recording-export-root",
		Artifact:    "artifact:export-root",
		Scope:       scope,
	})
	if err != nil {
		t.Fatalf("BindRecording: %v", err)
	}
	event := recordings.CanonicalEvent{
		ID: "export-root-event", Kind: "WORK_REQUEST",
		Scope:      scope,
		RecordedAt: time.Unix(1_700_000_000, 0).UTC(),
		Payload:    "{}",
		Cursor: recordings.CanonicalEventCursor{
			StreamGenerationID: "generation-export-root",
		},
	}
	if _, err := root.RecordRecordingEvent(recordings.RecordRecordingEventRequest{
		RecordingID: bound.Status.RecordingID,
		Event:       event,
	}); err != nil {
		t.Fatalf("RecordRecordingEvent: %v", err)
	}
	if _, err := root.BuildPortableArtifact(recordings.BuildPortableArtifactRequest{
		RecordingID: bound.Status.RecordingID,
	}); !errors.Is(err, recordings.ErrPortableArtifactUnavailable) {
		t.Fatalf("BuildPortableArtifact active = %v, want ErrPortableArtifactUnavailable", err)
	}
	if _, err := root.FinishRecording(recordings.FinishRecordingRequest{
		RecordingID: bound.Status.RecordingID,
		FinishedAt:  time.Unix(1_700_000_001, 0).UTC(),
	}); err != nil {
		t.Fatalf("FinishRecording: %v", err)
	}
	built, err := root.BuildPortableArtifact(recordings.BuildPortableArtifactRequest{
		RecordingID: bound.Status.RecordingID,
	})
	if err != nil {
		t.Fatalf("BuildPortableArtifact: %v", err)
	}
	if _, err := root.ValidatePortableArtifact(recordings.ValidatePortableArtifactRequest{
		Artifact: built.Artifact,
	}); err != nil {
		t.Fatalf("ValidatePortableArtifact: %v", err)
	}
	encoded, err := root.EncodePortableArtifact(recordings.EncodePortableArtifactRequest{
		Artifact: built.Artifact,
	})
	if err != nil || len(encoded.Payload) == 0 {
		t.Fatalf("EncodePortableArtifact = (%d bytes, %v)", len(encoded.Payload), err)
	}
	if _, err := root.DecodePortableArtifact(recordings.DecodePortableArtifactRequest{
		Payload: []byte(`{`),
	}); !errors.Is(err, recordings.ErrInvalidPortableArtifact) {
		t.Fatalf("DecodePortableArtifact malformed = %v, want ErrInvalidPortableArtifact", err)
	}
}

func TestRecordingsRootPortableExportRoundTripOmitsPrivateStorage(t *testing.T) {
	t.Parallel()

	const (
		privateServiceTarget = "/private/ledger/storage/recording-internal.json"
		reportedReference    = "artifact:reported-export"
	)
	planner := recordings.LiveRecordingTargetPlannerFunc(
		func(recordings.LiveRecordingTargetRequest) (recordings.LiveRecordingTarget, error) {
			return recordings.LiveRecordingTarget{
				ServicePath:  privateServiceTarget,
				ReportedPath: reportedReference,
			}, nil
		},
	)
	root := recordingsinternal.NewService(
		&unusedLedger{},
		recordingsinternal.NewProjectionService(),
		planner,
	)
	scope := recordings.CanonicalEventScope{FactorySessionID: "session-round-trip"}
	started, err := root.StartRecording(recordings.StartRecordingRequest{
		Enabled:     true,
		RecordingID: "recording-round-trip",
		Scope:       scope,
		Target: recordings.RecordingTargetRequest{
			HomeDir:           "home/operator",
			ReportedSessionID: "session-round-trip",
		},
	})
	if err != nil {
		t.Fatalf("StartRecording: %v", err)
	}
	event := recordings.CanonicalEvent{
		ID: "round-trip-event", Kind: "WORK_REQUEST",
		Scope:      scope,
		RecordedAt: time.Unix(1_700_000_000, 0).UTC(),
		Payload:    `{"public":true}`,
		Cursor: recordings.CanonicalEventCursor{
			StreamGenerationID: "generation-round-trip",
		},
	}
	if _, err := root.RecordRecordingEvent(recordings.RecordRecordingEventRequest{
		RecordingID: started.Status.RecordingID,
		Event:       event,
	}); err != nil {
		t.Fatalf("RecordRecordingEvent: %v", err)
	}
	if _, err := root.FinishRecording(recordings.FinishRecordingRequest{
		RecordingID: started.Status.RecordingID,
		FinishedAt:  time.Unix(1_700_000_001, 0).UTC(),
	}); err != nil {
		t.Fatalf("FinishRecording: %v", err)
	}
	built, err := root.BuildPortableArtifact(recordings.BuildPortableArtifactRequest{
		RecordingID: started.Status.RecordingID,
	})
	if err != nil {
		t.Fatalf("BuildPortableArtifact: %v", err)
	}
	if built.Artifact.SchemaVersion != recordings.PortableArtifactSchemaV1 ||
		built.Artifact.Summary.Reference != recordings.RecordingArtifactReference(reportedReference) ||
		built.Artifact.Summary.EventCount != 1 ||
		!built.Artifact.Summary.Available {
		t.Fatalf("portable artifact document = %#v", built.Artifact)
	}
	validated, err := root.ValidatePortableArtifact(recordings.ValidatePortableArtifactRequest{
		Artifact: built.Artifact,
	})
	if err != nil {
		t.Fatalf("ValidatePortableArtifact: %v", err)
	}
	if !reflect.DeepEqual(validated.Summary, built.Artifact.Summary) {
		t.Fatalf("validated summary = %#v, want %#v", validated.Summary, built.Artifact.Summary)
	}
	encoded, err := root.EncodePortableArtifact(recordings.EncodePortableArtifactRequest{
		Artifact: built.Artifact,
	})
	if err != nil || len(encoded.Payload) == 0 {
		t.Fatalf("EncodePortableArtifact = (%d bytes, %v)", len(encoded.Payload), err)
	}
	payloadText := string(encoded.Payload)
	if strings.Contains(payloadText, privateServiceTarget) {
		t.Fatalf("encoded payload leaked private service target:\n%s", payloadText)
	}
	decoded, err := root.DecodePortableArtifact(recordings.DecodePortableArtifactRequest{
		Payload: encoded.Payload,
	})
	if err != nil {
		t.Fatalf("DecodePortableArtifact: %v", err)
	}
	if decoded.Artifact.Integrity != built.Artifact.Integrity ||
		decoded.Artifact.Summary.RecordingID != built.Artifact.Summary.RecordingID ||
		decoded.Artifact.Summary.EventCount != built.Artifact.Summary.EventCount {
		t.Fatalf("decoded public facts = %#v, want %#v", decoded.Artifact, built.Artifact)
	}
	summarized, err := root.SummarizePortableArtifact(recordings.SummarizePortableArtifactRequest{
		Artifact: decoded.Artifact,
	})
	if err != nil {
		t.Fatalf("SummarizePortableArtifact: %v", err)
	}
	if !reflect.DeepEqual(summarized.Summary, built.Artifact.Summary) {
		t.Fatalf("summarized summary = %#v, want %#v", summarized.Summary, built.Artifact.Summary)
	}
}

func TestRecordingsRootFailedExportLeavesNoReadablePublicArtifact(t *testing.T) {
	t.Parallel()

	destination := filepath.Join(t.TempDir(), "destination-is-directory")
	if err := os.Mkdir(destination, 0o700); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	root := recordingsinternal.NewService(
		&unusedLedger{},
		recordingsinternal.NewProjectionService(),
	)
	scope := recordings.CanonicalEventScope{FactorySessionID: "session-failed-root-export"}
	bound, err := root.BindRecording(recordings.BindRecordingRequest{
		RecordingID: "recording-failed-root-export",
		Artifact:    recordings.RecordingArtifactReference(destination),
		Scope:       scope,
	})
	if err != nil {
		t.Fatalf("BindRecording: %v", err)
	}
	if _, err := root.FinishRecording(recordings.FinishRecordingRequest{
		RecordingID: bound.Status.RecordingID,
		FinishedAt:  time.Unix(1_700_000_001, 0).UTC(),
	}); err != nil {
		t.Fatalf("FinishRecording: %v", err)
	}
	_, err = root.ExportPortableArtifact(context.Background(), recordings.ExportPortableArtifactRequest{
		RecordingID: bound.Status.RecordingID,
	})
	if !errors.Is(err, recordings.ErrPortableArtifactExportFailed) {
		t.Fatalf("ExportPortableArtifact = %v, want ErrPortableArtifactExportFailed", err)
	}
	info, err := os.Stat(destination)
	if err != nil || !info.IsDir() {
		t.Fatalf("public destination stat = %v, want existing directory", err)
	}
	entries, err := os.ReadDir(destination)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".tmp") {
			t.Fatalf("temporary portable artifact remained after failure: %s", entry.Name())
		}
	}
	_, err = root.ReadPortableArtifact(context.Background(), recordings.ReadPortableArtifactRequest{
		RecordingID: bound.Status.RecordingID,
		Reference:   recordings.RecordingArtifactReference(destination),
	})
	if !errors.Is(err, recordings.ErrPortableArtifactUnavailable) &&
		!errors.Is(err, recordings.ErrInvalidPortableArtifact) {
		t.Fatalf("ReadPortableArtifact after failed export = %v, want unavailable or invalid", err)
	}
	if _, err := os.Stat(filepath.Join(destination, filepath.Base(destination))); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("nested publish path stat = %v, want not exist", err)
	}
}

func TestRecordingsRootReadPortableArtifactRejectsMissingAndForeignHandles(t *testing.T) {
	t.Parallel()

	root := recordingsinternal.NewService(
		&unusedLedger{},
		recordingsinternal.NewProjectionService(),
	)
	scope := recordings.CanonicalEventScope{FactorySessionID: "session-root-read-guards"}
	owner, err := root.BindRecording(recordings.BindRecordingRequest{
		RecordingID: "recording-root-owner",
		Artifact:    "artifact:root-owner",
		Scope:       scope,
	})
	if err != nil {
		t.Fatalf("BindRecording owner: %v", err)
	}
	other, err := root.BindRecording(recordings.BindRecordingRequest{
		RecordingID: "recording-root-other",
		Artifact:    "artifact:root-other",
		Scope:       scope,
	})
	if err != nil {
		t.Fatalf("BindRecording other: %v", err)
	}
	if _, err := root.FinishRecording(recordings.FinishRecordingRequest{
		RecordingID: owner.Status.RecordingID,
		FinishedAt:  time.Unix(1_700_000_001, 0).UTC(),
	}); err != nil {
		t.Fatalf("FinishRecording owner: %v", err)
	}
	if _, err := root.FinishRecording(recordings.FinishRecordingRequest{
		RecordingID: other.Status.RecordingID,
		FinishedAt:  time.Unix(1_700_000_001, 0).UTC(),
	}); err != nil {
		t.Fatalf("FinishRecording other: %v", err)
	}

	_, err = root.ReadPortableArtifact(context.Background(), recordings.ReadPortableArtifactRequest{
		RecordingID: owner.Status.RecordingID,
		Reference:   owner.Status.Artifact,
	})
	if !errors.Is(err, recordings.ErrPortableArtifactUnavailable) {
		t.Fatalf("missing published artifact = %v, want ErrPortableArtifactUnavailable", err)
	}

	_, err = root.ReadPortableArtifact(context.Background(), recordings.ReadPortableArtifactRequest{
		RecordingID: other.Status.RecordingID,
		Reference:   owner.Status.Artifact,
	})
	if !errors.Is(err, recordings.ErrForeignPortableArtifact) {
		t.Fatalf("foreign handle = %v, want ErrForeignPortableArtifact", err)
	}
	if strings.Contains(err.Error(), string(owner.Status.Artifact)) {
		t.Fatalf("foreign handle error leaked owner reference: %v", err)
	}
}

func TestRecordingsRootExportCancellationLeavesNoReadableArtifact(t *testing.T) {
	t.Parallel()

	root := recordingsinternal.NewService(
		&unusedLedger{},
		recordingsinternal.NewProjectionService(),
	)
	scope := recordings.CanonicalEventScope{FactorySessionID: "session-root-cancel-export"}
	bound, err := root.BindRecording(recordings.BindRecordingRequest{
		RecordingID: "recording-root-cancel-export",
		Artifact:    "artifact:root-cancel-export",
		Scope:       scope,
	})
	if err != nil {
		t.Fatalf("BindRecording: %v", err)
	}
	if _, err := root.FinishRecording(recordings.FinishRecordingRequest{
		RecordingID: bound.Status.RecordingID,
		FinishedAt:  time.Unix(1_700_000_001, 0).UTC(),
	}); err != nil {
		t.Fatalf("FinishRecording: %v", err)
	}
	cancelCause := errors.New("operator stopped export")
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(cancelCause)
	_, err = root.ExportPortableArtifact(ctx, recordings.ExportPortableArtifactRequest{
		RecordingID: bound.Status.RecordingID,
	})
	if !errors.Is(err, recordings.ErrPortableArtifactCancelled) ||
		!errors.Is(err, context.Canceled) ||
		!errors.Is(err, cancelCause) {
		t.Fatalf("pre-cancelled root export = %v, want typed cancellation", err)
	}
	_, err = root.ReadPortableArtifact(context.Background(), recordings.ReadPortableArtifactRequest{
		RecordingID: bound.Status.RecordingID,
		Reference:   bound.Status.Artifact,
	})
	if !errors.Is(err, recordings.ErrPortableArtifactUnavailable) {
		t.Fatalf("ReadPortableArtifact after cancelled export = %v, want ErrPortableArtifactUnavailable", err)
	}
}

func TestRecordingsRootReadPortableArtifactRejectsCancelledContext(t *testing.T) {
	t.Parallel()

	root := recordingsinternal.NewService(
		&unusedLedger{},
		recordingsinternal.NewProjectionService(),
	)
	scope := recordings.CanonicalEventScope{FactorySessionID: "session-root-cancel-read"}
	bound, err := root.BindRecording(recordings.BindRecordingRequest{
		RecordingID: "recording-root-cancel-read",
		Artifact:    "artifact:root-cancel-read",
		Scope:       scope,
	})
	if err != nil {
		t.Fatalf("BindRecording: %v", err)
	}
	if _, err := root.FinishRecording(recordings.FinishRecordingRequest{
		RecordingID: bound.Status.RecordingID,
		FinishedAt:  time.Unix(1_700_000_001, 0).UTC(),
	}); err != nil {
		t.Fatalf("FinishRecording: %v", err)
	}
	cancelCause := errors.New("operator stopped read")
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(cancelCause)
	_, err = root.ReadPortableArtifact(ctx, recordings.ReadPortableArtifactRequest{
		RecordingID: bound.Status.RecordingID,
		Reference:   bound.Status.Artifact,
	})
	if !errors.Is(err, recordings.ErrPortableArtifactCancelled) ||
		!errors.Is(err, context.Canceled) ||
		!errors.Is(err, cancelCause) {
		t.Fatalf("pre-cancelled root read = %v, want typed cancellation", err)
	}
}
