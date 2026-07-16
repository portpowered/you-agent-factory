package functionalscenarios

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

var qualifyingEvidenceRoots = []string{"tests/functional/", "tests/release/"}

// CheckEvidenceReferences resolves every citation against repository-owned
// customer-boundary tests. It is the filesystem boundary for manifest checks;
// status and inventory validation remain pure.
func CheckEvidenceReferences(repositoryRoot string, manifest *Manifest) error {
	if manifest == nil {
		return fmt.Errorf("check functional scenario evidence: manifest is nil")
	}
	for _, scenario := range manifest.Scenarios {
		for _, evidence := range scenario.Evidence {
			if err := checkEvidenceReference(repositoryRoot, evidence.Test); err != nil {
				return manifestCheckError(scenario.Interface, scenario.StableID, "evidence %q is not qualifying: %v", evidence.Test, err)
			}
		}
	}
	return nil
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
