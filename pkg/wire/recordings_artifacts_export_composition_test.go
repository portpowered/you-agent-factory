package wire

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"testing"
	"time"

	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
)

// TestInjectBundleComposesRecordingsArtifactExportThroughWireFactory proves the
// Wire recordings factory wires artifact close/export/read through the singular
// Recordings root rather than a second peer authority.
// backendsizecheck:ignore-function pre-existing baseline debt recorded 2026-08-08; split this oversized code into focused units and remove this exemption
// pkgmaintcheck:ignore-function-lines pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
func TestInjectBundleComposesRecordingsArtifactExportThroughWireFactory(t *testing.T) {
	t.Parallel()

	edges := injectedRecordingArtifactEdges()
	if _, err := InjectBundle(t.Context(), edges); err != nil {
		t.Fatalf("InjectBundle() error = %v", err)
	}
	reserver, err := provideRuntimeArtifactPathReserver()
	if err != nil {
		t.Fatalf("provideRuntimeArtifactPathReserver() error = %v", err)
	}

	root, err := provideRecordingsRoot(
		edges,
		provideLiveRecordingTargetPlanner(reserver),
		platformreplay.Local{},
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("provideRecordingsRoot() error = %v", err)
	}
	if root == nil {
		t.Fatal("provideRecordingsRoot() returned nil root")
	}
	var rootService recordings.Service = root
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
	if err != nil || exported.Reference != recordings.RecordingArtifactReference(artifactPath) {
		t.Fatalf("ExportPortableArtifact = (%#v, %v)", exported, err)
	}
	read, err := rootService.ReadPortableArtifact(context.Background(), recordings.ReadPortableArtifactRequest{
		RecordingID: bound.Status.RecordingID,
		Reference:   exported.Reference,
	})
	if err != nil || read.Artifact.Integrity != exported.Artifact.Integrity {
		t.Fatalf("ReadPortableArtifact = (%#v, %v), want exported artifact", read, err)
	}
	if _, err := rootService.DecodePortableArtifact(recordings.DecodePortableArtifactRequest{
		Payload: []byte(`{`),
	}); !errors.Is(err, recordings.ErrInvalidPortableArtifact) {
		t.Fatalf("DecodePortableArtifact malformed = %v, want ErrInvalidPortableArtifact", err)
	}
}

type injectedPublicationFile struct {
	name    string
	calls   *[]string
	payload []byte
}

func (file *injectedPublicationFile) Write(payload []byte) (int, error) {
	*file.calls = append(*file.calls, "write")
	file.payload = append(file.payload[:0], payload...)
	return len(payload), nil
}

func (file *injectedPublicationFile) Name() string { return file.name }

func (file *injectedPublicationFile) Chmod(fs.FileMode) error {
	*file.calls = append(*file.calls, "chmod")
	return nil
}

func (file *injectedPublicationFile) Sync() error {
	*file.calls = append(*file.calls, "sync")
	return nil
}

func (file *injectedPublicationFile) Close() error {
	*file.calls = append(*file.calls, "close")
	return nil
}

func injectedRecordingArtifactEdges() serviceedges.Edges {
	var calls []string
	temporaryFiles := make(map[string]*injectedPublicationFile)
	published := make(map[string][]byte)
	return serviceedges.Edges{
		RecordingMakeDirectories: func(string, fs.FileMode) error {
			calls = append(calls, "mkdir")
			return nil
		},
		RecordingCreateTempFile: func(dir, pattern string) (recordings.RecordingTemporaryFile, error) {
			calls = append(calls, "temp")
			file := &injectedPublicationFile{
				name:  filepath.Join(dir, pattern),
				calls: &calls,
			}
			temporaryFiles[file.name] = file
			return file, nil
		},
		RecordingRemovePath: func(path string) error {
			calls = append(calls, "remove")
			delete(temporaryFiles, path)
			return nil
		},
		RecordingRenamePath: func(source, destination string) error {
			calls = append(calls, "rename")
			file := temporaryFiles[source]
			if file == nil {
				return errors.New("injected temporary file missing")
			}
			published[destination] = append([]byte(nil), file.payload...)
			delete(temporaryFiles, source)
			return nil
		},
		RecordingReadFile: func(path string) ([]byte, error) {
			calls = append(calls, "read")
			payload, ok := published[path]
			if !ok {
				return nil, errors.New("injected artifact missing")
			}
			return append([]byte(nil), payload...), nil
		},
	}
}
