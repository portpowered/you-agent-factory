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
	reader := RevisionReader{OpenFile: func(path string) (RevisionFile, error) {
		return os.Open(path)
	}}
	write := func(value string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	revision := func() string {
		t.Helper()
		value, err := reader.ReadRevision(path)
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
	if value, err := reader.ReadRevision(path); value != "" || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing source = %q, %v", value, err)
	}
}

func TestReadRevisionPropagatesInjectedOpenAndCloseFailures(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "source")
	openErr := errors.New("read access unavailable")
	reader := RevisionReader{OpenFile: func(selected string) (RevisionFile, error) {
		if selected != path {
			t.Fatalf("opened %q, want %q", selected, path)
		}
		return nil, openErr
	}}
	if value, err := reader.ReadRevision(path); value != "" || !errors.Is(err, openErr) {
		t.Fatalf("open failure = %q, %v", value, err)
	}
	if err := os.WriteFile(path, []byte("source"), 0o600); err != nil {
		t.Fatal(err)
	}
	closeErr := errors.New("close unavailable")
	var opened *revisionCloseFailure
	reader.OpenFile = func(selected string) (RevisionFile, error) {
		file, err := os.Open(selected)
		if err != nil {
			return nil, err
		}
		opened = &revisionCloseFailure{File: file, err: closeErr}
		return opened, nil
	}
	if value, err := reader.ReadRevision(path); value != "" || !errors.Is(err, closeErr) {
		t.Fatalf("close failure = %q, %v", value, err)
	}
	if opened == nil || !opened.closed {
		t.Fatal("readable descriptor was not closed")
	}
}

type revisionCloseFailure struct {
	*os.File
	err    error
	closed bool
}

func (file *revisionCloseFailure) Close() error {
	file.closed = true
	if err := file.File.Close(); err != nil {
		return err
	}
	return file.err
}
