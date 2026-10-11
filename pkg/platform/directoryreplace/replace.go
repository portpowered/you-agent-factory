// Package directoryreplace provides policy-free atomic directory replacement
// with rollback support and a Windows fallback for watched directories.
package directoryreplace

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Local implements policy-free directory replacement against the local
// filesystem. Product policy supplies the target and interprets failures.
type Local struct {
	operatingSystem string
	stat            func(string) (fs.FileInfo, error)
	mkdirAll        func(string, fs.FileMode) error
	copyDirectory   func(source, destination string) error
}

// NewLocal binds directory replacement to the host operating system selected
// by Wire.
func NewLocal(
	operatingSystem string,
	stat func(string) (fs.FileInfo, error),
	mkdirAll func(string, fs.FileMode) error,
	copyDirectory func(source, destination string) error,
) Local {
	return Local{operatingSystem: operatingSystem, stat: stat, mkdirAll: mkdirAll, copyDirectory: copyDirectory}
}

// Commit replaces targetDir with stagingDir and returns the previous target
// directory as a rollback backup. An empty backup means the Windows in-place
// fallback completed and no rollback directory remains.
func (local Local) Commit(
	parentDir string,
	targetDir string,
	stagingDir string,
	retained ...string,
) (backupDir string, err error) {
	if err := local.retainDirectories(targetDir, stagingDir, retained); err != nil {
		return "", err
	}
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

// retainDirectories copies opaque caller-selected state before atomic publication.
// Callers must quiesce writers before invoking Commit.
func (local Local) retainDirectories(target, staging string, retained []string) error {
	for _, name := range retained {
		if name == "." || !filepath.IsLocal(name) {
			return fmt.Errorf("invalid retained directory %q", name)
		}
		source := filepath.Join(target, name)
		info, err := local.stat(source)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("inspect retained directory %q: %w", name, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("retained path %q is not a directory", name)
		}
		destination := filepath.Join(staging, name)
		if err := local.mkdirAll(destination, info.Mode().Perm()); err != nil {
			return fmt.Errorf("stage retained directory %q: %w", name, err)
		}
		if err := local.copyDirectory(source, destination); err != nil {
			return fmt.Errorf("copy retained directory %q: %w", name, err)
		}
	}
	return nil
}
