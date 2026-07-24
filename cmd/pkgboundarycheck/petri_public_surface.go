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

const (
	petriPublicSurfaceRequiredOwner   = "Factory Runtime internals"
	petriPublicSurfaceInternalPrefix  = "pkg/services/factory_runtime/internal/"
	factoryDefinitionsContractsImport = repositoryImportPrefix + "pkg/services/factory_definitions/contracts"
)

// prohibitedPetriPublicSurfaceSymbols maps raw Petri live-engine symbols to the
// shape name used in diagnostics. Authored Factory Definition selection of
// orchestrator.kind = PETRI is intentionally absent: configuration is allowed.
var prohibitedPetriPublicSurfaceSymbols = map[string]string{
	"Net":                    "raw net",
	"RuntimeNet":             "raw net",
	"PetriMarking":           "raw marking",
	"PetriMarkingSnapshot":   "raw marking",
	"RuntimeToken":           "raw token",
	"RuntimeTokenColor":      "raw token",
	"PetriTransition":        "raw transition",
	"EnabledTransition":      "enabled-transition engine shape",
	"EngineStateSnapshot":    "engine snapshot",
	"StateSnapshot":          "engine snapshot",
	"NewEngineStateSnapshot": "engine snapshot",
}

var petriPublicSurfaceWatchedImports = map[string]struct{}{
	factoryRuntimeRootImportPath:      {},
	factoryDefinitionsImportPath:      {},
	factoryDefinitionsContractsImport: {},
}

type petriPublicSurfaceFinding struct {
	Surface    string
	Symbol     string
	Shape      string
	ImportPath string
	FilePath   string
	Line       int
}

func scanPetriPublicSurface(repoRoot string) ([]petriPublicSurfaceFinding, error) {
	var findings []petriPublicSurfaceFinding
	seen := map[string]struct{}{}
	err := filepath.WalkDir(repoRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "node_modules", "vendor":
				if path != repoRoot {
					return filepath.SkipDir
				}
			}
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
		if isFactoryRuntimeInternalPath(rel) {
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
			return fmt.Errorf("parse Petri public surface file %s: %w", rel, err)
		}

		importsByName := map[string]string{}
		dotImports := map[string]struct{}{}
		for _, spec := range parsed.Imports {
			importPath, unquoteErr := strconv.Unquote(spec.Path.Value)
			if unquoteErr != nil {
				continue
			}
			if !isPetriPublicSurfaceWatchedImport(importPath) {
				continue
			}
			if spec.Name != nil && spec.Name.Name == "." {
				dotImports[importPath] = struct{}{}
				continue
			}
			if spec.Name != nil && spec.Name.Name == "_" {
				continue
			}
			name := filepath.Base(importPath)
			if spec.Name != nil {
				name = spec.Name.Name
			} else if importPath == factoryDefinitionsImportPath {
				name = "factorydefinitions"
			} else if importPath == factoryDefinitionsContractsImport {
				name = "contracts"
			} else if importPath == factoryRuntimeRootImportPath {
				name = "factory"
			}
			importsByName[name] = importPath
		}

		record := func(symbol, importPath string, pos token.Pos) {
			shape, prohibited := prohibitedPetriPublicSurfaceSymbols[symbol]
			if !prohibited {
				return
			}
			line := fset.Position(pos).Line
			key := fmt.Sprintf("%s|%s|%s|%d", rel, symbol, importPath, line)
			if _, exists := seen[key]; exists {
				return
			}
			seen[key] = struct{}{}
			findings = append(findings, petriPublicSurfaceFinding{
				Surface:    rel,
				Symbol:     symbol,
				Shape:      shape,
				ImportPath: importPath,
				FilePath:   rel,
				Line:       line,
			})
		}

		ast.Inspect(parsed, func(node ast.Node) bool {
			switch typed := node.(type) {
			case *ast.SelectorExpr:
				identifier, ok := typed.X.(*ast.Ident)
				if !ok {
					return true
				}
				importPath, imported := importsByName[identifier.Name]
				if !imported {
					return true
				}
				record(typed.Sel.Name, importPath, typed.Sel.Pos())
			case *ast.Ident:
				if len(dotImports) == 0 {
					return true
				}
				if _, prohibited := prohibitedPetriPublicSurfaceSymbols[typed.Name]; !prohibited {
					return true
				}
				for importPath := range dotImports {
					record(typed.Name, importPath, typed.Pos())
				}
			case *ast.IndexListExpr:
				if selector, ok := typed.X.(*ast.SelectorExpr); ok {
					identifier, ok := selector.X.(*ast.Ident)
					if !ok {
						return true
					}
					importPath, imported := importsByName[identifier.Name]
					if !imported {
						return true
					}
					record(selector.Sel.Name, importPath, selector.Sel.Pos())
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan Petri public surface: %w", err)
	}
	slices.SortFunc(findings, func(left, right petriPublicSurfaceFinding) int {
		if order := strings.Compare(left.FilePath, right.FilePath); order != 0 {
			return order
		}
		if order := strings.Compare(left.Symbol, right.Symbol); order != 0 {
			return order
		}
		return left.Line - right.Line
	})
	return findings, nil
}

func isFactoryRuntimeInternalPath(filePath string) bool {
	return strings.HasPrefix(filePath, petriPublicSurfaceInternalPrefix)
}

func isPetriPublicSurfaceWatchedImport(importPath string) bool {
	_, ok := petriPublicSurfaceWatchedImports[importPath]
	return ok
}

func writePetriPublicSurfaceFindings(stderr io.Writer, findings []petriPublicSurfaceFinding) {
	for _, finding := range findings {
		fmt.Fprintf(
			stderr,
			"[agent-factory:pkg-boundary] prohibited Petri public surface: %s (%s:%d)\n",
			finding.Symbol,
			finding.FilePath,
			finding.Line,
		)
		fmt.Fprintf(stderr, "  surface: %s\n", finding.Surface)
		fmt.Fprintf(stderr, "  symbol: %s (%s)\n", finding.Symbol, finding.Shape)
		fmt.Fprintf(
			stderr,
			"  required owner: %s (%s)\n",
			petriPublicSurfaceRequiredOwner,
			strings.TrimSuffix(petriPublicSurfaceInternalPrefix, "/"),
		)
		fmt.Fprintln(
			stderr,
			"  remediation: keep raw nets, markings, tokens, transitions/enabled-transition engine shapes, and engine snapshots inside Factory Runtime internals; authored orchestrator.kind = PETRI remains allowed as configuration.",
		)
	}
}

func petriPublicSurfaceFindingSummary(findings []petriPublicSurfaceFinding) string {
	parts := make([]string, 0, len(findings))
	for _, finding := range findings {
		parts = append(parts, fmt.Sprintf("%s|%s|%s|%d", finding.FilePath, finding.Symbol, finding.Shape, finding.Line))
	}
	return strings.Join(parts, "\n")
}
