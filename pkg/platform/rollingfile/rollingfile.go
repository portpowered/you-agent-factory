// Package rollingfile provides a lifecycle-owned rolling file writer.
//
// The writer deliberately performs retention work synchronously after a file
// is opened or rotated. This keeps the maintenance lifecycle bounded by the
// writer's Close call instead of leaving an internal maintenance goroutine
// alive after the owning runtime has stopped.
package rollingfile

import (
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	backupTimeLayout = "2006-01-02T15-04-05.000"
	defaultMaxSize   = 100
	compressSuffix   = ".gz"
)

// Writer writes to Filename and rotates the active file when it exceeds
// MaxSize megabytes. Backups use the same timestamped naming and retention
// policy as the runtime artifact writers.
type Writer struct {
	Filename   string
	MaxSize    int
	MaxAge     int
	MaxBackups int
	LocalTime  bool
	Compress   bool

	mu   sync.Mutex
	file *os.File
	size int64
}

var _ io.WriteCloser = (*Writer)(nil)

// Write appends p to the active file, rotating before the write when the
// configured size limit would be exceeded.
func (w *Writer) Write(p []byte) (int, error) {
	if w == nil {
		return 0, errors.New("rolling file writer is nil")
	}
	w.mu.Lock()
	defer w.mu.Unlock()

	maxSize := w.maxSize()
	writeLen := int64(len(p))
	if writeLen > maxSize {
		return 0, fmt.Errorf("write length %d exceeds maximum file size %d", writeLen, maxSize)
	}
	if w.file == nil {
		if err := w.openExistingOrNew(writeLen); err != nil {
			return 0, err
		}
	}
	if w.size+writeLen > maxSize {
		if err := w.rotate(); err != nil {
			return 0, err
		}
	}

	n, err := w.file.Write(p)
	w.size += int64(n)
	return n, err
}

// Close closes the active file. A later Write may reopen the file, matching
// the behavior expected from an io rolling writer; owner wrappers enforce
// their own closed-state policy when late writes must be rejected.
func (w *Writer) Close() error {
	if w == nil {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.close()
}

// Rotate closes the active file, moves it to a timestamped backup, and opens
// a fresh active file.
func (w *Writer) Rotate() error {
	if w == nil {
		return errors.New("rolling file writer is nil")
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.rotate()
}

func (w *Writer) maxSize() int64 {
	maxSize := w.MaxSize
	if maxSize <= 0 {
		maxSize = defaultMaxSize
	}
	return int64(maxSize) * 1024 * 1024
}

func (w *Writer) openExistingOrNew(writeLen int64) error {
	info, err := os.Stat(w.Filename)
	if errors.Is(err, os.ErrNotExist) {
		return w.openNew()
	}
	if err != nil {
		return fmt.Errorf("error getting log file info: %w", err)
	}
	if info.Size()+writeLen >= w.maxSize() {
		return w.rotate()
	}

	file, err := os.OpenFile(w.Filename, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		// A file can disappear between Stat and OpenFile. As with the old
		// rolling writer, fall back to creating a fresh active file.
		return w.openNew()
	}
	w.file = file
	w.size = info.Size()
	return nil
}

func (w *Writer) openNew() error {
	if err := os.MkdirAll(filepath.Dir(w.Filename), 0o755); err != nil {
		return fmt.Errorf("can't make directories for new logfile: %w", err)
	}

	mode := os.FileMode(0o600)
	if info, err := os.Stat(w.Filename); err == nil {
		mode = info.Mode()
		backupPath, err := w.nextBackupPath()
		if err != nil {
			return err
		}
		if err := os.Rename(w.Filename, backupPath); err != nil {
			return fmt.Errorf("can't rename log file: %w", err)
		}
	}

	file, err := os.OpenFile(w.Filename, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("can't open new logfile: %w", err)
	}
	w.file = file
	w.size = 0
	// Retention is intentionally synchronous and errors are diagnostic-only;
	// they must not turn a successful log write into a runtime failure.
	_ = w.cleanBackups()
	return nil
}

func (w *Writer) nextBackupPath() (string, error) {
	base := w.backupPath(time.Now())
	if _, err := os.Stat(base); errors.Is(err, os.ErrNotExist) {
		return base, nil
	} else if err != nil {
		return "", fmt.Errorf("inspect log backup path: %w", err)
	}
	for index := 1; ; index++ {
		candidate := backupPathWithSuffix(base, index)
		if _, err := os.Stat(candidate); errors.Is(err, os.ErrNotExist) {
			return candidate, nil
		} else if err != nil {
			return "", fmt.Errorf("inspect log backup path: %w", err)
		}
	}
}

func (w *Writer) backupPath(at time.Time) string {
	if !w.LocalTime {
		at = at.UTC()
	}
	ext := filepath.Ext(w.Filename)
	base := strings.TrimSuffix(filepath.Base(w.Filename), ext)
	return filepath.Join(filepath.Dir(w.Filename), base+"-"+at.Format(backupTimeLayout)+ext)
}

func backupPathWithSuffix(path string, index int) string {
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	return fmt.Sprintf("%s-%d%s", base, index, ext)
}

func (w *Writer) rotate() error {
	if err := w.close(); err != nil {
		return err
	}
	return w.openNew()
}

func (w *Writer) close() error {
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	w.size = 0
	return err
}

type backup struct {
	path      string
	name      string
	baseName  string
	timestamp time.Time
}

func (w *Writer) cleanBackups() error {
	entries, err := os.ReadDir(filepath.Dir(w.Filename))
	if err != nil {
		return err
	}
	backups := make([]backup, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		parsed, ok := w.parseBackup(entry.Name())
		if !ok {
			continue
		}
		backups = append(backups, backup{
			path: filepath.Join(filepath.Dir(w.Filename), entry.Name()), name: entry.Name(),
			baseName: parsed.baseName, timestamp: parsed.timestamp,
		})
	}
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].timestamp.Before(backups[j].timestamp)
	})

	remove := make(map[string]struct{})
	if w.MaxBackups > 0 {
		preserved := make(map[string]struct{})
		for _, file := range backups {
			preserved[file.baseName] = struct{}{}
			if len(preserved) > w.MaxBackups {
				remove[file.path] = struct{}{}
			}
		}
	}
	if w.MaxAge > 0 {
		cutoff := time.Now().Add(-time.Duration(w.MaxAge) * 24 * time.Hour)
		for _, file := range backups {
			if file.timestamp.Before(cutoff) {
				remove[file.path] = struct{}{}
			}
		}
	}

	var firstErr error
	for path := range remove {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) && firstErr == nil {
			firstErr = err
		}
	}
	if w.Compress {
		for _, file := range backups {
			if filepath.Ext(file.name) == compressSuffix {
				continue
			}
			if _, removed := remove[file.path]; removed {
				continue
			}
			if err := compress(file.path, file.path+compressSuffix); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

type parsedBackup struct {
	baseName  string
	timestamp time.Time
}

func (w *Writer) parseBackup(name string) (parsedBackup, bool) {
	ext := filepath.Ext(w.Filename)
	prefix := strings.TrimSuffix(filepath.Base(w.Filename), ext) + "-"
	if !strings.HasPrefix(name, prefix) {
		return parsedBackup{}, false
	}
	compressed := strings.HasSuffix(name, compressSuffix)
	withoutCompression := name
	if compressed {
		withoutCompression = strings.TrimSuffix(withoutCompression, compressSuffix)
	}
	if !strings.HasSuffix(withoutCompression, ext) {
		return parsedBackup{}, false
	}
	withoutExtension := strings.TrimSuffix(withoutCompression, ext)
	suffix := strings.TrimPrefix(withoutExtension, prefix)
	if len(suffix) < len(backupTimeLayout) {
		return parsedBackup{}, false
	}
	stamp := suffix[:len(backupTimeLayout)]
	parsed, err := time.Parse(backupTimeLayout, stamp)
	if err != nil {
		return parsedBackup{}, false
	}
	baseName := strings.TrimSuffix(withoutCompression, compressSuffix)
	return parsedBackup{baseName: baseName, timestamp: parsed}, true
}

func compress(sourcePath, targetPath string) error {
	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()
	target, err := os.Create(targetPath)
	if err != nil {
		return err
	}
	gzipWriter := gzip.NewWriter(target)
	_, copyErr := io.Copy(gzipWriter, source)
	closeGzipErr := gzipWriter.Close()
	closeTargetErr := target.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeGzipErr != nil {
		return closeGzipErr
	}
	if closeTargetErr != nil {
		return closeTargetErr
	}
	return os.Remove(sourcePath)
}
