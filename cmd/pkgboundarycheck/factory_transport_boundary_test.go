package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunRejectsProtectedDomainTransportImports(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoImportFile(t, repoRoot, "pkg/services/factory_runtime/runtime/transport.go", "runtime", "github.com/portpowered/infinite-you/pkg/transports/mapping")
	writeGoImportFile(t, repoRoot, "pkg/services/models/catalog/transport.go", "catalog", "github.com/portpowered/infinite-you/pkg/transports/http/generated")
	writeGoImportFile(t, repoRoot, "pkg/services/work/generated.go", "content", "github.com/portpowered/infinite-you/pkg/transports/http/generated")
	writeGoImportFile(t, repoRoot, "pkg/services/workers/inference/generated.go", "inference", "github.com/portpowered/infinite-you/pkg/transports/mapping/workcontent")
	writeGoImportFile(t, repoRoot, "pkg/root/cli.go", "root", "github.com/portpowered/infinite-you/pkg/transports/cli")

	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want reverse Factory-to-transport import rejected")
	}
	for _, want := range []string{
		"prohibited domain transport import: github.com/portpowered/infinite-you/pkg/transports/mapping (pkg/services/factory_runtime/runtime/transport.go)",
		"domain owner: pkg/services/factory_runtime/runtime",
		"prohibited domain transport import: github.com/portpowered/infinite-you/pkg/transports/http/generated (pkg/services/models/catalog/transport.go)",
		"domain owner: pkg/services/models/catalog",
		"prohibited domain transport import: github.com/portpowered/infinite-you/pkg/transports/http/generated (pkg/services/work/generated.go)",
		"domain owner: pkg/services/work",
		"prohibited domain transport import: github.com/portpowered/infinite-you/pkg/transports/mapping/workcontent (pkg/services/workers/inference/generated.go)",
		"domain owner: pkg/services/workers/inference",
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
	writeGoImportFile(t, repoRoot, "pkg/services/factory_runtime/runtime/runtime_test.go", "runtime", "github.com/portpowered/infinite-you/pkg/transports/mapping")
	writeGoImportFile(t, repoRoot, "pkg/services/models/host/contract_test.go", "modelhost", "github.com/portpowered/infinite-you/pkg/transports/http/generated")
	writeGoImportFile(t, repoRoot, "pkg/services/work/content_test.go", "content", "github.com/portpowered/infinite-you/pkg/transports/http/generated")
	writeGoImportFile(t, repoRoot, "pkg/services/workers/inference/inference_test.go", "inference", "github.com/portpowered/infinite-you/pkg/transports/http/generated")
	writeGoImportFile(t, repoRoot, "pkg/services/work/contract.go", "content", "github.com/portpowered/infinite-you/pkg/services/work")

	stderr := &bytes.Buffer{}
	if err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr); err != nil {
		t.Fatalf("run() error = %v, want test imports allowed; stderr=%q", err, stderr.String())
	}

	writeGoImportFile(t, repoRoot, "pkg/services/factory_definitions/definition/service.go", "factorydefinition", "github.com/portpowered/infinite-you/pkg/transports/http/generated")
	stderr.Reset()
	if err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr); err == nil {
		t.Fatal("run() error = nil, want retired Factory definition transport import rejected")
	}
}

func TestRunRejectsRetiredDomainTransportMigrationFiles(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoImportFile(t, repoRoot, "pkg/services/models/service/invoke.go", "service", "github.com/portpowered/infinite-you/pkg/transports/http/generated")

	stderr := &bytes.Buffer{}
	if err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr); err == nil {
		t.Fatal("run() error = nil, want retired model service invocation import rejected")
	}

	writeGoImportFile(t, repoRoot, "pkg/services/models/local/catalog.go", "local", "github.com/portpowered/infinite-you/pkg/transports/http/generated")
	stderr.Reset()
	if err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr); err == nil {
		t.Fatal("run() error = nil, want migrated model catalog import rejected")
	}

	for _, path := range []string{
		"pkg/services/models/service/api.go",
		"pkg/services/models/service/catalog.go",
	} {
		writeGoImportFile(t, repoRoot, path, "service", "github.com/portpowered/infinite-you/pkg/transports/http/generated")
	}
	stderr.Reset()
	if err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr); err == nil {
		t.Fatal("run() error = nil, want migrated model service catalog imports rejected")
	}

	for _, path := range []string{
		"pkg/services/models/local/managed_runtime.go",
		"pkg/services/models/host/catalog_host.go",
		"pkg/services/models/host/contract.go",
		"pkg/services/models/host/diagnostics.go",
		"pkg/services/models/host/lease_policy.go",
		"pkg/services/models/host/local_assets.go",
		"pkg/services/models/host/supervisor.go",
	} {
		writeGoImportFile(t, repoRoot, path, "modelhost", "github.com/portpowered/infinite-you/pkg/transports/http/generated")
	}
	stderr.Reset()
	if err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr); err == nil {
		t.Fatal("run() error = nil, want migrated model runtime or host imports rejected")
	}

	writeGoImportFile(t, repoRoot, "pkg/services/workers/inference/inference.go", "inference", "github.com/portpowered/infinite-you/pkg/transports/http/generated")
	stderr.Reset()
	if err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr); err == nil {
		t.Fatal("run() error = nil, want migrated worker inference import rejected")
	}

	writeGoImportFile(t, repoRoot, "pkg/services/workers/executor/agentrun/failure.go", "agentrun", "github.com/portpowered/infinite-you/pkg/transports/mapping")
	stderr.Reset()
	if err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr); err == nil {
		t.Fatal("run() error = nil, want migrated agent-run failure import rejected")
	}

	for _, path := range []string{
		"pkg/services/models/assets/puller.go",
		"pkg/services/models/local/puller.go",
	} {
		writeGoImportFile(t, repoRoot, path, "models", "github.com/portpowered/infinite-you/pkg/transports/mapping")
	}
	stderr.Reset()
	if err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr); err == nil {
		t.Fatal("run() error = nil, want migrated model asset pull imports rejected")
	}

	for _, path := range []string{
		"pkg/services/workers/provider/parityfixtures/mode_parity.go",
		"pkg/services/workers/provider/parityfixtures/suite.go",
		"pkg/services/workers/provider/parityfixtures/transport.go",
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
	writeGoImportFile(t, repoRoot, "pkg/services/factory_definitions/packages/tts/observability.go", "tts", "github.com/portpowered/infinite-you/pkg/transports/mapping")

	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want migrated packaged Factory transport import rejected")
	}
	if want := "prohibited domain transport import: github.com/portpowered/infinite-you/pkg/transports/mapping (pkg/services/factory_definitions/packages/tts/observability.go)"; !strings.Contains(stderr.String(), want) {
		t.Fatalf("run() stderr = %q, want %q", stderr.String(), want)
	}
}

func TestRunRejectsRetiredFactoryDefinitionHostTransportImport(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	path := "pkg/services/factory_definitions/definition/host.go"
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
	path := "pkg/services/factory_definitions/definition/validation.go"
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
		"pkg/services/factory_definitions/definition/save.go",
		"pkg/services/factory_definitions/definition/upsert.go",
	} {
		writeGoImportFile(t, repoRoot, path, "factorydefinition", "github.com/portpowered/infinite-you/pkg/transports/http/generated")
	}

	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want migrated Factory definition save imports rejected")
	}
	for _, path := range []string{
		"pkg/services/factory_definitions/definition/save.go",
		"pkg/services/factory_definitions/definition/upsert.go",
	} {
		if want := "prohibited domain transport import: github.com/portpowered/infinite-you/pkg/transports/http/generated (" + path + ")"; !strings.Contains(stderr.String(), want) {
			t.Fatalf("run() stderr = %q, want %q", stderr.String(), want)
		}
	}
}

func TestRunRejectsRetiredResponseStreamRemovalGateTransportImport(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	path := "pkg/services/factory_sessions/internal/responsestream/removalgate/gate.go"
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
