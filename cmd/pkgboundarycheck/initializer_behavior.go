package main

import (
	"encoding/json"
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

const initializerBehaviorBaselinePath = "initializer-behavior-baseline.json"
const initializerBehaviorBaselineStage = "wire-injection-full-blow"
const initializerBehaviorDeletionGate = "move the responsibility to its owning service or process edge, inject the resulting lifecycle-ready role, then delete this exact entry"

type initializerBehaviorFinding struct {
	kind     string
	symbol   string
	filePath string
	line     int
	count    int
}

type initializerBehaviorBaseline struct {
	Version int                                `json:"version"`
	Entries []initializerBehaviorBaselineEntry `json:"entries"`
}

type initializerBehaviorBaselineEntry struct {
	Kind         string `json:"kind"`
	Symbol       string `json:"symbol"`
	FilePath     string `json:"filePath"`
	Count        int    `json:"count"`
	Stage        string `json:"stage"`
	DeletionGate string `json:"deletionGate"`
}

func scanInitializerBehavior(repoRoot string) ([]initializerBehaviorFinding, error) {
	root := filepath.Join(repoRoot, "pkg", "initializer")
	findingsByKey := map[string]initializerBehaviorFinding{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if bytesContainGeneratedMarker(content) {
			return nil
		}
		filePath, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return err
		}
		filePath = filepath.ToSlash(filePath)
		fset := token.NewFileSet()
		parsed, err := parser.ParseFile(fset, path, content, 0)
		if err != nil {
			return err
		}
		imports := map[string]string{}
		dotImportedEdges := false
		for _, spec := range parsed.Imports {
			importPath, unquoteErr := strconv.Unquote(spec.Path.Value)
			if unquoteErr != nil {
				continue
			}
			if strings.HasPrefix(importPath, repositoryImportPrefix+"pkg/") &&
				!initializerRepositoryImportAllowed(importPath) &&
				!strings.HasPrefix(importPath, repositoryImportPrefix+"pkg/services/") &&
				!strings.HasPrefix(importPath, repositoryImportPrefix+"pkg/transports/") {
				recordInitializerBehavior(findingsByKey, fset, filePath, "non-lifecycle-repository-coupling", importPath, spec.Pos())
			}
			if strings.HasPrefix(importPath, repositoryImportPrefix+"pkg/services/") {
				recordInitializerBehavior(findingsByKey, fset, filePath, "service-coupling", importPath, spec.Pos())
			}
			if strings.HasPrefix(importPath, repositoryImportPrefix+"pkg/transports/") {
				recordInitializerBehavior(findingsByKey, fset, filePath, "transport-coupling", importPath, spec.Pos())
			}
			if spec.Name != nil && spec.Name.Name == "_" {
				continue
			}
			name := filepath.Base(importPath)
			if spec.Name != nil {
				name = spec.Name.Name
			}
			imports[name] = importPath
			if name == "." && importPath == repositoryImportPrefix+"pkg/services/edges" {
				dotImportedEdges = true
			}
			if importPath == "net/http" {
				recordInitializerBehavior(findingsByKey, fset, filePath, "http-coupling", "net/http", spec.Pos())
			}
		}

		ast.Inspect(parsed, func(node ast.Node) bool {
			switch typed := node.(type) {
			case *ast.TypeSpec:
				if parsed.Name.Name == "initializer" && (typed.Name.Name == "MCPApplication" || typed.Name.Name == "RuntimeDiagnosticsProvider") {
					recordInitializerBehavior(findingsByKey, fset, filePath, "retired-initializer-surface", typed.Name.Name, typed.Name.Pos())
				}
				if typed.Name.Name == "Mode" || typed.Name.Name == "ProcessMode" {
					recordInitializerBehavior(findingsByKey, fset, filePath, "product-lifecycle-mode", typed.Name.Name, typed.Name.Pos())
				}
				if typed.Name.Name == "Lifecycles" {
					recordInitializerBehavior(findingsByKey, fset, filePath, "product-lifecycle-slots", typed.Name.Name, typed.Name.Pos())
				}
			case *ast.ValueSpec:
				for _, name := range typed.Names {
					if strings.HasPrefix(name.Name, "ModeAPI") || strings.HasPrefix(name.Name, "ModeCLI") ||
						strings.HasPrefix(name.Name, "ModeMCP") || strings.HasPrefix(name.Name, "ProcessMode") {
						recordInitializerBehavior(findingsByKey, fset, filePath, "product-lifecycle-mode", name.Name, name.Pos())
					}
				}
			case *ast.Field:
				for _, name := range typed.Names {
					switch name.Name {
					case "API", "CLI", "MCP", "Runtime", "Workers", "FactoryVisualization":
						recordInitializerBehavior(findingsByKey, fset, filePath, "product-lifecycle-slot", name.Name, name.Pos())
					}
				}
				ast.Inspect(typed.Type, func(typeNode ast.Node) bool {
					if identifier, ok := typeNode.(*ast.Ident); ok && dotImportedEdges && identifier.Name == "Edges" {
						recordInitializerBehavior(findingsByKey, fset, filePath, "edge-bag", "pkg/services/edges.Edges", identifier.Pos())
						return true
					}
					selector, ok := typeNode.(*ast.SelectorExpr)
					if !ok || selector.Sel.Name != "Edges" {
						return true
					}
					identifier, ok := selector.X.(*ast.Ident)
					if ok && imports[identifier.Name] == repositoryImportPrefix+"pkg/services/edges" {
						recordInitializerBehavior(findingsByKey, fset, filePath, "edge-bag", "pkg/services/edges.Edges", selector.Pos())
					}
					return true
				})
				return false
			case *ast.FuncDecl:
				if parsed.Name.Name == "initializer" && typed.Recv == nil && (typed.Name.Name == "StartSidecar" || typed.Name.Name == "RuntimeDiagnostics") {
					recordInitializerBehavior(findingsByKey, fset, filePath, "retired-initializer-surface", typed.Name.Name, typed.Name.Pos())
				}
				if strings.HasPrefix(filePath, "pkg/initializer/application/") && parsed.Name.Name == "application" && typed.Name.Name == "NewCommand" && receiverTypeName(typed.Recv) == "Process" {
					recordInitializerBehavior(findingsByKey, fset, filePath, "exported-command-construction", "Process.NewCommand", typed.Name.Pos())
				}
			case *ast.CallExpr:
				selector, ok := typed.Fun.(*ast.SelectorExpr)
				if ok && selector.Sel.Name == "Stat" && strings.HasPrefix(filePath, "pkg/initializer/application/") {
					recordInitializerBehavior(findingsByKey, fset, filePath, "stream-stat-fallback", "Stat", selector.Sel.Pos())
				}
			}
			return true
		})
		return nil
	})
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan initializer behavior: %w", err)
	}
	findings := make([]initializerBehaviorFinding, 0, len(findingsByKey))
	for _, finding := range findingsByKey {
		findings = append(findings, finding)
	}
	slices.SortFunc(findings, compareInitializerBehaviorFindings)
	return findings, nil
}

func initializerRepositoryImportAllowed(importPath string) bool {
	if importPath == repositoryImportPrefix+"pkg/initializer" ||
		strings.HasPrefix(importPath, repositoryImportPrefix+"pkg/initializer/") {
		return true
	}
	return importPath == repositoryImportPrefix+"pkg/platform/runtimeartifact"
}

func receiverTypeName(receiver *ast.FieldList) string {
	if receiver == nil || len(receiver.List) != 1 {
		return ""
	}
	typeExpression := receiver.List[0].Type
	if pointer, ok := typeExpression.(*ast.StarExpr); ok {
		typeExpression = pointer.X
	}
	identifier, _ := typeExpression.(*ast.Ident)
	if identifier == nil {
		return ""
	}
	return identifier.Name
}

func recordInitializerBehavior(findings map[string]initializerBehaviorFinding, fset *token.FileSet, filePath, kind, symbol string, position token.Pos) {
	key := initializerBehaviorKey(filePath, kind, symbol)
	finding := findings[key]
	if finding.count == 0 {
		finding = initializerBehaviorFinding{kind: kind, symbol: symbol, filePath: filePath, line: fset.Position(position).Line}
	}
	finding.count++
	findings[key] = finding
}

func initializerBehaviorKey(filePath, kind, symbol string) string {
	return filePath + "\x00" + kind + "\x00" + symbol
}

func compareInitializerBehaviorFindings(left, right initializerBehaviorFinding) int {
	if comparison := strings.Compare(left.filePath, right.filePath); comparison != 0 {
		return comparison
	}
	if comparison := strings.Compare(left.kind, right.kind); comparison != 0 {
		return comparison
	}
	return strings.Compare(left.symbol, right.symbol)
}

func loadInitializerBehaviorBaseline(repoRoot string) (initializerBehaviorBaseline, error) {
	payload, err := os.ReadFile(filepath.Join(repoRoot, initializerBehaviorBaselinePath))
	if os.IsNotExist(err) {
		return initializerBehaviorBaseline{}, nil
	}
	if err != nil {
		return initializerBehaviorBaseline{}, fmt.Errorf("read initializer behavior baseline: %w", err)
	}
	var baseline initializerBehaviorBaseline
	if err := json.Unmarshal(payload, &baseline); err != nil {
		return initializerBehaviorBaseline{}, fmt.Errorf("decode initializer behavior baseline: %w", err)
	}
	if baseline.Version != 1 {
		return initializerBehaviorBaseline{}, fmt.Errorf("initializer behavior baseline version = %d, want 1", baseline.Version)
	}
	if err := requireNonEmptyMigrationBaseline(initializerBehaviorBaselinePath, len(baseline.Entries)); err != nil {
		return initializerBehaviorBaseline{}, err
	}
	return baseline, nil
}

func partitionInitializerBehaviorFindings(findings []initializerBehaviorFinding, baseline initializerBehaviorBaseline) ([]initializerBehaviorFinding, []initializerBehaviorBaselineEntry, error) {
	baselineByKey := make(map[string]initializerBehaviorBaselineEntry, len(baseline.Entries))
	for _, entry := range baseline.Entries {
		if err := validateInitializerBehaviorBaselineEntry(entry); err != nil {
			return nil, nil, err
		}
		key := initializerBehaviorKey(entry.FilePath, entry.Kind, entry.Symbol)
		if _, duplicate := baselineByKey[key]; duplicate {
			return nil, nil, fmt.Errorf("duplicate initializer behavior baseline entry: %s %s %s", entry.FilePath, entry.Kind, entry.Symbol)
		}
		baselineByKey[key] = entry
	}
	var blocking []initializerBehaviorFinding
	seen := map[string]struct{}{}
	for _, finding := range findings {
		key := initializerBehaviorKey(finding.filePath, finding.kind, finding.symbol)
		entry, recorded := baselineByKey[key]
		if !recorded || entry.Count != finding.count {
			blocking = append(blocking, finding)
		}
		seen[key] = struct{}{}
	}
	var stale []initializerBehaviorBaselineEntry
	for key, entry := range baselineByKey {
		if _, found := seen[key]; !found {
			stale = append(stale, entry)
		}
	}
	slices.SortFunc(stale, func(left, right initializerBehaviorBaselineEntry) int {
		return strings.Compare(initializerBehaviorKey(left.FilePath, left.Kind, left.Symbol), initializerBehaviorKey(right.FilePath, right.Kind, right.Symbol))
	})
	return blocking, stale, nil
}

func validateInitializerBehaviorBaselineEntry(entry initializerBehaviorBaselineEntry) error {
	if entry.Kind == "" || entry.Symbol == "" || entry.FilePath == "" || entry.Count < 1 || entry.Stage != initializerBehaviorBaselineStage || entry.DeletionGate != initializerBehaviorDeletionGate {
		return fmt.Errorf("invalid initializer behavior baseline entry: %+v", entry)
	}
	filePath := filepath.ToSlash(entry.FilePath)
	if !strings.HasPrefix(filePath, "pkg/initializer/") || !strings.HasSuffix(filePath, ".go") || strings.ContainsAny(filePath, "*?[]") {
		return fmt.Errorf("initializer behavior baseline entry is not an exact Initializer Go file: %s", entry.FilePath)
	}
	if strings.ContainsAny(entry.Symbol, "*?[]") {
		return fmt.Errorf("initializer behavior baseline entry contains a wildcard symbol: %s", entry.Symbol)
	}
	switch entry.Kind {
	case "edge-bag":
		if entry.Symbol == "pkg/services/edges.Edges" {
			return nil
		}
	case "http-coupling":
		if entry.Symbol == "net/http" {
			return nil
		}
	case "exported-command-construction":
		if entry.Symbol == "Process.NewCommand" {
			return nil
		}
	case "stream-stat-fallback":
		if entry.Symbol == "Stat" {
			return nil
		}
	case "service-coupling":
		if strings.HasPrefix(entry.Symbol, repositoryImportPrefix+"pkg/services/") {
			return nil
		}
	case "transport-coupling":
		if strings.HasPrefix(entry.Symbol, repositoryImportPrefix+"pkg/transports/") {
			return nil
		}
	}
	return fmt.Errorf("initializer behavior baseline entry names an unrecognized exact rule: %s %s", entry.Kind, entry.Symbol)
}

func writeInitializerBehaviorFindings(writer io.Writer, findings []initializerBehaviorFinding) {
	for _, finding := range findings {
		fmt.Fprintf(writer, "[agent-factory:pkg-boundary] prohibited Initializer behavior: %s (%s) in %s:%d (%d call(s))\n", finding.symbol, finding.kind, finding.filePath, finding.line, finding.count)
		fmt.Fprintln(writer, "  remediation: Initializer may coordinate lifecycle only; move ownership outward and inject the lifecycle-ready role.")
	}
}

func writeStaleInitializerBehaviorBaselineEntries(writer io.Writer, entries []initializerBehaviorBaselineEntry) {
	for _, entry := range entries {
		fmt.Fprintf(writer, "[agent-factory:pkg-boundary] stale initializer behavior baseline entry: %s (%s) in %s\n", entry.Symbol, entry.Kind, entry.FilePath)
		fmt.Fprintln(writer, "  remediation: delete the stale exact baseline entry; migration debt may only decrease.")
	}
}

func writeInitializerBehaviorBaselineSummary(writer io.Writer, count int) {
	if count > 0 {
		fmt.Fprintf(writer, "[agent-factory:pkg-boundary] active Initializer behavior migration baseline: %d exact entry(s)\n", count)
	}
}
