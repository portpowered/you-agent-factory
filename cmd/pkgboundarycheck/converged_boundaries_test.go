package main

import (
	"bytes"
	"fmt"
	"testing"
)

func TestRunAllowsCanonicalDomainAndInternalTestSupportImports(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoImportFile(t, repoRoot, "pkg/services/factory_runtime/internal/services/orchestration/runtime/contracts.go", "runtime", "github.com/portpowered/infinite-you/pkg/services/factory_definitions")
	writeGoImportFile(t, repoRoot, "pkg/services/factory_runtime/internal/services/orchestration/runtime/runtime_test.go", "runtime", "github.com/portpowered/infinite-you/internal/testutil")

	stderr := &bytes.Buffer{}
	if err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr); err != nil {
		t.Fatalf("run() error = %v, want canonical domain and internal test-support imports allowed; stderr=%q", err, stderr.String())
	}
}

func TestRunAllowsEdgeAggregatorToImportPublishedEffectContracts(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	for index, importPath := range []string{
		"github.com/portpowered/infinite-you/pkg/services/models",
		"github.com/portpowered/infinite-you/pkg/services/providers/wire",
		"github.com/portpowered/infinite-you/pkg/services/automations",
	} {
		writeGoImportFile(
			t,
			repoRoot,
			fmt.Sprintf("pkg/services/edges/model_effect_%d.go", index),
			"edges",
			importPath,
		)
	}

	stderr := &bytes.Buffer{}
	if err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr); err != nil {
		t.Fatalf("run() error = %v, want exact model effect-owner contracts allowed; stderr=%q", err, stderr.String())
	}
}

func TestRunAllowsPeerServicesToImportExactProviderInferenceContract(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	for index, owner := range []struct {
		path    string
		pkgName string
	}{
		{path: "factory_runtime", pkgName: "factory"},
		{path: "factory_runtime/internal/services/instance_host/build", pkgName: "runtimebuild"},
		{path: "recordings", pkgName: "recordings"},
		{path: "recordings/internal/artifacts", pkgName: "artifacts"},
		{path: "recordings/internal/replay", pkgName: "replay"},
	} {
		writeGoImportFile(
			t,
			repoRoot,
			fmt.Sprintf("pkg/services/%s/provider_contract_%d.go", owner.path, index),
			owner.pkgName,
			"github.com/portpowered/infinite-you/pkg/services/providers/wire",
		)
	}

	stderr := &bytes.Buffer{}
	if err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr); err != nil {
		t.Fatalf("run() error = %v, want exact provider inference contract allowed; stderr=%q", err, stderr.String())
	}
}

func TestRunAllowsTestsToImportServiceOwnedTransportAdapters(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoImportFile(
		t,
		repoRoot,
		"tests/functional/cli/session_test.go",
		"cli_test",
		"github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/cli/session",
	)

	stderr := &bytes.Buffer{}
	if err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr); err != nil {
		t.Fatalf("run() error = %v, want service-owned transport adapter import allowed; stderr=%q", err, stderr.String())
	}
}

func TestRunAllowsOwningServiceAndWireTestsToImportServiceInternals(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	const importPath = "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/runtimepersist"
	writeGoImportFile(
		t,
		repoRoot,
		"pkg/services/factory_sessions/internal/execution/harness_test.go",
		"execution",
		importPath,
	)
	writeGoImportFile(t, repoRoot, "pkg/wire/session_test.go", "wire", importPath)

	stderr := &bytes.Buffer{}
	if err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr); err != nil {
		t.Fatalf("run() error = %v, want owner and Wire tests allowed; stderr=%q", err, stderr.String())
	}
}

func TestRunAllowsExactExternalEffectContractInTests(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoImportFile(
		t,
		repoRoot,
		"pkg/transports/cli/session/process_edges_test.go",
		"session_test",
		"github.com/portpowered/infinite-you/pkg/services/models",
	)

	stderr := &bytes.Buffer{}
	if err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr); err != nil {
		t.Fatalf("run() error = %v, want exact external-effect contract allowed; stderr=%q", err, stderr.String())
	}
}
