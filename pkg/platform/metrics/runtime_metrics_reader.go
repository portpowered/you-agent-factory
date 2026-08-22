package metrics

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"regexp"
)

const (
	runtimeMetricsArtifactPrefix = `^[0-9]{6}\.[0-9]{9}-runtime-metrics-[A-Za-z0-9_.-]+`
	runtimeMetricsBackupSuffix   = `-[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}-[0-9]{2}-[0-9]{2}\.[0-9]{3}(?:-[0-9]+)?`
)

var (
	runtimeMetricsActiveName = regexp.MustCompile(runtimeMetricsArtifactPrefix + `\.log$`)
	runtimeMetricsBackupName = regexp.MustCompile(runtimeMetricsArtifactPrefix + runtimeMetricsBackupSuffix + `\.log(?:\.gz)?$`)
)

// RuntimeMetricRecord is one decoded JSON object from a runtime metrics
// artifact. The Platform Metrics reader deliberately preserves metric fields
// without interpreting their names, labels, units, or values.
type RuntimeMetricRecord map[string]any

// Reader is the original collecting read boundary consumed by higher-level
// observability services. It remains the required compatibility contract for
// injected readers; callers that can use streaming should opt into one of the
// capability interfaces below.
type Reader interface {
	Read(context.Context, string) ([]RuntimeMetricRecord, error)
}

// StreamingReader is an optional capability for visiting records without
// collecting the complete metrics store in memory.
type StreamingReader interface {
	Stream(context.Context, string, func(RuntimeMetricRecord) error) error
}

// SelectedReader is an optional selective streaming capability. Production
// readers can prune artifacts and envelopes before materializing records while
// legacy Read-only readers remain valid through the Reader contract.
type SelectedReader interface {
	StreamSelected(context.Context, string, StreamSelection, func(RuntimeMetricRecord) error) error
}

// RuntimeMetricRecordEnvelope contains only the string fields requested by a
// caller's envelope selector. Platform Metrics does not interpret those
// fields; it merely exposes a policy-free pre-decode selection point.
type RuntimeMetricRecordEnvelope struct {
	Path   string
	Fields map[string]string
}

// RuntimeMetricsReadStats reports request-local traversal work. It is an
// optional observation port for tests and diagnostics; the reader never
// retains it between operations.
type RuntimeMetricsReadStats struct {
	DirectoriesVisited int
	ArtifactsVisited   int
	ArtifactsOpened    int
	BytesRead          int64
	RecordsDecoded     int
}

// StreamSelection supplies policy-free callbacks for one metrics read. The
// callbacks are owned by the caller and may reject a directory, artifact, or
// decoded envelope. A rejected envelope is not materialized as a full record.
// When Path is nil, every path is visited. When IncludeEnvelope is nil, every
// complete JSON object is materialized as before.
type StreamSelection struct {
	Path            func(path string, isDirectory bool) bool
	EnvelopeFields  []string
	IncludeEnvelope func(RuntimeMetricRecordEnvelope) bool
	Stats           *RuntimeMetricsReadStats
}

// RuntimeMetricsReadError is a safe, typed failure from metrics discovery or
// decoding. The cause remains available to errors.Is/errors.As callers while
// the rendered message contains only operation and path context.
type RuntimeMetricsReadError struct {
	Operation string
	Path      string
	Cause     error
}

func (err *RuntimeMetricsReadError) Error() string {
	if err == nil {
		return ""
	}
	if err.Path == "" {
		return fmt.Sprintf("%s: %v", err.Operation, err.Cause)
	}
	return fmt.Sprintf("%s %q: %v", err.Operation, err.Path, err.Cause)
}

func (err *RuntimeMetricsReadError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Cause
}

// ArtifactFileSystem supplies the policy-free filesystem effects needed to
// discover and stream selected metrics artifacts. The reader owns no ambient
// filesystem implementation; Wire supplies the concrete Platform adapter.
type ArtifactFileSystem interface {
	Stat(string) (fs.FileInfo, error)
	WalkDir(string, fs.WalkDirFunc) error
	Open(string) (io.ReadCloser, error)
}

// RuntimeMetricsReader streams complete JSONL records from the active and
// retained runtime metrics artifacts below a supplied root.
type RuntimeMetricsReader struct {
	filesystem ArtifactFileSystem
}

var _ Reader = (*RuntimeMetricsReader)(nil)
var _ StreamingReader = (*RuntimeMetricsReader)(nil)
var _ SelectedReader = (*RuntimeMetricsReader)(nil)

// NewRuntimeMetricsReader constructs the stateless runtime metrics reader
// from the exact filesystem effects selected by the composition root.
func NewRuntimeMetricsReader(filesystem ArtifactFileSystem) (*RuntimeMetricsReader, error) {
	if filesystem == nil {
		return nil, errors.New("construct runtime metrics reader: filesystem is required")
	}
	return &RuntimeMetricsReader{filesystem: filesystem}, nil
}

// Read is a compatibility helper that collects the stream into memory. The
// production query uses Stream so a growing metrics store does not retain all
// decoded records at once. A failed read never returns partial records.
func (r *RuntimeMetricsReader) Read(ctx context.Context, root string) ([]RuntimeMetricRecord, error) {
	records := make([]RuntimeMetricRecord, 0)
	err := r.Stream(ctx, root, func(record RuntimeMetricRecord) error {
		records = append(records, record)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return records, nil
}

// Stream discovers metric artifacts below root and visits complete JSONL
// records in deterministic artifact and line order. An incomplete final line
// is tolerated because an active writer may be interrupted during a write.
func (r *RuntimeMetricsReader) Stream(
	ctx context.Context,
	root string,
	visit func(RuntimeMetricRecord) error,
) error {
	return r.StreamSelected(ctx, root, StreamSelection{}, visit)
}

// StreamSelected discovers metric artifacts and applies the supplied
// request-local path and envelope selection before full record decoding.
// Platform Metrics only executes callbacks and does not interpret their
// dimension or metric meaning.
func (r *RuntimeMetricsReader) StreamSelected(
	ctx context.Context,
	root string,
	selection StreamSelection,
	visit func(RuntimeMetricRecord) error,
) error {
	ctx, root, err := r.prepareStream(ctx, root, visit)
	if err != nil {
		return err
	}
	err = r.filesystem.WalkDir(root, r.selectedWalk(ctx, selection, visit))
	return wrapRuntimeMetricsWalkError(root, err)
}

func (r *RuntimeMetricsReader) prepareStream(
	ctx context.Context,
	root string,
	visit func(RuntimeMetricRecord) error,
) (context.Context, string, error) {
	switch {
	case r == nil || r.filesystem == nil:
		return nil, "", &RuntimeMetricsReadError{Operation: "read runtime metrics", Cause: errors.New("filesystem is required")}
	case visit == nil:
		return nil, "", &RuntimeMetricsReadError{Operation: "read runtime metrics", Cause: errors.New("record visitor is required")}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, "", &RuntimeMetricsReadError{Operation: "read runtime metrics", Cause: err}
	}
	if root == "" {
		return nil, "", &RuntimeMetricsReadError{Operation: "read runtime metrics", Cause: errors.New("root is required")}
	}
	root = filepath.Clean(root)
	info, err := r.filesystem.Stat(root)
	if err != nil {
		return nil, root, &RuntimeMetricsReadError{Operation: "read runtime metrics root", Path: root, Cause: err}
	}
	if !info.IsDir() {
		return nil, root, &RuntimeMetricsReadError{Operation: "read runtime metrics root", Path: root, Cause: errors.New("not a directory")}
	}
	return ctx, root, nil
}

func (r *RuntimeMetricsReader) selectedWalk(
	ctx context.Context,
	selection StreamSelection,
	visit func(RuntimeMetricRecord) error,
) fs.WalkDirFunc {
	return func(path string, entry fs.DirEntry, walkErr error) error {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if entry != nil && entry.IsDir() && selection.Stats != nil {
			selection.Stats.DirectoriesVisited++
		}
		if walkErr != nil {
			return &RuntimeMetricsReadError{Operation: "inspect runtime metrics path", Path: path, Cause: walkErr}
		}
		if selection.Path != nil && !selection.Path(path, entry.IsDir()) {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if !isReadableRuntimeMetricsEntry(entry) {
			return nil
		}
		if selection.Stats != nil {
			selection.Stats.ArtifactsVisited++
		}
		return readRuntimeMetricsArtifact(ctx, path, r.filesystem, selection, visit)
	}
}

func isReadableRuntimeMetricsEntry(entry fs.DirEntry) bool {
	return !entry.IsDir() &&
		entry.Type()&fs.ModeSymlink == 0 &&
		entry.Type().IsRegular() &&
		isRuntimeMetricsArtifact(entry.Name())
}

func wrapRuntimeMetricsWalkError(root string, err error) error {
	if err == nil {
		return nil
	}
	if _, ok := err.(*RuntimeMetricsReadError); ok {
		return err
	}
	return &RuntimeMetricsReadError{Operation: "discover runtime metrics under", Path: root, Cause: err}
}

func isRuntimeMetricsArtifact(name string) bool {
	return runtimeMetricsActiveName.MatchString(name) || runtimeMetricsBackupName.MatchString(name)
}

func readRuntimeMetricsArtifact(
	ctx context.Context,
	path string,
	filesystem ArtifactFileSystem,
	selection StreamSelection,
	visit func(RuntimeMetricRecord) error,
) (returnErr error) {
	file, err := filesystem.Open(path)
	if err != nil {
		return &RuntimeMetricsReadError{Operation: "read runtime metrics artifact", Path: path, Cause: err}
	}
	if selection.Stats != nil {
		selection.Stats.ArtifactsOpened++
	}
	defer func() {
		if closeErr := file.Close(); returnErr == nil && closeErr != nil {
			returnErr = &RuntimeMetricsReadError{
				Operation: "close runtime metrics artifact",
				Path:      path,
				Cause:     closeErr,
			}
		}
	}()

	var source io.Reader = file
	var gzipReader *gzip.Reader
	if filepath.Ext(path) == ".gz" {
		gzipReader, err = gzip.NewReader(file)
		if err != nil {
			return &RuntimeMetricsReadError{
				Operation: "open gzip decoder for runtime metrics artifact",
				Path:      path,
				Cause:     err,
			}
		}
		defer func() {
			if closeErr := gzipReader.Close(); returnErr == nil && closeErr != nil {
				returnErr = &RuntimeMetricsReadError{
					Operation: "close gzip decoder for runtime metrics artifact",
					Path:      path,
					Cause:     closeErr,
				}
			}
		}()
		source = gzipReader
	}

	err = readRuntimeMetricsJSONL(ctx, source, path, selection, visit)
	if err != nil {
		if _, ok := err.(*RuntimeMetricsReadError); ok {
			return err
		}
		return &RuntimeMetricsReadError{Operation: "decode runtime metrics artifact", Path: path, Cause: err}
	}
	return nil
}

func readRuntimeMetricsJSONL(
	ctx context.Context,
	source io.Reader,
	path string,
	selection StreamSelection,
	visit func(RuntimeMetricRecord) error,
) error {
	if selection.Stats != nil {
		source = &countingReader{reader: source, stats: selection.Stats}
	}
	reader := bufio.NewReader(source)
	lineNumber := 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		line, readErr := reader.ReadBytes('\n')
		lineNumber++
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return readErr
		}
		ignoreOnEOF, lineErr := processRuntimeMetricsLine(path, line, lineNumber, selection, visit)
		if lineErr != nil && !(ignoreOnEOF && errors.Is(readErr, io.EOF)) {
			return lineErr
		}
		if errors.Is(readErr, io.EOF) {
			return nil
		}
	}
}

func processRuntimeMetricsLine(
	path string,
	line []byte,
	lineNumber int,
	selection StreamSelection,
	visit func(RuntimeMetricRecord) error,
) (bool, error) {
	if len(bytes.TrimSpace(line)) == 0 {
		return false, nil
	}
	if selection.IncludeEnvelope != nil {
		envelope, selected, err := selectRuntimeMetricEnvelope(path, line, selection)
		if err != nil {
			return true, err
		}
		if !selected || !selection.IncludeEnvelope(envelope) {
			return false, nil
		}
	}
	record, err := decodeRuntimeMetricRecord(line)
	if err != nil {
		return true, fmt.Errorf("line %d: malformed JSON: %w", lineNumber, err)
	}
	if selection.Stats != nil {
		selection.Stats.RecordsDecoded++
	}
	return false, visit(record)
}

func selectRuntimeMetricEnvelope(
	path string,
	line []byte,
	selection StreamSelection,
) (RuntimeMetricRecordEnvelope, bool, error) {
	var rawFields map[string]json.RawMessage
	if err := json.Unmarshal(line, &rawFields); err != nil {
		return RuntimeMetricRecordEnvelope{}, false, fmt.Errorf("decode runtime metrics envelope: %w", err)
	}
	if rawFields == nil {
		return RuntimeMetricRecordEnvelope{}, false, errors.New("runtime metrics envelope must be an object")
	}
	fields := make(map[string]string, len(selection.EnvelopeFields))
	for _, field := range selection.EnvelopeFields {
		value, ok := rawFields[field]
		if !ok {
			continue
		}
		var text string
		if err := json.Unmarshal(value, &text); err == nil {
			fields[field] = text
		}
	}
	return RuntimeMetricRecordEnvelope{Path: path, Fields: fields}, true, nil
}

type countingReader struct {
	reader io.Reader
	stats  *RuntimeMetricsReadStats
}

func (reader *countingReader) Read(buffer []byte) (int, error) {
	count, err := reader.reader.Read(buffer)
	if reader.stats != nil {
		reader.stats.BytesRead += int64(count)
	}
	return count, err
}

func decodeRuntimeMetricRecord(line []byte) (RuntimeMetricRecord, error) {
	var record RuntimeMetricRecord
	if err := json.Unmarshal(line, &record); err != nil {
		return nil, err
	}
	if record == nil {
		return nil, errors.New("JSON record must be an object")
	}
	return record, nil
}
