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
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
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
	now  func() time.Time
}

// Checkpoint is a reversible position in the active rolling file. It is used
// by callers that publish a related external effect only after a transcript
// record has been made visible.
type Checkpoint struct {
	filename string
	size     int64
	active   bool
	existed  bool
}

var _ io.WriteCloser = (*Writer)(nil)

// CanWrite reports whether one write of writeLen bytes can fit without
// rotation. Transactional callers use this conservative check so a failed
// external publication can roll the transcript back within the same file.
func (w *Writer) CanWrite(writeLen int) bool {
	if w == nil || writeLen < 0 {
		return false
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	maxSize := w.maxSize()
	if int64(writeLen) > maxSize {
		return false
	}
	if w.file != nil {
		return w.size+int64(writeLen) <= maxSize
	}
	info, err := os.Stat(w.Filename)
	if errors.Is(err, os.ErrNotExist) {
		return true
	}
	if err != nil {
		return false
	}
	// openExistingOrNew rotates at the equality boundary when no active file
	// is open, so leave one byte of headroom in that state.
	return info.Size()+int64(writeLen) < maxSize
}

// Checkpoint captures the current active-file position without opening the
// file. A subsequent Rollback restores it when no rotation is required.
func (w *Writer) Checkpoint() (any, error) {
	if w == nil {
		return nil, errors.New("rolling file writer is nil")
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	checkpoint := Checkpoint{filename: w.Filename, active: w.file != nil, size: w.size}
	if w.file != nil {
		return checkpoint, nil
	}
	info, err := os.Stat(w.Filename)
	if errors.Is(err, os.ErrNotExist) {
		return checkpoint, nil
	}
	if err != nil {
		return nil, err
	}
	checkpoint.existed = true
	checkpoint.size = info.Size()
	return checkpoint, nil
}

// Rollback restores a checkpoint created by Checkpoint. The caller must have
// used CanWrite first; that prevents a transactional record from crossing a
// rolling boundary that cannot be restored as one file position.
func (w *Writer) Rollback(value any) error {
	if w == nil {
		return errors.New("rolling file writer is nil")
	}
	checkpoint, ok := value.(Checkpoint)
	if !ok || checkpoint.filename != w.Filename {
		return errors.New("invalid rolling file checkpoint")
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file != nil {
		if err := w.file.Truncate(checkpoint.size); err != nil {
			return err
		}
		if _, err := w.file.Seek(checkpoint.size, io.SeekStart); err != nil {
			return err
		}
		if checkpoint.active {
			w.size = checkpoint.size
			return nil
		}
		if err := w.file.Close(); err != nil {
			return err
		}
		w.file = nil
		w.size = 0
		if !checkpoint.existed {
			return os.Remove(w.Filename)
		}
		return nil
	}
	if checkpoint.active {
		file, err := os.OpenFile(w.Filename, os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		defer file.Close()
		if err := file.Truncate(checkpoint.size); err != nil {
			return err
		}
	}
	w.size = 0
	return nil
}

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
	base := w.backupPath(w.currentTime())
	parsedBase, ok := w.parseBackup(filepath.Base(base))
	if !ok {
		return "", fmt.Errorf("parse log backup path %q", base)
	}
	highestIndex := -1
	backups, err := w.readBackups()
	if err != nil {
		return "", fmt.Errorf("inspect existing log backups: %w", err)
	}
	for _, backup := range backups {
		if backup.timestamp.Equal(parsedBase.timestamp) && backup.collisionIndex > highestIndex {
			highestIndex = backup.collisionIndex
		}
	}
	for index := highestIndex + 1; ; index++ {
		candidate := base
		if index > 0 {
			candidate = backupPathWithSuffix(base, index)
		}
		occupied, err := backupPathOccupied(candidate)
		if err != nil {
			return "", fmt.Errorf("inspect log backup path: %w", err)
		}
		if !occupied {
			return candidate, nil
		}
	}
}

func backupPathOccupied(path string) (bool, error) {
	for _, candidate := range []string{path, path + compressSuffix} {
		if _, err := os.Lstat(candidate); err == nil {
			return true, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return false, err
		}
	}
	return false, nil
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
	path           string
	name           string
	baseName       string
	timestamp      time.Time
	collisionIndex int
}

func (w *Writer) cleanBackups() error {
	backups, err := w.readBackups()
	if err != nil {
		return err
	}
	remove := w.retentionRemovals(backups)
	firstErr := removeBackups(remove)
	compressionErr := w.compressBackups(backups, remove)
	if firstErr == nil {
		firstErr = compressionErr
	}
	return firstErr
}

func (w *Writer) readBackups() ([]backup, error) {
	entries, err := os.ReadDir(filepath.Dir(w.Filename))
	if err != nil {
		return nil, err
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
			collisionIndex: parsed.collisionIndex,
		})
	}
	sort.SliceStable(backups, func(i, j int) bool {
		if !backups[i].timestamp.Equal(backups[j].timestamp) {
			return backups[i].timestamp.After(backups[j].timestamp)
		}
		if backups[i].collisionIndex != backups[j].collisionIndex {
			return backups[i].collisionIndex > backups[j].collisionIndex
		}
		return backups[i].name < backups[j].name
	})
	return backups, nil
}

func (w *Writer) retentionRemovals(backups []backup) map[string]struct{} {
	remove := make(map[string]struct{})
	if w.MaxBackups > 0 {
		w.markExcessBackups(backups, remove)
	}
	if w.MaxAge > 0 {
		w.markExpiredBackups(backups, remove)
	}
	return remove
}

func (w *Writer) markExcessBackups(backups []backup, remove map[string]struct{}) {
	preserved := make(map[string]struct{})
	for _, file := range backups {
		preserved[file.baseName] = struct{}{}
		if len(preserved) > w.MaxBackups {
			remove[file.path] = struct{}{}
		}
	}
}

func (w *Writer) markExpiredBackups(backups []backup, remove map[string]struct{}) {
	cutoff := w.currentTime().Add(-time.Duration(w.MaxAge) * 24 * time.Hour)
	for _, file := range backups {
		if file.timestamp.Before(cutoff) {
			remove[file.path] = struct{}{}
		}
	}
}

func removeBackups(remove map[string]struct{}) error {
	var firstErr error
	for path := range remove {
		if err := os.Remove(path); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

func (w *Writer) compressBackups(backups []backup, remove map[string]struct{}) error {
	if !w.Compress {
		return nil
	}
	var firstErr error
	for _, file := range backups {
		if filepath.Ext(file.name) == compressSuffix {
			continue
		}
		if _, removed := remove[file.path]; removed {
			continue
		}
		if err := compress(file.path, file.path+compressSuffix); err != nil {
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

type parsedBackup struct {
	baseName       string
	timestamp      time.Time
	collisionIndex int
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
	collisionIndex := 0
	collisionSuffix := suffix[len(backupTimeLayout):]
	if collisionSuffix != "" {
		if !strings.HasPrefix(collisionSuffix, "-") {
			return parsedBackup{}, false
		}
		collisionIndex, err = strconv.Atoi(strings.TrimPrefix(collisionSuffix, "-"))
		if err != nil || collisionIndex < 1 {
			return parsedBackup{}, false
		}
	}
	return parsedBackup{baseName: baseName, timestamp: parsed, collisionIndex: collisionIndex}, true
}

func (w *Writer) currentTime() time.Time {
	if w != nil && w.now != nil {
		return w.now()
	}
	return time.Now()
}

func compress(sourcePath, targetPath string) error {
	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	sourceInfo, err := source.Stat()
	if err != nil {
		_ = source.Close()
		return err
	}
	compressErr := compressReader(source, sourceInfo, targetPath)
	closeSourceErr := source.Close()
	if compressErr != nil {
		return compressErr
	}
	if closeSourceErr != nil {
		_ = os.Remove(targetPath)
		return closeSourceErr
	}
	if err := os.Remove(sourcePath); err != nil {
		_ = os.Remove(targetPath)
		return err
	}
	return nil
}

func compressReader(source io.Reader, sourceInfo fs.FileInfo, targetPath string) (returnErr error) {
	target, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, sourceInfo.Mode().Perm())
	if err != nil {
		return err
	}
	targetClosed := false
	removeTarget := true
	defer func() {
		if !targetClosed {
			if err := target.Close(); returnErr == nil && err != nil {
				returnErr = err
			}
		}
		if removeTarget {
			_ = os.Remove(targetPath)
		}
	}()

	if err := target.Chmod(sourceInfo.Mode().Perm()); err != nil {
		return err
	}
	if err := preserveFileOwnership(targetPath, sourceInfo); err != nil {
		return err
	}

	gzipWriter := gzip.NewWriter(target)
	_, copyErr := io.Copy(gzipWriter, source)
	closeGzipErr := gzipWriter.Close()
	closeTargetErr := target.Close()
	targetClosed = true
	if copyErr != nil {
		return copyErr
	}
	if closeGzipErr != nil {
		return closeGzipErr
	}
	if closeTargetErr != nil {
		return closeTargetErr
	}
	removeTarget = false
	return nil
}
