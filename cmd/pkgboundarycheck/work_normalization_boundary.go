package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

const workServiceImportPath = repositoryImportPrefix + "pkg/services/work"

type testWorkNormalizationFinding struct {
	filePath string
	line     int
}

// scanTestWorkNormalization prevents tests and reusable test support from
// executing Work-owned policy as an assertion convenience. Owner-local Work
// tests may call the policy directly; all other tests observe their own public
// contracts or enter through the customer process.
func scanTestWorkNormalization(repoRoot string) ([]testWorkNormalizationFinding, error) {
	var findings []testWorkNormalizationFinding
	err := filepath.WalkDir(repoRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if shouldSkipRepositoryWalkDirectory(repoRoot, path, entry) {
			return filepath.SkipDir
		}
		if entry.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		rel, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if strings.HasPrefix(rel, "pkg/services/work/") || !isTestOwnedGoFile(rel) {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if bytesContainGeneratedMarker(content) {
			return nil
		}
		fset := token.NewFileSet()
		parsed, err := parser.ParseFile(fset, path, content, 0)
		if err != nil {
			return fmt.Errorf("parse test Work normalization file %s: %w", rel, err)
		}
		aliases := map[string]struct{}{}
		dotImported := false
		for _, spec := range parsed.Imports {
			importPath, unquoteErr := strconv.Unquote(spec.Path.Value)
			if unquoteErr != nil || importPath != workServiceImportPath {
				continue
			}
			if spec.Name != nil && spec.Name.Name == "." {
				dotImported = true
				continue
			}
			alias := "work"
			if spec.Name != nil {
				alias = spec.Name.Name
			}
			aliases[alias] = struct{}{}
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			switch called := call.Fun.(type) {
			case *ast.SelectorExpr:
				identifier, ok := called.X.(*ast.Ident)
				if !ok || called.Sel.Name != "NormalizeWorkRequest" {
					return true
				}
				if _, ownedImport := aliases[identifier.Name]; ownedImport {
					findings = append(findings, testWorkNormalizationFinding{rel, fset.Position(called.Sel.Pos()).Line})
				}
			case *ast.Ident:
				if dotImported && called.Name == "NormalizeWorkRequest" {
					findings = append(findings, testWorkNormalizationFinding{rel, fset.Position(called.Pos()).Line})
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan test Work normalization: %w", err)
	}
	slices.SortFunc(findings, func(a, b testWorkNormalizationFinding) int {
		if order := strings.Compare(a.filePath, b.filePath); order != 0 {
			return order
		}
		return a.line - b.line
	})
	return findings, nil
}

func isTestOwnedGoFile(filePath string) bool {
	return strings.HasSuffix(filePath, "_test.go") ||
		strings.HasPrefix(filePath, "internal/testutil/") ||
		strings.HasPrefix(filePath, "tests/functional/internal/support/")
}

func writeTestWorkNormalizationFindings(stderr io.Writer, findings []testWorkNormalizationFinding) {
	for _, finding := range findings {
		fmt.Fprintf(stderr, "[agent-factory:pkg-boundary] prohibited cross-owner test Work normalization: NormalizeWorkRequest (%s:%d)\n", finding.filePath, finding.line)
		fmt.Fprintln(stderr, "  remediation: assert the consumer owner's emitted public contract, relocate pure normalization scenarios to pkg/services/work, or exercise the customer process through root.BuildProcess.")
	}
}
