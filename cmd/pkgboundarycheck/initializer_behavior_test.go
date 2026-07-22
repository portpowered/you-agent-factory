package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunRejectsInitializerOwningNonLifecycleBehavior(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/initializer/core.go", `package initializer
import (
	"net/http"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
)
type Runtime struct { edges serviceedges.Edges; server *http.Server }
`)
	writeGoSourceFile(t, repoRoot, "pkg/initializer/application/process.go", `package application
type Process struct{}
func (p *Process) NewCommand() any { return nil }
func terminalInput(stream interface{ Stat() (any, error) }) bool { _, _ = stream.Stat(); return false }
`)

	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want Initializer behavior rejected")
	}
	for _, want := range []string{
		"prohibited Initializer behavior: pkg/services/edges.Edges (edge-bag)",
		"prohibited Initializer behavior: github.com/portpowered/infinite-you/pkg/services/edges (service-coupling)",
		"prohibited Initializer behavior: net/http (http-coupling)",
		"prohibited Initializer behavior: Process.NewCommand (exported-command-construction)",
		"prohibited Initializer behavior: Stat (stream-stat-fallback)",
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("run() stderr = %q, want %q", stderr.String(), want)
		}
	}
}

func TestRunRejectsInitializerTransportImport(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/initializer/application/open.go", `package application
import _ "github.com/portpowered/infinite-you/pkg/transports/mcp/stdio"
`)

	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want Initializer transport coupling rejected")
	}
	want := "prohibited Initializer behavior: github.com/portpowered/infinite-you/pkg/transports/mcp/stdio (transport-coupling)"
	if !strings.Contains(stderr.String(), want) {
		t.Fatalf("run() stderr = %q, want %q", stderr.String(), want)
	}
}

func TestRunRejectsInitializerRepositoryDependencyOutsideLifecycleAllowlist(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/initializer/application/open.go", `package application
import _ "github.com/portpowered/infinite-you/pkg/platform/filesystem"
`)

	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want non-lifecycle repository coupling rejected")
	}
	want := "prohibited Initializer behavior: github.com/portpowered/infinite-you/pkg/platform/filesystem (non-lifecycle-repository-coupling)"
	if !strings.Contains(stderr.String(), want) {
		t.Fatalf("run() stderr = %q, want %q", stderr.String(), want)
	}
}

func TestInitializerRepositoryDependencyAllowlistIsExact(t *testing.T) {
	t.Parallel()

	for _, importPath := range []string{
		"github.com/portpowered/infinite-you/pkg/initializer",
		"github.com/portpowered/infinite-you/pkg/initializer/lifecycle",
		"github.com/portpowered/infinite-you/pkg/platform/runtimeartifact",
	} {
		if !initializerRepositoryImportAllowed(importPath) {
			t.Fatalf("initializerRepositoryImportAllowed(%q) = false, want true", importPath)
		}
	}
	for _, importPath := range []string{
		"github.com/portpowered/infinite-you/pkg/platform/runtimeartifact/internal",
		"github.com/portpowered/infinite-you/pkg/platform/filesystem",
		"github.com/portpowered/infinite-you/pkg/root",
	} {
		if initializerRepositoryImportAllowed(importPath) {
			t.Fatalf("initializerRepositoryImportAllowed(%q) = true, want false", importPath)
		}
	}
}

func TestRunRejectsInitializerProductLifecycleModesAndSlots(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/initializer/runtimeapplication/managed.go", `package runtimeapplication
type Mode string
const ModeAPI Mode = "api"
type Lifecycles struct { API any; Workers any }
`)

	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want product lifecycle policy rejected")
	}
	for _, want := range []string{
		"prohibited Initializer behavior: Mode (product-lifecycle-mode)",
		"prohibited Initializer behavior: ModeAPI (product-lifecycle-mode)",
		"prohibited Initializer behavior: Lifecycles (product-lifecycle-slots)",
		"prohibited Initializer behavior: API (product-lifecycle-slot)",
		"prohibited Initializer behavior: Workers (product-lifecycle-slot)",
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("run() stderr = %q, want %q", stderr.String(), want)
		}
	}
}

func TestRunRejectsRetiredInitializerSurfaces(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/initializer/retired.go", `package initializer
type MCPApplication interface{}
type RuntimeDiagnosticsProvider interface{}
func StartSidecar() {}
func RuntimeDiagnostics() {}
`)

	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want retired Initializer surfaces rejected")
	}
	for _, symbol := range []string{"MCPApplication", "RuntimeDiagnosticsProvider", "StartSidecar", "RuntimeDiagnostics"} {
		want := "prohibited Initializer behavior: " + symbol + " (retired-initializer-surface)"
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("run() stderr = %q, want %q", stderr.String(), want)
		}
	}
}

func TestInitializerBehaviorBaselineIsExactAndDeletionOnly(t *testing.T) {
	t.Parallel()

	entry := initializerBehaviorBaselineEntry{
		Kind:         "http-coupling",
		Symbol:       "net/http",
		FilePath:     "pkg/initializer/runtime.go",
		Count:        1,
		Stage:        initializerBehaviorBaselineStage,
		DeletionGate: initializerBehaviorDeletionGate,
	}
	finding := initializerBehaviorFinding{
		kind: entry.Kind, symbol: entry.Symbol, filePath: entry.FilePath, count: entry.Count,
	}
	blocking, stale, err := partitionInitializerBehaviorFindings(
		[]initializerBehaviorFinding{finding},
		initializerBehaviorBaseline{Version: 1, Entries: []initializerBehaviorBaselineEntry{entry}},
	)
	if err != nil || len(blocking) != 0 || len(stale) != 0 {
		t.Fatalf("matching exact baseline: blocking=%v stale=%v err=%v", blocking, stale, err)
	}

	blocking, stale, err = partitionInitializerBehaviorFindings(
		nil,
		initializerBehaviorBaseline{Version: 1, Entries: []initializerBehaviorBaselineEntry{entry}},
	)
	if err != nil || len(blocking) != 0 || len(stale) != 1 {
		t.Fatalf("deleted finding: blocking=%v stale=%v err=%v, want one stale entry", blocking, stale, err)
	}
}

func TestLoadInitializerBehaviorBaselineRejectsBroadOrMalformedEntry(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	payload := `{"version":1,"entries":[{"kind":"edge-bag","symbol":"pkg/services/edges.Edges","filePath":"pkg/initializer","count":0,"stage":"wire-injection-full-blow","deletionGate":"broad allowance"}]}`
	if err := os.WriteFile(filepath.Join(repoRoot, initializerBehaviorBaselinePath), []byte(payload), 0o644); err != nil {
		t.Fatalf("write baseline: %v", err)
	}
	baseline, err := loadInitializerBehaviorBaseline(repoRoot)
	if err != nil {
		t.Fatalf("load baseline: %v", err)
	}
	if _, _, err := partitionInitializerBehaviorFindings(nil, baseline); err == nil {
		t.Fatal("partition error = nil, want malformed broad baseline rejected")
	}
}

func TestInitializerBehaviorBaselineRejectsWildcardEntry(t *testing.T) {
	t.Parallel()

	entry := initializerBehaviorBaselineEntry{
		Kind: "service-coupling", Symbol: "github.com/portpowered/infinite-you/pkg/services/*",
		FilePath: "pkg/initializer/*.go", Count: 1,
		Stage: initializerBehaviorBaselineStage, DeletionGate: initializerBehaviorDeletionGate,
	}
	if _, _, err := partitionInitializerBehaviorFindings(nil, initializerBehaviorBaseline{Version: 1, Entries: []initializerBehaviorBaselineEntry{entry}}); err == nil {
		t.Fatal("partition error = nil, want wildcard baseline rejected")
	}
}
