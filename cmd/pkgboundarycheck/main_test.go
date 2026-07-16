package main

import (
	"bytes"
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
		"pkg/work",
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

func TestRunAllowsOnlyStartupOwnersToImportApplicationGraph(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoImportFile(t, repoRoot, "pkg/wire/graph.go", "wire", "github.com/portpowered/infinite-you/pkg/factory/contracts")
	writeGoImportFile(t, repoRoot, "pkg/root/root.go", "root", applicationGraphImportPath)
	writeGoImportFile(t, repoRoot, "pkg/initializer/core.go", "initializer", applicationGraphImportPath)

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

func TestRunAllowsPlatformObservabilityAndRejectsRetiredImports(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoImportFile(t, repoRoot, "pkg/factory/runtime/canonical.go", "runtime", "github.com/portpowered/infinite-you/pkg/platform/logging")
	writeGoImportFile(t, repoRoot, "pkg/factory/runtime/metrics.go", "runtime", "github.com/portpowered/infinite-you/pkg/factory/metrics")
	writeGoImportFile(t, repoRoot, "pkg/wire/metrics.go", "wire", "github.com/portpowered/infinite-you/pkg/platform/metrics")

	stderr := &bytes.Buffer{}
	if err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr); err != nil {
		t.Fatalf("run() error = %v, want canonical platform logging import allowed; stderr=%q", err, stderr.String())
	}

	writeGoImportFile(t, repoRoot, "pkg/factory/runtime/retired.go", "runtime", "github.com/portpowered/infinite-you/pkg/logging")
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

	writeGoImportFile(t, repoRoot, "pkg/factory/runtime/retired_metrics.go", "runtime", "github.com/portpowered/infinite-you/pkg/internal/metrics")
	stderr.Reset()
	err = run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want retired metrics import rejected")
	}
	for _, want := range []string{
		"prohibited retired package import: github.com/portpowered/infinite-you/pkg/internal/metrics",
		"canonical owner: pkg/factory/metrics for domain contracts and pkg/platform/metrics for file-backed recording",
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
		{"pkg/factory/runtime/api.go", "github.com/portpowered/infinite-you/pkg/api", "pkg/transports/http"},
		{"pkg/factory/runtime/mapping.go", "github.com/portpowered/infinite-you/pkg/apisurface/factorysession", "pkg/transports/mapping"},
		{"pkg/root/cli.go", "github.com/portpowered/infinite-you/pkg/cli", "pkg/transports/cli"},
		{"pkg/root/mcp.go", "github.com/portpowered/infinite-you/pkg/mcp/server", "pkg/transports/mcp"},
		{"pkg/factory/runtime/client.go", "github.com/portpowered/infinite-you/pkg/generatedclient", "pkg/transports/http/client"},
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
	writeGoImportFile(t, repoRoot, "pkg/factory/runtime/composition.go", "runtime", applicationGraphImportPath)

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
		"[agent-factory:pkg-boundary] prohibited application composition import: pkg/factory/runtime (pkg/factory/runtime/composition.go)",
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
		{packagePath: "pkg/modelhost", canonicalOwner: "pkg/models/host"},
		{packagePath: "pkg/localmodels", canonicalOwner: "pkg/models/local or pkg/models/assets"},
		{packagePath: "pkg/hostedworkers", canonicalOwner: "pkg/workers/hosted"},
		{packagePath: "pkg/invocations", canonicalOwner: "pkg/work/invocation, pkg/factory/sessions/invocation, pkg/workers/inference, or pkg/workers/skippermissions, according to the concern"},
		{packagePath: "pkg/materialize", canonicalOwner: "pkg/work/materialize"},
		{packagePath: "pkg/timework", canonicalOwner: "pkg/work/timework"},
		{packagePath: "pkg/workcontent", canonicalOwner: "pkg/work/content"},
		{packagePath: "pkg/workgraph", canonicalOwner: "pkg/work/graph"},
		{packagePath: "pkg/workquery", canonicalOwner: "pkg/work/query"},
		{packagePath: "pkg/interfaces", canonicalOwner: "the defining domain under pkg/factory, pkg/work, pkg/workers, or pkg/models"},
		{packagePath: "pkg/replay", canonicalOwner: "pkg/factory/replay for Factory-event replay policy and pkg/platform/replay for artifact filesystem mechanics"},
		{packagePath: "pkg/testutil", canonicalOwner: "internal/testutil or package-local test helpers"},
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
			importPath:     "github.com/portpowered/infinite-you/pkg/modelhost",
			retiredRoot:    "pkg/modelhost",
			canonicalOwner: "pkg/models/host",
		},
		{
			importPath:     "github.com/portpowered/infinite-you/pkg/localmodels/assets",
			retiredRoot:    "pkg/localmodels",
			canonicalOwner: "pkg/models/local or pkg/models/assets",
		},
		{
			importPath:     "github.com/portpowered/infinite-you/pkg/hostedworkers/linear",
			retiredRoot:    "pkg/hostedworkers",
			canonicalOwner: "pkg/workers/hosted",
		},
		{
			importPath:     "github.com/portpowered/infinite-you/pkg/invocations/inference",
			retiredRoot:    "pkg/invocations",
			canonicalOwner: "pkg/work/invocation, pkg/factory/sessions/invocation, pkg/workers/inference, or pkg/workers/skippermissions, according to the concern",
		},
		{
			importPath:     "github.com/portpowered/infinite-you/pkg/materialize",
			retiredRoot:    "pkg/materialize",
			canonicalOwner: "pkg/work/materialize",
		},
		{
			importPath:     "github.com/portpowered/infinite-you/pkg/timework",
			retiredRoot:    "pkg/timework",
			canonicalOwner: "pkg/work/timework",
		},
		{
			importPath:     "github.com/portpowered/infinite-you/pkg/workcontent",
			retiredRoot:    "pkg/workcontent",
			canonicalOwner: "pkg/work/content",
		},
		{
			importPath:     "github.com/portpowered/infinite-you/pkg/workgraph",
			retiredRoot:    "pkg/workgraph",
			canonicalOwner: "pkg/work/graph",
		},
		{
			importPath:     "github.com/portpowered/infinite-you/pkg/workquery",
			retiredRoot:    "pkg/workquery",
			canonicalOwner: "pkg/work/query",
		},
		{
			importPath:     "github.com/portpowered/infinite-you/pkg/interfaces",
			retiredRoot:    "pkg/interfaces",
			canonicalOwner: "the defining domain under pkg/factory, pkg/work, pkg/workers, or pkg/models",
		},
		{
			importPath:     "github.com/portpowered/infinite-you/pkg/replay",
			retiredRoot:    "pkg/replay",
			canonicalOwner: "pkg/factory/replay for Factory-event replay policy and pkg/platform/replay for artifact filesystem mechanics",
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
			writeGoImportFile(t, repoRoot, "pkg/factory/retired_import.go", "factory", tt.importPath)

			stderr := &bytes.Buffer{}
			err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
			if err == nil {
				t.Fatal("run() error = nil, want retired package import failure")
			}

			got := stderr.String()
			for _, want := range []string{
				"prohibited retired package import: " + tt.importPath + " (pkg/factory/retired_import.go)",
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

func TestRunAllowsCanonicalModelAndWorkerSubpackages(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoImportFile(t, repoRoot, "pkg/factory/canonical_imports.go", "factory", "github.com/portpowered/infinite-you/pkg/models/host")
	writeGoImportFile(t, repoRoot, "pkg/models/host/host.go", "modelhost", "github.com/portpowered/infinite-you/pkg/models/local")
	writeGoImportFile(t, repoRoot, "pkg/workers/service/hosted.go", "service", "github.com/portpowered/infinite-you/pkg/workers/hosted")

	stderr := &bytes.Buffer{}
	if err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr); err != nil {
		t.Fatalf("run() error = %v, want canonical nested packages allowed; stderr=%q", err, stderr.String())
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
			want:     "target owner pkg/experimental must be an approved product package family",
			families: []string{"pkg/transports"},
		},
		{
			name: "missing work item",
			mutate: func(exception *migrationPackageException) {
				exception.workItem = ""
			},
			want:     "must name an active Batch 006, Batch 007, or Batch 008 work item",
			families: []string{"pkg/transports"},
		},
		{
			name: "inactive work item",
			mutate: func(exception *migrationPackageException) {
				exception.workItem = "Batch 005 — Retired move"
			},
			want:     "must name an active Batch 006, Batch 007, or Batch 008 work item",
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
	makeDir(t, repoRoot, "pkg/service")
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
			fmt.Sprintf("pkg/service/retired_import_%d.go", index),
			"service",
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
		writeGoImportFile(
			t,
			repoRoot,
			fmt.Sprintf("pkg/service/canonical_import_%d.go", index),
			"service",
			repositoryImportPrefix+owner.canonicalOwner,
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
	makeDir(t, repoRoot, "pkg/service")
	writeMigrationShimCompatFile(t, repoRoot, "pkg/workflowpreview", "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/preview")

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
		"canonical target: github.com/portpowered/infinite-you/pkg/orchestrators/javascript/preview",
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
	writeMigrationShimCompatFile(t, repoRoot, "pkg/workflowsource", "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source")

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
		"canonical target: github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source",
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
	writeGoImportFile(t, fixtureRoot, "pkg/work/query/composition.go", "query", applicationGraphImportPath)

	cmd := exec.Command("make", "pkg-boundary", "PACKAGE_BOUNDARY_ROOT="+fixtureRoot)
	cmd.Dir = repoRoot
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("make pkg-boundary succeeded, want domain composition import failure; output:\n%s", output)
	}

	got := string(output)
	for _, want := range []string{
		"prohibited application composition import: pkg/work/query (pkg/work/query/composition.go)",
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
