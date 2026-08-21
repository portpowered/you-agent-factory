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
	"sort"
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

// Reader is the narrow read boundary consumed by higher-level observability
// services. The reader is stateless; callers supply the artifact root for each
// operation.
type Reader interface {
	Read(context.Context, string) ([]RuntimeMetricRecord, error)
}

// ArtifactFileSystem supplies the policy-free filesystem effects needed to
// discover and stream selected metrics artifacts. The reader owns no ambient
// filesystem implementation; Wire supplies the concrete Platform adapter.
type ArtifactFileSystem interface {
	Stat(string) (fs.FileInfo, error)
	WalkDir(string, fs.WalkDirFunc) error
	Open(string) (io.ReadCloser, error)
}

// RuntimeMetricsReader reads complete JSONL records from the active and
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

// Read discovers metric artifacts below root and returns their complete JSON
// records in deterministic artifact and line order. An incomplete final line
// is tolerated because an active writer may be interrupted during a write.
func (r *RuntimeMetricsReader) Read(ctx context.Context, root string) ([]RuntimeMetricRecord, error) {
	if r == nil || r.filesystem == nil {
		return nil, errors.New("read runtime metrics: filesystem is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("read runtime metrics: %w", err)
	}

	artifacts, err := discoverRuntimeMetricsArtifacts(ctx, root, r.filesystem)
	if err != nil {
		return nil, err
	}
	records := make([]RuntimeMetricRecord, 0)
	for _, path := range artifacts {
		if err := ctx.Err(); err != nil {
			return records, fmt.Errorf("read runtime metrics artifact %q: %w", path, err)
		}
		artifactRecords, err := readRuntimeMetricsArtifact(ctx, path, r.filesystem)
		records = append(records, artifactRecords...)
		if err != nil {
			return records, err
		}
	}
	return records, nil
}

func discoverRuntimeMetricsArtifacts(
	ctx context.Context,
	root string,
	filesystem ArtifactFileSystem,
) ([]string, error) {
	if root == "" {
		return nil, errors.New("read runtime metrics: root is required")
	}
	root = filepath.Clean(root)
	info, err := filesystem.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("read runtime metrics root %q: %w", root, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("read runtime metrics root %q: not a directory", root)
	}

	artifacts := make([]string, 0)
	err = filesystem.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if walkErr != nil {
			return fmt.Errorf("inspect runtime metrics path %q: %w", path, walkErr)
		}
		if entry.IsDir() || entry.Type()&fs.ModeSymlink != 0 || !isRuntimeMetricsArtifact(entry.Name()) {
			return nil
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		artifacts = append(artifacts, path)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("discover runtime metrics under %q: %w", root, err)
	}
	sort.Strings(artifacts)
	return artifacts, nil
}

func isRuntimeMetricsArtifact(name string) bool {
	return runtimeMetricsActiveName.MatchString(name) || runtimeMetricsBackupName.MatchString(name)
}

func readRuntimeMetricsArtifact(
	ctx context.Context,
	path string,
	filesystem ArtifactFileSystem,
) (records []RuntimeMetricRecord, returnErr error) {
	file, err := filesystem.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read runtime metrics artifact %q: %w", path, err)
	}
	defer func() {
		if closeErr := file.Close(); returnErr == nil && closeErr != nil {
			returnErr = fmt.Errorf("read runtime metrics artifact %q: close: %w", path, closeErr)
		}
	}()

	var source io.Reader = file
	var gzipReader *gzip.Reader
	if filepath.Ext(path) == ".gz" {
		gzipReader, err = gzip.NewReader(file)
		if err != nil {
			return nil, fmt.Errorf("read runtime metrics artifact %q: open gzip decoder: %w", path, err)
		}
		defer func() {
			if closeErr := gzipReader.Close(); returnErr == nil && closeErr != nil {
				returnErr = fmt.Errorf("read runtime metrics artifact %q: close gzip decoder: %w", path, closeErr)
			}
		}()
		source = gzipReader
	}

	records, err = readRuntimeMetricsJSONL(ctx, source)
	if err != nil {
		return records, fmt.Errorf("read runtime metrics artifact %q: %w", path, err)
	}
	return records, nil
}

func readRuntimeMetricsJSONL(ctx context.Context, source io.Reader) ([]RuntimeMetricRecord, error) {
	reader := bufio.NewReader(source)
	records := make([]RuntimeMetricRecord, 0)
	lineNumber := 0
	for {
		if err := ctx.Err(); err != nil {
			return records, err
		}
		line, readErr := reader.ReadBytes('\n')
		lineNumber++
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return records, readErr
		}
		if len(bytes.TrimSpace(line)) > 0 {
			record, decodeErr := decodeRuntimeMetricRecord(line)
			if decodeErr != nil {
				if errors.Is(readErr, io.EOF) {
					return records, nil
				}
				return records, fmt.Errorf("line %d: malformed JSON: %w", lineNumber, decodeErr)
			}
			records = append(records, record)
		}
		if errors.Is(readErr, io.EOF) {
			return records, nil
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
