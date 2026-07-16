package functionalscenarios

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

var qualifyingEvidenceRoots = []string{"tests/functional/", "tests/release/"}

const (
	EvidenceRegistryRelativePath  = "contracts/functional-scenario-evidence.json"
	EvidenceRegistryFormatVersion = 1
)

type EvidenceRegistry struct {
	FormatVersion int                   `json:"formatVersion"`
	Declarations  []EvidenceDeclaration `json:"declarations"`
}

type EvidenceDeclaration struct {
	Test      string   `json:"test"`
	StableIDs []string `json:"stableIds"`
}

// CheckEvidenceReferences resolves every citation against repository-owned
// customer-boundary tests. It is the filesystem boundary for manifest checks;
// status and inventory validation remain pure.
func CheckEvidenceReferences(repositoryRoot string, manifest *Manifest) error {
	if manifest == nil {
		return fmt.Errorf("check functional scenario evidence: manifest is nil")
	}
	registry, err := loadEvidenceRegistry(repositoryRoot)
	if err != nil {
		return err
	}
	declared, err := indexEvidenceRegistry(registry)
	if err != nil {
		return err
	}
	used := make(map[string]map[string]bool)
	for _, scenario := range manifest.Scenarios {
		for _, evidence := range scenario.Evidence {
			if err := checkEvidenceReference(repositoryRoot, evidence.Test); err != nil {
				return manifestCheckError(scenario.Interface, scenario.StableID, "evidence %q is not qualifying: %v", evidence.Test, err)
			}
			stableIDs, ok := declared[evidence.Test]
			if !ok || !slices.Contains(stableIDs, scenario.StableID) {
				return manifestCheckError(
					scenario.Interface,
					scenario.StableID,
					"evidence %q is not declared by its customer-boundary test for this component (declared stable IDs: %v)",
					evidence.Test,
					stableIDs,
				)
			}
			if used[evidence.Test] == nil {
				used[evidence.Test] = make(map[string]bool)
			}
			used[evidence.Test][scenario.StableID] = true
		}
	}
	for _, declaration := range registry.Declarations {
		for _, stableID := range declaration.StableIDs {
			if !used[declaration.Test][stableID] {
				return fmt.Errorf(
					"check functional scenario evidence registry: %q declares unused stable ID %q; add matching manifest evidence or correct %s",
					declaration.Test,
					stableID,
					EvidenceRegistryRelativePath,
				)
			}
		}
	}
	return nil
}

// CheckEvidenceDeclaration is called by customer-boundary tests after their
// observable assertions pass. It keeps the runtime declaration beside the
// behavior synchronized with the registry consumed by the read-only drift gate.
func CheckEvidenceDeclaration(repositoryRoot, reference string, stableIDs []string) error {
	registry, err := loadEvidenceRegistry(repositoryRoot)
	if err != nil {
		return err
	}
	declared, err := indexEvidenceRegistry(registry)
	if err != nil {
		return err
	}
	want, ok := declared[reference]
	got := append([]string(nil), stableIDs...)
	slices.Sort(got)
	if !ok || !slices.Equal(got, want) {
		return fmt.Errorf("test %q declares stable IDs %v, registry requires %v", reference, got, want)
	}
	return nil
}

func loadEvidenceRegistry(repositoryRoot string) (*EvidenceRegistry, error) {
	path := filepath.Join(repositoryRoot, filepath.FromSlash(EvidenceRegistryRelativePath))
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read functional scenario evidence registry %s: %w", path, err)
	}
	registry := &EvidenceRegistry{}
	if err := json.Unmarshal(data, registry); err != nil {
		return nil, fmt.Errorf("decode functional scenario evidence registry %s: %w", path, err)
	}
	return registry, nil
}

func indexEvidenceRegistry(registry *EvidenceRegistry) (map[string][]string, error) {
	if registry.FormatVersion != EvidenceRegistryFormatVersion {
		return nil, fmt.Errorf("check functional scenario evidence registry: formatVersion = %d, want %d", registry.FormatVersion, EvidenceRegistryFormatVersion)
	}
	declared := make(map[string][]string, len(registry.Declarations))
	previousReference := ""
	for _, declaration := range registry.Declarations {
		if declaration.Test == "" || len(declaration.StableIDs) == 0 {
			return nil, fmt.Errorf("check functional scenario evidence registry: every declaration requires a test and stable IDs")
		}
		if _, exists := declared[declaration.Test]; exists {
			return nil, fmt.Errorf("check functional scenario evidence registry: duplicate test declaration %q", declaration.Test)
		}
		if previousReference != "" && declaration.Test < previousReference {
			return nil, fmt.Errorf("check functional scenario evidence registry: declarations must be sorted by test")
		}
		stableIDs := append([]string(nil), declaration.StableIDs...)
		if !slices.IsSorted(stableIDs) {
			return nil, fmt.Errorf("check functional scenario evidence registry: stable IDs for %q must be sorted", declaration.Test)
		}
		for index, stableID := range stableIDs {
			if stableID == "" || (index > 0 && stableID == stableIDs[index-1]) {
				return nil, fmt.Errorf("check functional scenario evidence registry: %q has an empty or duplicate stable ID", declaration.Test)
			}
		}
		declared[declaration.Test] = stableIDs
		previousReference = declaration.Test
	}
	return declared, nil
}

func checkEvidenceReference(repositoryRoot, reference string) error {
	testPath, testName, found := strings.Cut(reference, "::")
	if !found || testPath == "" || testName == "" {
		return fmt.Errorf("use path::TestName")
	}
	normalizedPath := filepath.ToSlash(filepath.Clean(testPath))
	if filepath.IsAbs(testPath) || normalizedPath == ".." || strings.HasPrefix(normalizedPath, "../") {
		return fmt.Errorf("test path must stay within the repository")
	}
	if !hasQualifyingEvidenceRoot(normalizedPath) {
		return fmt.Errorf("test path must be under tests/functional or tests/release, not an internal package test")
	}

	filename := filepath.Join(repositoryRoot, filepath.FromSlash(normalizedPath))
	parsed, err := parser.ParseFile(token.NewFileSet(), filename, nil, 0)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("cited test file does not exist")
		}
		return fmt.Errorf("parse cited test file: %w", err)
	}
	for _, declaration := range parsed.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if ok && function.Name.Name == testName && hasGoTestSignature(function) {
			return nil
		}
	}
	return fmt.Errorf("cited executable test symbol %q does not exist in %s", testName, normalizedPath)
}

func hasGoTestSignature(function *ast.FuncDecl) bool {
	if function.Recv != nil || function.Type.Results != nil || function.Type.Params == nil || len(function.Type.Params.List) != 1 {
		return false
	}
	parameter := function.Type.Params.List[0]
	pointer, ok := parameter.Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	selector, ok := pointer.X.(*ast.SelectorExpr)
	return ok && selector.Sel.Name == "T"
}

func hasQualifyingEvidenceRoot(testPath string) bool {
	for _, root := range qualifyingEvidenceRoots {
		if strings.HasPrefix(testPath, root) {
			return true
		}
	}
	return false
}
