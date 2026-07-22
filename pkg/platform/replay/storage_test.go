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
