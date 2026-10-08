// Package replay owns policy-free replay artifact persistence mechanics.
package replay

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

const (
	artifactReplaceAttempts = 20
	artifactReplaceDelay    = 10 * time.Millisecond
)

// Storage is the exact replay-artifact filesystem effect consumed by the
// Recordings owner.
type Storage interface {
	WriteFile(string, []byte) error
	ReadFile(string) ([]byte, error)
}

// Appender is the optional append-and-sync effect used by append-only replay
// recordings. It is intentionally separate from Storage so existing callers
// that only read or replace v1 artifacts do not acquire append semantics.
type Appender interface {
	AppendFile(string, []byte) error
}

// DirectoryScanner enumerates artifact names in bounded batches through the
// explicitly selected filesystem effect. The caller owns interpretation.
type DirectoryScanner interface {
	ScanDirectory(string, int, func([]os.DirEntry) error) error
}

type replayAppendFile interface {
	io.Writer
	io.Seeker
	io.Closer
	Sync() error
	Truncate(int64) error
}

// Local is the policy-free local artifact adapter for one Wire-selected host
// operating system.
type Local struct {
	operatingSystem string
	rename          func(string, string) error
}

// NewLocal binds replacement mechanics to the host operating system selected
// by Wire.
func NewLocal(operatingSystem string) Local {
	return Local{operatingSystem: operatingSystem}
}

func (local Local) ScanDirectory(path string, batchSize int, visit func([]os.DirEntry) error) error {
	if batchSize <= 0 || visit == nil {
		return errors.New("replay directory scan requires a positive batch size and visitor")
	}
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = directory.Close() }()
	for {
		entries, err := directory.ReadDir(batchSize)
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		if visitErr := visit(entries); visitErr != nil {
			return visitErr
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
	}
}

// WriteFile atomically replaces path with a completed artifact snapshot.
func (local Local) WriteFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create replay artifact directory: %w", err)
	}

	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create replay artifact temp file: %w", err)
	}
	tmpPath := tmp.Name()
	cleanupTemp := true
	defer func() {
		if cleanupTemp {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write replay artifact temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync replay artifact temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close replay artifact temp file: %w", err)
	}

	rename := local.rename
	if rename == nil {
		rename = os.Rename
	}
	if err := rename(tmpPath, path); err == nil {
		cleanupTemp = false
		return nil
	} else if local.operatingSystem != "windows" {
		return fmt.Errorf("replace replay artifact with temp file: %w", err)
	}

	var replaceErr error
	for range artifactReplaceAttempts {
		// Rename already requests replacement on Windows. A sharing violation
		// may be transient, but deleting the destination first destroys the
		// previous snapshot if this or any later replacement attempt fails.
		if err := rename(tmpPath, path); err != nil {
			replaceErr = fmt.Errorf("replace replay artifact from temp file: %w", err)
		} else {
			cleanupTemp = false
			return nil
		}
		time.Sleep(artifactReplaceDelay)
	}
	return replaceErr
}

// AppendFile appends one complete replay-framing suffix and synchronizes it
// before returning. It never replaces or renames the existing artifact.
func (local Local) AppendFile(path string, data []byte) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0o600)
	// Existing journals need only open, append and sync. Avoid inspecting the
	// parent directory on every record; create it only for a missing path.
	if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("create replay artifact directory: %w", err)
		}
		file, err = os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0o600)
	}
	if err != nil {
		return fmt.Errorf("open replay artifact for append: %w", err)
	}
	return appendReplaySuffix(file, data)
}

func appendReplaySuffix(file replayAppendFile, data []byte) (err error) {
	defer func() {
		if closeErr := file.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("close replay artifact append: %w", closeErr)
		}
	}()

	originalSize, err := file.Seek(0, io.SeekEnd)
	if err != nil {
		return fmt.Errorf("locate replay artifact append position: %w", err)
	}
	written, err := file.Write(data)
	if err != nil {
		return rollbackReplayAppend(file, originalSize, fmt.Errorf("append replay artifact: %w", err))
	}
	if written != len(data) {
		return rollbackReplayAppend(file, originalSize, fmt.Errorf("append replay artifact: %w", io.ErrShortWrite))
	}
	if err := file.Sync(); err != nil {
		return rollbackReplayAppend(file, originalSize, fmt.Errorf("sync replay artifact append: %w", err))
	}
	return nil
}

func rollbackReplayAppend(file replayAppendFile, originalSize int64, cause error) error {
	if err := file.Truncate(originalSize); err != nil {
		return fmt.Errorf("%w; rollback replay artifact append: %w", cause, err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("%w; sync replay artifact rollback: %w", cause, err)
	}
	return cause
}

// ReadFile reads one artifact snapshot, retrying transient Windows replacement
// races without interpreting the artifact contents.
func (local Local) ReadFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err == nil || local.operatingSystem != "windows" {
		return data, err
	}

	lastErr := err
	for range artifactReplaceAttempts {
		time.Sleep(artifactReplaceDelay)
		data, err = os.ReadFile(path)
		if err == nil {
			return data, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

var _ Storage = Local{}
var _ Appender = Local{}
