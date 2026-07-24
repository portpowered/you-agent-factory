package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPetriPublicSurfaceProhibitionEncodesRequiredVocabulary(t *testing.T) {
	t.Parallel()

	requiredShapes := map[string]string{
		"Net":                  "raw net",
		"PetriMarkingSnapshot": "raw marking",
		"RuntimeToken":         "raw token",
		"PetriTransition":      "raw transition",
		"EnabledTransition":    "enabled-transition engine shape",
		"EngineStateSnapshot":  "engine snapshot",
		"StateSnapshot":        "engine snapshot",
	}
	for symbol, shape := range requiredShapes {
		got, ok := prohibitedPetriPublicSurfaceSymbols[symbol]
		if !ok {
			t.Fatalf("prohibitedPetriPublicSurfaceSymbols missing %q", symbol)
		}
		if got != shape {
			t.Fatalf("prohibitedPetriPublicSurfaceSymbols[%q] = %q, want %q", symbol, got, shape)
		}
	}
}

func TestScanPetriPublicSurfaceRejectsEachRawVocabularyOutsideRuntimeInternals(t *testing.T) {
	t.Parallel()

	for _, tc := range petriPublicSurfaceOutsideVocabularyCases() {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assertPetriPublicSurfaceFixtureFails(t, tc)
		})
	}
}

type petriPublicSurfaceFixtureCase struct {
	name       string
	filePath   string
	source     string
	symbol     string
	shape      string
	symbolLine string
}

func petriPublicSurfaceOutsideVocabularyCases() []petriPublicSurfaceFixtureCase {
	return []petriPublicSurfaceFixtureCase{
		{
			name:     "raw net",
			filePath: "pkg/workers/outside/net_leak.go",
			source: `package outside
import runtime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
func leak(net runtime.Net) {}
`,
			symbol: "Net", shape: "raw net", symbolLine: "symbol: Net (raw net)",
		},
		{
			name:     "raw marking",
			filePath: "pkg/workers/outside/marking_leak.go",
			source: `package outside
import runtime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
func leak(marking runtime.PetriMarkingSnapshot) {}
`,
			symbol: "PetriMarkingSnapshot", shape: "raw marking",
			symbolLine: "symbol: PetriMarkingSnapshot (raw marking)",
		},
		{
			name:     "raw token",
			filePath: "pkg/workers/outside/token_leak.go",
			source: `package outside
import runtime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
func leak(token runtime.RuntimeToken) {}
`,
			symbol: "RuntimeToken", shape: "raw token", symbolLine: "symbol: RuntimeToken (raw token)",
		},
		{
			name:     "raw transition",
			filePath: "pkg/workers/outside/transition_leak.go",
			source: `package outside
import runtime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
func leak(transition runtime.PetriTransition) {}
`,
			symbol: "PetriTransition", shape: "raw transition",
			symbolLine: "symbol: PetriTransition (raw transition)",
		},
		{
			name:     "enabled-transition engine shape",
			filePath: "pkg/workers/outside/enabled_transition_leak.go",
			source: `package outside
import contracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"
func leak(et contracts.EnabledTransition) {}
`,
			symbol: "EnabledTransition", shape: "enabled-transition engine shape",
			symbolLine: "symbol: EnabledTransition (enabled-transition engine shape)",
		},
		{
			name:     "engine snapshot",
			filePath: "pkg/workers/outside/engine_snapshot_leak.go",
			source: `package outside
import runtime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
func leak(snapshot runtime.StateSnapshot) {}
`,
			symbol: "StateSnapshot", shape: "engine snapshot",
			symbolLine: "symbol: StateSnapshot (engine snapshot)",
		},
		{
			name:     "engine state snapshot constructor",
			filePath: "pkg/workers/outside/engine_state_snapshot_leak.go",
			source: `package outside
import contracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"
func leak() {
  _ = contracts.NewEngineStateSnapshot[any, any](nil, nil, nil)
}
`,
			symbol: "NewEngineStateSnapshot", shape: "engine snapshot",
			symbolLine: "symbol: NewEngineStateSnapshot (engine snapshot)",
		},
	}
}

func assertPetriPublicSurfaceFixtureFails(t *testing.T, tc petriPublicSurfaceFixtureCase) {
	t.Helper()
	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, tc.filePath, tc.source)

	findings, err := scanPetriPublicSurface(repoRoot)
	if err != nil {
		t.Fatalf("scanPetriPublicSurface() error = %v", err)
	}
	joined := petriPublicSurfaceFindingSummary(findings)
	wantFinding := tc.filePath + "|" + tc.symbol + "|" + tc.shape + "|"
	if !strings.Contains(joined, wantFinding) {
		t.Fatalf("findings = %q, want %q", joined, wantFinding)
	}

	stderr := &bytes.Buffer{}
	writePetriPublicSurfaceFindings(stderr, findings)
	diagnostic := stderr.String()
	for _, want := range []string{
		"prohibited Petri public surface",
		"surface: " + tc.filePath,
		tc.symbolLine,
		"required owner: Factory Runtime internals",
		"pkg/services/factory_runtime/internal",
	} {
		if !strings.Contains(diagnostic, want) {
			t.Fatalf("diagnostics = %q, want %q", diagnostic, want)
		}
	}
}

func TestScanPetriPublicSurfaceRejectsRequiredPublicSurfaceCategories(t *testing.T) {
	t.Parallel()

	for _, tc := range petriPublicSurfaceCategoryCases() {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assertPetriPublicSurfaceFixtureFails(t, tc)
		})
	}
}

func petriPublicSurfaceCategoryCases() []petriPublicSurfaceFixtureCase {
	return []petriPublicSurfaceFixtureCase{
		{
			name:     "public API",
			filePath: "pkg/api/public_petri_leak.go",
			source: `package api
import runtime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
func leak(net runtime.Net) {}
`,
			symbol: "Net", shape: "raw net", symbolLine: "symbol: Net (raw net)",
		},
		{
			name:     "transport",
			filePath: "pkg/transports/http/petri_marking_leak.go",
			source: `package http
import runtime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
func leak(marking runtime.PetriMarkingSnapshot) {}
`,
			symbol: "PetriMarkingSnapshot", shape: "raw marking",
			symbolLine: "symbol: PetriMarkingSnapshot (raw marking)",
		},
		{
			name:     "integration contract",
			filePath: "pkg/transports/http/contracttests/petri_token_contract_test.go",
			source: `package contracttests
import runtime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
func leak(token runtime.RuntimeToken) {}
`,
			symbol: "RuntimeToken", shape: "raw token", symbolLine: "symbol: RuntimeToken (raw token)",
		},
		{
			name:     "functional test",
			filePath: "tests/functional/runtime_api/petri_engine_snapshot_test.go",
			source: `package runtime_api
import runtime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
func assertEngineSnapshot(snapshot runtime.StateSnapshot) {}
`,
			symbol: "StateSnapshot", shape: "engine snapshot",
			symbolLine: "symbol: StateSnapshot (engine snapshot)",
		},
	}
}

func TestScanPetriPublicSurfaceAllowsRuntimeInternalVocabulary(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/services/factory_runtime/internal/orchestrators/petri/engine.go", `package petri
import runtime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
func internalEngine() {
  _ = runtime.Net{}
  _ = runtime.PetriMarkingSnapshot{}
  _ = runtime.RuntimeToken{}
  _ = runtime.PetriTransition{}
  _ = runtime.StateSnapshot{}
  _ = runtime.EngineStateSnapshot[any, any]{}
}
`)

	findings, err := scanPetriPublicSurface(repoRoot)
	if err != nil {
		t.Fatalf("scanPetriPublicSurface() error = %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings = %#v, want none for Factory Runtime internals", findings)
	}
}

func TestScanPetriPublicSurfaceAllowsAuthoredOrchestratorKindPetri(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/services/factory_definitions/config_authoring.go", `package factory_definitions
import contracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"
func authoredPetriFactory() contracts.FactoryOrchestratorConfig {
  return contracts.FactoryOrchestratorConfig{Kind: contracts.OrchestratorKindPetri}
}
`)
	writeGoSourceFile(t, repoRoot, "pkg/transports/mapping/factoryconfig/orchestrator.go", `package factoryconfig
const kind = "PETRI"
func selectKind() string { return kind }
`)

	findings, err := scanPetriPublicSurface(repoRoot)
	if err != nil {
		t.Fatalf("scanPetriPublicSurface() error = %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings = %#v, want none for authored orchestrator.kind = PETRI", findings)
	}
}

func TestRunRejectsPetriPublicSurfaceLeakOutsideRuntimeInternals(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	const filePath = "pkg/transports/http/petri_leak.go"
	writeGoSourceFile(t, repoRoot, filePath, `package http
import runtime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
func leak(net runtime.Net) {}
`)

	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want Petri public-surface prohibition failure")
	}
	got := stderr.String()
	for _, want := range []string{
		"prohibited Petri public surface: Net",
		filePath,
		"symbol: Net (raw net)",
		"required owner: Factory Runtime internals",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("run() stderr = %q, want %q", got, want)
		}
	}
}

func TestRunAcceptsExactPetriPublicSurfaceDeletionOnlyBaseline(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	const filePath = "pkg/transports/http/petri_leak.go"
	writeGoSourceFile(t, repoRoot, filePath, `package http
import runtime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
func leak(net runtime.Net) {}
`)
	writePetriPublicSurfaceTestBaseline(t, repoRoot, petriPublicSurfaceBaselineEntry{
		FilePath:     filePath,
		Symbol:       "Net",
		Shape:        "raw net",
		ImportPath:   factoryRuntimeRootImportPath,
		Count:        1,
		Stage:        petriPublicSurfaceBaselineStage,
		DeletionGate: petriPublicSurfaceDeletionGate,
	})

	stderr := &bytes.Buffer{}
	if err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr); err != nil {
		t.Fatalf("run() error = %v, want exact deletion-only baseline accepted; stderr=%q", err, stderr.String())
	}
}

func TestRunRejectsStalePetriPublicSurfaceBaselineEntry(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	const filePath = "pkg/transports/http/petri_leak.go"
	writeGoSourceFile(t, repoRoot, filePath, `package http
import runtime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
func leak(net runtime.Net) {}
`)
	writePetriPublicSurfaceTestBaseline(t, repoRoot, petriPublicSurfaceBaselineEntry{
		FilePath:     filePath,
		Symbol:       "Net",
		Shape:        "raw net",
		ImportPath:   factoryRuntimeRootImportPath,
		Count:        1,
		Stage:        petriPublicSurfaceBaselineStage,
		DeletionGate: petriPublicSurfaceDeletionGate,
	})

	writeGoSourceFile(t, repoRoot, filePath, "package http\nfunc clean() {}\n")
	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
	if err == nil || !strings.Contains(stderr.String(), "stale Petri public surface baseline entry") {
		t.Fatalf("run() error = %v, stderr=%q, want exact stale-baseline rejection", err, stderr.String())
	}
}

func TestRunRejectsPetriPublicSurfaceBaselineGrowth(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	const baselinePath = "pkg/transports/http/petri_leak.go"
	const growthPath = "pkg/transports/cli/new_petri_leak.go"
	writeGoSourceFile(t, repoRoot, baselinePath, `package http
import runtime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
func leak(net runtime.Net) {}
`)
	writePetriPublicSurfaceTestBaseline(t, repoRoot, petriPublicSurfaceBaselineEntry{
		FilePath:     baselinePath,
		Symbol:       "Net",
		Shape:        "raw net",
		ImportPath:   factoryRuntimeRootImportPath,
		Count:        1,
		Stage:        petriPublicSurfaceBaselineStage,
		DeletionGate: petriPublicSurfaceDeletionGate,
	})
	writeGoSourceFile(t, repoRoot, growthPath, `package cli
import runtime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
func leak(token runtime.RuntimeToken) {}
`)

	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want baseline growth rejected")
	}
	got := stderr.String()
	if !strings.Contains(got, "prohibited Petri public surface: RuntimeToken") {
		t.Fatalf("run() stderr = %q, want new RuntimeToken finding blocked", got)
	}
	if strings.Contains(got, "prohibited Petri public surface: Net") {
		t.Fatalf("run() stderr = %q, baselined Net must remain non-blocking", got)
	}
}

func TestPetriPublicSurfaceBaselineRejectsWildcardOrWrongDeletionGate(t *testing.T) {
	t.Parallel()

	cases := []petriPublicSurfaceBaselineEntry{
		{
			FilePath: "pkg/transports/http/*.go", Symbol: "Net", Shape: "raw net",
			ImportPath: factoryRuntimeRootImportPath, Count: 1,
			Stage: petriPublicSurfaceBaselineStage, DeletionGate: petriPublicSurfaceDeletionGate,
		},
		{
			FilePath: "pkg/transports/http/petri_leak.go", Symbol: "Net", Shape: "raw net",
			ImportPath: factoryRuntimeRootImportPath, Count: 1,
			Stage: petriPublicSurfaceBaselineStage, DeletionGate: "broad allowance",
		},
	}
	for _, entry := range cases {
		if _, _, err := partitionPetriPublicSurfaceFindings(nil, petriPublicSurfaceBaseline{
			Version: 1,
			Entries: []petriPublicSurfaceBaselineEntry{entry},
		}); err == nil {
			t.Fatalf("partition error = nil for invalid entry %#v", entry)
		}
	}
}

func TestCreatePetriPublicSurfaceBaselineWritesExactDeletionOnlyEntries(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	const filePath = "pkg/transports/http/petri_leak.go"
	writeGoSourceFile(t, repoRoot, filePath, `package http
import runtime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
func leak(net runtime.Net) {
	_ = runtime.Net{}
}
`)

	if err := createPetriPublicSurfaceBaseline(config{root: repoRoot}); err != nil {
		t.Fatalf("createPetriPublicSurfaceBaseline() error = %v", err)
	}
	payload, err := os.ReadFile(filepath.Join(repoRoot, petriPublicSurfaceBaselinePath))
	if err != nil {
		t.Fatalf("read created baseline: %v", err)
	}
	var baseline petriPublicSurfaceBaseline
	if err := json.Unmarshal(payload, &baseline); err != nil {
		t.Fatalf("decode created baseline: %v", err)
	}
	if baseline.Version != 1 || len(baseline.Entries) != 1 {
		t.Fatalf("created baseline = %#v, want one exact Net edge", baseline)
	}
	entry := baseline.Entries[0]
	if entry.FilePath != filePath || entry.Symbol != "Net" || entry.Count != 2 ||
		entry.Stage != petriPublicSurfaceBaselineStage || entry.DeletionGate != petriPublicSurfaceDeletionGate {
		t.Fatalf("created entry = %#v, want exact aggregated Net edge with count 2", entry)
	}

	stderr := &bytes.Buffer{}
	if err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr); err != nil {
		t.Fatalf("run() after create error = %v; stderr=%q", err, stderr.String())
	}
}

func TestCreatePetriPublicSurfaceBaselineRejectsEmptyDebt(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/transports/http/clean.go", "package http\nfunc clean() {}\n")
	err := createPetriPublicSurfaceBaseline(config{root: repoRoot})
	if err == nil {
		t.Fatal("createPetriPublicSurfaceBaseline() error = nil, want empty-debt rejection")
	}
	if _, statErr := os.Stat(filepath.Join(repoRoot, petriPublicSurfaceBaselinePath)); !os.IsNotExist(statErr) {
		t.Fatalf("empty baseline artifact stat error = %v, want file absent", statErr)
	}
}

func writePetriPublicSurfaceTestBaseline(t *testing.T, repoRoot string, entries ...petriPublicSurfaceBaselineEntry) {
	t.Helper()
	baseline := petriPublicSurfaceBaseline{Version: 1, Entries: entries}
	payload, err := json.MarshalIndent(baseline, "", "  ")
	if err != nil {
		t.Fatalf("marshal Petri public surface baseline: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, petriPublicSurfaceBaselinePath), append(payload, '\n'), 0o644); err != nil {
		t.Fatalf("write Petri public surface baseline: %v", err)
	}
}
