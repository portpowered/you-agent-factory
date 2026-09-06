package managedbackend

import (
	"archive/zip"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
)

func TestBackendRuntimeFailureWrapsWithoutLeakingCause(t *testing.T) {
	t.Parallel()

	cause := errors.New("token=sensitive backend path=C:\\private\\backend.exe")
	wrapped := WrapBackendExtractFailure(cause)
	if !errors.Is(wrapped, cause) {
		t.Fatal("backend extract wrapper did not preserve the original cause")
	}
	var classified interface {
		ModelRuntimeStage() string
		ModelRuntimeFailureClass() string
	}
	if !errors.As(wrapped, &classified) {
		t.Fatal("backend extract wrapper did not expose the private classifier")
	}
	if classified.ModelRuntimeStage() != "BACKEND_EXTRACT" || classified.ModelRuntimeFailureClass() != "EXTRACTION_FAILED" {
		t.Fatalf("classification = (%q, %q), want BACKEND_EXTRACT/EXTRACTION_FAILED", classified.ModelRuntimeStage(), classified.ModelRuntimeFailureClass())
	}
	if strings.Contains(wrapped.Error(), "sensitive") || strings.Contains(wrapped.Error(), "C:\\private") {
		t.Fatalf("backend wrapper leaked cause: %q", wrapped.Error())
	}
}

func TestResolveManagedBackendLaunchExtractsPinnedWindowsPackage(t *testing.T) {
	t.Parallel()

	archivePath := filepath.Join(t.TempDir(), "backend.zip")
	archiveFile, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("create archive: %v", err)
	}
	zipWriter := zip.NewWriter(archiveFile)
	for name, body := range map[string]string{
		"llama-cpp-cpu-all.exe": "backend executable",
		"libwinpthread-1.dll":   "runtime dependency",
	} {
		entry, createErr := zipWriter.Create(name)
		if createErr != nil {
			t.Fatalf("create archive entry %q: %v", name, createErr)
		}
		if _, writeErr := entry.Write([]byte(body)); writeErr != nil {
			t.Fatalf("write archive entry %q: %v", name, writeErr)
		}
	}
	if err := zipWriter.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}
	if err := archiveFile.Close(); err != nil {
		t.Fatalf("close archive: %v", err)
	}

	launch, err := ResolveManagedBackendLaunch(context.Background(), serviceedges.HostProcessStartSpec{
		Backend:      "localai-llamacpp",
		ModelPath:    filepath.Join(t.TempDir(), "model.gguf"),
		BackendFiles: []string{archivePath},
	})
	if err != nil {
		t.Fatalf("ResolveManagedBackendLaunch: %v", err)
	}
	if !strings.EqualFold(filepath.Base(launch.Command), "llama-cpp-cpu-all.exe") {
		t.Fatalf("managed backend command = %q, want pinned executable", launch.Command)
	}
	if launch.WorkDir == "" || launch.Endpoint == "" || len(launch.Args) != 1 || !strings.HasPrefix(launch.Args[0], "--addr=") {
		t.Fatalf("managed backend launch = %#v, want extracted workdir, endpoint, and address arg", launch)
	}
	if _, err := os.Stat(launch.Command); err != nil {
		t.Fatalf("extracted executable = %q: %v", launch.Command, err)
	}
	workDir := launch.WorkDir
	launch.Cleanup()
	if _, err := os.Stat(workDir); !os.IsNotExist(err) {
		t.Fatalf("managed backend workspace after cleanup = %v, want removed", err)
	}
}

func TestResolveManagedBackendLaunchRejectsUnsafeArchivePath(t *testing.T) {
	t.Parallel()

	archivePath := filepath.Join(t.TempDir(), "backend.zip")
	archiveFile, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("create archive: %v", err)
	}
	zipWriter := zip.NewWriter(archiveFile)
	entry, err := zipWriter.Create("../llama-cpp-cpu-all.exe")
	if err != nil {
		t.Fatalf("create unsafe archive entry: %v", err)
	}
	if _, err := entry.Write([]byte("backend executable")); err != nil {
		t.Fatalf("write unsafe archive entry: %v", err)
	}
	if err := zipWriter.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}
	if err := archiveFile.Close(); err != nil {
		t.Fatalf("close archive: %v", err)
	}

	_, launchErr := ResolveManagedBackendLaunch(context.Background(), serviceedges.HostProcessStartSpec{
		Backend:      "localai-llamacpp",
		BackendFiles: []string{archivePath},
	})
	if launchErr == nil {
		t.Fatal("unsafe archive launch error = nil, want bounded extract failure")
	}
	var classified runtimeFailureClassifier
	if !errors.As(launchErr, &classified) {
		t.Fatalf("unsafe archive launch error = %v, want private runtime classification", launchErr)
	}
	if classified.ModelRuntimeStage() != runtimeStageBackendExtract ||
		classified.ModelRuntimeFailureClass() != runtimeFailureExtraction {
		t.Fatalf("classification = (%q, %q), want BACKEND_EXTRACT/EXTRACTION_FAILED", classified.ModelRuntimeStage(), classified.ModelRuntimeFailureClass())
	}
	if strings.Contains(launchErr.Error(), "unsafe") || strings.Contains(launchErr.Error(), archivePath) {
		t.Fatalf("bounded extract failure leaked raw cause: %q", launchErr.Error())
	}
}
