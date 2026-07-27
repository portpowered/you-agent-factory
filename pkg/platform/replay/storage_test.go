package replay

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestWriteAndReadFileReplaceSnapshot(t *testing.T) {
	storage := NewLocal(runtime.GOOS)
	path := filepath.Join(t.TempDir(), "nested", "run.replay.json")
	for _, want := range []string{"first", "replacement"} {
		if err := storage.WriteFile(path, []byte(want)); err != nil {
			t.Fatalf("WriteFile(%q): %v", want, err)
		}
		got, err := storage.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile(): %v", err)
		}
		if string(got) != want {
			t.Fatalf("ReadFile() = %q, want %q", got, want)
		}
	}
}

func TestWriteAndReadFileFailuresAreActionable(t *testing.T) {
	storage := NewLocal(runtime.GOOS)
	parentFile := filepath.Join(t.TempDir(), "parent-file")
	if err := os.WriteFile(parentFile, []byte("occupied"), 0o600); err != nil {
		t.Fatalf("write parent fixture: %v", err)
	}
	path := filepath.Join(parentFile, "run.replay.json")
	if err := storage.WriteFile(path, []byte("{}")); err == nil || !strings.Contains(err.Error(), "create replay artifact directory") {
		t.Fatalf("WriteFile() error = %v, want directory context", err)
	}
	if _, err := storage.ReadFile(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("ReadFile() error = nil, want missing-file error")
	}
}

func TestWriteFileWindowsReplaceRetriesUntilFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory replace blocking behaves differently on Windows filesystem")
	}
	t.Parallel()

	dir := t.TempDir()
	blockingPath := filepath.Join(dir, "blocked")
	if err := os.Mkdir(blockingPath, 0o755); err != nil {
		t.Fatalf("mkdir blocking path: %v", err)
	}

	storage := NewLocal("windows")
	err := storage.WriteFile(blockingPath, []byte("payload"))
	if err == nil {
		t.Fatal("WriteFile() error = nil, want replace failure")
	}
	if !strings.Contains(err.Error(), "temp artifact left at") {
		t.Fatalf("WriteFile() error = %v, want temp artifact context", err)
	}
}

func TestWriteFileNonWindowsReplaceFailureIsActionable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("non-windows replace failure semantics differ on Windows filesystem")
	}
	t.Parallel()

	dir := t.TempDir()
	blockingPath := filepath.Join(dir, "blocked")
	if err := os.Mkdir(blockingPath, 0o755); err != nil {
		t.Fatalf("mkdir blocking path: %v", err)
	}

	storage := NewLocal("linux")
	err := storage.WriteFile(blockingPath, []byte("payload"))
	if err == nil {
		t.Fatal("WriteFile() error = nil, want replace failure")
	}
	if !strings.Contains(err.Error(), "replace replay artifact with temp file") {
		t.Fatalf("WriteFile() error = %v, want non-windows replace context", err)
	}
	if strings.Contains(err.Error(), "temp artifact left at") {
		t.Fatalf("WriteFile() error = %v, want immediate non-windows failure without temp retention", err)
	}
}

func TestReadFileWindowsRetriesMissingArtifact(t *testing.T) {
	t.Parallel()

	storage := NewLocal("windows")
	missing := filepath.Join(t.TempDir(), "missing.replay.json")
	_, err := storage.ReadFile(missing)
	if err == nil {
		t.Fatal("ReadFile() error = nil, want missing-file error")
	}
	if !os.IsNotExist(err) {
		t.Fatalf("ReadFile() error = %v, want missing-file error", err)
	}
}
