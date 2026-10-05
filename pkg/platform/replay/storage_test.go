package replay

import (
	"errors"
	"io"
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

func TestDirectoryScanBoundsBatchesAndPropagatesVisitorFailure(t *testing.T) {
	t.Parallel()
	storage := NewLocal(runtime.GOOS)
	root := t.TempDir()
	for _, name := range []string{"first", "second", "third"} {
		if err := storage.WriteFile(filepath.Join(root, name), []byte("record")); err != nil {
			t.Fatal(err)
		}
	}
	seen := make(map[string]bool)
	err := storage.ScanDirectory(root, 2, func(entries []os.DirEntry) error {
		if len(entries) > 2 {
			t.Fatalf("directory batch size=%d, want <=2", len(entries))
		}
		for _, entry := range entries {
			if seen[entry.Name()] {
				t.Fatalf("duplicate entry %q", entry.Name())
			}
			seen[entry.Name()] = true
		}
		return nil
	})
	if err != nil || len(seen) != 3 {
		t.Fatalf("scan entries=%v error=%v", seen, err)
	}
	stop := errors.New("visitor stopped")
	if err := storage.ScanDirectory(root, 2, func([]os.DirEntry) error { return stop }); !errors.Is(err, stop) {
		t.Fatalf("visitor error=%v", err)
	}
	if err := storage.ScanDirectory(filepath.Join(root, "missing"), 2, func([]os.DirEntry) error { return nil }); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing directory error=%v", err)
	}
}

func TestAppendAndReadFilePreservesCompletePrefix(t *testing.T) {
	storage := NewLocal(runtime.GOOS)
	path := filepath.Join(t.TempDir(), "nested", "run.replay.jsonl")
	for _, value := range []string{"header\n", "event\n", "terminal\n"} {
		if err := storage.AppendFile(path, []byte(value)); err != nil {
			t.Fatalf("AppendFile(%q): %v", value, err)
		}
	}
	got, err := storage.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(): %v", err)
	}
	if string(got) != "header\nevent\nterminal\n" {
		t.Fatalf("ReadFile() = %q, want appended JSONL prefix", got)
	}
}

func TestAppendReplaySuffixRollsBackPartialWriteForRetry(t *testing.T) {
	file := &replayAppendTestFile{data: []byte("prefix"), maxWrite: 2}
	if err := appendReplaySuffix(file, []byte("-suffix")); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("appendReplaySuffix() error = %v, want short-write error", err)
	}
	if string(file.data) != "prefix" {
		t.Fatalf("partial append data = %q, want original prefix", file.data)
	}

	file.maxWrite = -1
	if err := appendReplaySuffix(file, []byte("-suffix")); err != nil {
		t.Fatalf("retry appendReplaySuffix(): %v", err)
	}
	if string(file.data) != "prefix-suffix" {
		t.Fatalf("retry append data = %q, want one suffix", file.data)
	}
}

func TestAppendFileRecreatesMissingParent(t *testing.T) {
	t.Parallel()
	storage := NewLocal(runtime.GOOS)
	parent := filepath.Join(t.TempDir(), "nested")
	path := filepath.Join(parent, "run.replay.jsonl")
	if err := storage.AppendFile(path, []byte("first\n")); err != nil {
		t.Fatal(err)
	}
	// External cleanup of this test-owned directory must not leave a cached
	// directory-ready assumption that prevents a subsequent append.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(parent); err != nil {
		t.Fatal(err)
	}
	if err := storage.AppendFile(path, []byte("second\n")); err != nil {
		t.Fatal(err)
	}
	got, err := storage.ReadFile(path)
	if err != nil || string(got) != "second\n" {
		t.Fatalf("ReadFile() = %q, %v, want second suffix", got, err)
	}
}

func TestAppendFileRejectsOccupiedPath(t *testing.T) {
	t.Parallel()
	storage := NewLocal(runtime.GOOS)
	path := filepath.Join(t.TempDir(), "occupied")
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	err := storage.AppendFile(path, []byte("event\n"))
	if err == nil || !strings.Contains(err.Error(), "open replay artifact for append") {
		t.Fatalf("AppendFile() error = %v, want open failure", err)
	}
	entries, err := os.ReadDir(path)
	if err != nil || len(entries) != 0 {
		t.Fatalf("occupied directory changed: %v, %v", entries, err)
	}
}

func TestAppendReplaySuffixRollsBackAfterSyncFailureForRetry(t *testing.T) {
	file := &replayAppendTestFile{data: []byte("prefix"), syncFailures: 1}
	if err := appendReplaySuffix(file, []byte("-suffix")); err == nil || !strings.Contains(err.Error(), "sync replay artifact append") {
		t.Fatalf("appendReplaySuffix() error = %v, want sync error", err)
	}
	if string(file.data) != "prefix" {
		t.Fatalf("sync-failed append data = %q, want original prefix", file.data)
	}

	if err := appendReplaySuffix(file, []byte("-suffix")); err != nil {
		t.Fatalf("retry appendReplaySuffix(): %v", err)
	}
	if string(file.data) != "prefix-suffix" {
		t.Fatalf("retry append data = %q, want one suffix", file.data)
	}
}

type replayAppendTestFile struct {
	data         []byte
	position     int64
	maxWrite     int
	syncFailures int
}

func (file *replayAppendTestFile) Write(data []byte) (int, error) {
	count := len(data)
	if file.maxWrite > 0 && count > file.maxWrite {
		count = file.maxWrite
	}
	end := int(file.position) + count
	if end > len(file.data) {
		file.data = append(file.data, make([]byte, end-len(file.data))...)
	}
	copy(file.data[int(file.position):end], data[:count])
	file.position = int64(end)
	return count, nil
}

func (file *replayAppendTestFile) Seek(offset int64, whence int) (int64, error) {
	if whence != io.SeekEnd || offset != 0 {
		return 0, errors.New("test append file only supports seeking to end")
	}
	file.position = int64(len(file.data))
	return file.position, nil
}

func (file *replayAppendTestFile) Sync() error {
	if file.syncFailures > 0 {
		file.syncFailures--
		return errors.New("injected sync failure")
	}
	return nil
}

func (file *replayAppendTestFile) Truncate(size int64) error {
	if size < 0 || size > int64(len(file.data)) {
		return errors.New("invalid test truncate size")
	}
	file.data = file.data[:size]
	if file.position > size {
		file.position = size
	}
	return nil
}

func (file *replayAppendTestFile) Close() error { return nil }

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
	t.Parallel()

	dir := t.TempDir()
	blockingPath := filepath.Join(dir, "blocked")
	if err := os.MkdirAll(filepath.Join(blockingPath, "occupied"), 0o755); err != nil {
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
