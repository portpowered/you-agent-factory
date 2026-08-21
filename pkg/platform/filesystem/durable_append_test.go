package filesystem

import (
	"errors"
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

func TestLocalRenameReplacingMissingSourcePreservesDestination(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	oldPath := filepath.Join(dir, "missing.tmp")
	newPath := filepath.Join(dir, "recording.json")
	if err := os.WriteFile(newPath, []byte("durable recording"), 0o600); err != nil {
		t.Fatalf("write destination: %v", err)
	}

	if err := (Local{}).RenameReplacing(oldPath, newPath); err == nil {
		t.Fatal("RenameReplacing(missing source) succeeded, want error")
	}
	contents, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatalf("read destination after failed replacement: %v", err)
	}
	if got, want := string(contents), "durable recording"; got != want {
		t.Fatalf("destination contents = %q, want %q", got, want)
	}
}

func TestRenameReplacingDoesNotFallbackForNonContentionError(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("source is not replaceable")
	renameCalls := 0
	removeCalls := 0
	err := renameReplacing(
		"old",
		"new",
		true,
		func(string, string) error {
			renameCalls++
			return sentinel
		},
		func(string) error {
			removeCalls++
			return nil
		},
		func(string) (os.FileInfo, error) { return nil, nil },
	)
	if !errors.Is(err, sentinel) {
		t.Fatalf("rename error = %v, want %v", err, sentinel)
	}
	if renameCalls != 1 || removeCalls != 0 {
		t.Fatalf("fallback calls = rename:%d remove:%d, want rename:1 remove:0", renameCalls, removeCalls)
	}
}

func TestRenameReplacingRetriesWindowsDestinationContention(t *testing.T) {
	t.Parallel()

	renameCalls := 0
	removeCalls := 0
	err := renameReplacing(
		"old",
		"new",
		true,
		func(string, string) error {
			renameCalls++
			if renameCalls == 1 {
				return os.ErrExist
			}
			return nil
		},
		func(string) error {
			removeCalls++
			return nil
		},
		func(string) (os.FileInfo, error) { return nil, nil },
	)
	if err != nil {
		t.Fatalf("renameReplacing(contention): %v", err)
	}
	if renameCalls != 2 || removeCalls != 1 {
		t.Fatalf("retry calls = rename:%d remove:%d, want rename:2 remove:1", renameCalls, removeCalls)
	}
}
