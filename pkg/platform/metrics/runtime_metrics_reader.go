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

// Reader is the narrow streaming read boundary consumed by higher-level
// observability services. The reader is stateless; callers supply the artifact
// root and record visitor for each operation.
type Reader interface {
	Stream(context.Context, string, func(RuntimeMetricRecord) error) error
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
	if r == nil || r.filesystem == nil {
		return &RuntimeMetricsReadError{Operation: "read runtime metrics", Cause: errors.New("filesystem is required")}
	}
	if visit == nil {
		return &RuntimeMetricsReadError{Operation: "read runtime metrics", Cause: errors.New("record visitor is required")}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return &RuntimeMetricsReadError{Operation: "read runtime metrics", Cause: err}
	}
	if root == "" {
		return &RuntimeMetricsReadError{Operation: "read runtime metrics", Cause: errors.New("root is required")}
	}
	root = filepath.Clean(root)
	info, err := r.filesystem.Stat(root)
	if err != nil {
		return &RuntimeMetricsReadError{Operation: "read runtime metrics root", Path: root, Cause: err}
	}
	if !info.IsDir() {
		return &RuntimeMetricsReadError{Operation: "read runtime metrics root", Path: root, Cause: errors.New("not a directory")}
	}

	err = r.filesystem.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if walkErr != nil {
			return &RuntimeMetricsReadError{Operation: "inspect runtime metrics path", Path: path, Cause: walkErr}
		}
		if entry.IsDir() || entry.Type()&fs.ModeSymlink != 0 || !isRuntimeMetricsArtifact(entry.Name()) {
			return nil
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		return readRuntimeMetricsArtifact(ctx, path, r.filesystem, visit)
	})
	if err != nil {
		if _, ok := err.(*RuntimeMetricsReadError); ok {
			return err
		}
		return &RuntimeMetricsReadError{Operation: "discover runtime metrics under", Path: root, Cause: err}
	}
	return nil
}

func isRuntimeMetricsArtifact(name string) bool {
	return runtimeMetricsActiveName.MatchString(name) || runtimeMetricsBackupName.MatchString(name)
}

func readRuntimeMetricsArtifact(
	ctx context.Context,
	path string,
	filesystem ArtifactFileSystem,
	visit func(RuntimeMetricRecord) error,
) (returnErr error) {
	file, err := filesystem.Open(path)
	if err != nil {
		return &RuntimeMetricsReadError{Operation: "read runtime metrics artifact", Path: path, Cause: err}
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

	err = readRuntimeMetricsJSONL(ctx, source, visit)
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
	visit func(RuntimeMetricRecord) error,
) error {
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
		if len(bytes.TrimSpace(line)) > 0 {
			record, decodeErr := decodeRuntimeMetricRecord(line)
			if decodeErr != nil {
				if errors.Is(readErr, io.EOF) {
					return nil
				}
				return fmt.Errorf("line %d: malformed JSON: %w", lineNumber, decodeErr)
			}
			if err := visit(record); err != nil {
				return err
			}
		}
		if errors.Is(readErr, io.EOF) {
			return nil
		}
	}
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
