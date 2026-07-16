package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunRejectsProtectedDomainTransportImports(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoImportFile(t, repoRoot, "pkg/factory/runtime/transport.go", "runtime", "github.com/portpowered/infinite-you/pkg/transports/mapping")
	writeGoImportFile(t, repoRoot, "pkg/models/catalog/transport.go", "catalog", "github.com/portpowered/infinite-you/pkg/transports/http/generated")
	writeGoImportFile(t, repoRoot, "pkg/work/content/generated.go", "content", "github.com/portpowered/infinite-you/pkg/transports/http/generated")
	writeGoImportFile(t, repoRoot, "pkg/workers/inference/generated.go", "inference", "github.com/portpowered/infinite-you/pkg/transports/mapping/workcontent")
	writeGoImportFile(t, repoRoot, "pkg/root/cli.go", "root", "github.com/portpowered/infinite-you/pkg/transports/cli")

	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want reverse Factory-to-transport import rejected")
	}
	for _, want := range []string{
		"prohibited domain transport import: github.com/portpowered/infinite-you/pkg/transports/mapping (pkg/factory/runtime/transport.go)",
		"domain owner: pkg/factory/runtime",
		"prohibited domain transport import: github.com/portpowered/infinite-you/pkg/transports/http/generated (pkg/models/catalog/transport.go)",
		"domain owner: pkg/models/catalog",
		"prohibited domain transport import: github.com/portpowered/infinite-you/pkg/transports/http/generated (pkg/work/content/generated.go)",
		"domain owner: pkg/work/content",
		"prohibited domain transport import: github.com/portpowered/infinite-you/pkg/transports/mapping/workcontent (pkg/workers/inference/generated.go)",
		"domain owner: pkg/workers/inference",
		"protected domain packages must not consume transport contracts or adapters",
		"define the input at its domain owner and map generated values under pkg/transports/mapping",
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("run() stderr = %q, want %q", stderr.String(), want)
		}
	}
}

func TestRunAllowsProtectedDomainTransportImportsOnlyForTests(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoImportFile(t, repoRoot, "pkg/factory/runtime/runtime_test.go", "runtime", "github.com/portpowered/infinite-you/pkg/transports/mapping")
	writeGoImportFile(t, repoRoot, "pkg/models/host/contract_test.go", "modelhost", "github.com/portpowered/infinite-you/pkg/transports/http/generated")
	writeGoImportFile(t, repoRoot, "pkg/work/content/content_test.go", "content", "github.com/portpowered/infinite-you/pkg/transports/http/generated")
	writeGoImportFile(t, repoRoot, "pkg/workers/inference/inference_test.go", "inference", "github.com/portpowered/infinite-you/pkg/transports/http/generated")
	writeGoImportFile(t, repoRoot, "pkg/work/content/contract.go", "content", "github.com/portpowered/infinite-you/pkg/work")

	stderr := &bytes.Buffer{}
	if err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr); err != nil {
		t.Fatalf("run() error = %v, want test imports allowed; stderr=%q", err, stderr.String())
	}

	writeGoImportFile(t, repoRoot, "pkg/factory/definition/service.go", "factorydefinition", "github.com/portpowered/infinite-you/pkg/transports/http/generated")
	stderr.Reset()
	if err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr); err == nil {
		t.Fatal("run() error = nil, want retired Factory definition transport import rejected")
	}
}

func TestRunAllowsOnlyDocumentedDomainTransportMigrationFiles(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoImportFile(t, repoRoot, "pkg/models/service/api.go", "service", "github.com/portpowered/infinite-you/pkg/transports/http/generated")

	stderr := &bytes.Buffer{}
	if err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr); err != nil {
		t.Fatalf("run() error = %v, want exact migration files allowed; stderr=%q", err, stderr.String())
	}

	writeGoImportFile(t, repoRoot, "pkg/models/local/catalog.go", "local", "github.com/portpowered/infinite-you/pkg/transports/http/generated")
	stderr.Reset()
	if err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr); err == nil {
		t.Fatal("run() error = nil, want migrated model catalog import rejected")
	}

	for _, path := range []string{
		"pkg/models/local/managed_runtime.go",
		"pkg/models/host/catalog_host.go",
		"pkg/models/host/contract.go",
		"pkg/models/host/diagnostics.go",
		"pkg/models/host/lease_policy.go",
		"pkg/models/host/local_assets.go",
		"pkg/models/host/supervisor.go",
	} {
		writeGoImportFile(t, repoRoot, path, "modelhost", "github.com/portpowered/infinite-you/pkg/transports/http/generated")
	}
	stderr.Reset()
	if err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr); err == nil {
		t.Fatal("run() error = nil, want migrated model runtime or host imports rejected")
	}

	writeGoImportFile(t, repoRoot, "pkg/workers/inference/inference.go", "inference", "github.com/portpowered/infinite-you/pkg/transports/http/generated")
	stderr.Reset()
	if err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr); err == nil {
		t.Fatal("run() error = nil, want migrated worker inference import rejected")
	}

	writeGoImportFile(t, repoRoot, "pkg/workers/executor/agentrun/failure.go", "agentrun", "github.com/portpowered/infinite-you/pkg/transports/mapping")
	stderr.Reset()
	if err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr); err == nil {
		t.Fatal("run() error = nil, want migrated agent-run failure import rejected")
	}

	for _, path := range []string{
		"pkg/models/assets/puller.go",
		"pkg/models/local/puller.go",
	} {
		writeGoImportFile(t, repoRoot, path, "models", "github.com/portpowered/infinite-you/pkg/transports/mapping")
	}
	stderr.Reset()
	if err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr); err == nil {
		t.Fatal("run() error = nil, want migrated model asset pull imports rejected")
	}

	for _, path := range []string{
		"pkg/workers/provider/parityfixtures/mode_parity.go",
		"pkg/workers/provider/parityfixtures/suite.go",
		"pkg/workers/provider/parityfixtures/transport.go",
	} {
		writeGoImportFile(t, repoRoot, path, "parityfixtures", "github.com/portpowered/infinite-you/pkg/transports/mapping")
	}
	stderr.Reset()
	if err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr); err == nil {
		t.Fatal("run() error = nil, want internalized provider parity imports rejected")
	}
}

func TestRunRejectsRetiredPackagedFactoryTransportImport(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoImportFile(t, repoRoot, "pkg/factory/packages/tts/observability.go", "tts", "github.com/portpowered/infinite-you/pkg/transports/mapping")

	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want migrated packaged Factory transport import rejected")
	}
	if want := "prohibited domain transport import: github.com/portpowered/infinite-you/pkg/transports/mapping (pkg/factory/packages/tts/observability.go)"; !strings.Contains(stderr.String(), want) {
		t.Fatalf("run() stderr = %q, want %q", stderr.String(), want)
	}
}

func TestRunRejectsRetiredFactoryDefinitionHostTransportImport(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	path := "pkg/factory/definition/host.go"
	writeGoImportFile(t, repoRoot, path, "factorydefinition", "github.com/portpowered/infinite-you/pkg/transports/http/generated")

	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want migrated Factory definition host import rejected")
	}
	if want := "prohibited domain transport import: github.com/portpowered/infinite-you/pkg/transports/http/generated (" + path + ")"; !strings.Contains(stderr.String(), want) {
		t.Fatalf("run() stderr = %q, want %q", stderr.String(), want)
	}
}

func TestRunRejectsRetiredFactoryDefinitionValidationTransportImport(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	path := "pkg/factory/definition/validation.go"
	writeGoImportFile(t, repoRoot, path, "factorydefinition", "github.com/portpowered/infinite-you/pkg/transports/mapping/validationentry")

	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want migrated Factory definition validation import rejected")
	}
	if want := "prohibited domain transport import: github.com/portpowered/infinite-you/pkg/transports/mapping/validationentry (" + path + ")"; !strings.Contains(stderr.String(), want) {
		t.Fatalf("run() stderr = %q, want %q", stderr.String(), want)
	}
}

func TestRunRejectsRetiredFactoryDefinitionSaveTransportImports(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	for _, path := range []string{
		"pkg/factory/definition/save.go",
		"pkg/factory/definition/upsert.go",
	} {
		writeGoImportFile(t, repoRoot, path, "factorydefinition", "github.com/portpowered/infinite-you/pkg/transports/http/generated")
	}

	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want migrated Factory definition save imports rejected")
	}
	for _, path := range []string{
		"pkg/factory/definition/save.go",
		"pkg/factory/definition/upsert.go",
	} {
		if want := "prohibited domain transport import: github.com/portpowered/infinite-you/pkg/transports/http/generated (" + path + ")"; !strings.Contains(stderr.String(), want) {
			t.Fatalf("run() stderr = %q, want %q", stderr.String(), want)
		}
	}
}

func TestRunRejectsRetiredResponseStreamRemovalGateTransportImport(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	path := "pkg/factory/sessions/responsestream/removalgate/gate.go"
	writeGoImportFile(t, repoRoot, path, "removalgate", "github.com/portpowered/infinite-you/pkg/transports/cli/docs")

	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want migrated response-stream removal-gate import rejected")
	}
	if want := "prohibited domain transport import: github.com/portpowered/infinite-you/pkg/transports/cli/docs (" + path + ")"; !strings.Contains(stderr.String(), want) {
		t.Fatalf("run() stderr = %q, want %q", stderr.String(), want)
	}
}
