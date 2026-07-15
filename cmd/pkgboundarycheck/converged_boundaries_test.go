package main

import (
	"bytes"
	"testing"
)

func TestRunAllowsCanonicalDomainAndInternalTestSupportImports(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoImportFile(t, repoRoot, "pkg/factory/runtime/contracts.go", "runtime", "github.com/portpowered/infinite-you/pkg/factory/contracts")
	writeGoImportFile(t, repoRoot, "pkg/factory/runtime/runtime_test.go", "runtime", "github.com/portpowered/infinite-you/internal/testutil")

	stderr := &bytes.Buffer{}
	if err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr); err != nil {
		t.Fatalf("run() error = %v, want canonical domain and internal test-support imports allowed; stderr=%q", err, stderr.String())
	}
}
