package filesystem

import (
	"bytes"
	"errors"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestLocalWorkingDirectoryLeavesHostDirectoryUnchanged(t *testing.T) {
	t.Parallel()
	before, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	owned := t.TempDir()
	got, err := (Local{WorkingDirectory: owned}).Getwd()
	if err != nil || got != owned {
		t.Fatalf("owned directory = %q, %v; want %q", got, err, owned)
	}
	after, err := (Local{}).Getwd()
	if err != nil || after != before {
		t.Fatalf("host directory = %q, %v; want unchanged %q", after, err, before)
	}
}

func TestLocalReadFileBounded(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "snapshot")
	data := []byte("§ — secret-prompt")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, limit := range []int64{0, int64(len(data) - 1), int64(len(data)), int64(len(data) + 1)} {
		got, err := (Local{}).ReadFileBounded(path, limit)
		if limit < int64(len(data)) {
			var sizeErr *ReadSizeLimitError
			if got != nil || !errors.As(err, &sizeErr) || sizeErr.ReadSizeLimit() != limit {
				t.Fatalf("bounded read at %d = %q, %v", limit, got, err)
			}
		} else if err != nil || !bytes.Equal(got, data) {
			t.Fatalf("accepted read at %d = %q, %v", limit, got, err)
		}
	}
	for _, limit := range []int64{-1, math.MaxInt64} {
		if _, err := (Local{}).ReadFileBounded(path, limit); err == nil {
			t.Fatalf("unsafe limit %d accepted", limit)
		}
	}
	if _, err := (Local{}).ReadFileBounded(path+".missing", 1); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing read = %v", err)
	}
}

func TestLocalRenameNoReplacePreservesBytesAndCollisions(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	source, archive := filepath.Join(dir, "source"), filepath.Join(dir, "archive")
	data, prior := []byte("§ — damaged secret-prompt"), []byte("prior archive")
	for path, content := range map[string][]byte{source: data, archive: prior} {
		if err := os.WriteFile(path, content, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := (Local{}).RenameNoReplace(source, archive); !errors.Is(err, os.ErrExist) {
		t.Fatalf("collision = %v", err)
	}
	for path, want := range map[string][]byte{source: data, archive: prior} {
		got, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("collision changed %s: %q, %v", path, got, err)
		}
	}
	destination := source + ".unreadable"
	if err := (Local{}).RenameNoReplace(source, destination); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(destination)
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("preserved bytes = %q, %v", got, err)
	}
	if _, err := os.Stat(source); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("source retained after successful move: %v", err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(destination)
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("archive permissions broadened: %v, %v", info, err)
		}
	}
	if err := (Local{}).RenameNoReplace(dir, filepath.Join(dir, "directory")); err == nil {
		t.Fatal("directory accepted as a snapshot")
	}
}
