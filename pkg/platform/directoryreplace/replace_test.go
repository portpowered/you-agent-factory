package directoryreplace

import (
	"os"
	"path/filepath"
	"runtime"
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
