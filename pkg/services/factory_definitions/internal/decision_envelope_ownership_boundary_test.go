package internal_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

const (
	decisionEnvelopeImportPath = factoryDefinitionsModule + "/internal/services/invocation_policy/decisionenvelope"
	distributionServiceRoot    = "services/distribution"
)

var distributionDecisionEnvelopeForbiddenImports = []string{
	decisionEnvelopeImportPath,
	factoryDefinitionsModule + "/packages/goal",
}

var distributionDecisionEnvelopeForbiddenSymbols = []string{
	"ParseDecisionEnvelopeJSON",
	"WorkResultFromDecisionEnvelope",
	"WorkResultFromGoalRoutingDecisionEnvelope",
	"UsesDecisionEnvelopeOutcome",
	"UsesGoalRoutingDecisionEnvelope",
	"NormalizeGoalRoutingDecision",
}

// pss-cln-def-fold-distribution-004: distribution owns packaged asset/materialize
// modules only. Decision-envelope interpretation remains in decisionenvelope and
// the transitional packages/goal shim until invocation_policy cutover.
func TestDecisionEnvelopeOwnershipBoundary_DistributionDoesNotImportEnvelopeImplementation(t *testing.T) {
	t.Parallel()

	assertDirectoryTreeDoesNotImportDecisionEnvelopePaths(t, distributionServiceRoot)
}

func TestDecisionEnvelopeOwnershipBoundary_DistributionDoesNotHostDecisionEnvelopePackage(t *testing.T) {
	t.Parallel()

	envelopeUnderDistribution := filepath.Join(distributionServiceRoot, "decisionenvelope")
	if _, err := os.Stat(envelopeUnderDistribution); !os.IsNotExist(err) {
		t.Fatalf(
			"%s must not host decisionenvelope; envelope interpretation stays outside distribution ownership",
			envelopeUnderDistribution,
		)
	}

	err := filepath.WalkDir(distributionServiceRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		if strings.Contains(filepath.Base(path), "decision_envelope") {
			t.Fatalf(
				"%s must not define decision-envelope interpretation; use decisionenvelope until invocation_policy cutover",
				path,
			)
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		source := string(content)
		for _, symbol := range distributionDecisionEnvelopeForbiddenSymbols {
			if strings.Contains(source, symbol) {
				t.Fatalf(
					"%s must not define decision-envelope symbol %q; distribution owns asset/materialize only",
					path,
					symbol,
				)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", distributionServiceRoot, err)
	}
}

func TestDecisionEnvelopeOwnershipBoundary_TransitionalGoalShimDelegatesToDecisionEnvelope(t *testing.T) {
	t.Parallel()

	envelopePath := filepath.Join("services", "invocation_policy", "decisionenvelope", "service.go")
	if _, err := os.Stat(envelopePath); err != nil {
		t.Fatalf("invocation_policy decisionenvelope owner path must exist after residual deletion: %v", err)
	}
}

func TestDecisionEnvelopeOwnershipBoundary_InventoryDoesNotSuccessorDecisionEnvelopeToDistribution(t *testing.T) {
	t.Parallel()

	got, err := ownershipinventory.MapPackage("pkg/services/factory_definitions/internal/services/invocation_policy/decisionenvelope")
	if err != nil {
		t.Fatalf("MapPackage(decisionenvelope) error = %v", err)
	}
	if strings.Contains(got.Successor, "internal/services/distribution") {
		t.Fatalf(
			"decisionenvelope successor = %q, must not fold into distribution ownership",
			got.Successor,
		)
	}
}

func assertDirectoryTreeDoesNotImportDecisionEnvelopePaths(t *testing.T, root string) {
	t.Helper()

	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		assertGoFileDoesNotImportDecisionEnvelopePaths(t, path)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
}

func assertGoFileDoesNotImportDecisionEnvelopePaths(t *testing.T, path string) {
	t.Helper()

	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, path, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	for _, importSpec := range file.Imports {
		importPath := strings.Trim(importSpec.Path.Value, `"`)
		for _, forbidden := range distributionDecisionEnvelopeForbiddenImports {
			if importPath == forbidden || strings.HasPrefix(importPath, forbidden+"/") {
				t.Fatalf(
					"%s must not import decision-envelope implementation %s; distribution owns asset/materialize only",
					path,
					importPath,
				)
			}
		}
	}
}
