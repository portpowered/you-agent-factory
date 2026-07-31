// backendsizecheck:ignore-file service-ownership migration preserves this consolidated surface until a dedicated responsibility split removes the exemption.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunSucceedsWithApprovedRootPackageFamilies(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	for _, packagePath := range []string{
		"pkg/root",
		"pkg/wire",
		"pkg/transports",
		"pkg/services",
		"pkg/platform",
	} {
		makeDir(t, repoRoot, packagePath)
	}
	makeDir(t, repoRoot, "pkg/transports/http/client")

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, stdout, stderr)
	if err != nil {
		t.Fatalf("run() error = %v, want nil", err)
	}
	if got := stdout.String(); !strings.Contains(got, "package boundary passed (no blocking package-boundary violations)") {
		t.Fatalf("run() stdout = %q, want package-boundary success message", got)
	}
	if got := stdout.String(); !strings.Contains(got, "active generated-code exceptions: pkg/transports/http/client (root), pkg/transports/http/generated (root)") {
		t.Fatalf("run() stdout = %q, want generated-code exception summary", got)
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("run() stderr = %q, want empty", got)
	}
}

func TestRunAllowsOnlyRootAndWireToImportApplicationGraph(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoImportFile(t, repoRoot, "pkg/wire/graph.go", "wire", "github.com/portpowered/infinite-you/pkg/services/factory_definitions")
	writeGoImportFile(t, repoRoot, "pkg/root/root.go", "root", applicationGraphImportPath)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, stdout, stderr)
	if err != nil {
		t.Fatalf("run() error = %v, want allowed composition direction", err)
	}
	if got := stdout.String(); !strings.Contains(got, "package boundary passed") {
		t.Fatalf("run() stdout = %q, want package-boundary success", got)
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("run() stderr = %q, want empty", got)
	}
}

func TestRunRejectsInitializerImportingApplicationGraph(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoImportFile(t, repoRoot, "pkg/initializer/core.go", "initializer", applicationGraphImportPath)

	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want Initializer-to-Wire composition import rejected")
	}
	if !strings.Contains(stderr.String(), "prohibited application composition import") {
		t.Fatalf("run() stderr = %q, want application-composition diagnostic", stderr.String())
	}
}

func TestRunRejectsApplicationGraphImportsFromTestsOutsidePackageRoot(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/root/root.go", "package root\n")
	writeGoImportFile(t, repoRoot, "tests/stress/alternate_graph_test.go", "stress", applicationGraphImportPath)

	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want alternate test injector rejected")
	}
	for _, want := range []string{
		"prohibited application composition import",
		"tests/stress/alternate_graph_test.go",
		"inject the collaborator through pkg/root",
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("run() stderr = %q, want %q", stderr.String(), want)
		}
	}
}

func TestRunAllowsServiceConstructionOnlyInOwnerAndWire(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/services/work/owner_test.go", `package work_test

import work "github.com/portpowered/infinite-you/pkg/services/work"

func ownerInvariant() { _ = work.NewRequestPreparationService() }
`)
	writeGoSourceFile(t, repoRoot, "pkg/wire/work.go", `package wire

import work "github.com/portpowered/infinite-you/pkg/services/work"

func provideWorkPreparation() { _ = work.NewRequestPreparationService() }
`)
	writeGoSourceFile(t, repoRoot, "pkg/transports/http/value.go", `package http

import work "github.com/portpowered/infinite-you/pkg/services/work"

func selectValue() { _ = work.ListOptions{WorkTypeName: "story"} }
`)

	stderr := &bytes.Buffer{}
	if err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr); err != nil {
		t.Fatalf("run() error = %v, want owner-local, Wire, and value construction allowed; stderr=%q", err, stderr.String())
	}
}

func TestRunRejectsProductServiceConstructionFromTransportInitializerAndExternalTest(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/transports/http/staging.go", `package http

import work "github.com/portpowered/infinite-you/pkg/services/work"

func build() { _ = work.NewFutureService() }
`)
	writeGoSourceFile(t, repoRoot, "pkg/initializer/dashboard/view.go", `package dashboard

import visualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"

var runtimeFactory = visualization.New
`)
	writeGoSourceFile(t, repoRoot, "tests/functional/session_test.go", `package functional

import sessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"

func directSession() { _ = sessions.NewLiveSession("", "", nil, nil) }
`)

	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want product-service construction rejected")
	}
	got := stderr.String()
	for _, want := range []string{
		"prohibited product-service construction: github.com/portpowered/infinite-you/pkg/services/work.NewFutureService",
		"prohibited product-service construction: github.com/portpowered/infinite-you/pkg/services/factory_visualization.New",
		"prohibited product-service construction: github.com/portpowered/infinite-you/pkg/services/factory_sessions.NewLiveSession",
		"prohibited Initializer behavior: github.com/portpowered/infinite-you/pkg/services/factory_visualization (service-coupling)",
		"construct the collaborator in pkg/wire and inject its service-root role",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("run() stderr = %q, want substring %q", got, want)
		}
	}
	if got := err.Error(); got != "[agent-factory:pkg-boundary] found 4 package-boundary violation(s)" {
		t.Fatalf("run() error = %q, want four violations", got)
	}
}

func TestRunRejectsUnaliasedServiceConstructionWhenPackageNameDiffersFromPathBase(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/services/factory_runtime/contracts.go", `package factory

func NewFutureRuntime() {}
`)
	writeGoSourceFile(t, repoRoot, "pkg/transports/cli/runtime.go", `package cli

import "github.com/portpowered/infinite-you/pkg/services/factory_runtime"

func build() { factory.NewFutureRuntime() }
`)

	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want package-clause-resolved construction failure")
	}
	for _, want := range []string{
		"prohibited product-service construction: github.com/portpowered/infinite-you/pkg/services/factory_runtime.NewFutureRuntime",
		"pkg/transports/cli/runtime.go",
		"construct the collaborator in pkg/wire and inject its service-root role",
	} {
		if got := stderr.String(); !strings.Contains(got, want) {
			t.Fatalf("run() stderr = %q, want substring %q", got, want)
		}
	}
}

func TestRunRejectsConstructionThroughPermittedExternalEffectContractImport(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/services/automations/internal/services/hosted_sources/contract.go", `package hostedsources

func New() {}
`)
	writeGoSourceFile(t, repoRoot, "tests/functional/edge_test.go", `package functional

import hostedsources "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/hosted_sources"

func directImplementation() { hostedsources.New() }
`)

	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want external-effect implementation construction rejected")
	}
	for _, want := range []string{
		"prohibited product-service construction: github.com/portpowered/infinite-you/pkg/services/automations/internal/services/hosted_sources.New",
		"construct the collaborator in pkg/wire and inject its service-root role",
	} {
		if got := stderr.String(); !strings.Contains(got, want) {
			t.Fatalf("run() stderr = %q, want substring %q", got, want)
		}
	}
}

func TestRunRequiresExactDeletionOnlyServiceConstructionBaseline(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	const filePath = "pkg/transports/http/preparation.go"
	const importPath = "github.com/portpowered/infinite-you/pkg/services/work"
	writeGoSourceFile(t, repoRoot, filePath, `package http

import work "github.com/portpowered/infinite-you/pkg/services/work"

func build() { _ = work.NewRequestPreparationService() }
`)
	baseline := serviceConstructionBaseline{
		Version: 1,
		Entries: []serviceConstructionBaselineEntry{{
			Owner:        "work",
			ImportPath:   importPath,
			Symbol:       "NewRequestPreparationService",
			FilePath:     filePath,
			Count:        1,
			Stage:        serviceConstructionBaselineStage,
			DeletionGate: serviceConstructionDeletionGate,
		}},
	}
	payload, err := json.Marshal(baseline)
	if err != nil {
		t.Fatalf("marshal service construction baseline: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(filepath.Join(repoRoot, serviceConstructionBaselinePath)), 0o755); err != nil {
		t.Fatalf("create service construction baseline directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, serviceConstructionBaselinePath), payload, 0o644); err != nil {
		t.Fatalf("write service construction baseline: %v", err)
	}
	if err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatalf("run() error = %v, want exact baseline accepted", err)
	}

	writeGoSourceFile(t, repoRoot, filePath, `package http

import work "github.com/portpowered/infinite-you/pkg/services/work"

func build() { _ = work.NormalizeList }
`)
	stderr := &bytes.Buffer{}
	err = run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want stale baseline rejected")
	}
	if got := stderr.String(); !strings.Contains(got, "stale service construction baseline entry: "+filePath+" -> "+importPath+".NewRequestPreparationService") {
		t.Fatalf("run() stderr = %q, want exact stale-baseline diagnostic", got)
	}
}

func TestMigrationBaselinesMustBeDeletedAtZero(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	payload, err := json.Marshal(serviceConstructionBaseline{Version: 1, Entries: []serviceConstructionBaselineEntry{}})
	if err != nil {
		t.Fatalf("marshal empty service construction baseline: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(filepath.Join(repoRoot, serviceConstructionBaselinePath)), 0o755); err != nil {
		t.Fatalf("create service construction baseline directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, serviceConstructionBaselinePath), payload, 0o644); err != nil {
		t.Fatalf("write empty service construction baseline: %v", err)
	}

	_, err = loadServiceConstructionBaseline(repoRoot)
	if err == nil || !strings.Contains(err.Error(), "delete the file to record zero debt") {
		t.Fatalf("loadServiceConstructionBaseline() error = %v, want deletion requirement", err)
	}
}

func TestServiceConstructionBaselineRejectsWildcardEntry(t *testing.T) {
	t.Parallel()

	entry := serviceConstructionBaselineEntry{
		Owner:        "work",
		ImportPath:   "github.com/portpowered/infinite-you/pkg/services/work",
		Symbol:       "New*",
		FilePath:     "tests/functional/*.go",
		Count:        1,
		Stage:        serviceConstructionBaselineStage,
		DeletionGate: serviceConstructionDeletionGate,
	}
	if err := validateServiceConstructionBaselineEntry(entry); err == nil || !strings.Contains(err.Error(), "wildcards") {
		t.Fatalf("validate wildcard error = %v, want wildcard rejection", err)
	}
}

func TestTestServiceImportBaselineRejectsWildcardAndEmptyCreation(t *testing.T) {
	t.Parallel()

	entry := testServiceImportBaselineEntry{
		Owner:        "factory_sessions",
		ImportPath:   "github.com/portpowered/infinite-you/pkg/services/factory_sessions/*",
		FilePath:     "tests/functional/*.go",
		TargetRoot:   "pkg/services/factory_sessions",
		Stage:        testServiceImportBaselineStage,
		DeletionGate: testServiceImportDeletionGate,
	}
	if err := validateTestServiceImportBaselineEntry(entry); err == nil || !strings.Contains(err.Error(), "wildcards") {
		t.Fatalf("validate wildcard error = %v, want wildcard rejection", err)
	}

	if err := createTestServiceImportBaseline(config{root: t.TempDir()}); err == nil || !strings.Contains(err.Error(), "refusing to create empty") {
		t.Fatalf("create empty baseline error = %v, want empty creation rejected", err)
	}
}

func TestRunAllowsPlatformObservabilityAndRejectsRetiredImports(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoImportFile(t, repoRoot, "pkg/services/factory_runtime/internal/services/orchestration/runtime/canonical.go", "runtime", "github.com/portpowered/infinite-you/pkg/platform/logging")
	writeGoImportFile(t, repoRoot, "pkg/services/factory_runtime/internal/services/orchestration/runtime/metrics.go", "runtime", "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/metrics")
	writeGoImportFile(t, repoRoot, "pkg/wire/metrics.go", "wire", "github.com/portpowered/infinite-you/pkg/platform/metrics")

	stderr := &bytes.Buffer{}
	if err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr); err != nil {
		t.Fatalf("run() error = %v, want canonical platform logging import allowed; stderr=%q", err, stderr.String())
	}

	writeGoImportFile(t, repoRoot, "pkg/services/factory_runtime/internal/services/orchestration/runtime/retired.go", "runtime", "github.com/portpowered/infinite-you/pkg/logging")
	stderr.Reset()
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want retired logging import rejected")
	}
	for _, want := range []string{
		"prohibited retired package import: github.com/portpowered/infinite-you/pkg/logging",
		"canonical owner: pkg/platform/logging",
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("run() stderr = %q, want %q", stderr.String(), want)
		}
	}

	writeGoImportFile(t, repoRoot, "pkg/services/factory_runtime/internal/services/orchestration/runtime/retired_metrics.go", "runtime", "github.com/portpowered/infinite-you/pkg/internal/metrics")
	stderr.Reset()
	err = run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want retired metrics import rejected")
	}
	for _, want := range []string{
		"prohibited retired package import: github.com/portpowered/infinite-you/pkg/internal/metrics",
		"canonical owner: pkg/services/factory_runtime/internal/services/orchestration/metrics for domain contracts and pkg/platform/metrics for file-backed recording",
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("run() stderr = %q, want %q", stderr.String(), want)
		}
	}
}

func TestRunRejectsRetiredTransportImports(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	imports := []struct {
		filePath       string
		retiredPath    string
		canonicalOwner string
	}{
		{"pkg/services/factory_runtime/internal/services/orchestration/runtime/api.go", "github.com/portpowered/infinite-you/pkg/api", "pkg/transports/http"},
		{"pkg/services/factory_runtime/internal/services/orchestration/runtime/mapping.go", "github.com/portpowered/infinite-you/pkg/apisurface/factorysession", "pkg/transports/mapping"},
		{"pkg/root/cli.go", "github.com/portpowered/infinite-you/pkg/cli", "pkg/transports/cli"},
		{"pkg/root/mcp.go", "github.com/portpowered/infinite-you/pkg/mcp/server", "pkg/transports/mcp"},
		{"pkg/services/factory_runtime/internal/services/orchestration/runtime/client.go", "github.com/portpowered/infinite-you/pkg/generatedclient", "pkg/transports/http/client"},
	}
	for _, fixture := range imports {
		writeGoImportFile(t, repoRoot, fixture.filePath, filepath.Base(filepath.Dir(fixture.filePath)), fixture.retiredPath)
	}

	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want retired transport import failures")
	}
	for _, fixture := range imports {
		got := stderr.String()
		if !strings.Contains(got, "prohibited retired package import: "+fixture.retiredPath) {
			t.Fatalf("run() stderr = %q, want retired import %q", got, fixture.retiredPath)
		}
		if !strings.Contains(got, "canonical owner: "+fixture.canonicalOwner) {
			t.Fatalf("run() stderr = %q, want canonical successor %q", got, fixture.canonicalOwner)
		}
	}
	if got := err.Error(); got != "[agent-factory:pkg-boundary] found 5 package-boundary violation(s)" {
		t.Fatalf("run() error = %q, want five violations", got)
	}
}

func TestRunRejectsDomainPackageImportOfApplicationGraph(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoImportFile(t, repoRoot, "pkg/services/factory_runtime/internal/services/orchestration/runtime/composition.go", "runtime", applicationGraphImportPath)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, stdout, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want prohibited composition import failure")
	}
	if got := stdout.String(); got != "" {
		t.Fatalf("run() stdout = %q, want empty", got)
	}

	got := stderr.String()
	for _, want := range []string{
		"[agent-factory:pkg-boundary] prohibited application composition import: pkg/services/factory_runtime/internal/services/orchestration/runtime (pkg/services/factory_runtime/internal/services/orchestration/runtime/composition.go)",
		"pkg/wire is the outward application composition root and must not be imported by domain or transport packages",
		"depend on a narrow domain-owned contract and inject the collaborator through pkg/root or pkg/initializer",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("run() stderr = %q, want substring %q", got, want)
		}
	}
	if got := err.Error(); got != "[agent-factory:pkg-boundary] found 1 package-boundary violation(s)" {
		t.Fatalf("run() error = %q, want one violation", got)
	}
}

func TestRunRejectsDomainPackageImportOfApplicationGraphSubpackage(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoImportFile(t, repoRoot, "pkg/transports/http/composition.go", "http", applicationGraphImportPath+"/internal")

	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want prohibited composition subpackage import failure")
	}
	if got := stderr.String(); !strings.Contains(got, "prohibited application composition import: pkg/transports/http") {
		t.Fatalf("run() stderr = %q, want transport composition import diagnostic", got)
	}
}

func TestRunRejectsRetiredPackageRootsWithCanonicalOwners(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		packagePath    string
		canonicalOwner string
	}{
		{packagePath: "pkg/models", canonicalOwner: "pkg/services/models"},
		{packagePath: "pkg/work", canonicalOwner: "pkg/services/work"},
		{packagePath: "pkg/workers", canonicalOwner: "pkg/services/workers"},
		{packagePath: "pkg/modelhost", canonicalOwner: "pkg/services/models"},
		{packagePath: "pkg/localmodels", canonicalOwner: "pkg/services/models"},
		{packagePath: "pkg/hostedworkers", canonicalOwner: "Automation Hosted Sources (hosted polling / observation, secret resolution for observation, poll/restart/checkpoint, observation normalization, and commanding Work admission) or Workers Hosted Runner (remote Work execution request/result, execution lifecycle observation, cancellation, and normalized execution outcome under the Runner contract); transitional pkg/services/workers/services/hosted_logic location alone is not durable ownership"},
		{packagePath: "pkg/invocations", canonicalOwner: "pkg/services/work, pkg/services/factory_sessions, or pkg/services/workers, according to the concern"},
		{packagePath: "pkg/materialize", canonicalOwner: "pkg/services/work"},
		{packagePath: "pkg/timework", canonicalOwner: "pkg/services/automations/internal/services/cron"},
		{packagePath: "pkg/workcontent", canonicalOwner: "pkg/services/work"},
		{packagePath: "pkg/workgraph", canonicalOwner: "pkg/services/work"},
		{packagePath: "pkg/workquery", canonicalOwner: "pkg/services/work"},
		{packagePath: "pkg/interfaces", canonicalOwner: "the defining domain under pkg/services"},
		{packagePath: "pkg/replay", canonicalOwner: "pkg/services/recordings/replay for Factory-event replay policy and pkg/platform/replay for artifact filesystem mechanics"},
		{packagePath: "pkg/testutil", canonicalOwner: "internal/testutil or package-local test helpers"},
		{packagePath: "pkg/platform/runtimeinput", canonicalOwner: "bounded owner requests assembled by pkg/wire"},
	} {
		t.Run(tt.packagePath, func(t *testing.T) {
			t.Parallel()
			repoRoot := t.TempDir()
			makeDir(t, repoRoot, tt.packagePath)

			stderr := &bytes.Buffer{}
			err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
			if err == nil {
				t.Fatal("run() error = nil, want retired package root failure")
			}

			got := stderr.String()
			for _, want := range []string{
				"prohibited retired package root: " + tt.packagePath,
				"canonical owner: " + tt.canonicalOwner,
				"move the code to " + tt.canonicalOwner + " and delete the retired root",
			} {
				if !strings.Contains(got, want) {
					t.Fatalf("run() stderr = %q, want substring %q", got, want)
				}
			}
			if strings.Contains(got, "unapproved root package family") {
				t.Fatalf("run() stderr = %q, want retired-root diagnostic instead of generic root diagnostic", got)
			}
		})
	}
}

func TestRunRejectsRetiredPackageImportsWithCanonicalOwners(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		importPath     string
		retiredRoot    string
		canonicalOwner string
	}{
		{
			importPath:     "github.com/portpowered/infinite-you/pkg/models/service",
			retiredRoot:    "pkg/models",
			canonicalOwner: "pkg/services/models",
		},
		{
			importPath:     "github.com/portpowered/infinite-you/pkg/work/query",
			retiredRoot:    "pkg/work",
			canonicalOwner: "pkg/services/work",
		},
		{
			importPath:     "github.com/portpowered/infinite-you/pkg/workers/provider",
			retiredRoot:    "pkg/workers",
			canonicalOwner: "pkg/services/workers",
		},
		{
			importPath:     "github.com/portpowered/infinite-you/pkg/modelhost",
			retiredRoot:    "pkg/modelhost",
			canonicalOwner: "pkg/services/models",
		},
		{
			importPath:     "github.com/portpowered/infinite-you/pkg/localmodels/assets",
			retiredRoot:    "pkg/localmodels",
			canonicalOwner: "pkg/services/models",
		},
		{
			importPath:     "github.com/portpowered/infinite-you/pkg/hostedworkers/linear",
			retiredRoot:    "pkg/hostedworkers",
			canonicalOwner: "Automation Hosted Sources (hosted polling / observation, secret resolution for observation, poll/restart/checkpoint, observation normalization, and commanding Work admission) or Workers Hosted Runner (remote Work execution request/result, execution lifecycle observation, cancellation, and normalized execution outcome under the Runner contract); transitional pkg/services/workers/services/hosted_logic location alone is not durable ownership",
		},
		{
			importPath:     "github.com/portpowered/infinite-you/pkg/invocations/inference",
			retiredRoot:    "pkg/invocations",
			canonicalOwner: "pkg/services/work, pkg/services/factory_sessions, or pkg/services/workers, according to the concern",
		},
		{
			importPath:     "github.com/portpowered/infinite-you/pkg/materialize",
			retiredRoot:    "pkg/materialize",
			canonicalOwner: "pkg/services/work",
		},
		{
			importPath:     "github.com/portpowered/infinite-you/pkg/timework",
			retiredRoot:    "pkg/timework",
			canonicalOwner: "pkg/services/automations/internal/services/cron",
		},
		{
			importPath:     "github.com/portpowered/infinite-you/pkg/workcontent",
			retiredRoot:    "pkg/workcontent",
			canonicalOwner: "pkg/services/work",
		},
		{
			importPath:     "github.com/portpowered/infinite-you/pkg/workgraph",
			retiredRoot:    "pkg/workgraph",
			canonicalOwner: "pkg/services/work",
		},
		{
			importPath:     "github.com/portpowered/infinite-you/pkg/workquery",
			retiredRoot:    "pkg/workquery",
			canonicalOwner: "pkg/services/work",
		},
		{
			importPath:     "github.com/portpowered/infinite-you/pkg/interfaces",
			retiredRoot:    "pkg/interfaces",
			canonicalOwner: "the defining domain under pkg/services",
		},
		{
			importPath:     "github.com/portpowered/infinite-you/pkg/replay",
			retiredRoot:    "pkg/replay",
			canonicalOwner: "pkg/services/recordings/replay for Factory-event replay policy and pkg/platform/replay for artifact filesystem mechanics",
		},
		{
			importPath:     "github.com/portpowered/infinite-you/pkg/testutil/runtimefixtures",
			retiredRoot:    "pkg/testutil",
			canonicalOwner: "internal/testutil or package-local test helpers",
		},
	} {
		t.Run(tt.retiredRoot, func(t *testing.T) {
			t.Parallel()
			repoRoot := t.TempDir()
			writeGoImportFile(t, repoRoot, "pkg/services/factory_runtime/retired_import.go", "factory", tt.importPath)

			stderr := &bytes.Buffer{}
			err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
			if err == nil {
				t.Fatal("run() error = nil, want retired package import failure")
			}

			got := stderr.String()
			for _, want := range []string{
				"prohibited retired package import: " + tt.importPath + " (pkg/services/factory_runtime/retired_import.go)",
				"canonical owner: " + tt.canonicalOwner,
				"do not recreate or depend on " + tt.retiredRoot,
			} {
				if !strings.Contains(got, want) {
					t.Fatalf("run() stderr = %q, want substring %q", got, want)
				}
			}
		})
	}
}

func TestRunAllowsSameOwnerSubpackagesAndPeerRoots(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoImportFile(t, repoRoot, "pkg/services/factory_runtime/canonical_imports.go", "factory", "github.com/portpowered/infinite-you/pkg/services/models")
	writeGoImportFile(t, repoRoot, "pkg/services/models/host/host.go", "modelhost", "github.com/portpowered/infinite-you/pkg/services/models/local")
	writeGoImportFile(t, repoRoot, "pkg/services/automations/service/hosted.go", "service", "github.com/portpowered/infinite-you/pkg/services/workers")

	stderr := &bytes.Buffer{}
	if err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr); err != nil {
		t.Fatalf("run() error = %v, want same-owner subpackages and peer roots allowed; stderr=%q", err, stderr.String())
	}
}

func TestRunAllowsAnyImportableRepositoryPackageFromWire(t *testing.T) {
	t.Parallel()

	importPaths := []string{
		"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionservice",
		"github.com/portpowered/infinite-you/pkg/services/example/new_internal_adapter",
		"github.com/portpowered/infinite-you/pkg/transports/http/generated",
		"github.com/portpowered/infinite-you/pkg/platform/metrics",
	}

	allowedRoot := t.TempDir()
	for index, importPath := range importPaths {
		writeGoImportFile(t, allowedRoot, fmt.Sprintf("pkg/wire/concrete_%d.go", index), "wire", importPath)
	}
	writeGoImportFile(t, allowedRoot, "pkg/services/factory_sessions/internal_adapter.go", "factorysessions", importPaths[0])
	if err := run(config{root: allowedRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatalf("run() error = %v, want unrestricted Wire composition imports allowed", err)
	}

	for index, importPath := range importPaths[:2] {
		rejectedRoot := t.TempDir()
		filePath := fmt.Sprintf("pkg/initializer/concrete_%d.go", index)
		writeGoImportFile(t, rejectedRoot, filePath, "initializer", importPath)
		stderr := &bytes.Buffer{}
		err := run(config{root: rejectedRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
		if err == nil {
			t.Fatal("run() error = nil, want non-Wire concrete service import rejected")
		}
		for _, want := range []string{
			"prohibited external service subpackage import: " + importPath,
			filePath,
		} {
			if !strings.Contains(stderr.String(), want) {
				t.Fatalf("run() stderr = %q, want substring %q", stderr.String(), want)
			}
		}
	}
}

func TestRunRejectsExternalImportsOfConvergedServiceSubpackages(t *testing.T) {
	t.Parallel()

	importPaths := []string{
		"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/cursors",
		"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution",
		"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/fixtures",
		"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/invocation",
		"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/logicaltarget",
		"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responseeventstore",
		"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responsestream",
		"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtime",
		"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding",
		"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/editable",
		"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy/decisionenvelope",
		"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy/invocationinterpolation",
		"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy/invocationoutput",
		"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy/invocationworktype",
		"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/loadedsource",
		"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/loading",
		"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/portableconfig",
		"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog/persistence",
		"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy/quorumpolicy",
		"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/replayconfig",
		"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/runtimeconfig",
		"github.com/portpowered/infinite-you/pkg/services/factory_definitions/scaffold",
		"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/capture",
		"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy/ttsobservability",
		"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy/workstationexecution",
		"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy/workpropagation",
		"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/impl",
		"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/definitionmapping",
		"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/replayhooks",
		"github.com/portpowered/infinite-you/pkg/services/recordings/projections/dashboard",
		"github.com/portpowered/infinite-you/pkg/services/recordings/replay",
		"github.com/portpowered/infinite-you/pkg/services/recordings/service",
		"github.com/portpowered/infinite-you/pkg/services/workers/invocation",
	}
	for index, importPath := range importPaths {
		allowedRoot := t.TempDir()
		ownerPath := "pkg/services/factory_sessions"
		ownerPackage := "factorysessions"
		if strings.Contains(importPath, "/pkg/services/factory_definitions/") {
			ownerPath = "pkg/services/factory_definitions"
			ownerPackage = "factorydefinitions"
		}
		if strings.Contains(importPath, "/pkg/services/factory_runtime/") {
			ownerPath = "pkg/services/factory_runtime"
			ownerPackage = "factoryruntime"
		}
		if strings.Contains(importPath, "/pkg/services/recordings/") {
			ownerPath = "pkg/services/recordings"
			ownerPackage = "recordings"
		}
		if strings.Contains(importPath, "/pkg/services/workers/") {
			ownerPath = "pkg/services/workers"
			ownerPackage = "workers"
		}
		writeGoImportFile(
			t,
			allowedRoot,
			fmt.Sprintf("%s/internal_%d.go", ownerPath, index),
			ownerPackage,
			importPath,
		)
		if err := run(config{root: allowedRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
			t.Fatalf("run() error = %v, want owner-internal import allowed", err)
		}
		writeGoImportFile(
			t,
			allowedRoot,
			fmt.Sprintf("pkg/wire/converged_%d.go", index),
			"wire",
			importPath,
		)
		if err := run(config{root: allowedRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
			t.Fatalf("run() error = %v, want unrestricted Wire import allowed", err)
		}

		rejectedRoot := t.TempDir()
		filePath := fmt.Sprintf("pkg/initializer/factory_sessions_%d.go", index)
		writeGoImportFile(t, rejectedRoot, filePath, "initializer", importPath)
		stderr := &bytes.Buffer{}
		err := run(config{root: rejectedRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
		if err == nil {
			t.Fatal("run() error = nil, want external service subpackage import rejected")
		}
		for _, want := range []string{
			"prohibited external service subpackage import: " + importPath,
			filePath,
		} {
			if !strings.Contains(stderr.String(), want) {
				t.Fatalf("run() stderr = %q, want substring %q", stderr.String(), want)
			}
		}
	}
}

func TestRunRejectsExternalImportsOfUnlistedServiceSubpackagesByDefault(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	importPath := "github.com/portpowered/infinite-you/pkg/services/example/new_internal_adapter"
	filePath := "pkg/initializer/example_adapter.go"
	writeGoImportFile(t, repoRoot, filePath, "initializer", importPath)

	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want unlisted service subpackage import rejected")
	}
	for _, want := range []string{
		"prohibited external service subpackage import: " + importPath,
		filePath,
		"service subpackages are owner-internal for ordinary consumers",
		"import the exact pkg/services/<service-name> root",
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("run() stderr = %q, want substring %q", stderr.String(), want)
		}
	}
}

func TestRunAllowsUnlistedServiceSubpackageWithinItsOwner(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoImportFile(
		t,
		repoRoot,
		"pkg/services/example/service.go",
		"example",
		"github.com/portpowered/infinite-you/pkg/services/example/new_internal_adapter",
	)

	if err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatalf("run() error = %v, want owner-internal import allowed", err)
	}
}

func TestRunRejectsTransportImportsOfServiceImplementations(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoImportFile(
		t,
		repoRoot,
		"pkg/transports/http/runtime_state.go",
		"http",
		"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state",
	)

	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want transport implementation import rejected")
	}
	for _, want := range []string{
		"prohibited transport service implementation import: github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state",
		"pkg/transports/http/runtime_state.go",
		"transports may consume only service root contracts or explicitly public service subservices",
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("run() stderr = %q, want substring %q", stderr.String(), want)
		}
	}
}

func TestRunRejectsTransportImportsOfRetiredModelContractFacades(t *testing.T) {
	t.Parallel()

	for _, facade := range []string{"assets", "catalog", "inference", "managedruntime"} {
		facade := facade
		t.Run(facade, func(t *testing.T) {
			t.Parallel()

			repoRoot := t.TempDir()
			importPath := "github.com/portpowered/infinite-you/pkg/services/models/" + facade
			writeGoImportFile(t, repoRoot, "pkg/transports/http/model_contract.go", "http", importPath)

			stderr := &bytes.Buffer{}
			err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
			if err == nil {
				t.Fatalf("run() error = nil, want model %s facade import rejected", facade)
			}
			if want := "prohibited external service subpackage import: " + importPath; !strings.Contains(stderr.String(), want) {
				t.Fatalf("run() stderr = %q, want substring %q", stderr.String(), want)
			}
		})
	}
}

func TestRunRejectsTransportImportsOfRetiredWorkerDiagnosticsFacade(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	const importPath = "github.com/portpowered/infinite-you/pkg/services/workers/diagnostics"
	writeGoImportFile(t, repoRoot, "pkg/transports/http/diagnostics.go", "http", importPath)

	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want Worker diagnostics facade import rejected")
	}
	if want := "prohibited external service subpackage import: " + importPath; !strings.Contains(stderr.String(), want) {
		t.Fatalf("run() stderr = %q, want substring %q", stderr.String(), want)
	}
}

func TestRunAllowsTransportImportsOfServiceRootContracts(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoImportFile(
		t,
		repoRoot,
		"pkg/transports/http/public_contract.go",
		"http",
		"github.com/portpowered/infinite-you/pkg/services/workers",
	)

	stderr := &bytes.Buffer{}
	if err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr); err != nil {
		t.Fatalf("run() error = %v, want public service contract allowed; stderr=%q", err, stderr.String())
	}
}

func TestRunAllowsValidMigrationPackageException(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	makeDir(t, repoRoot, "pkg/legacytransport")
	policy := boundaryPolicy{
		approvedProductPackageFamilies: []string{"pkg/transports"},
		migrationPackageExceptions: []migrationPackageException{{
			packagePath:  "pkg/legacytransport",
			targetOwner:  "pkg/transports",
			workItem:     batch006TransportFamilyMove,
			deletionGate: "remove after callers move to pkg/transports",
		}},
	}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := runWithPolicy(config{root: repoRoot, packageRoot: defaultScanRoot}, policy, stdout, stderr)
	if err != nil {
		t.Fatalf("runWithPolicy() error = %v, want nil", err)
	}
	if got := stdout.String(); !strings.Contains(got, "package boundary passed") {
		t.Fatalf("runWithPolicy() stdout = %q, want success message", got)
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("runWithPolicy() stderr = %q, want empty", got)
	}
}

func TestValidatePolicyRejectsInvalidMigrationMetadata(t *testing.T) {
	t.Parallel()

	valid := migrationPackageException{
		packagePath:  "pkg/legacytransport",
		targetOwner:  "pkg/transports",
		workItem:     batch006TransportFamilyMove,
		deletionGate: "remove after callers move to pkg/transports",
	}
	tests := []struct {
		name     string
		mutate   func(*migrationPackageException)
		want     string
		families []string
	}{
		{
			name: "missing target owner",
			mutate: func(exception *migrationPackageException) {
				exception.targetOwner = ""
			},
			want:     "target owner must not be empty",
			families: []string{"pkg/transports"},
		},
		{
			name: "unapproved target owner",
			mutate: func(exception *migrationPackageException) {
				exception.targetOwner = "pkg/experimental"
			},
			want:     "target owner pkg/experimental must be an approved or documented migration package family",
			families: []string{"pkg/transports"},
		},
		{
			name: "missing work item",
			mutate: func(exception *migrationPackageException) {
				exception.workItem = ""
			},
			want:     "must name a recognized active work item",
			families: []string{"pkg/transports"},
		},
		{
			name: "inactive work item",
			mutate: func(exception *migrationPackageException) {
				exception.workItem = "Batch 005 — Retired move"
			},
			want:     "must name a recognized active work item",
			families: []string{"pkg/transports"},
		},
		{
			name: "work item targets another owner",
			mutate: func(exception *migrationPackageException) {
				exception.workItem = batch006PlatformFamilyMove
			},
			want:     "targets pkg/platform, not pkg/transports",
			families: []string{"pkg/transports", "pkg/platform"},
		},
		{
			name: "missing deletion gate",
			mutate: func(exception *migrationPackageException) {
				exception.deletionGate = " "
			},
			want:     "deletion gate must not be empty",
			families: []string{"pkg/transports"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			exception := valid
			tt.mutate(&exception)
			policy := boundaryPolicy{
				approvedProductPackageFamilies: tt.families,
				migrationPackageExceptions:     []migrationPackageException{exception},
			}

			err := validatePolicy(policy)
			if err == nil {
				t.Fatal("validatePolicy() error = nil, want invalid migration metadata rejection")
			}
			if got := err.Error(); !strings.Contains(got, tt.want) {
				t.Fatalf("validatePolicy() error = %q, want substring %q", got, tt.want)
			}
		})
	}
}

func TestValidatePolicyRejectsMigrationExceptionAsProductFamily(t *testing.T) {
	t.Parallel()

	policy := boundaryPolicy{
		approvedProductPackageFamilies: []string{"pkg/transports", "pkg/legacytransport"},
		migrationPackageExceptions: []migrationPackageException{{
			packagePath:  "pkg/legacytransport",
			targetOwner:  "pkg/transports",
			workItem:     batch006TransportFamilyMove,
			deletionGate: "remove after callers move to pkg/transports",
		}},
	}

	err := validatePolicy(policy)
	if err == nil {
		t.Fatal("validatePolicy() error = nil, want migration/product-family overlap rejection")
	}
	if got := err.Error(); got != "migration-only package exception pkg/legacytransport must not also be an approved product package family" {
		t.Fatalf("validatePolicy() error = %q, want overlap diagnostic", got)
	}
}

func TestRunAllowsDocumentedGeneratedCodeExceptions(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGeneratedGoFile(t, repoRoot, "pkg/transports/http/client/client.gen.go")
	writeGeneratedGoFile(t, repoRoot, "pkg/transports/http/generated/server.gen.go")

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, stdout, stderr)
	if err != nil {
		t.Fatalf("run() error = %v, want nil", err)
	}
	for _, want := range []string{
		"package boundary passed",
		"active generated-code exceptions: pkg/transports/http/client (root), pkg/transports/http/generated (root)",
	} {
		if got := stdout.String(); !strings.Contains(got, want) {
			t.Fatalf("run() stdout = %q, want substring %q", got, want)
		}
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("run() stderr = %q, want empty", got)
	}
}

func TestRunRejectsHandwrittenGoInGeneratedOnlyPackage(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoImportFile(t, repoRoot, "pkg/transports/http/client/compatibility.go", "generatedclient", "context")

	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want handwritten generated-package failure")
	}
	for _, want := range []string{
		"handwritten Go file in generated-only package: pkg/transports/http/client (pkg/transports/http/client/compatibility.go)",
		"standard Code generated ... DO NOT EDIT. marker",
		"move handwritten mapping or policy to pkg/transports/http or pkg/transports/mapping",
	} {
		if got := stderr.String(); !strings.Contains(got, want) {
			t.Fatalf("run() stderr = %q, want substring %q", got, want)
		}
	}
}

func TestRunRejectsGeneratedLookingRootOutsideDocumentedExceptions(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGeneratedGoFile(t, repoRoot, "pkg/transports/http/client/client.gen.go")
	writeGeneratedGoFile(t, repoRoot, "pkg/generatedexperimental/client.gen.go")

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, stdout, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want unapproved generated-looking root failure")
	}
	if got := stdout.String(); got != "" {
		t.Fatalf("run() stdout = %q, want empty", got)
	}

	got := stderr.String()
	for _, want := range []string{
		"[agent-factory:pkg-boundary] unapproved root package family: pkg/generatedexperimental",
		"outside the approved package-family allowlist",
		"active generated-code exceptions: pkg/transports/http/client (root), pkg/transports/http/generated (root)",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("run() stderr = %q, want substring %q", got, want)
		}
	}
	if strings.Contains(got, "unapproved root package family: pkg/transports/http/client") {
		t.Fatalf("run() stderr = %q, documented generated-code root should not be rejected", got)
	}
}

func TestValidatePolicyRejectsGeneratedExceptionAsProductFamily(t *testing.T) {
	t.Parallel()

	policy := boundaryPolicy{
		approvedProductPackageFamilies: []string{"pkg/transports/http/client"},
		generatedCodeExceptions: []generatedCodeException{
			{packagePath: "pkg/transports/http/client", scope: generatedCodeExceptionScopeRoot},
		},
	}

	err := validatePolicy(policy)
	if err == nil {
		t.Fatal("validatePolicy() error = nil, want generated-code/product-family overlap rejection")
	}
	if got := err.Error(); got != "generated-code exception pkg/transports/http/client must not also be an approved product package family" {
		t.Fatalf("validatePolicy() error = %q, want overlap diagnostic", got)
	}
}

func TestValidatePolicyRejectsGeneratedExceptionAsMigrationException(t *testing.T) {
	t.Parallel()

	policy := boundaryPolicy{
		approvedProductPackageFamilies: []string{"pkg/transports"},
		migrationPackageExceptions: []migrationPackageException{{
			packagePath:  "pkg/transports/http/client",
			targetOwner:  "pkg/transports",
			workItem:     batch006TransportFamilyMove,
			deletionGate: "remove after generated clients move to pkg/transports",
		}},
		generatedCodeExceptions: []generatedCodeException{
			{packagePath: "pkg/transports/http/client", scope: generatedCodeExceptionScopeRoot},
		},
	}

	err := validatePolicy(policy)
	if err == nil {
		t.Fatal("validatePolicy() error = nil, want generated-code/migration overlap rejection")
	}
	if got := err.Error(); got != "generated-code exception pkg/transports/http/client must not also be a migration-only package exception" {
		t.Fatalf("validatePolicy() error = %q, want overlap diagnostic", got)
	}
}

func TestRunFailsForUnapprovedRootPackageFamily(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	makeDir(t, repoRoot, "pkg/config")
	makeDir(t, repoRoot, "pkg/experimental")

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, stdout, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want package-boundary violation")
	}
	if got := stdout.String(); got != "" {
		t.Fatalf("run() stdout = %q, want empty", got)
	}

	wantOutput := strings.Join([]string{
		"[agent-factory:pkg-boundary] unapproved root package family: pkg/experimental",
		"  reason: pkg/experimental is outside the approved package-family allowlist.",
		"  remediation: move the code under an approved owner or deliberately update the allowlist with ownership rationale.",
		"[agent-factory:pkg-boundary] active generated-code exceptions: pkg/transports/http/client (root), pkg/transports/http/generated (root)",
		"",
	}, "\n")
	if got := stderr.String(); got != wantOutput {
		t.Fatalf("run() stderr = %q, want diagnostic %q", got, wantOutput)
	}
	if got := err.Error(); got != "[agent-factory:pkg-boundary] found 1 package-boundary violation(s)" {
		t.Fatalf("run() error = %q, want violation count", got)
	}
}

func TestRunReportsMultipleUnapprovedRootPackagesDeterministically(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	makeDir(t, repoRoot, "pkg/zeta")
	makeDir(t, repoRoot, "pkg/experimental")
	makeDir(t, repoRoot, "pkg/alpha")

	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want package-boundary violations")
	}

	errOutput := stderr.String()
	alphaIndex := strings.Index(errOutput, "[agent-factory:pkg-boundary] unapproved root package family: pkg/alpha")
	experimentalIndex := strings.Index(errOutput, "[agent-factory:pkg-boundary] unapproved root package family: pkg/experimental")
	zetaIndex := strings.Index(errOutput, "[agent-factory:pkg-boundary] unapproved root package family: pkg/zeta")
	if alphaIndex < 0 || experimentalIndex < 0 || zetaIndex < 0 {
		t.Fatalf("run() stderr = %q, want all unapproved roots reported", errOutput)
	}
	if !(alphaIndex < experimentalIndex && experimentalIndex < zetaIndex) {
		t.Fatalf("run() stderr = %q, want package roots reported in path order", errOutput)
	}
	if got := strings.Count(errOutput, "outside the approved package-family allowlist"); got != 3 {
		t.Fatalf("run() stderr = %q, want remediation details for each violation", errOutput)
	}
	if got := err.Error(); got != "[agent-factory:pkg-boundary] found 3 package-boundary violation(s)" {
		t.Fatalf("run() error = %q, want three violation count", got)
	}
}

func TestRunRejectsRecreatedRetiredPackageRootsWithCanonicalOwners(t *testing.T) {
	t.Parallel()

	for _, owner := range factoryRetiredPackageRoots {
		owner := owner
		t.Run(owner.packagePath, func(t *testing.T) {
			t.Parallel()

			repoRoot := t.TempDir()
			makeDir(t, repoRoot, owner.packagePath)

			stderr := &bytes.Buffer{}
			err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
			if err == nil {
				t.Fatal("run() error = nil, want retired package root failure")
			}
			for _, want := range []string{
				"prohibited retired package root: " + owner.packagePath,
				"canonical owner: " + owner.canonicalOwner,
				"move the code to " + owner.canonicalOwner + " and delete the retired root",
			} {
				if got := stderr.String(); !strings.Contains(got, want) {
					t.Fatalf("run() stderr = %q, want substring %q", got, want)
				}
			}
			if got := stderr.String(); strings.Contains(got, "unapproved root package family: "+owner.packagePath) {
				t.Fatalf("run() stderr = %q, want actionable retired-owner diagnostic only", got)
			}
		})
	}
}

func TestRunRejectsRetiredFactoryPackageImportsWithCanonicalOwners(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	for index, owner := range factoryRetiredPackageRoots {
		writeGoImportFile(
			t,
			repoRoot,
			fmt.Sprintf("pkg/config/retired_import_%d.go", index),
			"config",
			repositoryImportPrefix+owner.packagePath+"/legacy",
		)
	}

	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want retired package import failures")
	}
	for _, owner := range factoryRetiredPackageRoots {
		for _, want := range []string{
			"prohibited retired package import: " + repositoryImportPrefix + owner.packagePath + "/legacy",
			"canonical owner: " + owner.canonicalOwner,
			"import " + owner.canonicalOwner + " directly; do not recreate or depend on " + owner.packagePath,
		} {
			if got := stderr.String(); !strings.Contains(got, want) {
				t.Fatalf("run() stderr = %q, want substring %q", got, want)
			}
		}
	}
	if got := err.Error(); got != fmt.Sprintf("[agent-factory:pkg-boundary] found %d package-boundary violation(s)", len(factoryRetiredPackageRoots)) {
		t.Fatalf("run() error = %q, want one violation per retired import", got)
	}
}

func TestRunAcceptsCanonicalConvergedPackageImports(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	for index, owner := range factoryRetiredPackageRoots {
		canonicalOwner := strings.Split(owner.canonicalOwner, ",")[0]
		canonicalOwner = strings.Split(canonicalOwner, " or ")[0]
		consumerPath := fmt.Sprintf("pkg/config/canonical_import_%d.go", index)
		consumerPackage := "config"
		if serviceOwner, isServiceSubpackage := serviceSubpackageOwner(canonicalOwner); isServiceSubpackage {
			consumerPath = fmt.Sprintf("pkg/services/%s/canonical_import_%d.go", serviceOwner, index)
			consumerPackage = strings.ReplaceAll(serviceOwner, "_", "")
		}
		writeGoImportFile(
			t,
			repoRoot,
			consumerPath,
			consumerPackage,
			repositoryImportPrefix+canonicalOwner,
		)
	}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, stdout, stderr)
	if err != nil {
		t.Fatalf("run() error = %v, want canonical owner imports accepted", err)
	}
	if got := stdout.String(); !strings.Contains(got, "package boundary passed") {
		t.Fatalf("run() stdout = %q, want package-boundary success", got)
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("run() stderr = %q, want empty", got)
	}
}

func TestRunBlocksMigrationShimPatternEvenWhenRootFamilyApproved(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	makeDir(t, repoRoot, "pkg/config")
	writeMigrationShimCompatFile(t, repoRoot, "pkg/workflowpreview", "github.com/portpowered/infinite-you/pkg/services/factory_runtime")

	policy := defaultBoundaryPolicy()
	policy.approvedProductPackageFamilies = append(policy.approvedProductPackageFamilies, "pkg/workflowpreview")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := runWithPolicy(config{root: repoRoot, packageRoot: defaultScanRoot}, policy, stdout, stderr)
	if err == nil {
		t.Fatal("runWithPolicy() error = nil, want migration-shim blocking failure")
	}
	if got := stdout.String(); got != "" {
		t.Fatalf("runWithPolicy() stdout = %q, want empty", got)
	}

	got := stderr.String()
	for _, want := range []string{
		"[agent-factory:pkg-boundary] blocked migration-only compatibility shim: pkg/workflowpreview",
		"marker: Batch 001 compatibility shim",
		"canonical target: github.com/portpowered/infinite-you/pkg/services/factory_runtime",
		"remediation: import the canonical owner directly and do not recreate Batch 001 root compatibility shims.",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("runWithPolicy() stderr = %q, want substring %q", got, want)
		}
	}
	if strings.Contains(got, "unapproved root package family: pkg/workflowpreview") {
		t.Fatalf("runWithPolicy() stderr = %q, migration-shim fixture should fail without root-family diagnostic", got)
	}
	if got := err.Error(); got != "[agent-factory:pkg-boundary] found 1 package-boundary violation(s)" {
		t.Fatalf("runWithPolicy() error = %q, want migration-shim violation count", got)
	}
}

func TestRunReportsMigrationShimBlockingAlongsideRootViolations(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	makeDir(t, repoRoot, "pkg/experimental")
	writeMigrationShimCompatFile(t, repoRoot, "pkg/workflowsource", "github.com/portpowered/infinite-you/pkg/services/factory_runtime")

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, stdout, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want unapproved root failure")
	}
	if got := stdout.String(); got != "" {
		t.Fatalf("run() stdout = %q, want empty", got)
	}

	got := stderr.String()
	for _, want := range []string{
		"[agent-factory:pkg-boundary] unapproved root package family: pkg/experimental",
		"[agent-factory:pkg-boundary] blocked migration-only compatibility shim: pkg/workflowsource",
		"marker: Batch 001 compatibility shim",
		"canonical target: github.com/portpowered/infinite-you/pkg/services/factory_runtime",
		"remediation: import the canonical owner directly and do not recreate Batch 001 root compatibility shims.",
		"[agent-factory:pkg-boundary] found 3 package-boundary violation(s)",
	} {
		if !strings.Contains(got, want) && !strings.Contains(err.Error(), want) {
			t.Fatalf("run() diagnostics = stdout:%q stderr:%q err:%q, want substring %q", stdout.String(), got, err.Error(), want)
		}
	}
}

func TestRunRejectsEmptyPackageRoot(t *testing.T) {
	t.Parallel()

	err := run(config{root: t.TempDir()}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || err.Error() != "package root must not be empty" {
		t.Fatalf("run() error = %v, want package root validation", err)
	}
}

func TestMakePkgBoundaryTargetFailsForUnapprovedRootPackageFamily(t *testing.T) {
	repoRoot := filepath.Clean(filepath.Join("..", ".."))
	fixtureRoot := t.TempDir()
	makeDir(t, fixtureRoot, "pkg/experimental")

	cmd := exec.Command("make", "pkg-boundary", "PACKAGE_BOUNDARY_ROOT="+fixtureRoot)
	cmd.Dir = repoRoot
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("make pkg-boundary succeeded, want unapproved root failure; output:\n%s", output)
	}

	got := string(output)
	for _, want := range []string{
		"go run ./cmd/pkgboundarycheck -root " + fixtureRoot,
		"[agent-factory:pkg-boundary] unapproved root package family: pkg/experimental",
		"outside the approved package-family allowlist",
		"move the code under an approved owner or deliberately update the allowlist with ownership rationale",
		"[agent-factory:pkg-boundary] found 1 package-boundary violation(s)",
		"pkg-boundary] Error",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("make pkg-boundary output = %q, want substring %q", got, want)
		}
	}
}

func TestMakePkgBoundaryTargetFailsForDomainApplicationGraphImport(t *testing.T) {
	repoRoot := filepath.Clean(filepath.Join("..", ".."))
	fixtureRoot := t.TempDir()
	writeGoImportFile(t, fixtureRoot, "pkg/services/work/query/composition.go", "query", applicationGraphImportPath)

	cmd := exec.Command("make", "pkg-boundary", "PACKAGE_BOUNDARY_ROOT="+fixtureRoot)
	cmd.Dir = repoRoot
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("make pkg-boundary succeeded, want domain composition import failure; output:\n%s", output)
	}

	got := string(output)
	for _, want := range []string{
		"prohibited application composition import: pkg/services/work/query (pkg/services/work/query/composition.go)",
		"pkg/wire is the outward application composition root",
		"inject the collaborator through pkg/root or pkg/initializer",
		"[agent-factory:pkg-boundary] found 1 package-boundary violation(s)",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("make pkg-boundary output = %q, want substring %q", got, want)
		}
	}
}

func TestMakeLintPathFailsForUnapprovedRootPackageFamily(t *testing.T) {
	repoRoot := filepath.Clean(filepath.Join("..", ".."))
	fixtureRoot := t.TempDir()
	makeDir(t, fixtureRoot, "pkg/experimental")

	cmd := exec.Command("make", "lint", "LINT_TARGETS=pkg-boundary", "PACKAGE_BOUNDARY_ROOT="+fixtureRoot)
	cmd.Dir = repoRoot
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("make lint succeeded, want unapproved root failure through lint path; output:\n%s", output)
	}

	got := string(output)
	for _, want := range []string{
		"go run ./cmd/pkgboundarycheck -root " + fixtureRoot,
		"[agent-factory:pkg-boundary] unapproved root package family: pkg/experimental",
		"outside the approved package-family allowlist",
		"move the code under an approved owner or deliberately update the allowlist with ownership rationale",
		"[agent-factory:pkg-boundary] found 1 package-boundary violation(s)",
		"pkg-boundary] Error",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("make lint output = %q, want substring %q", got, want)
		}
	}
}

func writeMigrationShimCompatFile(t *testing.T, repoRoot string, packagePath string, canonicalTarget string) {
	t.Helper()

	packageName := filepath.Base(filepath.FromSlash(packagePath))
	absolutePath := filepath.Join(repoRoot, filepath.FromSlash(packagePath), "compat.go")
	if err := os.MkdirAll(filepath.Dir(absolutePath), 0o755); err != nil {
		t.Fatalf("create parent directory for %s: %v", packagePath, err)
	}
	content := fmt.Sprintf(`// Deprecated: use %s instead.
// This package is a Batch 001 compatibility shim; core runtime and API code must import the orchestrator-owned path directly.
// backendsizecheck:ignore-file service-ownership migration preserves this consolidated surface until a dedicated responsibility split removes the exemption.
package %s

import target "%s"

type Request = target.Request
`, canonicalTarget, packageName, canonicalTarget)
	if err := os.WriteFile(absolutePath, []byte(content), 0o644); err != nil {
		t.Fatalf("write migration shim fixture %s: %v", packagePath, err)
	}
}

func makeDir(t *testing.T, repoRoot string, relativePath string) {
	t.Helper()

	absolutePath := filepath.Join(repoRoot, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(absolutePath, 0o755); err != nil {
		t.Fatalf("create directory %s: %v", relativePath, err)
	}
}

func writeGeneratedGoFile(t *testing.T, repoRoot string, relativePath string) {
	t.Helper()

	absolutePath := filepath.Join(repoRoot, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(absolutePath), 0o755); err != nil {
		t.Fatalf("create parent directory for %s: %v", relativePath, err)
	}
	content := []byte("// Code generated by package-boundary test. DO NOT EDIT.\n\npackage generated\n")
	if err := os.WriteFile(absolutePath, content, 0o644); err != nil {
		t.Fatalf("write generated file %s: %v", relativePath, err)
	}
}

func writeGoImportFile(t *testing.T, repoRoot string, relativePath string, packageName string, importPath string) {
	t.Helper()

	absolutePath := filepath.Join(repoRoot, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(absolutePath), 0o755); err != nil {
		t.Fatalf("create parent directory for %s: %v", relativePath, err)
	}
	content := fmt.Sprintf("package %s\n\nimport _ %q\n", packageName, importPath)
	if err := os.WriteFile(absolutePath, []byte(content), 0o644); err != nil {
		t.Fatalf("write import fixture %s: %v", relativePath, err)
	}
}

func writeGoSourceFile(t *testing.T, repoRoot string, relativePath string, content string) {
	t.Helper()

	absolutePath := filepath.Join(repoRoot, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(absolutePath), 0o755); err != nil {
		t.Fatalf("create parent directory for %s: %v", relativePath, err)
	}
	if err := os.WriteFile(absolutePath, []byte(content), 0o644); err != nil {
		t.Fatalf("write Go fixture %s: %v", relativePath, err)
	}
}

func TestServiceOwnedTransportClassification(t *testing.T) {
	tests := []struct {
		name       string
		consumer   string
		importPath string
		want       bool
	}{
		{
			name:       "matching HTTP composer",
			consumer:   "pkg/transports/http/server.go",
			importPath: "pkg/services/models/transports/http",
			want:       true,
		},
		{
			name:       "matching CLI composer test",
			consumer:   "pkg/transports/cli/root_test.go",
			importPath: "pkg/services/models/transports/cli",
			want:       true,
		},
		{
			name:       "different protocol",
			consumer:   "pkg/transports/http/server.go",
			importPath: "pkg/services/models/transports/cli",
			want:       false,
		},
		{
			name:       "ordinary service implementation",
			consumer:   "pkg/transports/http/server.go",
			importPath: "pkg/services/models/service",
			want:       false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isMatchingServiceOwnedTransportConsumer(tt.consumer, tt.importPath); got != tt.want {
				t.Fatalf("classification = %t, want %t", got, tt.want)
			}
		})
	}
}
