package directoryreplace

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestCommitAndRestore(t *testing.T) {
	parentDir := t.TempDir()
	targetDir := filepath.Join(parentDir, "alpha")
	stagingDir := filepath.Join(parentDir, ".alpha.staging")
	writeFixture(t, targetDir, "value.txt", "old")
	writeFixture(t, stagingDir, "value.txt", "new")

	store := NewLocal(runtime.GOOS)
	backupDir, err := store.Commit(parentDir, targetDir, stagingDir)
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if backupDir == "" {
		t.Skip("Windows watched-directory fallback does not retain a backup")
	}
	assertFixture(t, targetDir, "value.txt", "new")
	assertFixture(t, backupDir, "value.txt", "old")

	store.Restore(targetDir, backupDir)
	assertFixture(t, targetDir, "value.txt", "old")
	if _, err := os.Stat(backupDir); !os.IsNotExist(err) {
		t.Fatalf("backup after Restore: %v, want not found", err)
	}
}

func TestCommitRollsBackWhenStagingIsMissing(t *testing.T) {
	parentDir := t.TempDir()
	targetDir := filepath.Join(parentDir, "alpha")
	stagingDir := filepath.Join(parentDir, ".alpha.staging")
	writeFixture(t, targetDir, "value.txt", "old")

	store := NewLocal(runtime.GOOS)
	backupDir, err := store.Commit(parentDir, targetDir, stagingDir)
	if err == nil {
		t.Fatal("Commit succeeded with a missing staging directory")
	}
	if backupDir != "" {
		t.Fatalf("backup on failed Commit = %q, want empty", backupDir)
	}
	if !strings.Contains(err.Error(), "commit directory") {
		t.Fatalf("Commit error = %q, want contextual commit failure", err)
	}
	assertFixture(t, targetDir, "value.txt", "old")

	previousDirs, err := filepath.Glob(filepath.Join(parentDir, ".alpha.previous-*"))
	if err != nil {
		t.Fatalf("find replacement backups: %v", err)
	}
	if len(previousDirs) != 0 {
		t.Fatalf("replacement backups after rollback = %v, want none", previousDirs)
	}
}

func TestRestoreIsNoOpForEmptyOrMissingBackup(t *testing.T) {
	tests := []struct {
		name       string
		backupPath func(parentDir string) string
	}{
		{
			name: "empty backup",
			backupPath: func(string) string {
				return ""
			},
		},
		{
			name: "missing backup",
			backupPath: func(parentDir string) string {
				return filepath.Join(parentDir, "missing-backup")
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			parentDir := t.TempDir()
			targetDir := filepath.Join(parentDir, "alpha")
			writeFixture(t, targetDir, "value.txt", "old")
			beforeEntries := directoryEntries(t, parentDir)

			NewLocal(runtime.GOOS).Restore(targetDir, test.backupPath(parentDir))

			assertFixture(t, targetDir, "value.txt", "old")
			afterEntries := directoryEntries(t, parentDir)
			if !reflect.DeepEqual(afterEntries, beforeEntries) {
				t.Fatalf("parent entries after no-op Restore = %v, want %v", afterEntries, beforeEntries)
			}
		})
	}
}

func TestReplaceWatchedDirectoryContentsReplacesContentAndCleansUp(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows in-place replacement is only applicable on Windows")
	}

	parentDir := t.TempDir()
	targetDir := filepath.Join(parentDir, "alpha")
	stagingDir := filepath.Join(parentDir, ".alpha.staging")
	backupDir := filepath.Join(parentDir, ".alpha.previous")
	writeFixture(t, targetDir, "obsolete.txt", "old")
	writeFixture(t, stagingDir, "value.txt", "new")
	if err := os.Mkdir(backupDir, 0o755); err != nil {
		t.Fatalf("mkdir backup: %v", err)
	}

	if err := replaceWatchedDirectoryContents(targetDir, stagingDir, backupDir); err != nil {
		t.Fatalf("replace watched directory contents: %v", err)
	}

	assertFixture(t, targetDir, "value.txt", "new")
	assertMissing(t, filepath.Join(targetDir, "obsolete.txt"))
	assertMissing(t, stagingDir)
	assertMissing(t, backupDir)
}

func directoryEntries(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read directory entries: %v", err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

func assertMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("path %s after operation: %v, want not found", path, err)
	}
}

func writeFixture(t *testing.T, dir, name, value string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(value), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}

func assertFixture(t *testing.T, dir, name, want string) {
	t.Helper()
	got, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if string(got) != want {
		t.Fatalf("fixture = %q, want %q", got, want)
	}
}
