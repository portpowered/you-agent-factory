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

const (
	petriPublicSurfaceRequiredOwner   = "Factory Runtime internals"
	petriPublicSurfaceInternalPrefix  = "pkg/services/factory_runtime/internal/"
	petriPublicSurfaceBaselinePath    = "petri-public-surface-baseline.json"
	petriPublicSurfaceBaselineStage   = "imp-run-01-petri-boundary-retirement"
	petriPublicSurfaceDeletionGate    = "retire this exact Petri public-surface leak under Runtime Petri-boundary retirement / IMP-RUN-01, then delete this exact baseline entry"
)

type petriPublicSurfaceBaseline struct {
	Version int                              `json:"version"`
	Entries []petriPublicSurfaceBaselineEntry `json:"entries"`
}

type petriPublicSurfaceBaselineEntry struct {
	FilePath     string `json:"filePath"`
	Symbol       string `json:"symbol"`
	Shape        string `json:"shape"`
	ImportPath   string `json:"importPath"`
	Count        int    `json:"count"`
	Stage        string `json:"stage"`
	DeletionGate string `json:"deletionGate"`
}

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
	factoryRuntimeRootImportPath: {},
	factoryDefinitionsImportPath: {},
	factoryDefinitionsInternalContractsImportPath: {},
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
		fileFindings, err := scanPetriPublicSurfaceFile(path, rel, seen)
		if err != nil {
			return err
		}
		findings = append(findings, fileFindings...)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan Petri public surface: %w", err)
	}
	slices.SortFunc(findings, comparePetriPublicSurfaceFindings)
	return findings, nil
}

func comparePetriPublicSurfaceFindings(left, right petriPublicSurfaceFinding) int {
	if order := strings.Compare(left.FilePath, right.FilePath); order != 0 {
		return order
	}
	if order := strings.Compare(left.Symbol, right.Symbol); order != 0 {
		return order
	}
	return left.Line - right.Line
}

func scanPetriPublicSurfaceFile(path, rel string, seen map[string]struct{}) ([]petriPublicSurfaceFinding, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if bytesContainGeneratedMarker(content) {
		return nil, nil
	}
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, path, content, 0)
	if err != nil {
		return nil, fmt.Errorf("parse Petri public surface file %s: %w", rel, err)
	}
	importsByName, dotImports := petriPublicSurfaceImports(parsed)
	var findings []petriPublicSurfaceFinding
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
	inspectPetriPublicSurfaceAST(parsed, importsByName, dotImports, record)
	return findings, nil
}

func petriPublicSurfaceImports(parsed *ast.File) (map[string]string, map[string]struct{}) {
	importsByName := map[string]string{}
	dotImports := map[string]struct{}{}
	for _, spec := range parsed.Imports {
		importPath, unquoteErr := strconv.Unquote(spec.Path.Value)
		if unquoteErr != nil || !isPetriPublicSurfaceWatchedImport(importPath) {
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
		} else if importPath == factoryRuntimeRootImportPath {
			name = "factory"
		}
		importsByName[name] = importPath
	}
	return importsByName, dotImports
}

func inspectPetriPublicSurfaceAST(
	parsed *ast.File,
	importsByName map[string]string,
	dotImports map[string]struct{},
	record func(symbol, importPath string, pos token.Pos),
) {
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
			selector, ok := typed.X.(*ast.SelectorExpr)
			if !ok {
				return true
			}
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
		return true
	})
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

func petriPublicSurfaceKey(filePath, symbol, importPath string) string {
	return filepath.ToSlash(filePath) + "\x00" + symbol + "\x00" + importPath
}

func aggregatePetriPublicSurfaceFindings(findings []petriPublicSurfaceFinding) []petriPublicSurfaceBaselineEntry {
	type aggregate struct {
		entry    petriPublicSurfaceBaselineEntry
		firstIdx int
	}
	byKey := map[string]*aggregate{}
	order := make([]string, 0, len(findings))
	for index, finding := range findings {
		key := petriPublicSurfaceKey(finding.FilePath, finding.Symbol, finding.ImportPath)
		existing, ok := byKey[key]
		if !ok {
			byKey[key] = &aggregate{
				entry: petriPublicSurfaceBaselineEntry{
					FilePath:   finding.FilePath,
					Symbol:     finding.Symbol,
					Shape:      finding.Shape,
					ImportPath: finding.ImportPath,
					Count:      1,
				},
				firstIdx: index,
			}
			order = append(order, key)
			continue
		}
		existing.entry.Count++
	}
	entries := make([]petriPublicSurfaceBaselineEntry, 0, len(order))
	for _, key := range order {
		entries = append(entries, byKey[key].entry)
	}
	slices.SortFunc(entries, func(left, right petriPublicSurfaceBaselineEntry) int {
		if order := strings.Compare(left.FilePath, right.FilePath); order != 0 {
			return order
		}
		if order := strings.Compare(left.Symbol, right.Symbol); order != 0 {
			return order
		}
		return strings.Compare(left.ImportPath, right.ImportPath)
	})
	return entries
}

func loadPetriPublicSurfaceBaseline(repoRoot string) (petriPublicSurfaceBaseline, error) {
	payload, err := os.ReadFile(filepath.Join(repoRoot, petriPublicSurfaceBaselinePath))
	if os.IsNotExist(err) {
		return petriPublicSurfaceBaseline{}, nil
	}
	if err != nil {
		return petriPublicSurfaceBaseline{}, fmt.Errorf("read Petri public surface baseline: %w", err)
	}
	var baseline petriPublicSurfaceBaseline
	if err := json.Unmarshal(payload, &baseline); err != nil {
		return petriPublicSurfaceBaseline{}, fmt.Errorf("decode Petri public surface baseline: %w", err)
	}
	if baseline.Version != 1 {
		return petriPublicSurfaceBaseline{}, fmt.Errorf("Petri public surface baseline version = %d, want 1", baseline.Version)
	}
	if err := requireNonEmptyMigrationBaseline(petriPublicSurfaceBaselinePath, len(baseline.Entries)); err != nil {
		return petriPublicSurfaceBaseline{}, err
	}
	return baseline, nil
}

func partitionPetriPublicSurfaceFindings(
	findings []petriPublicSurfaceFinding,
	baseline petriPublicSurfaceBaseline,
) ([]petriPublicSurfaceFinding, []petriPublicSurfaceBaselineEntry, error) {
	baselineByKey := make(map[string]petriPublicSurfaceBaselineEntry, len(baseline.Entries))
	for _, entry := range baseline.Entries {
		if err := validatePetriPublicSurfaceBaselineEntry(entry); err != nil {
			return nil, nil, err
		}
		key := petriPublicSurfaceKey(entry.FilePath, entry.Symbol, entry.ImportPath)
		if _, duplicate := baselineByKey[key]; duplicate {
			return nil, nil, fmt.Errorf(
				"duplicate Petri public surface baseline entry: %s %s %s",
				entry.FilePath,
				entry.Symbol,
				entry.ImportPath,
			)
		}
		baselineByKey[key] = entry
	}

	aggregated := aggregatePetriPublicSurfaceFindings(findings)
	blockingKeys := map[string]struct{}{}
	seen := map[string]struct{}{}
	for _, edge := range aggregated {
		key := petriPublicSurfaceKey(edge.FilePath, edge.Symbol, edge.ImportPath)
		seen[key] = struct{}{}
		entry, recorded := baselineByKey[key]
		if !recorded || entry.Count != edge.Count || entry.Shape != edge.Shape {
			blockingKeys[key] = struct{}{}
		}
	}

	var blocking []petriPublicSurfaceFinding
	for _, finding := range findings {
		key := petriPublicSurfaceKey(finding.FilePath, finding.Symbol, finding.ImportPath)
		if _, blocked := blockingKeys[key]; blocked {
			blocking = append(blocking, finding)
		}
	}

	var stale []petriPublicSurfaceBaselineEntry
	for key, entry := range baselineByKey {
		if _, found := seen[key]; !found {
			stale = append(stale, entry)
		}
	}
	slices.SortFunc(stale, func(left, right petriPublicSurfaceBaselineEntry) int {
		return strings.Compare(
			petriPublicSurfaceKey(left.FilePath, left.Symbol, left.ImportPath),
			petriPublicSurfaceKey(right.FilePath, right.Symbol, right.ImportPath),
		)
	})
	return blocking, stale, nil
}

func validatePetriPublicSurfaceBaselineEntry(entry petriPublicSurfaceBaselineEntry) error {
	if entry.FilePath == "" || entry.Symbol == "" || entry.Shape == "" || entry.ImportPath == "" ||
		entry.Count < 1 || entry.Stage != petriPublicSurfaceBaselineStage ||
		entry.DeletionGate != petriPublicSurfaceDeletionGate {
		return fmt.Errorf("Petri public surface baseline entry is incomplete or has an unrecognized deletion gate: %#v", entry)
	}
	filePath := filepath.ToSlash(entry.FilePath)
	if strings.ContainsAny(filePath, "*?[]") || !strings.HasSuffix(filePath, ".go") {
		return fmt.Errorf("Petri public surface baseline entry must be an exact Go file path: %s", entry.FilePath)
	}
	if isFactoryRuntimeInternalPath(filePath) {
		return fmt.Errorf("Petri public surface baseline entry must not cover Factory Runtime internals: %s", entry.FilePath)
	}
	if strings.ContainsAny(entry.Symbol, "*?[]") {
		return fmt.Errorf("Petri public surface baseline entry must be exact and cannot contain wildcards: %#v", entry)
	}
	shape, prohibited := prohibitedPetriPublicSurfaceSymbols[entry.Symbol]
	if !prohibited || shape != entry.Shape {
		return fmt.Errorf("Petri public surface baseline entry names an unrecognized rule: %s %s", entry.Symbol, entry.Shape)
	}
	if !isPetriPublicSurfaceWatchedImport(entry.ImportPath) {
		return fmt.Errorf("Petri public surface baseline entry names an unwatched import: %s", entry.ImportPath)
	}
	return nil
}

func createPetriPublicSurfaceBaseline(cfg config) error {
	repoRoot, err := filepath.Abs(cfg.root)
	if err != nil {
		return fmt.Errorf("resolve repo root: %w", err)
	}
	path := filepath.Join(repoRoot, petriPublicSurfaceBaselinePath)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("create Petri public surface baseline: %w", err)
	}
	findings, err := scanPetriPublicSurface(repoRoot)
	if err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return err
	}
	if len(findings) == 0 {
		_ = file.Close()
		_ = os.Remove(path)
		return fmt.Errorf("refuse to create empty %s; absence is the zero-debt state", petriPublicSurfaceBaselinePath)
	}
	baseline := petriPublicSurfaceBaseline{Version: 1}
	for _, edge := range aggregatePetriPublicSurfaceFindings(findings) {
		baseline.Entries = append(baseline.Entries, petriPublicSurfaceBaselineEntry{
			FilePath:     edge.FilePath,
			Symbol:       edge.Symbol,
			Shape:        edge.Shape,
			ImportPath:   edge.ImportPath,
			Count:        edge.Count,
			Stage:        petriPublicSurfaceBaselineStage,
			DeletionGate: petriPublicSurfaceDeletionGate,
		})
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(baseline); err != nil {
		_ = file.Close()
		return fmt.Errorf("encode Petri public surface baseline: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close Petri public surface baseline: %w", err)
	}
	fmt.Fprintf(
		stdoutWriter,
		"[agent-factory:pkg-boundary] created %s with %d deletion-only edge(s)\n",
		petriPublicSurfaceBaselinePath,
		len(baseline.Entries),
	)
	return nil
}

func writeStalePetriPublicSurfaceBaselineEntries(writer io.Writer, entries []petriPublicSurfaceBaselineEntry) {
	for _, entry := range entries {
		fmt.Fprintf(
			writer,
			"[agent-factory:pkg-boundary] stale Petri public surface baseline entry: %s %s %s\n",
			entry.FilePath,
			entry.Symbol,
			entry.ImportPath,
		)
		fmt.Fprintf(writer, "  remediation: remove this entry from %s in the same change.\n", petriPublicSurfaceBaselinePath)
	}
}

func writePetriPublicSurfaceBaselineSummary(writer io.Writer, count int) {
	if count > 0 {
		fmt.Fprintf(
			writer,
			"[agent-factory:pkg-boundary] active Petri public-surface migration baseline: %d exact file/symbol edge(s)\n",
			count,
		)
		fmt.Fprintln(
			writer,
			"  deletion gate: retire each leak under Runtime Petri-boundary retirement / IMP-RUN-01, then delete its exact baseline entry.",
		)
	}
}
