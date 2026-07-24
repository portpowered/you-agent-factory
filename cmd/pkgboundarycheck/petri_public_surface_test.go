package main

import (
	"bytes"
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

	cases := []struct {
		name       string
		filePath   string
		source     string
		symbol     string
		shape      string
		symbolLine string
	}{
		{
			name:     "raw net",
			filePath: "pkg/workers/outside/net_leak.go",
			source: `package outside
import runtime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
func leak(net runtime.Net) {}
`,
			symbol:     "Net",
			shape:      "raw net",
			symbolLine: "symbol: Net (raw net)",
		},
		{
			name:     "raw marking",
			filePath: "pkg/workers/outside/marking_leak.go",
			source: `package outside
import runtime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
func leak(marking runtime.PetriMarkingSnapshot) {}
`,
			symbol:     "PetriMarkingSnapshot",
			shape:      "raw marking",
			symbolLine: "symbol: PetriMarkingSnapshot (raw marking)",
		},
		{
			name:     "raw token",
			filePath: "pkg/workers/outside/token_leak.go",
			source: `package outside
import runtime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
func leak(token runtime.RuntimeToken) {}
`,
			symbol:     "RuntimeToken",
			shape:      "raw token",
			symbolLine: "symbol: RuntimeToken (raw token)",
		},
		{
			name:     "raw transition",
			filePath: "pkg/workers/outside/transition_leak.go",
			source: `package outside
import runtime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
func leak(transition runtime.PetriTransition) {}
`,
			symbol:     "PetriTransition",
			shape:      "raw transition",
			symbolLine: "symbol: PetriTransition (raw transition)",
		},
		{
			name:     "enabled-transition engine shape",
			filePath: "pkg/workers/outside/enabled_transition_leak.go",
			source: `package outside
import contracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"
func leak(et contracts.EnabledTransition) {}
`,
			symbol:     "EnabledTransition",
			shape:      "enabled-transition engine shape",
			symbolLine: "symbol: EnabledTransition (enabled-transition engine shape)",
		},
		{
			name:     "engine snapshot",
			filePath: "pkg/workers/outside/engine_snapshot_leak.go",
			source: `package outside
import runtime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
func leak(snapshot runtime.StateSnapshot) {}
`,
			symbol:     "StateSnapshot",
			shape:      "engine snapshot",
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
			symbol:     "NewEngineStateSnapshot",
			shape:      "engine snapshot",
			symbolLine: "symbol: NewEngineStateSnapshot (engine snapshot)",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
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
		})
	}
}

func TestScanPetriPublicSurfaceRejectsRequiredPublicSurfaceCategories(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		filePath   string
		source     string
		symbol     string
		shape      string
		symbolLine string
	}{
		{
			name:     "public API",
			filePath: "pkg/api/public_petri_leak.go",
			source: `package api
import runtime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
func leak(net runtime.Net) {}
`,
			symbol:     "Net",
			shape:      "raw net",
			symbolLine: "symbol: Net (raw net)",
		},
		{
			name:     "transport",
			filePath: "pkg/transports/http/petri_marking_leak.go",
			source: `package http
import runtime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
func leak(marking runtime.PetriMarkingSnapshot) {}
`,
			symbol:     "PetriMarkingSnapshot",
			shape:      "raw marking",
			symbolLine: "symbol: PetriMarkingSnapshot (raw marking)",
		},
		{
			name:     "integration contract",
			filePath: "pkg/transports/http/contracttests/petri_token_contract_test.go",
			source: `package contracttests
import runtime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
func leak(token runtime.RuntimeToken) {}
`,
			symbol:     "RuntimeToken",
			shape:      "raw token",
			symbolLine: "symbol: RuntimeToken (raw token)",
		},
		{
			name:     "functional test",
			filePath: "tests/functional/runtime_api/petri_engine_snapshot_test.go",
			source: `package runtime_api
import runtime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
func assertEngineSnapshot(snapshot runtime.StateSnapshot) {}
`,
			symbol:     "StateSnapshot",
			shape:      "engine snapshot",
			symbolLine: "symbol: StateSnapshot (engine snapshot)",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
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
		})
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
