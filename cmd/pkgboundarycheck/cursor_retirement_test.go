package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunAllowsProviderSessionRootAndCanonicalClockImports(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	writeGoImportFile(t, repoRoot, "pkg/services/factory_runtime/internal/services/orchestration/runtime/clock.go", "runtime", "github.com/portpowered/infinite-you/pkg/platform/clock")
	writeGoImportFile(t, repoRoot, "pkg/transports/http/provider_sessions.go", "http", "github.com/portpowered/infinite-you/pkg/services/provider_sessions")

	stderr := &bytes.Buffer{}
	if err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr); err != nil {
		t.Fatalf("run() error = %v, want canonical platform imports allowed; stderr=%q", err, stderr.String())
	}
}

func TestRunRejectsRecreatedCursorRoots(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		retiredRoot string
		owner       string
	}{
		{retiredRoot: "pkg/sessionpersistence", owner: "pkg/services/factory_sessions/internal/cursors/persistence"},
		{retiredRoot: "pkg/internal/cursorstorage", owner: "pkg/services/provider_sessions/internal/services/cursor_reader/internal/cursor"},
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
