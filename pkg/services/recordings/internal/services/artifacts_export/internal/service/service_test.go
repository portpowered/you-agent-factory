package service_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	artifactsexportservice "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/artifacts_export/internal/service"
	recordinglifecycle "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/recording_lifecycle"
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
	}, nil)
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
	}, nil)
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
	assertPortableArtifactRoundTrip(t, service, built.Artifact, encoded.Payload, nil)
}

func TestBuildPortableArtifactRoundTripOmitsPrivateServiceTarget(t *testing.T) {
	t.Parallel()

	const (
		privateServiceTarget = "/private/ledger/storage/recording-internal.json"
		reportedReference    = "artifact:reported-export"
	)
	finalizedAt := time.Unix(1_700_000_000, 0).UTC()
	scope := recordings.CanonicalEventScope{FactorySessionID: "session-private-export"}
	event := recordings.CanonicalEvent{
		ID: "event-private-export", Kind: "WORK_REQUEST",
		Sequence: 0,
		Scope:    scope,
		Cursor: recordings.CanonicalEventCursor{
			StreamGenerationID: "generation-private-export",
			Sequence:           0,
		},
		RecordedAt: time.Unix(1_700_000_000, 0).UTC(),
		Payload:    `{"public":true}`,
	}
	service := artifactsexportservice.New(snapshotSourceFake{
		snapshot: recordinglifecycle.Snapshot{
			Status: recordings.RecordingStatusFacts{
				RecordingID: "recording-private-export",
				Artifact:    recordings.RecordingArtifactReference(reportedReference),
				Scope:       scope,
				State:       recordings.RecordingFinalized,
				FinalizedAt: &finalizedAt,
			},
			Events: []recordings.CanonicalEvent{event},
		},
	}, nil)
	built, err := service.BuildPortableArtifact(recordings.BuildPortableArtifactRequest{
		RecordingID: "recording-private-export",
	})
	if err != nil {
		t.Fatalf("BuildPortableArtifact: %v", err)
	}
	if built.Artifact.Summary.Reference != recordings.RecordingArtifactReference(reportedReference) {
		t.Fatalf("summary reference = %q, want reported reference %q", built.Artifact.Summary.Reference, reportedReference)
	}
	encoded, err := service.EncodePortableArtifact(recordings.EncodePortableArtifactRequest{
		Artifact: built.Artifact,
	})
	if err != nil {
		t.Fatalf("EncodePortableArtifact: %v", err)
	}
	assertPortableArtifactRoundTrip(
		t,
		service,
		built.Artifact,
		encoded.Payload,
		[]string{privateServiceTarget, "__factory_session_id__"},
	)
}

func TestExportPortableArtifactFailedPublishLeavesNoPartialPublicArtifact(t *testing.T) {
	t.Parallel()

	finalizedAt := time.Unix(1_700_000_000, 0).UTC()
	scope := recordings.CanonicalEventScope{FactorySessionID: "session-failed-export"}
	event := recordings.CanonicalEvent{
		ID: "event-failed-export", Kind: "WORK_REQUEST",
		Sequence: 0,
		Scope:    scope,
		Cursor: recordings.CanonicalEventCursor{
			StreamGenerationID: "generation-failed-export",
			Sequence:           0,
		},
		RecordedAt: time.Unix(1_700_000_000, 0).UTC(),
		Payload:    "{}",
	}
	destination := filepath.Join(t.TempDir(), "public-export.json")
	publication, err := artifactsexportservice.NewPublication(
		os.MkdirAll,
		func(dir, pattern string) (artifactsexportservice.PublicationTemporaryFile, error) {
			return os.CreateTemp(dir, pattern)
		},
		os.Remove,
		func(string, string) error { return errors.New("publish blocked") },
		os.ReadFile,
	)
	if err != nil {
		t.Fatalf("NewPublication: %v", err)
	}
	service := artifactsexportservice.New(snapshotSourceFake{
		snapshot: recordinglifecycle.Snapshot{
			Status: recordings.RecordingStatusFacts{
				RecordingID: "recording-failed-export",
				Artifact:    recordings.RecordingArtifactReference(destination),
				Scope:       scope,
				State:       recordings.RecordingFinalized,
				FinalizedAt: &finalizedAt,
			},
			Events: []recordings.CanonicalEvent{event},
		},
	}, publication)
	_, err = service.ExportPortableArtifact(recordings.ExportPortableArtifactRequest{
		RecordingID: "recording-failed-export",
	})
	if !errors.Is(err, recordings.ErrPortableArtifactExportFailed) {
		t.Fatalf("ExportPortableArtifact = %v, want ErrPortableArtifactExportFailed", err)
	}
	if _, err := os.Stat(destination); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("public destination stat = %v, want not exist", err)
	}
	entries, err := os.ReadDir(filepath.Dir(destination))
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".tmp") {
			t.Fatalf("temporary portable artifact remained after failure: %s", entry.Name())
		}
	}
	_, err = service.ReadPortableArtifact(recordings.ReadPortableArtifactRequest{
		RecordingID: "recording-failed-export",
		Reference:   recordings.RecordingArtifactReference(destination),
	})
	if !errors.Is(err, recordings.ErrPortableArtifactUnavailable) {
		t.Fatalf("ReadPortableArtifact after failed export = %v, want ErrPortableArtifactUnavailable", err)
	}
}

func TestExportPortableArtifactPublishesCompleteReadableArtifact(t *testing.T) {
	t.Parallel()

	finalizedAt := time.Unix(1_700_000_000, 0).UTC()
	scope := recordings.CanonicalEventScope{FactorySessionID: "session-export-success"}
	event := recordings.CanonicalEvent{
		ID: "event-export-success", Kind: "WORK_REQUEST",
		Sequence: 0,
		Scope:    scope,
		Cursor: recordings.CanonicalEventCursor{
			StreamGenerationID: "generation-export-success",
			Sequence:           0,
		},
		RecordedAt: time.Unix(1_700_000_000, 0).UTC(),
		Payload:    "{}",
	}
	destination := filepath.Join(t.TempDir(), "public-export.json")
	publication, err := artifactsexportservice.NewOSPublication()
	if err != nil {
		t.Fatalf("NewOSPublication: %v", err)
	}
	service := artifactsexportservice.New(snapshotSourceFake{
		snapshot: recordinglifecycle.Snapshot{
			Status: recordings.RecordingStatusFacts{
				RecordingID: "recording-export-success",
				Artifact:    recordings.RecordingArtifactReference(destination),
				Scope:       scope,
				State:       recordings.RecordingFinalized,
				FinalizedAt: &finalizedAt,
			},
			Events: []recordings.CanonicalEvent{event},
		},
	}, publication)
	exported, err := service.ExportPortableArtifact(recordings.ExportPortableArtifactRequest{
		RecordingID: "recording-export-success",
	})
	if err != nil {
		t.Fatalf("ExportPortableArtifact: %v", err)
	}
	if exported.Reference != recordings.RecordingArtifactReference(destination) {
		t.Fatalf("export reference = %q, want %q", exported.Reference, destination)
	}
	read, err := service.ReadPortableArtifact(recordings.ReadPortableArtifactRequest{
		RecordingID: "recording-export-success",
		Reference:   exported.Reference,
	})
	if err != nil {
		t.Fatalf("ReadPortableArtifact: %v", err)
	}
	if !reflect.DeepEqual(read.Artifact.Summary, exported.Artifact.Summary) {
		t.Fatalf("read summary = %#v, want %#v", read.Artifact.Summary, exported.Artifact.Summary)
	}
}

func TestReadPortableArtifactRejectsMissingRecordingAndHandle(t *testing.T) {
	t.Parallel()

	finalizedAt := time.Unix(1_700_000_000, 0).UTC()
	scope := recordings.CanonicalEventScope{FactorySessionID: "session-missing-read"}
	snapshot := recordinglifecycle.Snapshot{
		Status: recordings.RecordingStatusFacts{
			RecordingID: "recording-missing-read",
			Artifact:    "artifact:missing-read",
			Scope:       scope,
			State:       recordings.RecordingFinalized,
			FinalizedAt: &finalizedAt,
		},
	}
	publication, err := artifactsexportservice.NewOSPublication()
	if err != nil {
		t.Fatalf("NewOSPublication: %v", err)
	}
	service := artifactsexportservice.New(snapshotSourceFake{snapshot: snapshot}, publication)

	_, err = service.ReadPortableArtifact(recordings.ReadPortableArtifactRequest{
		RecordingID: "recording-missing-read",
		Reference:   "artifact:missing-read",
	})
	if !errors.Is(err, recordings.ErrPortableArtifactUnavailable) {
		t.Fatalf("missing published artifact = %v, want ErrPortableArtifactUnavailable", err)
	}

	_, err = service.ReadPortableArtifact(recordings.ReadPortableArtifactRequest{
		RecordingID: "recording-absent",
		Reference:   "artifact:missing-read",
	})
	if !errors.Is(err, recordings.ErrPortableArtifactUnavailable) {
		t.Fatalf("missing recording = %v, want ErrPortableArtifactUnavailable", err)
	}
}

func TestReadPortableArtifactRejectsForeignHandle(t *testing.T) {
	t.Parallel()

	finalizedAt := time.Unix(1_700_000_000, 0).UTC()
	scope := recordings.CanonicalEventScope{FactorySessionID: "session-foreign-read"}
	event := recordings.CanonicalEvent{
		ID: "event-foreign-read", Kind: "WORK_REQUEST",
		Sequence: 0,
		Scope:    scope,
		Cursor: recordings.CanonicalEventCursor{
			StreamGenerationID: "generation-foreign-read",
			Sequence:           0,
		},
		RecordedAt: time.Unix(1_700_000_000, 0).UTC(),
		Payload:    `{"public":true}`,
	}
	destination := filepath.Join(t.TempDir(), "foreign-read.json")
	publication, err := artifactsexportservice.NewOSPublication()
	if err != nil {
		t.Fatalf("NewOSPublication: %v", err)
	}
	ownerSnapshot := recordinglifecycle.Snapshot{
		Status: recordings.RecordingStatusFacts{
			RecordingID: "recording-owner",
			Artifact:    recordings.RecordingArtifactReference(destination),
			Scope:       scope,
			State:       recordings.RecordingFinalized,
			FinalizedAt: &finalizedAt,
		},
		Events: []recordings.CanonicalEvent{event},
	}
	otherSnapshot := recordinglifecycle.Snapshot{
		Status: recordings.RecordingStatusFacts{
			RecordingID: "recording-other",
			Artifact:    "artifact:other-handle",
			Scope:       scope,
			State:       recordings.RecordingFinalized,
			FinalizedAt: &finalizedAt,
		},
	}
	snapshots := map[recordings.RecordingID]recordinglifecycle.Snapshot{
		"recording-owner": ownerSnapshot,
		"recording-other": otherSnapshot,
	}
	service := artifactsexportservice.New(snapshotSourceMapFake{snapshots: snapshots}, publication)
	exported, err := service.ExportPortableArtifact(recordings.ExportPortableArtifactRequest{
		RecordingID: "recording-owner",
	})
	if err != nil {
		t.Fatalf("ExportPortableArtifact: %v", err)
	}

	_, err = service.ReadPortableArtifact(recordings.ReadPortableArtifactRequest{
		RecordingID: "recording-other",
		Reference:   exported.Reference,
	})
	if !errors.Is(err, recordings.ErrForeignPortableArtifact) {
		t.Fatalf("foreign handle by reference = %v, want ErrForeignPortableArtifact", err)
	}

	_, err = service.ReadPortableArtifact(recordings.ReadPortableArtifactRequest{
		RecordingID: "recording-owner",
		Reference:   "artifact:other-handle",
	})
	if !errors.Is(err, recordings.ErrForeignPortableArtifact) {
		t.Fatalf("foreign handle by owner = %v, want ErrForeignPortableArtifact", err)
	}
}

func TestReadPortableArtifactErrorsOmitPrivatePaths(t *testing.T) {
	t.Parallel()

	const privateServiceTarget = "/private/ledger/storage/recording-internal.json"
	finalizedAt := time.Unix(1_700_000_000, 0).UTC()
	scope := recordings.CanonicalEventScope{FactorySessionID: "session-private-read-error"}
	snapshot := recordinglifecycle.Snapshot{
		Status: recordings.RecordingStatusFacts{
			RecordingID: "recording-private-read-error",
			Artifact:    "artifact:reported-read-error",
			Scope:       scope,
			State:       recordings.RecordingFinalized,
			FinalizedAt: &finalizedAt,
		},
	}
	publication, err := artifactsexportservice.NewOSPublication()
	if err != nil {
		t.Fatalf("NewOSPublication: %v", err)
	}
	service := artifactsexportservice.New(snapshotSourceFake{snapshot: snapshot}, publication)

	_, err = service.ReadPortableArtifact(recordings.ReadPortableArtifactRequest{
		RecordingID: "recording-private-read-error",
		Reference:   "artifact:foreign-handle",
	})
	if !errors.Is(err, recordings.ErrForeignPortableArtifact) {
		t.Fatalf("foreign handle = %v, want ErrForeignPortableArtifact", err)
	}
	if strings.Contains(err.Error(), privateServiceTarget) ||
		strings.Contains(err.Error(), "artifact:reported-read-error") {
		t.Fatalf("foreign handle error leaked private path: %v", err)
	}

	_, err = service.ReadPortableArtifact(recordings.ReadPortableArtifactRequest{
		RecordingID: "recording-private-read-error",
		Reference:   "artifact:reported-read-error",
	})
	if !errors.Is(err, recordings.ErrPortableArtifactUnavailable) {
		t.Fatalf("missing artifact = %v, want ErrPortableArtifactUnavailable", err)
	}
	if strings.Contains(err.Error(), privateServiceTarget) {
		t.Fatalf("missing artifact error leaked private path: %v", err)
	}
}

type snapshotSourceMapFake struct {
	snapshots map[recordings.RecordingID]recordinglifecycle.Snapshot
	err       error
}

func (fake snapshotSourceMapFake) Snapshot(
	id recordings.RecordingID,
) (recordinglifecycle.Snapshot, error) {
	if fake.err != nil {
		return recordinglifecycle.Snapshot{}, fake.err
	}
	snapshot, ok := fake.snapshots[id]
	if !ok {
		return recordinglifecycle.Snapshot{}, recordings.ErrMissingRecordingTarget
	}
	return snapshot, nil
}

func assertPortableArtifactRoundTrip(
	t *testing.T,
	service *artifactsexportservice.Service,
	artifact recordings.PortableArtifact,
	payload []byte,
	forbiddenSubstrings []string,
) {
	t.Helper()

	validated, err := service.ValidatePortableArtifact(recordings.ValidatePortableArtifactRequest{
		Artifact: artifact,
	})
	if err != nil {
		t.Fatalf("ValidatePortableArtifact: %v", err)
	}
	if !reflect.DeepEqual(validated.Summary, artifact.Summary) {
		t.Fatalf("validated summary = %#v, want %#v", validated.Summary, artifact.Summary)
	}
	decoded, err := service.DecodePortableArtifact(recordings.DecodePortableArtifactRequest{
		Payload: payload,
	})
	if err != nil {
		t.Fatalf("DecodePortableArtifact: %v", err)
	}
	if decoded.Artifact.Integrity != artifact.Integrity ||
		decoded.Artifact.Summary.RecordingID != artifact.Summary.RecordingID ||
		decoded.Artifact.Summary.EventCount != artifact.Summary.EventCount ||
		decoded.Artifact.Summary.Reference != artifact.Summary.Reference {
		t.Fatalf("decoded public facts = %#v, want %#v", decoded.Artifact, artifact)
	}
	summarized, err := service.SummarizePortableArtifact(recordings.SummarizePortableArtifactRequest{
		Artifact: decoded.Artifact,
	})
	if err != nil {
		t.Fatalf("SummarizePortableArtifact: %v", err)
	}
	if !reflect.DeepEqual(summarized.Summary, artifact.Summary) {
		t.Fatalf("summarized summary = %#v, want %#v", summarized.Summary, artifact.Summary)
	}
	payloadText := string(payload)
	for _, forbidden := range forbiddenSubstrings {
		if strings.Contains(payloadText, forbidden) {
			t.Fatalf("encoded payload leaked %q:\n%s", forbidden, payloadText)
		}
	}
}
