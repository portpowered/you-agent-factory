package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunAllowsCanonicalCursorAndClockImports(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	writeGoImportFile(t, repoRoot, "pkg/factory/runtime/clock.go", "runtime", "github.com/portpowered/infinite-you/pkg/platform/clock")
	writeGoImportFile(t, repoRoot, "pkg/transports/http/cursors.go", "http", "github.com/portpowered/infinite-you/pkg/platform/cursors")

	stderr := &bytes.Buffer{}
	if err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr); err != nil {
		t.Fatalf("run() error = %v, want canonical platform imports allowed; stderr=%q", err, stderr.String())
	}
}

func TestRunRejectsRetiredCursorImports(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	for _, retired := range []struct {
		filePath string
		path     string
	}{
		{filePath: "pkg/factory/runtime/session.go", path: "github.com/portpowered/infinite-you/pkg/sessionpersistence"},
		{filePath: "pkg/transports/http/storage.go", path: "github.com/portpowered/infinite-you/pkg/internal/cursorstorage"},
	} {
		writeGoImportFile(t, repoRoot, retired.filePath, filepath.Base(filepath.Dir(retired.filePath)), retired.path)
	}

	stderr := &bytes.Buffer{}
	if err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr); err == nil {
		t.Fatal("run() error = nil, want retired cursor imports rejected")
	}
	for _, want := range []string{
		"prohibited retired package import: github.com/portpowered/infinite-you/pkg/sessionpersistence",
		"prohibited retired package import: github.com/portpowered/infinite-you/pkg/internal/cursorstorage",
		"canonical owner: pkg/platform/cursors",
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("run() stderr = %q, want %q", stderr.String(), want)
		}
	}
}

func TestRunRejectsRecreatedCursorRoots(t *testing.T) {
	t.Parallel()
	for _, retiredRoot := range []string{"pkg/sessionpersistence", "pkg/internal/cursorstorage"} {
		retiredRoot := retiredRoot
		t.Run(retiredRoot, func(t *testing.T) {
			t.Parallel()
			repoRoot := t.TempDir()
			makeDir(t, repoRoot, retiredRoot)

			stderr := &bytes.Buffer{}
			if err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr); err == nil {
				t.Fatal("run() error = nil, want retired cursor root rejected")
			}
			for _, want := range []string{
				"prohibited retired package root: " + retiredRoot,
				"canonical owner: pkg/platform/cursors",
			} {
				if !strings.Contains(stderr.String(), want) {
					t.Fatalf("run() stderr = %q, want %q", stderr.String(), want)
				}
			}
		})
	}
}
