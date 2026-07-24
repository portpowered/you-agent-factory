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

func TestScanPetriPublicSurfaceRejectsRawVocabularyOutsideRuntimeInternals(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/transports/http/petri_leak.go", `package http
import runtime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
func leak() {
  _ = runtime.Net{}
  _ = runtime.PetriMarkingSnapshot{}
  _ = runtime.RuntimeToken{}
  _ = runtime.PetriTransition{}
  _ = runtime.StateSnapshot{}
}
`)

	findings, err := scanPetriPublicSurface(repoRoot)
	if err != nil {
		t.Fatalf("scanPetriPublicSurface() error = %v", err)
	}
	joined := petriPublicSurfaceFindingSummary(findings)
	for _, symbol := range []string{"Net", "PetriMarkingSnapshot", "RuntimeToken", "PetriTransition", "StateSnapshot"} {
		want := "pkg/transports/http/petri_leak.go|" + symbol + "|"
		if !strings.Contains(joined, want) {
			t.Fatalf("findings = %q, want symbol %q on transport surface", joined, symbol)
		}
	}

	stderr := &bytes.Buffer{}
	writePetriPublicSurfaceFindings(stderr, findings)
	diagnostic := stderr.String()
	for _, want := range []string{
		"prohibited Petri public surface",
		"surface: pkg/transports/http/petri_leak.go",
		"symbol: Net",
		"required owner: Factory Runtime internals",
		"pkg/services/factory_runtime/internal",
	} {
		if !strings.Contains(diagnostic, want) {
			t.Fatalf("diagnostics = %q, want %q", diagnostic, want)
		}
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

func TestScanPetriPublicSurfaceRejectsEnabledTransitionEngineShape(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/apisurface/enabled.go", `package apisurface
import contracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"
func leak(et contracts.EnabledTransition) {}
`)

	findings, err := scanPetriPublicSurface(repoRoot)
	if err != nil {
		t.Fatalf("scanPetriPublicSurface() error = %v", err)
	}
	joined := petriPublicSurfaceFindingSummary(findings)
	want := "pkg/apisurface/enabled.go|EnabledTransition|"
	if !strings.Contains(joined, want) {
		t.Fatalf("findings = %q, want %q", joined, want)
	}
}
