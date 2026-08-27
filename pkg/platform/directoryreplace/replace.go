// Package directoryreplace provides policy-free atomic directory replacement
// with rollback support and a Windows fallback for watched directories.
package directoryreplace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Local implements policy-free directory replacement against the local
// filesystem. Product policy supplies the target and interprets failures.
type Local struct {
	operatingSystem string
}

// NewLocal binds directory replacement to the host operating system selected
// by Wire.
func NewLocal(operatingSystem string) Local {
	return Local{operatingSystem: operatingSystem}
}

// Commit replaces targetDir with stagingDir and returns the previous target
// directory as a rollback backup. An empty backup means the Windows in-place
// fallback completed and no rollback directory remains.
func (local Local) Commit(
	parentDir string,
	targetDir string,
	stagingDir string,
) (backupDir string, err error) {
	segment := filepath.Base(targetDir)
	backupDir, err = os.MkdirTemp(parentDir, "."+segment+".previous-")
	if err != nil {
		return "", fmt.Errorf("prepare replacement backup for directory %q: %w", segment, err)
	}
	if err := os.Remove(backupDir); err != nil {
		return "", fmt.Errorf("prepare replacement backup for directory %q: %w", segment, err)
	}
	rollbackDir := backupDir

	if err := os.Rename(targetDir, backupDir); err != nil {
		if local.operatingSystem == "windows" {
			if replaceErr := replaceWatchedDirectoryContents(targetDir, stagingDir, backupDir); replaceErr == nil {
				return "", nil
			} else {
				return "", fmt.Errorf(
					"backup existing directory %q: %w; Windows in-place replacement failed: %v",
					segment,
					err,
					replaceErr,
				)
			}
		}
		return "", fmt.Errorf("backup existing directory %q: %w", segment, err)
	}

	committed := false
	defer func() {
		if committed {
			return
		}
		if restoreErr := os.Rename(rollbackDir, targetDir); restoreErr != nil {
			return
		}
		_ = os.RemoveAll(rollbackDir)
	}()

	if err := os.Rename(stagingDir, targetDir); err != nil {
		return "", fmt.Errorf("commit directory %q: %w", segment, err)
	}
	committed = true
	return backupDir, nil
}

// Restore swaps a committed target back to its backup. It is intentionally
// best-effort so it can be retained as a rollback callback.
func (Local) Restore(targetDir, backupDir string) {
	if strings.TrimSpace(targetDir) == "" || strings.TrimSpace(backupDir) == "" {
		return
	}
	if _, err := os.Stat(backupDir); err != nil {
		return
	}

	parentDir := filepath.Dir(targetDir)
	segment := filepath.Base(targetDir)
	trashDir, err := os.MkdirTemp(parentDir, "."+segment+".rollback-trash-")
	if err != nil {
		return
	}
	if err := os.Remove(trashDir); err != nil {
		return
	}

	if err := os.Rename(targetDir, trashDir); err != nil {
		_ = os.RemoveAll(trashDir)
		return
	}
	if err := os.Rename(backupDir, targetDir); err != nil {
		_ = os.Rename(trashDir, targetDir)
		return
	}
	_ = os.RemoveAll(trashDir)
}

func replaceWatchedDirectoryContents(targetDir, stagingDir, backupDir string) error {
	if err := os.CopyFS(backupDir, os.DirFS(targetDir)); err != nil {
		return fmt.Errorf("snapshot existing directory: %w", err)
	}
	if err := clearDirectoryContents(targetDir); err != nil {
		return err
	}
	if err := os.CopyFS(targetDir, os.DirFS(stagingDir)); err != nil {
		_ = clearDirectoryContents(targetDir)
		_ = os.CopyFS(targetDir, os.DirFS(backupDir))
		return fmt.Errorf("copy staged directory: %w", err)
	}
	if err := os.RemoveAll(stagingDir); err != nil {
		return fmt.Errorf("remove staged directory after commit: %w", err)
	}
	if err := os.RemoveAll(backupDir); err != nil {
		return fmt.Errorf("remove directory backup after commit: %w", err)
	}
	return nil
}

func clearDirectoryContents(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read directory for replacement: %w", err)
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(dir, entry.Name())); err != nil {
			return fmt.Errorf("remove existing directory entry %q: %w", entry.Name(), err)
		}
	}
	return nil
}
