package metrics

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
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
	if len(got) != 0 {
		t.Fatalf("records returned with malformed line = %#v, want no partial result", got)
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

func TestRuntimeMetricsReaderStreamsCancellationAndClosesArtifact(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, readerActiveName)
	if err := os.WriteFile(path, []byte(
		`{"record_id":"first"}`+"\n"+`{"record_id":"second"}`+"\n",
	), 0o600); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}

	filesystem := &trackingArtifactFileSystem{}
	reader, err := NewRuntimeMetricsReader(filesystem)
	if err != nil {
		t.Fatalf("NewRuntimeMetricsReader() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	visited := 0
	err = reader.Stream(ctx, root, func(RuntimeMetricRecord) error {
		visited++
		cancel()
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Stream() error = %v, want context.Canceled", err)
	}
	if visited != 1 {
		t.Fatalf("visited records = %d, want cancellation before the second record", visited)
	}
	if filesystem.opened != 1 || filesystem.closed != 1 {
		t.Fatalf("artifact handles opened=%d closed=%d, want one closed handle", filesystem.opened, filesystem.closed)
	}
	var readErr *RuntimeMetricsReadError
	if !errors.As(err, &readErr) {
		t.Fatalf("Stream() error = %v, want typed RuntimeMetricsReadError", err)
	}
}

func assertRuntimeMetricsReaderClosesArtifactsAfterSuccessAndVisitorFailure(t *testing.T) {
	t.Helper()
	root := installReaderFixtureTree(t)
	visitorErr := errors.New("stop after first record")

	filesystem := &trackingArtifactFileSystem{}
	reader, err := NewRuntimeMetricsReader(filesystem)
	if err != nil {
		t.Fatalf("NewRuntimeMetricsReader() error = %v", err)
	}
	if err := reader.Stream(context.Background(), root, func(RuntimeMetricRecord) error { return nil }); err != nil {
		t.Fatalf("successful Stream() error = %v", err)
	}
	if filesystem.opened == 0 || filesystem.closed != filesystem.opened {
		t.Fatalf("successful stream artifact handles opened=%d closed=%d, want every opened artifact closed", filesystem.opened, filesystem.closed)
	}

	filesystem = &trackingArtifactFileSystem{}
	reader, err = NewRuntimeMetricsReader(filesystem)
	if err != nil {
		t.Fatalf("NewRuntimeMetricsReader() error = %v", err)
	}
	err = reader.Stream(context.Background(), root, func(RuntimeMetricRecord) error { return visitorErr })
	if !errors.Is(err, visitorErr) {
		t.Fatalf("visitor-failure Stream() error = %v, want %v", err, visitorErr)
	}
	if filesystem.opened == 0 || filesystem.closed != filesystem.opened {
		t.Fatalf("visitor-failure stream artifact handles opened=%d closed=%d, want every opened artifact closed", filesystem.opened, filesystem.closed)
	}
}

func TestRuntimeMetricsReaderRequiresFilesystem(t *testing.T) {
	if reader, err := NewRuntimeMetricsReader(nil); reader != nil || err == nil {
		t.Fatalf("NewRuntimeMetricsReader(nil) = (%#v, %v), want construction failure", reader, err)
	}
}

func TestRuntimeMetricsReaderSelectsPathsAndEnvelopesBeforeMaterializingRecords(t *testing.T) {
	result := streamSelectedMetrics(t)
	assertSelectedRecords(t, result.records)
	assertSelectedEnvelopes(t, result.selectedPath, result.envelopes)
	assertSelectedReadStats(t, result.stats)
	if !containsPath(result.paths, result.unselectedDir) {
		t.Fatalf("path selector was not consulted for unselected directory: %v", result.paths)
	}
}

type selectedMetricsReadResult struct {
	selectedPath  string
	unselectedDir string
	stats         *RuntimeMetricsReadStats
	envelopes     []RuntimeMetricRecordEnvelope
	paths         []string
	records       []RuntimeMetricRecord
}

func streamSelectedMetrics(t *testing.T) selectedMetricsReadResult {
	t.Helper()
	root := t.TempDir()
	selectedDir := filepath.Join(root, "2026", "08", "20")
	unselectedDir := filepath.Join(root, "2026", "08", "21")
	if err := os.MkdirAll(selectedDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(selectedDir): %v", err)
	}
	if err := os.MkdirAll(unselectedDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(unselectedDir): %v", err)
	}
	selectedPath := filepath.Join(selectedDir, readerActiveName)
	unselectedPath := filepath.Join(unselectedDir, readerActiveName)
	selectedData := []byte(
		`{"record_id":"keep","metric_name":"dispatch.started","value":1}` + "\n" +
			`{"record_id":"drop","metric_name":"dispatch.completed","value":2}` + "\n" +
			"\n",
	)
	if err := os.WriteFile(selectedPath, selectedData, 0o600); err != nil {
		t.Fatalf("WriteFile(selectedPath): %v", err)
	}
	if err := os.WriteFile(unselectedPath, []byte(`{"record_id":"outside"}`+"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(unselectedPath): %v", err)
	}
	if err := os.WriteFile(filepath.Join(selectedDir, "operator-note.txt"), []byte("ignored"), 0o600); err != nil {
		t.Fatalf("WriteFile(operator note): %v", err)
	}

	reader := newRuntimeMetricsReader(t)
	stats := &RuntimeMetricsReadStats{}
	var envelopes []RuntimeMetricRecordEnvelope
	var paths []string
	var records []RuntimeMetricRecord
	err := reader.StreamSelected(nil, root, StreamSelection{
		Path: func(path string, isDirectory bool) bool {
			paths = append(paths, path)
			if isDirectory {
				return path == root || (strings.HasPrefix(path, filepath.Join(root, "2026")) && path != unselectedDir)
			}
			return path != filepath.Join(selectedDir, "operator-note.txt")
		},
		EnvelopeFields: []string{"record_id", "metric_name", "value", "missing"},
		IncludeEnvelope: func(envelope RuntimeMetricRecordEnvelope) bool {
			envelopes = append(envelopes, envelope)
			return envelope.Fields["record_id"] == "keep"
		},
		Stats: stats,
	}, func(record RuntimeMetricRecord) error {
		records = append(records, record)
		return nil
	})
	if err != nil {
		t.Fatalf("StreamSelected(): %v", err)
	}
	return selectedMetricsReadResult{
		selectedPath:  selectedPath,
		unselectedDir: unselectedDir,
		stats:         stats,
		envelopes:     envelopes,
		paths:         paths,
		records:       records,
	}
}

func assertSelectedRecords(t *testing.T, records []RuntimeMetricRecord) {
	t.Helper()
	if len(records) != 1 || records[0]["record_id"] != "keep" {
		t.Fatalf("selected records = %#v, want only keep record", records)
	}
}

func assertSelectedEnvelopes(t *testing.T, selectedPath string, envelopes []RuntimeMetricRecordEnvelope) {
	t.Helper()
	if len(envelopes) != 2 {
		t.Fatalf("envelopes = %#v, want both complete JSON records", envelopes)
	}
	if envelopes[0].Path != selectedPath || envelopes[0].Fields["metric_name"] != "dispatch.started" {
		t.Fatalf("first envelope = %#v, want selected path and string fields", envelopes[0])
	}
	if _, exists := envelopes[0].Fields["value"]; exists {
		t.Fatalf("numeric envelope field was materialized as text: %#v", envelopes[0].Fields)
	}
}

func assertSelectedReadStats(t *testing.T, stats *RuntimeMetricsReadStats) {
	t.Helper()
	if stats.DirectoriesVisited == 0 || stats.ArtifactsVisited != 1 || stats.ArtifactsOpened != 1 ||
		stats.BytesRead == 0 || stats.RecordsDecoded != 1 {
		t.Fatalf("read stats = %#v, want pruned traversal and one decoded record", *stats)
	}
}

func TestRuntimeMetricsReaderValidatesInputs(t *testing.T) {
	var nilReader *RuntimeMetricsReader
	if err := nilReader.Stream(context.Background(), t.TempDir(), func(RuntimeMetricRecord) error { return nil }); err == nil || !strings.Contains(err.Error(), "filesystem is required") {
		t.Fatalf("nil reader error = %v, want filesystem validation", err)
	}
	reader := newRuntimeMetricsReader(t)
	root := t.TempDir()
	fileRoot := filepath.Join(root, "not-a-directory")
	if err := os.WriteFile(fileRoot, []byte("x"), 0o600); err != nil {
		t.Fatalf("WriteFile(fileRoot): %v", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	tests := []struct {
		name string
		call func() error
		want string
	}{
		{name: "visitor", call: func() error { return reader.Stream(context.Background(), root, nil) }, want: "record visitor is required"},
		{name: "canceled", call: func() error { return reader.Stream(canceled, root, func(RuntimeMetricRecord) error { return nil }) }, want: "context canceled"},
		{name: "root", call: func() error {
			return reader.Stream(context.Background(), "", func(RuntimeMetricRecord) error { return nil })
		}, want: "root is required"},
		{name: "not directory", call: func() error {
			return reader.Stream(context.Background(), fileRoot, func(RuntimeMetricRecord) error { return nil })
		}, want: "not a directory"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.call()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestRuntimeMetricsReaderReportsDiscoveryFailure(t *testing.T) {
	root := t.TempDir()
	readError := errors.New("walk failed")
	walkReader, err := NewRuntimeMetricsReader(&walkErrorArtifactFileSystem{err: readError})
	if err != nil {
		t.Fatalf("NewRuntimeMetricsReader(walk error): %v", err)
	}
	if err := walkReader.Stream(context.Background(), root, func(RuntimeMetricRecord) error { return nil }); err == nil || !errors.Is(err, readError) || !strings.Contains(err.Error(), "discover runtime metrics") {
		t.Fatalf("walk error = %v, want wrapped discovery failure", err)
	}
}

func TestRuntimeMetricsReaderReportsOpenAndVisitorFailures(t *testing.T) {
	reader := newRuntimeMetricsReader(t)
	root := t.TempDir()
	artifactPath := filepath.Join(root, readerActiveName)
	if err := os.WriteFile(artifactPath, []byte(`{"record_id":"one"}`+"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(artifactPath): %v", err)
	}
	openError := errors.New("open failed")
	openReader, err := NewRuntimeMetricsReader(&openErrorArtifactFileSystem{openPath: artifactPath, err: openError})
	if err != nil {
		t.Fatalf("NewRuntimeMetricsReader(open error): %v", err)
	}
	if err := openReader.Stream(context.Background(), root, func(RuntimeMetricRecord) error { return nil }); err == nil || !errors.Is(err, openError) || !strings.Contains(err.Error(), filepath.Base(artifactPath)) {
		t.Fatalf("open error = %v, want artifact context", err)
	}

	visitError := errors.New("consumer stopped")
	if err := reader.Stream(context.Background(), root, func(RuntimeMetricRecord) error { return visitError }); err == nil || !errors.Is(err, visitError) || !strings.Contains(err.Error(), "decode runtime metrics artifact") {
		t.Fatalf("visitor error = %v, want wrapped decode boundary", err)
	}
	assertRuntimeMetricsReaderClosesArtifactsAfterSuccessAndVisitorFailure(t)
}

func TestRuntimeMetricsReaderReportsCloseFailure(t *testing.T) {
	root := t.TempDir()
	artifactPath := filepath.Join(root, readerActiveName)
	if err := os.WriteFile(artifactPath, []byte(`{"record_id":"one"}`+"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(artifactPath): %v", err)
	}
	closeError := errors.New("close failed")
	closeReader, err := NewRuntimeMetricsReader(&closeErrorArtifactFileSystem{closeErr: closeError})
	if err != nil {
		t.Fatalf("NewRuntimeMetricsReader(close error): %v", err)
	}
	if err := closeReader.Stream(context.Background(), root, func(RuntimeMetricRecord) error { return nil }); err == nil || !errors.Is(err, closeError) || !strings.Contains(err.Error(), "close runtime metrics artifact") {
		t.Fatalf("close error = %v, want close boundary", err)
	}
}

func TestRuntimeMetricsReadErrorPreservesCause(t *testing.T) {
	readError := errors.New("walk failed")
	typed := &RuntimeMetricsReadError{Operation: "read", Cause: readError}
	if typed.Error() != "read: walk failed" || !errors.Is(typed, readError) {
		t.Fatalf("typed read error = %q, unwrap=%v", typed.Error(), typed.Unwrap())
	}
	var nilTyped *RuntimeMetricsReadError
	if nilTyped.Error() != "" || nilTyped.Unwrap() != nil {
		t.Fatalf("nil typed read error = (%q, %v), want empty and nil", nilTyped.Error(), nilTyped.Unwrap())
	}
}

func containsPath(paths []string, want string) bool {
	for _, path := range paths {
		if path == want {
			return true
		}
	}
	return false
}

type walkErrorArtifactFileSystem struct {
	err error
}

func (filesystem *walkErrorArtifactFileSystem) Stat(string) (fs.FileInfo, error) {
	return os.Stat(".")
}

func (filesystem *walkErrorArtifactFileSystem) WalkDir(string, fs.WalkDirFunc) error {
	return filesystem.err
}

func (filesystem *walkErrorArtifactFileSystem) Open(string) (io.ReadCloser, error) {
	return nil, fmt.Errorf("unexpected Open call")
}

type openErrorArtifactFileSystem struct {
	openPath string
	err      error
}

func (filesystem *openErrorArtifactFileSystem) Stat(path string) (fs.FileInfo, error) {
	return os.Stat(path)
}

func (filesystem *openErrorArtifactFileSystem) WalkDir(root string, walk fs.WalkDirFunc) error {
	return filepath.WalkDir(root, walk)
}

func (filesystem *openErrorArtifactFileSystem) Open(path string) (io.ReadCloser, error) {
	if path == filesystem.openPath {
		return nil, filesystem.err
	}
	return os.Open(path)
}

type closeErrorArtifactFileSystem struct {
	closeErr error
}

func (filesystem *closeErrorArtifactFileSystem) Stat(path string) (fs.FileInfo, error) {
	return os.Stat(path)
}

func (filesystem *closeErrorArtifactFileSystem) WalkDir(root string, walk fs.WalkDirFunc) error {
	return filepath.WalkDir(root, walk)
}

func (filesystem *closeErrorArtifactFileSystem) Open(path string) (io.ReadCloser, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	return closeErrorReadCloser{ReadCloser: file, err: filesystem.closeErr}, nil
}

type closeErrorReadCloser struct {
	io.ReadCloser
	err error
}

func (file closeErrorReadCloser) Close() error {
	_ = file.ReadCloser.Close()
	return file.err
}

func newRuntimeMetricsReader(t *testing.T) *RuntimeMetricsReader {
	t.Helper()
	reader, err := NewRuntimeMetricsReader(platformfilesystem.Local{})
	if err != nil {
		t.Fatalf("NewRuntimeMetricsReader() error = %v", err)
	}
	return reader
}

type trackingArtifactFileSystem struct {
	platformfilesystem.Local
	opened int
	closed int
}

func (filesystem *trackingArtifactFileSystem) Open(path string) (io.ReadCloser, error) {
	file, err := filesystem.Local.Open(path)
	if err != nil {
		return nil, err
	}
	filesystem.opened++
	return trackingArtifactFile{ReadCloser: file, close: func() { filesystem.closed++ }}, nil
}

type trackingArtifactFile struct {
	io.ReadCloser
	close func()
}

func (file trackingArtifactFile) Close() error {
	if file.close != nil {
		file.close()
	}
	return file.ReadCloser.Close()
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
