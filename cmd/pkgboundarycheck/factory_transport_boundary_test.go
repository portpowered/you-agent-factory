package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunRejectsFactoryDomainTransportImports(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoImportFile(t, repoRoot, "pkg/factory/runtime/transport.go", "runtime", "github.com/portpowered/infinite-you/pkg/transports/mapping")
	writeGoImportFile(t, repoRoot, "pkg/root/cli.go", "root", "github.com/portpowered/infinite-you/pkg/transports/cli")

	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want reverse Factory-to-transport import rejected")
	}
	for _, want := range []string{
		"prohibited Factory domain transport import: github.com/portpowered/infinite-you/pkg/transports/mapping (pkg/factory/runtime/transport.go)",
		"domain owner: pkg/factory/runtime",
		"Factory domain packages must not consume transport contracts or adapters",
		"define the input at its Factory or Factory Session owner and map generated values under pkg/transports/mapping",
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("run() stderr = %q, want %q", stderr.String(), want)
		}
	}
}

func TestRunAllowsFactoryTransportImportsOnlyForDocumentedMigrationFilesAndTests(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoImportFile(t, repoRoot, "pkg/factory/definition/service.go", "factorydefinition", "github.com/portpowered/infinite-you/pkg/transports/http/generated")
	writeGoImportFile(t, repoRoot, "pkg/factory/runtime/runtime_test.go", "runtime", "github.com/portpowered/infinite-you/pkg/transports/mapping")

	stderr := &bytes.Buffer{}
	if err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr); err != nil {
		t.Fatalf("run() error = %v, want exact migration inventory and test imports allowed; stderr=%q", err, stderr.String())
	}

	writeGoImportFile(t, repoRoot, "pkg/factory/definition/new_adapter.go", "factorydefinition", "github.com/portpowered/infinite-you/pkg/transports/http/generated")
	stderr.Reset()
	if err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr); err == nil {
		t.Fatal("run() error = nil, want unlisted Factory definition transport import rejected")
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
	if want := "prohibited Factory domain transport import: github.com/portpowered/infinite-you/pkg/transports/mapping (pkg/factory/packages/tts/observability.go)"; !strings.Contains(stderr.String(), want) {
		t.Fatalf("run() stderr = %q, want %q", stderr.String(), want)
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
	if want := "prohibited Factory domain transport import: github.com/portpowered/infinite-you/pkg/transports/cli/docs (" + path + ")"; !strings.Contains(stderr.String(), want) {
		t.Fatalf("run() stderr = %q, want %q", stderr.String(), want)
	}
}
