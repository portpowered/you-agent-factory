package factorydefinitions_test

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/impl"
	workerconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/authoredmodel/workers"
)

var validationPeerSurfacePackages = []string{
	factoryDefinitionsRoot + "/internal/services/validation/impl",
}

var validationPeerSurfaceSourceFiles = []string{
	"validation_contract.go",
	filepath.Join("internal", "contracts", "validation.go"),
}

// prohibitedValidationPeerPetriEngineSymbols maps raw Petri live-engine symbols
// that must not appear on the Definitions validation peer surface. Authored
// Factory Definition orchestrator.kind = PETRI configuration remains allowed.
var prohibitedValidationPeerPetriEngineSymbols = map[string]struct{}{
	"Net":                    {},
	"RuntimeNet":             {},
	"PetriMarking":           {},
	"PetriMarkingSnapshot":   {},
	"RuntimeToken":           {},
	"RuntimeTokenColor":      {},
	"PetriTransition":        {},
	"EnabledTransition":      {},
	"EngineStateSnapshot":    {},
	"StateSnapshot":          {},
	"NewEngineStateSnapshot": {},
	"MarkingMutation":        {},
}

var forbiddenValidationPeerImportPrefixes = []string{
	modulePrefix + "pkg/services/factory_runtime",
	modulePrefix + "pkg/factory",
	modulePrefix + "pkg/orchestrators/petri",
	modulePrefix + "pkg/petri",
}

// TestValidationPeerSurfacePackagesImportNoPetriEngineOrRuntimeInternals seals
// CUT-DEF-RUN story 003: the public validation package may not import Factory
// Runtime, Petri engine, or legacy factory helper packages.
func TestValidationPeerSurfacePackagesImportNoPetriEngineOrRuntimeInternals(t *testing.T) {
	t.Parallel()

	for _, pkg := range validationPeerSurfacePackages {
		pkg := pkg
		t.Run(shortFactoryDefinitionsPackageName(pkg), func(t *testing.T) {
			t.Parallel()
			assertValidationPeerSurfaceImportsForbidden(t, pkg)
		})
	}
}

// TestValidationPeerSurfaceContractsDeclareOnlyDefinitionsValidationVocabulary
// fails closed when validation peer contract sources export Petri engine types
// or require Runtime/Petri implementation packages.
func TestValidationPeerSurfaceContractsDeclareOnlyDefinitionsValidationVocabulary(t *testing.T) {
	t.Parallel()

	for _, relPath := range validationPeerSurfaceSourceFiles {
		relPath := relPath
		t.Run(filepath.ToSlash(relPath), func(t *testing.T) {
			t.Parallel()
			assertValidationPeerSurfaceSourceUsesDefinitionsVocabularyOnly(t, relPath)
		})
	}
}

// TestValidationPeerSurface_ReturnsDefinitionsOwnedTargets proves validation
// results returned through the peer surface use Definitions-owned targets with
// code, severity, and subject fields rather than Petri-shaped diagnostics.
func TestValidationPeerSurface_ReturnsDefinitionsOwnedTargets(t *testing.T) {
	t.Parallel()

	cfg := peerSurfacePetriScopedFactoryConfig()
	cfg.Workstations[0].Outputs = []factorydefinitions.IOConfig{{
		WorkTypeName: "task",
		StateName:    "missing-state",
	}}

	result := factoryvalidation.ValidateGraphTopology(cfg)
	if !result.HasBlockingTargets() {
		t.Fatal("expected blocking topology targets")
	}

	found := false
	for _, target := range result.Targets {
		assertValidationTargetUsesDefinitionsVocabulary(t, target)
		if target.Code == factoryvalidation.CodeDanglingPlaceReference {
			found = true
		}
	}
	if !found {
		t.Fatalf("validation targets = %#v, want typed dangling place topology target", result.Targets)
	}
}

// TestValidationPeerSurface_PeerConsumesValidationContractsWithoutPetriImports
// proves a peer-shaped consumer can depend on validation contracts and exercise
// validation outcomes using only Definitions-owned vocabulary.
func TestValidationPeerSurface_PeerConsumesValidationContractsWithoutPetriImports(t *testing.T) {
	t.Parallel()

	var validator factorydefinitions.Validator = factoryvalidation.New(nil)
	cfg := &factorydefinitions.FactoryConfig{
		Name: "unsupported-orchestrator",
		Orchestrator: &factorydefinitions.FactoryOrchestratorConfig{
			Kind: "LEGACY",
		},
	}

	result := validator.Validate(context.Background(), cfg, nil)
	for _, target := range result.Targets {
		assertValidationTargetUsesDefinitionsVocabulary(t, target)
	}
}

// TestValidationPeerSurfaceScannerFailsClosedOnSyntheticPetriLeak proves the
// peer-surface guard rejects prohibited Petri engine symbols instead of only
// passing on the current tree.
func TestValidationPeerSurfaceScannerFailsClosedOnSyntheticPetriLeak(t *testing.T) {
	t.Parallel()

	const synthetic = `
package synthetic

type LeakedValidationSurface struct {
	Marking PetriMarkingSnapshot
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "synthetic.go", synthetic, 0)
	if err != nil {
		t.Fatalf("parse synthetic source: %v", err)
	}
	if !validationPeerSurfaceFileDeclaresProhibitedPetriSymbol(file) {
		t.Fatal("expected synthetic Petri engine symbol to be detected")
	}
}

func assertValidationPeerSurfaceImportsForbidden(t *testing.T, packagePath string) {
	t.Helper()
	assertProductionImportsUseRuntimeRootOnly(t, packagePath)

	output, err := execListImports(packagePath)
	if err != nil {
		t.Fatalf("go list imports for %s: %v\n%s", packagePath, err, output)
	}
	for _, importPath := range strings.Fields(output) {
		for _, forbidden := range forbiddenValidationPeerImportPrefixes {
			if importPath == forbidden || strings.HasPrefix(importPath, forbidden+"/") {
				t.Fatalf(
					"%s validation peer import %s is forbidden; keep Petri engine and Runtime implementation types off the validation peer surface",
					packagePath,
					importPath,
				)
			}
		}
	}
}

func execListImports(packagePath string) (string, error) {
	cmd := exec.Command("go", "list", "-f", "{{join .Imports \"\\n\"}}", packagePath)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func assertValidationPeerSurfaceSourceUsesDefinitionsVocabularyOnly(t *testing.T, relPath string) {
	t.Helper()

	sourcePath := relPath
	if _, err := os.Stat(sourcePath); err != nil {
		t.Fatalf("stat %s: %v", relPath, err)
	}
	file, err := parser.ParseFile(token.NewFileSet(), sourcePath, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", relPath, err)
	}
	if validationPeerSurfaceFileDeclaresProhibitedPetriSymbol(file) {
		t.Fatalf("%s exports or references prohibited Petri engine symbols on the validation peer surface", relPath)
	}
}

func validationPeerSurfaceFileDeclaresProhibitedPetriSymbol(file *ast.File) bool {
	found := false
	ast.Inspect(file, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.Ident:
			if _, prohibited := prohibitedValidationPeerPetriEngineSymbols[typed.Name]; prohibited {
				found = true
				return false
			}
		case *ast.TypeSpec:
			if _, prohibited := prohibitedValidationPeerPetriEngineSymbols[typed.Name.Name]; prohibited {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

func assertValidationTargetUsesDefinitionsVocabulary(t *testing.T, target factorydefinitions.ValidationTarget) {
	t.Helper()

	if strings.TrimSpace(target.Code) == "" {
		t.Fatalf("validation target must carry Definitions-owned code: %#v", target)
	}
	if target.Severity != factorydefinitions.ValidationSeverityError &&
		target.Severity != factorydefinitions.ValidationSeverityWarning &&
		target.Severity != factorydefinitions.ValidationSeverityHint {
		t.Fatalf("validation target severity = %q, want Definitions-owned severity", target.Severity)
	}
	if strings.TrimSpace(string(target.Subject.Type)) == "" {
		t.Fatalf("validation target must carry Definitions-owned subject type: %#v", target)
	}
	if strings.Contains(strings.ToLower(target.Code), "petrimarking") ||
		strings.Contains(strings.ToLower(target.Code), "runtime.token") ||
		strings.Contains(strings.ToLower(target.Message), "petri marking") ||
		strings.Contains(strings.ToLower(target.Message), "enabled transition") {
		t.Fatalf("validation target leaked Petri engine vocabulary: %#v", target)
	}
}

func peerSurfacePetriScopedFactoryConfig() *factorydefinitions.FactoryConfig {
	return &factorydefinitions.FactoryConfig{
		Name: "peer-surface-topology",
		WorkTypes: []factorydefinitions.WorkTypeConfig{{
			Name: "task",
			States: []factorydefinitions.StateConfig{
				{Name: "init", Type: factorydefinitions.StateTypeInitial},
				{Name: "done", Type: factorydefinitions.StateTypeTerminal},
				{Name: "failed", Type: factorydefinitions.StateTypeFailed},
			},
		}},
		Workers: []workerconfig.Config{{Name: "worker-a"}},
		Workstations: []factorydefinitions.FactoryWorkstationConfig{{
			Name:           "process",
			WorkerTypeName: "worker-a",
			Inputs:         []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "init"}},
			Outputs:        []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "done"}},
			OnFailure:      []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "failed"}},
		}},
	}
}
