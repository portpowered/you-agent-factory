package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunRejectsReusableSupportImplementationImportAndAllowsRootAndEdgeContracts(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	makeDir(t, repoRoot, "pkg/root")
	writeGoSourceFile(t, repoRoot, "internal/testutil/hidden_runtime.go", `package testutil

import execution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution"

var OpenRuntime = execution.New
`)
	writeGoSourceFile(t, repoRoot, "internal/testutil/root_fake.go", `package testutil

import sessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"

var SessionContract sessions.Service
`)
	writeGoSourceFile(t, repoRoot, "internal/testutil/edge_fake.go", `package testutil

import inference "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"

var ProviderEdge inference.Provider
`)

	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want reusable support implementation import rejected")
	}
	got := stderr.String()
	for _, want := range []string{
		"prohibited reusable support service implementation import: github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution (internal/testutil/hidden_runtime.go)",
		"repository-wide reusable test support is not an application composition root",
		"use the owning service root contract, a typed edge fake, package-local owner coverage, or root.BuildProcess",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("run() stderr = %q, want substring %q", got, want)
		}
	}
	if strings.Contains(got, "root_fake.go") || strings.Contains(got, "edge_fake.go") {
		t.Fatalf("run() stderr = %q, root and edge contracts must remain allowed", got)
	}
}

func TestRunRejectsFunctionalReusableSupportImplementationImport(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	makeDir(t, repoRoot, "pkg/root")
	writeGoSourceFile(t, repoRoot, "tests/functional/internal/support/hidden_runtime.go", `package support

import execution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution"

var OpenRuntime = execution.New
`)

	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want functional reusable support implementation import rejected")
	}
	if got := stderr.String(); !strings.Contains(
		got,
		"prohibited reusable support service implementation import: github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution (tests/functional/internal/support/hidden_runtime.go)",
	) {
		t.Fatalf("run() stderr = %q, want functional reusable-support diagnostic", got)
	}
}

func TestRunRequiresExactDeletionOnlyReusableSupportBaseline(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	makeDir(t, repoRoot, "pkg/root")
	const filePath = "internal/testutil/runtime.go"
	const importPath = "github.com/portpowered/infinite-you/pkg/services/factory_runtime/service"
	writeGoImportFile(t, repoRoot, filePath, "testutil", importPath)
	writeSupportServiceBaseline(t, repoRoot, supportServiceImportBaseline{
		Version: 1,
		Entries: []supportServiceImportBaselineEntry{{
			Owner:        "factory_runtime",
			ImportPath:   importPath,
			FilePath:     filePath,
			TargetRoot:   "pkg/services/factory_runtime",
			Stage:        supportServiceImportBaselineStage,
			DeletionGate: supportServiceImportDeletionGate,
		}},
	})

	if err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatalf("run() error = %v, want exact reusable-support baseline accepted", err)
	}

	writeGoImportFile(
		t,
		repoRoot,
		filePath,
		"testutil",
		"github.com/portpowered/infinite-you/pkg/services/factory_runtime",
	)
	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want stale reusable-support baseline rejected")
	}
	if got := stderr.String(); !strings.Contains(
		got,
		"stale reusable support service import baseline entry: "+filePath+" -> "+importPath,
	) {
		t.Fatalf("run() stderr = %q, want stale reusable-support baseline diagnostic", got)
	}
}

func TestRunRequiresExactDeletionOnlyBaselineForReusableSupportWireImport(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	makeDir(t, repoRoot, "pkg/root")
	const (
		filePath   = "internal/configcontractsmoke/family.go"
		importPath = "github.com/portpowered/infinite-you/pkg/wire/factorydefinitions"
	)
	writeGoImportFile(t, repoRoot, filePath, "configcontractsmoke", importPath)

	stderr := &bytes.Buffer{}
	if err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr); err == nil {
		t.Fatal("run() error = nil, want reusable-support Wire import rejected")
	}
	if got := stderr.String(); !strings.Contains(got, "prohibited reusable support service implementation import: "+importPath) {
		t.Fatalf("run() stderr = %q, want Wire import diagnostic", got)
	}

	writeSupportServiceBaseline(t, repoRoot, supportServiceImportBaseline{
		Version: 1,
		Entries: []supportServiceImportBaselineEntry{{
			Owner:        "wire",
			ImportPath:   importPath,
			FilePath:     filePath,
			TargetRoot:   "pkg/wire",
			Stage:        supportServiceImportBaselineStage,
			DeletionGate: supportServiceImportDeletionGate,
		}},
	})
	if err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatalf("run() error = %v, want exact Wire-support baseline accepted", err)
	}
}

func TestReusableSupportScanDoesNotConflateTestFiles(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoImportFile(
		t,
		repoRoot,
		"internal/testutil/runtime_test.go",
		"testutil",
		"github.com/portpowered/infinite-you/pkg/services/factory_runtime/service",
	)
	findings, err := scanSupportServiceSubpackageImports(repoRoot)
	if err != nil {
		t.Fatalf("scanSupportServiceSubpackageImports() error = %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("reusable support findings = %#v, want _test.go handled only by test baseline", findings)
	}
}

func TestCreateReusableSupportBaselineRefusesOverwrite(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoImportFile(
		t,
		repoRoot,
		"internal/testutil/runtime.go",
		"testutil",
		"github.com/portpowered/infinite-you/pkg/services/factory_runtime/service",
	)
	if err := createSupportServiceImportBaseline(config{root: repoRoot}); err != nil {
		t.Fatalf("createSupportServiceImportBaseline() error = %v", err)
	}
	baseline, err := loadSupportServiceImportBaseline(repoRoot)
	if err != nil {
		t.Fatalf("loadSupportServiceImportBaseline() error = %v", err)
	}
	if len(baseline.Entries) != 1 {
		t.Fatalf("baseline entries = %#v, want one exact edge", baseline.Entries)
	}
	if err := createSupportServiceImportBaseline(config{root: repoRoot}); err == nil ||
		!strings.Contains(err.Error(), "refusing to overwrite") {
		t.Fatalf("second create error = %v, want overwrite refusal", err)
	}
}

func TestReusableSupportBaselineRejectsWildcardAndEmptyCreation(t *testing.T) {
	t.Parallel()

	entry := supportServiceImportBaselineEntry{
		Owner:        "factory_runtime",
		ImportPath:   "github.com/portpowered/infinite-you/pkg/services/factory_runtime/*",
		FilePath:     "internal/testutil/*.go",
		TargetRoot:   "pkg/services/factory_runtime",
		Stage:        supportServiceImportBaselineStage,
		DeletionGate: supportServiceImportDeletionGate,
	}
	if err := validateSupportServiceImportBaselineEntry(entry); err == nil || !strings.Contains(err.Error(), "wildcards") {
		t.Fatalf("validate wildcard error = %v, want wildcard rejection", err)
	}

	if err := createSupportServiceImportBaseline(config{root: t.TempDir()}); err == nil || !strings.Contains(err.Error(), "refusing to create empty") {
		t.Fatalf("create empty baseline error = %v, want empty creation rejected", err)
	}
}

func writeSupportServiceBaseline(t *testing.T, repoRoot string, baseline supportServiceImportBaseline) {
	t.Helper()
	payload, err := json.Marshal(baseline)
	if err != nil {
		t.Fatalf("marshal reusable support baseline: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, supportServiceImportBaselinePath), payload, 0o644); err != nil {
		t.Fatalf("write reusable support baseline: %v", err)
	}
}
