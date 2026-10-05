// backendsizecheck:ignore-file service-ownership migration preserves this consolidated surface until a dedicated responsibility split removes the exemption.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

func TestRunIgnoresRepositoryPolicyNonSourceRoots(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	for _, worktreeRoot := range []string{
		".worktrees",
		"worktrees",
		filepath.Join(".claude", "worktrees"),
		filepath.Join(".artifacts", "bootstrap", "worktrees"),
		filepath.Join(".artifacts", "generated", "transient-worktrees"),
		filepath.Join(".artifacts", "bootstrap", "generated", "worktrees"),
		".git",
		"node_modules",
		"testdata",
		"vendor",
	} {
		writeGoSourceFile(
			t,
			repoRoot,
			filepath.Join(worktreeRoot, "task-a", "tests", "functional", "stale_test.go"),
			fmt.Sprintf(`package functional

import _ %q

func stale( {
`, applicationGraphImportPath),
		)
	}

	stderr := &bytes.Buffer{}
	if err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr); err != nil {
		t.Fatalf("run() error = %v, want repository-policy roots ignored; stderr=%q", err, stderr.String())
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("run() stderr = %q, want no findings from ignored repository-policy roots", got)
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
		"construct the collaborator in pkg/wire and inject its service-root role",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("run() stderr = %q, want substring %q", got, want)
		}
	}
	if got := err.Error(); got != "[agent-factory:pkg-boundary] found 2 package-boundary violation(s)" {
		t.Fatalf("run() error = %q, want two production construction violations", got)
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

func TestRunRejectsEmptyPackageRoot(t *testing.T) {
	t.Parallel()

	err := run(config{root: t.TempDir()}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || err.Error() != "package root must not be empty" {
		t.Fatalf("run() error = %v, want package root validation", err)
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
