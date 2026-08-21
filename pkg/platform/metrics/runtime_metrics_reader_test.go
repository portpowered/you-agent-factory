package metrics

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
)

const (
	readerFixtureTree      = "testdata/reader-tree"
	readerMalformedFixture = "testdata/malformed"
	readerActiveName       = "120000.000000000-runtime-metrics-session-reader-runtime-reader-fixture.log"
	readerCompressedName   = "120000.000000000-runtime-metrics-session-reader-runtime-reader-fixture-2026-08-20T12-02-00.000-1.log.gz"
)

func TestRuntimeMetricsReaderReadsRetainedArtifactsAndToleratesTornTail(t *testing.T) {
	root := installReaderFixtureTree(t)
	reader := newRuntimeMetricsReader(t)

	want := []RuntimeMetricRecord{
		{"record_id": "rotated-1", "metric_name": "provider.completed"},
		{"record_id": "compressed-1", "metric_name": "provider.duration"},
		{"record_id": "active-1", "metric_name": "dispatch.started"},
		{"record_id": "active-2", "metric_name": "dispatch.completed"},
	}
	got, err := reader.Read(context.Background(), root)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Read() = %#v, want %#v", got, want)
	}

	repeated, err := reader.Read(context.Background(), root)
	if err != nil {
		t.Fatalf("repeated Read() error = %v", err)
	}
	if !reflect.DeepEqual(repeated, got) {
		t.Fatalf("repeated Read() = %#v, want deterministic result %#v", repeated, got)
	}
}

func TestRuntimeMetricsReaderRejectsMalformedCompleteLineWithArtifactContext(t *testing.T) {
	root := filepath.Join(t.TempDir(), "metrics")
	copyFixtureTree(t, readerMalformedFixture, root)
	path := filepath.Join(root, "120000.000000000-runtime-metrics-session-reader-runtime-reader-malformed.log")

	got, err := newRuntimeMetricsReader(t).Read(context.Background(), root)
	if err == nil {
		t.Fatal("Read() error = nil, want malformed complete-line error")
	}
	if len(got) != 1 || got[0]["record_id"] != "valid" {
		t.Fatalf("records returned with malformed line = %#v, want earlier complete record", got)
	}
	message := err.Error()
	for _, want := range []string{filepath.Base(path), "line 2", "malformed JSON"} {
		if !strings.Contains(message, want) {
			t.Fatalf("error = %q, want it to contain %q", message, want)
		}
	}
	if strings.Contains(message, "secret-payload") {
		t.Fatalf("error leaked record payload: %q", message)
	}
}

func TestRuntimeMetricsReaderRejectsNonObjectJSONRecord(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, readerActiveName)
	if err := os.WriteFile(path, []byte("null\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}

	_, err := newRuntimeMetricsReader(t).Read(context.Background(), root)
	if err == nil {
		t.Fatal("Read() error = nil, want JSON object validation error")
	}
	for _, want := range []string{filepath.Base(path), "line 1", "JSON record must be an object"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, want it to contain %q", err, want)
		}
	}
}

func TestRuntimeMetricsReaderReportsGzipDecoderFailureWithArtifactContext(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, readerCompressedName)
	if err := os.WriteFile(path, []byte("not-gzip-secret"), 0o600); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}

	_, err := newRuntimeMetricsReader(t).Read(context.Background(), root)
	if err == nil {
		t.Fatal("Read() error = nil, want gzip decoder error")
	}
	if !strings.Contains(err.Error(), filepath.Base(path)) || !strings.Contains(err.Error(), "gzip") {
		t.Fatalf("error = %q, want artifact and gzip context", err)
	}
	if strings.Contains(err.Error(), "not-gzip-secret") {
		t.Fatalf("error leaked artifact payload: %q", err)
	}
}

func TestRuntimeMetricsReaderHonorsCancellationAndMissingRoot(t *testing.T) {
	root := installReaderFixtureTree(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := newRuntimeMetricsReader(t).Read(ctx, root); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Read() error = %v, want context.Canceled", err)
	}

	missing := filepath.Join(t.TempDir(), "does-not-exist")
	if _, err := newRuntimeMetricsReader(t).Read(context.Background(), missing); err == nil || !strings.Contains(err.Error(), missing) {
		t.Fatalf("missing-root error = %v, want root context", err)
	}
}

func TestRuntimeMetricsReaderRequiresFilesystem(t *testing.T) {
	if reader, err := NewRuntimeMetricsReader(nil); reader != nil || err == nil {
		t.Fatalf("NewRuntimeMetricsReader(nil) = (%#v, %v), want construction failure", reader, err)
	}
}

func newRuntimeMetricsReader(t *testing.T) *RuntimeMetricsReader {
	t.Helper()
	reader, err := NewRuntimeMetricsReader(platformfilesystem.Local{})
	if err != nil {
		t.Fatalf("NewRuntimeMetricsReader() error = %v", err)
	}
	return reader
}

func installReaderFixtureTree(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "metrics")
	copyFixtureTree(t, readerFixtureTree, root)

	activePath := filepath.Join(root, "2026", "08", "20", readerActiveName)
	data, err := os.ReadFile(activePath)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", activePath, err)
	}
	data = bytes.TrimSuffix(data, []byte("\n"))
	if err := os.WriteFile(activePath, data, 0o600); err != nil {
		t.Fatalf("WriteFile(%q): %v", activePath, err)
	}

	source := filepath.Join(root, "compressed-source.jsonl")
	compressedPath := filepath.Join(root, "2026", "08", "20", readerCompressedName)
	compressFixture(t, source, compressedPath)
	return root
}

func copyFixtureTree(t *testing.T, source, destination string) {
	t.Helper()
	err := filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o600)
	})
	if err != nil {
		t.Fatalf("copy fixture tree %q: %v", source, err)
	}
}

func compressFixture(t *testing.T, source, destination string) {
	t.Helper()
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", source, err)
	}
	file, err := os.Create(destination)
	if err != nil {
		t.Fatalf("Create(%q): %v", destination, err)
	}
	writer := gzip.NewWriter(file)
	if _, err := io.Copy(writer, bytes.NewReader(data)); err != nil {
		_ = file.Close()
		t.Fatalf("compress %q: %v", source, err)
	}
	if err := writer.Close(); err != nil {
		_ = file.Close()
		t.Fatalf("close gzip fixture %q: %v", destination, err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close fixture %q: %v", destination, err)
	}
}
