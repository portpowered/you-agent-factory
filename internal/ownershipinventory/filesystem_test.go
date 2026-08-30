package ownershipinventory

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSnapshotFileSystemDefaultOS(t *testing.T) {
	files := osFileSystem{}
	root := t.TempDir()
	directory := filepath.Join(root, "nested", "destination")
	if err := files.mkdirAll(directory, 0o755); err != nil {
		t.Fatalf("mkdirAll() error = %v", err)
	}

	stage, err := files.createTemp(directory, ".ownership-snapshot-*")
	if err != nil {
		t.Fatalf("createTemp() error = %v", err)
	}
	stagePath := stage.Name()
	payload := []byte("snapshot\n")
	if written, err := stage.Write(payload); err != nil {
		t.Fatalf("stage.Write() error = %v", err)
	} else if written != len(payload) {
		t.Fatalf("stage.Write() wrote %d bytes, want %d", written, len(payload))
	}
	if err := stage.Chmod(0o600); err != nil {
		t.Fatalf("stage.Chmod() error = %v", err)
	}
	if err := stage.Close(); err != nil {
		t.Fatalf("stage.Close() error = %v", err)
	}

	info, err := files.lstat(stagePath)
	if err != nil {
		t.Fatalf("lstat() error = %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("stage mode = %o, want 600", info.Mode().Perm())
	}
	got, err := files.readFile(stagePath)
	if err != nil {
		t.Fatalf("readFile() error = %v", err)
	}
	if string(got) != string(payload) {
		t.Fatalf("readFile() = %q, want %q", got, payload)
	}

	targetPath := filepath.Join(directory, "target.json")
	if err := files.rename(stagePath, targetPath); err != nil {
		t.Fatalf("rename() error = %v", err)
	}
	if err := files.remove(targetPath); err != nil {
		t.Fatalf("remove() error = %v", err)
	}
	if _, err := files.lstat(targetPath); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("lstat() after remove error = %v, want not-exist", err)
	}
}

var _ snapshotFile = (*os.File)(nil)
