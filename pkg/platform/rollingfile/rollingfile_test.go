package rollingfile

import (
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
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

func assertPathExists(t *testing.T, path string, want bool) {
	t.Helper()
	_, err := os.Stat(path)
	got := err == nil
	if got != want {
		t.Fatalf("Stat(%q) exists = %t, want %t (err=%v)", path, got, want, err)
	}
}
