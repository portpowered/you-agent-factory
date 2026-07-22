package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunAllowsProviderSessionRootAndCanonicalClockImports(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	writeGoImportFile(t, repoRoot, "pkg/services/factory_runtime/runtime/clock.go", "runtime", "github.com/portpowered/infinite-you/pkg/platform/clock")
	writeGoImportFile(t, repoRoot, "pkg/transports/http/provider_sessions.go", "http", "github.com/portpowered/infinite-you/pkg/services/provider_sessions")

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
		{filePath: "pkg/services/factory_runtime/runtime/session.go", path: "github.com/portpowered/infinite-you/pkg/sessionpersistence"},
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
		"canonical owner: pkg/services/factory_sessions/cursors/persistence",
		"canonical owner: pkg/services/provider_sessions/cursor",
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("run() stderr = %q, want %q", stderr.String(), want)
		}
	}
}

func TestRunRejectsRecreatedCursorRoots(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		retiredRoot string
		owner       string
	}{
		{retiredRoot: "pkg/sessionpersistence", owner: "pkg/services/factory_sessions/cursors/persistence"},
		{retiredRoot: "pkg/internal/cursorstorage", owner: "pkg/services/provider_sessions/cursor"},
	} {
		tc := tc
		t.Run(tc.retiredRoot, func(t *testing.T) {
			t.Parallel()
			repoRoot := t.TempDir()
			makeDir(t, repoRoot, tc.retiredRoot)

			stderr := &bytes.Buffer{}
			if err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr); err == nil {
				t.Fatal("run() error = nil, want retired cursor root rejected")
			}
			for _, want := range []string{
				"prohibited retired package root: " + tc.retiredRoot,
				"canonical owner: " + tc.owner,
			} {
				if !strings.Contains(stderr.String(), want) {
					t.Fatalf("run() stderr = %q, want %q", stderr.String(), want)
				}
			}
		})
	}
}
