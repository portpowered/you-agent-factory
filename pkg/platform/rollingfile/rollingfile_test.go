package rollingfile

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestWriterRetainsNewestBackups(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.log")
	writer := &Writer{Filename: path, MaxBackups: 2}
	now := time.Now().UTC()
	oldest := writer.backupPath(now.Add(-3 * time.Hour))
	oldestCompressed := oldest + compressSuffix
	middle := writer.backupPath(now.Add(-2 * time.Hour))
	newest := writer.backupPath(now.Add(-1 * time.Hour))
	for _, backupPath := range []string{oldest, oldestCompressed, middle, newest} {
		if err := os.WriteFile(backupPath, []byte(backupPath), 0o600); err != nil {
			t.Fatalf("WriteFile(%q): %v", backupPath, err)
		}
	}

	if err := writer.cleanBackups(); err != nil {
		t.Fatalf("cleanBackups(): %v", err)
	}
	assertPathExists(t, oldest, false)
	assertPathExists(t, oldestCompressed, false)
	assertPathExists(t, middle, true)
	assertPathExists(t, newest, true)
}

func TestWriterRemovesExpiredBackups(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.log")
	writer := &Writer{Filename: path, MaxAge: 1}
	now := time.Now().UTC()
	old := writer.backupPath(now.Add(-48 * time.Hour))
	recent := writer.backupPath(now.Add(-time.Hour))
	for _, backupPath := range []string{old, recent} {
		if err := os.WriteFile(backupPath, []byte("backup"), 0o600); err != nil {
			t.Fatalf("WriteFile(%q): %v", backupPath, err)
		}
	}

	if err := writer.cleanBackups(); err != nil {
		t.Fatalf("cleanBackups(): %v", err)
	}
	assertPathExists(t, old, false)
	assertPathExists(t, recent, true)
}

func TestWriterRotatesAndCompressesBackups(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.log")
	writer := &Writer{Filename: path, MaxBackups: 2, Compress: true}
	const payload = "rotated payload"
	if _, err := writer.Write([]byte(payload)); err != nil {
		t.Fatalf("Write(): %v", err)
	}
	if err := writer.Rotate(); err != nil {
		t.Fatalf("Rotate(): %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close(): %v", err)
	}

	matches, err := filepath.Glob(strings.TrimSuffix(path, filepath.Ext(path)) + "-*" + filepath.Ext(path) + compressSuffix)
	if err != nil {
		t.Fatalf("Glob(): %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("compressed backups = %v, want one rotated backup", matches)
	}
	compressed, err := os.Open(matches[0])
	if err != nil {
		t.Fatalf("Open(%q): %v", matches[0], err)
	}
	defer compressed.Close()
	reader, err := gzip.NewReader(compressed)
	if err != nil {
		t.Fatalf("gzip.NewReader(): %v", err)
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll(): %v", err)
	}
	if string(data) != payload {
		t.Fatalf("compressed backup = %q, want %q", data, payload)
	}
	assertPathExists(t, path, true)
}

func TestWriterCompressedSameTimestampUsesUniqueBackups(t *testing.T) {
	at := time.Date(2026, time.August, 11, 12, 13, 14, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "runtime.log")
	writer := newFixedTimeWriter(path, 4, at)
	payloads := []string{"first", "second", "third", "fourth"}
	writeAndRotatePayloads(t, writer, payloads)
	closeWriter(t, writer)

	base := writer.backupPath(at)
	for index, payload := range payloads {
		assertCompressedBackup(t, collisionBackupPath(base, index), payload)
	}
}

func TestWriterCompressedSameTimestampRetainsNewestBackups(t *testing.T) {
	at := time.Date(2026, time.August, 11, 12, 13, 14, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "runtime.log")
	writer := newFixedTimeWriter(path, 2, at)
	payloads := []string{"oldest", "old", "new", "newest"}
	writeAndRotatePayloads(t, writer, payloads)
	closeWriter(t, writer)

	base := writer.backupPath(at)
	for index, payload := range payloads {
		backupPath := collisionBackupPath(base, index)
		want := index >= len(payloads)-writer.MaxBackups
		_, err := os.Stat(backupPath)
		if (err == nil) != want {
			t.Fatalf("backup %q exists = %t, want %t (err=%v)", backupPath, err == nil, want, err)
		}
		if want {
			assertCompressedBackup(t, backupPath, payload)
		}
	}
}

func newFixedTimeWriter(path string, maxBackups int, at time.Time) *Writer {
	return &Writer{
		Filename:   path,
		MaxBackups: maxBackups,
		Compress:   true,
		now:        func() time.Time { return at },
	}
}

func writeAndRotatePayloads(t *testing.T, writer *Writer, payloads []string) {
	t.Helper()
	for _, payload := range payloads {
		if _, err := writer.Write([]byte(payload)); err != nil {
			t.Fatalf("Write(%q): %v", payload, err)
		}
		if err := writer.Rotate(); err != nil {
			t.Fatalf("Rotate(%q): %v", payload, err)
		}
	}
}

func closeWriter(t *testing.T, writer *Writer) {
	t.Helper()
	if err := writer.Close(); err != nil {
		t.Fatalf("Close(): %v", err)
	}
}

func collisionBackupPath(base string, index int) string {
	if index > 0 {
		base = backupPathWithSuffix(base, index)
	}
	return base + compressSuffix
}

func assertCompressedBackup(t *testing.T, path, want string) {
	t.Helper()
	if got := readCompressedBackup(t, path); got != want {
		t.Fatalf("backup %q = %q, want %q", path, got, want)
	}
}

func TestCompressPreservesSourceMode(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "source.log")
	targetPath := sourcePath + compressSuffix
	if err := os.WriteFile(sourcePath, []byte("private payload"), 0o600); err != nil {
		t.Fatalf("WriteFile(): %v", err)
	}
	if err := os.Chmod(sourcePath, 0o600); err != nil {
		t.Fatalf("Chmod(source): %v", err)
	}
	sourceInfo, err := os.Stat(sourcePath)
	if err != nil {
		t.Fatalf("Stat(source): %v", err)
	}

	if err := compress(sourcePath, targetPath); err != nil {
		t.Fatalf("compress(): %v", err)
	}
	targetInfo, err := os.Stat(targetPath)
	if err != nil {
		t.Fatalf("Stat(target): %v", err)
	}
	if targetInfo.Mode().Perm() != sourceInfo.Mode().Perm() {
		t.Fatalf("target mode = %#o, want source mode %#o", targetInfo.Mode().Perm(), sourceInfo.Mode().Perm())
	}
	if runtime.GOOS != "windows" && targetInfo.Mode().Perm() != 0o600 {
		t.Fatalf("target mode = %#o, want owner-only %#o", targetInfo.Mode().Perm(), 0o600)
	}
}

func TestCompressRemovesPartialTargetAfterCopyFailure(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "source.log")
	targetPath := sourcePath + compressSuffix
	if err := os.WriteFile(sourcePath, []byte("source"), 0o600); err != nil {
		t.Fatalf("WriteFile(): %v", err)
	}
	sourceInfo, err := os.Stat(sourcePath)
	if err != nil {
		t.Fatalf("Stat(source): %v", err)
	}

	err = compressReader(failingReader{}, sourceInfo, targetPath)
	if err == nil || !errors.Is(err, errFailingReader) {
		t.Fatalf("compressReader() error = %v, want %v", err, errFailingReader)
	}
	if _, statErr := os.Stat(targetPath); !errors.Is(statErr, fs.ErrNotExist) {
		t.Fatalf("partial target stat error = %v, want not exist", statErr)
	}
}

var errFailingReader = errors.New("failing reader")

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errFailingReader }

func readCompressedBackup(t *testing.T, path string) string {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("Open(%q): %v", path, err)
	}
	defer file.Close()
	reader, err := gzip.NewReader(file)
	if err != nil {
		t.Fatalf("gzip.NewReader(%q): %v", path, err)
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll(%q): %v", path, err)
	}
	return string(data)
}

func TestWriterCloseReopensExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.log")
	writer := &Writer{Filename: path}
	if _, err := writer.Write([]byte("first\n")); err != nil {
		t.Fatalf("first Write(): %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("first Close(): %v", err)
	}
	if _, err := writer.Write([]byte("second\n")); err != nil {
		t.Fatalf("second Write(): %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("second Close(): %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", path, err)
	}
	if string(data) != "first\nsecond\n" {
		t.Fatalf("reopened contents = %q, want %q", data, "first\nsecond\n")
	}
}

func TestWriterPrepareRollsBackAndCommitsAcrossRotation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.log")
	const maxBytes = 1024 * 1024
	original := bytes.Repeat([]byte("x"), maxBytes-4)
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatalf("WriteFile(): %v", err)
	}
	at := time.Date(2026, time.August, 15, 12, 13, 14, 0, time.UTC)
	writer := &Writer{Filename: path, MaxSize: 1, now: func() time.Time { return at }}
	if _, err := writer.Write(nil); err != nil {
		t.Fatalf("open existing file: %v", err)
	}

	checkpoint, err := writer.Prepare(8)
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if _, err := writer.Write([]byte("rejected")); err != nil {
		t.Fatalf("Write() before rollback: %v", err)
	}
	if err := writer.Rollback(checkpoint); err != nil {
		t.Fatalf("Rollback() error = %v", err)
	}
	if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, original) {
		t.Fatalf("rolled-back active file = (%q, %v), want original payload", got, err)
	}
	if got := writer.size; got != int64(len(original)) || writer.file == nil {
		t.Fatalf("rolled-back writer state = (size %d, active %t), want (%d, true)", got, writer.file != nil, len(original))
	}
	fitCheckpoint, err := writer.Prepare(1)
	if err != nil {
		t.Fatalf("fit Prepare() error = %v", err)
	}
	if _, err := writer.Write([]byte("x")); err != nil {
		t.Fatalf("fit Write(): %v", err)
	}
	if err := writer.Rollback(fitCheckpoint); err != nil {
		t.Fatalf("fit Rollback() error = %v", err)
	}

	checkpoint, err = writer.Prepare(5)
	if err != nil {
		t.Fatalf("second Prepare() error = %v", err)
	}
	if _, err := writer.Write([]byte("kept")); err != nil {
		t.Fatalf("Write() after rotation: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close(): %v", err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "kept" {
		t.Fatalf("active file after committed rotation = (%q, %v), want kept payload", got, err)
	}
	backup := writer.backupPath(at)
	if got, err := os.ReadFile(backup); err != nil || !bytes.Equal(got, original) {
		t.Fatalf("rotation backup = (%q, %v), want original payload", got, err)
	}
}

func TestWriterPrepareRollsBackExistingInactiveFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.log")
	original := []byte("existing")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatalf("WriteFile(): %v", err)
	}
	writer := &Writer{Filename: path, MaxSize: 1}
	checkpoint, err := writer.Prepare(len(" appended"))
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if _, err := writer.Write([]byte(" appended")); err != nil {
		t.Fatalf("Write() before rollback: %v", err)
	}
	if err := writer.Rollback(checkpoint); err != nil {
		t.Fatalf("Rollback() error = %v", err)
	}
	if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, original) {
		t.Fatalf("rolled-back existing file = (%q, %v), want original payload", got, err)
	}
	if writer.file != nil || writer.size != 0 {
		t.Fatalf("rolled-back inactive writer state = (file %v, size %d), want inactive", writer.file, writer.size)
	}
}

func TestWriterPrepareRollsBackInactiveRotation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.log")
	const maxBytes = 1024 * 1024
	original := bytes.Repeat([]byte("x"), maxBytes-1)
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatalf("WriteFile(): %v", err)
	}
	writer := &Writer{Filename: path, MaxSize: 1}
	checkpoint, err := writer.Prepare(2)
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if _, err := writer.Write([]byte("new")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := writer.Rollback(checkpoint); err != nil {
		t.Fatalf("Rollback() error = %v", err)
	}
	if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, original) {
		t.Fatalf("rolled-back inactive rotation = (%q, %v), want original payload", got, err)
	}
	if writer.file != nil || writer.size != 0 {
		t.Fatalf("rolled-back inactive rotation state = (file %v, size %d), want inactive", writer.file, writer.size)
	}
}

func TestWriterPrepareCreatesAndRollsBackNewFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.log")
	writer := &Writer{Filename: path, MaxSize: 1}
	checkpoint, err := writer.Prepare(4)
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if _, err := writer.Write([]byte("new")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := writer.Rollback(checkpoint); err != nil {
		t.Fatalf("Rollback() error = %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("rolled-back new file exists, stat error = %v", err)
	}
}

func TestWriterPrepareAndRollbackValidateInputs(t *testing.T) {
	var nilWriter *Writer
	if _, err := nilWriter.Prepare(1); err == nil {
		t.Fatal("nil Prepare() error = nil, want error")
	}
	if err := nilWriter.Rollback(nil); err == nil {
		t.Fatal("nil Rollback() error = nil, want error")
	}
	writer := &Writer{Filename: filepath.Join(t.TempDir(), "runtime.log"), MaxSize: 1}
	if _, err := writer.Prepare(-1); err == nil {
		t.Fatal("negative Prepare() error = nil, want error")
	}
	if err := writer.Rollback(struct{}{}); err == nil {
		t.Fatal("invalid Rollback() error = nil, want error")
	}

	filePath := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(filePath, []byte("file"), 0o600); err != nil {
		t.Fatalf("WriteFile(): %v", err)
	}
	if _, err := (&Writer{Filename: filepath.Join(filePath, "child"), MaxSize: 1}).Prepare(1); err == nil {
		t.Fatal("Prepare(stat failure) error = nil, want error")
	}
}

func TestWriterRejectsNilAndOversizedWrites(t *testing.T) {
	var nilWriter *Writer
	if _, err := nilWriter.Write([]byte("x")); err == nil {
		t.Fatal("nil Write() error = nil, want error")
	}
	writer := &Writer{Filename: filepath.Join(t.TempDir(), "runtime.log"), MaxSize: 1}
	if _, err := writer.Write(bytes.Repeat([]byte("x"), 1024*1024+1)); err == nil {
		t.Fatal("oversized Write() error = nil, want error")
	}
}

func TestWriterPrepareRejectsOversizedWrite(t *testing.T) {
	writer := &Writer{Filename: filepath.Join(t.TempDir(), "runtime.log"), MaxSize: 1}
	if _, err := writer.Prepare(1024*1024 + 1); err == nil {
		t.Fatal("Prepare() error = nil, want oversized write rejection")
	}
}

func assertPathExists(t *testing.T, path string, want bool) {
	t.Helper()
	_, err := os.Stat(path)
	got := err == nil
	if got != want {
		t.Fatalf("Stat(%q) exists = %t, want %t (err=%v)", path, got, want, err)
	}
}
