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
