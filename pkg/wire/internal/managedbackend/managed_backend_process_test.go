package managedbackend

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
)

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

	if _, err := ResolveManagedBackendLaunch(context.Background(), serviceedges.HostProcessStartSpec{
		Backend:      "localai-llamacpp",
		BackendFiles: []string{archivePath},
	}); err == nil || !strings.Contains(err.Error(), "unsafe") {
		t.Fatalf("unsafe archive launch error = %v, want unsafe path failure", err)
	}
}
