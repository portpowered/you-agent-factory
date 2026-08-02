package wire_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingswire "github.com/portpowered/infinite-you/pkg/services/recordings/wire"
)

func TestProviderBridgesConstructPublishedSurfaces(t *testing.T) {
	t.Parallel()

	if recordingswire.NewProjectionService() == nil {
		t.Fatal("NewProjectionService() returned nil")
	}

	ledger := recordingswire.NewRuntimeLedger(nil, time.Now, "wire-provider-gen", nil)
	if ledger == nil {
		t.Fatal("NewRuntimeLedger() returned nil")
	}

	if recordingswire.NewReplayClock(&recordings.ReplayArtifact{}) == nil {
		t.Fatal("NewReplayClock() returned nil")
	}

	_, _ = recordingswire.NewLifecycleRuntimeRecorder(
		time.Second,
		nil,
		time.Now,
		"",
		nil,
	)

	_, _, _, _, _ = recordingswire.NewReplayExecution(&recordings.ReplayArtifact{}, nil, nil)

	_, err := recordingswire.NewPortableRecordingWriter(
		os.MkdirAll,
		func(dir, pattern string) (recordings.RecordingTemporaryFile, error) {
			return os.CreateTemp(dir, pattern)
		},
		os.Remove,
		os.Rename,
	)
	if err != nil {
		t.Fatalf("NewPortableRecordingWriter() error = %v", err)
	}
	_ = recordingswire.NewReplayArtifactLoader(nil, nil)
}

func TestNewServiceWithProjectionRejectsMissingProjection(t *testing.T) {
	t.Parallel()

	service, err := recordingswire.NewServiceWithProjection(
		stubLedger{},
		nil,
		nil,
		func(string, []byte) error { return nil },
		os.MkdirAll,
		func(dir, pattern string) (recordings.RecordingTemporaryFile, error) {
			return os.CreateTemp(dir, pattern)
		},
		os.Remove,
		os.Rename,
		os.ReadFile,
	)
	if err == nil {
		t.Fatal("NewServiceWithProjection() error = nil, want missing projection dependency")
	}
	if err.Error() != "construct Recordings: projection is required" {
		t.Fatalf("NewServiceWithProjection() error = %q, want projection required", err.Error())
	}
	if service != nil {
		t.Fatalf("NewServiceWithProjection() = %#v, want nil service", service)
	}
}

func TestRecordingsOwnerConstructionDoesNotSelectHostOSPublication(t *testing.T) {
	t.Parallel()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() did not identify the test file")
	}
	ownerRoot := filepath.Dir(currentFile)
	productionFiles := []string{
		filepath.Join(ownerRoot, "providers.go"),
		filepath.Join(ownerRoot, "..", "internal", "portable_artifact_publication.go"),
		filepath.Join(ownerRoot, "..", "internal", "services", "artifacts_export", "wire", "publication.go"),
	}
	for _, path := range productionFiles {
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		text := string(source)
		if strings.Contains(text, `"os"`) || strings.Contains(text, "os.") || strings.Contains(text, "NewOSPublication") {
			t.Fatalf("Recordings owner construction file %s selects host OS publication effects", path)
		}
	}
}
