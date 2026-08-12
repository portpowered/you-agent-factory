package filesystem

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestLocalAppendDurableCreatesPrivateArtifactAndAppendsRecords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "dead-letter.jsonl")
	local := Local{}
	if err := local.AppendDurable(path, []byte("first\n")); err != nil {
		t.Fatalf("AppendDurable(first) error = %v", err)
	}
	if err := local.AppendDurable(path, []byte("second\n")); err != nil {
		t.Fatalf("AppendDurable(second) error = %v", err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read appended artifact: %v", err)
	}
	if got, want := string(contents), "first\nsecond\n"; got != want {
		t.Fatalf("artifact contents = %q, want %q", got, want)
	}
}

func TestLocalAppendDurableRejectsInvalidTargets(t *testing.T) {
	local := Local{}
	if err := local.AppendDurable(" ", []byte("ignored")); err == nil {
		t.Fatal("AppendDurable(blank path) succeeded, want validation error")
	}

	parentFile := filepath.Join(t.TempDir(), "parent-file")
	if err := os.WriteFile(parentFile, []byte("occupied"), 0o600); err != nil {
		t.Fatalf("create blocking parent file: %v", err)
	}
	if err := local.AppendDurable(filepath.Join(parentFile, "child.jsonl"), []byte("ignored")); err == nil {
		t.Fatal("AppendDurable(child below regular file) succeeded, want directory error")
	}

	directoryTarget := filepath.Join(t.TempDir(), "directory-target")
	if err := os.MkdirAll(directoryTarget, 0o700); err != nil {
		t.Fatalf("create directory target: %v", err)
	}
	if err := local.AppendDurable(directoryTarget, []byte("ignored")); err == nil {
		t.Fatal("AppendDurable(directory target) succeeded, want open error")
	}

	if runtime.GOOS != "linux" {
		return
	}
	if err := local.AppendDurable("/dev/full", []byte("ignored")); err == nil {
		t.Fatal("AppendDurable(/dev/full) succeeded, want write error")
	}
	if err := local.AppendDurable(os.DevNull, []byte("ignored")); err == nil {
		t.Fatal("AppendDurable(os.DevNull) succeeded, want sync error")
	}
}
