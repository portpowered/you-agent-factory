//go:build windows || linux

package filesystem

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestReadRevisionDetectsRestoredWriteTimeAndReplacement(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "source")
	write := func(value string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	revision := func() string {
		t.Helper()
		value, err := (Local{}).ReadRevision(path)
		if err != nil {
			t.Fatal(err)
		}
		if value == "" {
			t.Skip("filesystem does not support reliable revisions")
		}
		return value
	}
	write("first")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	first := revision()
	if revision() != first {
		t.Fatal("read-only checks changed the revision")
	}
	write("other") // Same length; restoring mtime must still expose the write.
	if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	second := revision()
	if second == first {
		t.Fatal("in-place write with restored mtime retained its revision")
	}
	// Retain the original inode so it cannot be reused for the replacement.
	if err := os.Rename(path, path+".old"); err != nil {
		t.Fatal(err)
	}
	write("other")
	if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if revision() == second {
		t.Fatal("replacement with equal size and mtime retained its revision")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if value, err := (Local{}).ReadRevision(path); value != "" || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing source = %q, %v", value, err)
	}
}
