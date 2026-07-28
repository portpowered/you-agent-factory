package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunAllowsCanonicalDomainAndInternalTestSupportImports(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoImportFile(t, repoRoot, "pkg/services/factory_runtime/runtime/contracts.go", "runtime", "github.com/portpowered/infinite-you/pkg/services/factory_definitions")
	writeGoImportFile(t, repoRoot, "pkg/services/factory_runtime/runtime/runtime_test.go", "runtime", "github.com/portpowered/infinite-you/internal/testutil")

	stderr := &bytes.Buffer{}
	if err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr); err != nil {
		t.Fatalf("run() error = %v, want canonical domain and internal test-support imports allowed; stderr=%q", err, stderr.String())
	}
}

func TestRunRejectsUnapprovedPeerServiceSubpackageImport(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoImportFile(
		t,
		repoRoot,
		"pkg/services/factory_sessions/internal/sessionservice/runtime.go",
		"service",
		"github.com/portpowered/infinite-you/pkg/services/factory_runtime/contracts",
	)

	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want peer implementation import rejected")
	}
	for _, want := range []string{
		"prohibited peer service subpackage import",
		"pkg/services/factory_runtime/contracts",
		"service owner: pkg/services/factory_sessions; peer owner: pkg/services/factory_runtime",
		"import only that peer root",
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("run() stderr = %q, want %q", stderr.String(), want)
		}
	}
}

func TestRunAllowsEdgeAggregatorToImportPublishedEffectContracts(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	for index, importPath := range []string{
		"github.com/portpowered/infinite-you/pkg/services/models",
		"github.com/portpowered/infinite-you/pkg/services/workers/agypty",
		providersLeafEffectContractImport,
		"github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract",
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
		{path: "factory_runtime/build", pkgName: "runtimebuild"},
		{path: "factory_runtime/build", pkgName: "runtimebuild"},
		{path: "recordings", pkgName: "recordings"},
		{path: "recordings/replay", pkgName: "replay"},
		{path: "recordings/service", pkgName: "service"},
	} {
		writeGoImportFile(
			t,
			repoRoot,
			fmt.Sprintf("pkg/services/%s/provider_contract_%d.go", owner.path, index),
			owner.pkgName,
			"github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract",
		)
	}

	stderr := &bytes.Buffer{}
	if err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr); err != nil {
		t.Fatalf("run() error = %v, want exact provider inference contract allowed; stderr=%q", err, stderr.String())
	}
}

func TestRunRejectsInitializerImportingWorkersPTYImplementation(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoImportFile(
		t,
		repoRoot,
		"pkg/initializer/application/session_execution.go",
		"application",
		"github.com/portpowered/infinite-you/pkg/services/workers/agypty",
	)

	stderr := &bytes.Buffer{}
	if err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr); err == nil {
		t.Fatal("run() error = nil, want Initializer-to-Workers implementation import rejected")
	}
	if !strings.Contains(stderr.String(), "prohibited external service subpackage import") {
		t.Fatalf("run() stderr = %q, want service-implementation diagnostic", stderr.String())
	}
}

func TestRunRejectsEdgeAggregatorImportingComposedModelService(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoImportFile(
		t,
		repoRoot,
		"pkg/services/edges/models_service.go",
		"edges",
		"github.com/portpowered/infinite-you/pkg/services/models/service",
	)

	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want composed model service import rejected")
	}
	if !strings.Contains(stderr.String(), "prohibited peer service subpackage import") {
		t.Fatalf("run() stderr = %q, want peer-service diagnostic", stderr.String())
	}
}

func TestRunAllowsRecordedPeerServiceMigrationEdge(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoRoot, "pkg", "services"), 0o755); err != nil {
		t.Fatalf("create services root: %v", err)
	}
	const (
		filePath   = "pkg/services/factory_sessions/internal/sessionservice/runtime.go"
		importPath = "github.com/portpowered/infinite-you/pkg/services/factory_runtime/contracts"
	)
	writeGoImportFile(t, repoRoot, filePath, "service", importPath)
	writePeerServiceBaseline(t, repoRoot, filePath, importPath, "factory_sessions", "factory_runtime")

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	if err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, stdout, stderr); err != nil {
		t.Fatalf("run() error = %v, want migration baseline edge allowed; stderr=%q", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "active peer-service root-contract migration baseline: 1 edge(s)") {
		t.Fatalf("run() stdout = %q, want active migration baseline summary", stdout.String())
	}
}

func TestRunRejectsStalePeerServiceMigrationEdge(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoRoot, "pkg", "services"), 0o755); err != nil {
		t.Fatalf("create services root: %v", err)
	}
	const (
		filePath   = "pkg/services/factory_sessions/internal/sessionservice/runtime.go"
		importPath = "github.com/portpowered/infinite-you/pkg/services/factory_runtime/contracts"
	)
	writePeerServiceBaseline(t, repoRoot, filePath, importPath, "factory_sessions", "factory_runtime")

	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want stale migration baseline rejected")
	}
	if !strings.Contains(stderr.String(), "stale peer service import baseline entry") {
		t.Fatalf("run() stderr = %q, want stale baseline diagnostic", stderr.String())
	}
}

func TestRunRejectsUnrecordedCrossOwnerTestServiceInternal(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoImportFile(
		t,
		repoRoot,
		"pkg/transports/cli/session/resume_test.go",
		"session_test",
		"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/runtimepersist",
	)

	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want cross-owner test service import rejected")
	}
	for _, want := range []string{
		"prohibited test import of service internals",
		"pkg/services/factory_sessions/internal/execution/runtimepersist",
		"use the service root contract",
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("run() stderr = %q, want %q", stderr.String(), want)
		}
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

func TestRunAllowsRecordedTestServiceInternalAndRejectsStaleEntry(t *testing.T) {
	t.Parallel()

	const (
		filePath   = "pkg/transports/cli/session/resume_test.go"
		importPath = "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/runtimepersist"
	)
	t.Run("active", func(t *testing.T) {
		repoRoot := t.TempDir()
		writeGoImportFile(t, repoRoot, filePath, "session_test", importPath)
		writeTestServiceImportBaseline(t, repoRoot, filePath, importPath, "factory_sessions")

		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}
		if err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, stdout, stderr); err != nil {
			t.Fatalf("run() error = %v, want recorded migration edge allowed; stderr=%q", err, stderr.String())
		}
		if !strings.Contains(stdout.String(), "active test service-internal migration baseline: 1 edge(s)") {
			t.Fatalf("run() stdout = %q, want active test baseline summary", stdout.String())
		}
	})
	t.Run("stale", func(t *testing.T) {
		repoRoot := t.TempDir()
		if err := os.MkdirAll(filepath.Join(repoRoot, "pkg"), 0o755); err != nil {
			t.Fatalf("create package root: %v", err)
		}
		writeTestServiceImportBaseline(t, repoRoot, filePath, importPath, "factory_sessions")

		stderr := &bytes.Buffer{}
		err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
		if err == nil || !strings.Contains(stderr.String(), "stale test service import baseline entry") {
			t.Fatalf("run() = %v stderr=%q, want stale test baseline failure", err, stderr.String())
		}
	})
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

func TestRunRejectsRetiredExecutionTestHarnessPackageAndImport(t *testing.T) {
	t.Parallel()

	const (
		packagePath = "pkg/services/factory_sessions/internal/execution/testharness"
		importPath  = "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/testharness"
	)
	t.Run("package", func(t *testing.T) {
		repoRoot := t.TempDir()
		writeGoImportFile(t, repoRoot, packagePath+"/harness.go", "testharness", "fmt")

		stderr := &bytes.Buffer{}
		err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
		if err == nil || !strings.Contains(stderr.String(), "prohibited retired package root: "+packagePath) {
			t.Fatalf("run() = %v stderr=%q, want retired package root rejected", err, stderr.String())
		}
	})
	t.Run("import", func(t *testing.T) {
		repoRoot := t.TempDir()
		writeGoImportFile(t, repoRoot, "pkg/wire/session_test.go", "wire", importPath)

		stderr := &bytes.Buffer{}
		err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
		if err == nil || !strings.Contains(stderr.String(), "prohibited retired package import: "+importPath) {
			t.Fatalf("run() = %v stderr=%q, want retired package import rejected", err, stderr.String())
		}
	})
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

func writeTestServiceImportBaseline(
	t *testing.T,
	repoRoot string,
	filePath string,
	importPath string,
	owner string,
) {
	t.Helper()
	content := `{
  "version": 1,
  "entries": [
    {
      "owner": "` + owner + `",
      "importPath": "` + importPath + `",
      "filePath": "` + filePath + `",
      "targetRoot": "pkg/services/` + owner + `",
      "stage": "` + testServiceImportBaselineStage + `",
      "deletionGate": "` + testServiceImportDeletionGate + `"
    }
  ]
}
`
	if err := os.WriteFile(filepath.Join(repoRoot, testServiceImportBaselinePath), []byte(content), 0o600); err != nil {
		t.Fatalf("write test service baseline: %v", err)
	}
}

func writePeerServiceBaseline(
	t *testing.T,
	repoRoot string,
	filePath string,
	importPath string,
	owner string,
	peer string,
) {
	t.Helper()
	content := `{
  "version": 1,
  "entries": [
    {
      "owner": "` + owner + `",
      "peer": "` + peer + `",
      "importPath": "` + importPath + `",
      "filePath": "` + filePath + `",
      "targetRoot": "pkg/services/` + peer + `",
      "stage": "` + peerServiceImportBaselineStage + `",
      "deletionGate": "` + peerServiceImportDeletionGate + `"
    }
  ]
}
`
	if err := os.WriteFile(filepath.Join(repoRoot, peerServiceImportBaselinePath), []byte(content), 0o600); err != nil {
		t.Fatalf("write peer service baseline: %v", err)
	}
}

func TestPeerServiceImportBaselineRejectsWildcardAndUnrecognizedMigrationContract(t *testing.T) {
	t.Parallel()

	entry := peerServiceImportBaselineEntry{
		Owner:        "factory_sessions",
		Peer:         "factory_runtime",
		ImportPath:   "github.com/portpowered/infinite-you/pkg/services/factory_runtime/*",
		FilePath:     "pkg/services/factory_sessions/*.go",
		TargetRoot:   "pkg/services/factory_runtime",
		Stage:        peerServiceImportBaselineStage,
		DeletionGate: peerServiceImportDeletionGate,
	}
	if err := validatePeerServiceImportBaselineEntry(entry); err == nil || !strings.Contains(err.Error(), "wildcards") {
		t.Fatalf("validate wildcard error = %v, want wildcard rejection", err)
	}

	entry.ImportPath = "github.com/portpowered/infinite-you/pkg/services/factory_runtime/runtime"
	entry.FilePath = "pkg/services/factory_sessions/internal/sessionservice.go"
	entry.Stage = "unreviewed migration"
	if err := validatePeerServiceImportBaselineEntry(entry); err == nil || !strings.Contains(err.Error(), "recognized peer-service migration contract") {
		t.Fatalf("validate migration contract error = %v, want exact stage rejection", err)
	}
}
