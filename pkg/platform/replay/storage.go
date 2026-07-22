// Package replay owns policy-free replay artifact persistence mechanics.
package replay

import (
	"errors"
	"fmt"
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

// Local is the policy-free local artifact adapter for one Wire-selected host
// operating system.
type Local struct {
	operatingSystem string
}

// NewLocal binds replacement mechanics to the host operating system selected
// by Wire.
func NewLocal(operatingSystem string) Local {
	return Local{operatingSystem: operatingSystem}
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

	if err := os.Rename(tmpPath, path); err == nil {
		cleanupTemp = false
		return nil
	} else if local.operatingSystem != "windows" {
		return fmt.Errorf("replace replay artifact with temp file: %w; temp artifact left at %s", err, tmpPath)
	}

	var replaceErr error
	for range artifactReplaceAttempts {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			replaceErr = fmt.Errorf("remove previous replay artifact before replace: %w", err)
		} else if err := os.Rename(tmpPath, path); err != nil {
			replaceErr = fmt.Errorf("replace replay artifact from temp file: %w", err)
		} else {
			cleanupTemp = false
			return nil
		}
		time.Sleep(artifactReplaceDelay)
	}
	return fmt.Errorf("%w; temp artifact left at %s", replaceErr, tmpPath)
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
