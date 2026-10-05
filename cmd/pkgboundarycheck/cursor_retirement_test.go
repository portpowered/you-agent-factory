package main

import (
	"bytes"
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
