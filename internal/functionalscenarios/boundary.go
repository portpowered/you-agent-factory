package functionalscenarios

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

const (
	FunctionalBoundaryViolationCategory = "direct-product-boundary"
	repositoryImportPrefix              = "github.com/portpowered/infinite-you/"
)

// FunctionalBoundaryViolation identifies one direct implementation boundary
// used as an alternate product interface by a functional test.
type FunctionalBoundaryViolation struct {
	File     string
	Line     int
	Boundary string
	Symbol   string
}

// FunctionalBoundaryReport identifies exact legacy files held behind the
// reviewed content-hash baseline. New or changed violations still fail.
type FunctionalBoundaryReport struct {
	BaselinedLegacyFiles int
}

func (violation FunctionalBoundaryViolation) Error() string {
	return fmt.Sprintf(
		"functional test boundary [%s]: %s:%d directly uses %s implementation %q; invoke or observe the product through REST, CLI, MCP, or SSE, and keep composition options or injected edge fakes in functional support",
		FunctionalBoundaryViolationCategory,
		violation.File,
		violation.Line,
		violation.Boundary,
		violation.Symbol,
	)
}

// CheckFunctionalTestBoundariesReport checks the complete functional tree and
// reports exact unchanged legacy files quarantined by the reviewed baseline.
func CheckFunctionalTestBoundariesReport(repositoryRoot string) (FunctionalBoundaryReport, error) {
	scanRoot := filepath.Join(repositoryRoot, "tests", "functional")
	var violations []FunctionalBoundaryViolation
	err := filepath.WalkDir(scanRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" {
			return nil
		}
		fileViolations, err := checkFunctionalBoundaryFile(repositoryRoot, path)
		if err != nil {
			return err
		}
		violations = append(violations, fileViolations...)
		return nil
	})
	if err != nil {
		return FunctionalBoundaryReport{}, fmt.Errorf("scan functional test boundaries: %w", err)
	}
	baseline, err := loadFunctionalBoundaryBaseline(repositoryRoot)
	if err != nil {
		return FunctionalBoundaryReport{}, err
	}
	legacyFiles, violations, err := applyFunctionalBoundaryBaseline(repositoryRoot, violations, baseline)
	if err != nil {
		return FunctionalBoundaryReport{}, err
	}
	return FunctionalBoundaryReport{BaselinedLegacyFiles: legacyFiles}, functionalBoundaryViolationsError(violations)
}

func functionalBoundaryViolationsError(violations []FunctionalBoundaryViolation) error {
	if len(violations) == 0 {
		return nil
	}
	slices.SortFunc(violations, func(left, right FunctionalBoundaryViolation) int {
		if comparison := strings.Compare(left.File, right.File); comparison != 0 {
			return comparison
		}
		if left.Line != right.Line {
			return left.Line - right.Line
		}
		return strings.Compare(left.Symbol, right.Symbol)
	})
	errs := make([]error, len(violations))
	for index := range violations {
		errs[index] = violations[index]
	}
	return errors.Join(errs...)
}

type functionalBoundaryImport struct {
	path     string
	boundary string
}

type functionalBoundaryType struct {
	functionalBoundaryImport
	name string
}

func checkFunctionalBoundaryFile(repositoryRoot, path string) ([]FunctionalBoundaryViolation, error) {
	fileSet := token.NewFileSet()
	parsed, err := parser.ParseFile(fileSet, path, nil, parser.SkipObjectResolution)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", filepath.ToSlash(path), err)
	}
	imports := functionalBoundaryImports(parsed)
	if len(imports) == 0 {
		return nil, nil
	}
	relativePath, err := filepath.Rel(repositoryRoot, path)
	if err != nil {
		return nil, fmt.Errorf("resolve functional test path %s: %w", filepath.ToSlash(path), err)
	}

	typedIdentifiers := functionalBoundaryTypedIdentifiers(parsed, imports)
	var violations []FunctionalBoundaryViolation
	ast.Inspect(parsed, func(node ast.Node) bool {
		if literal, ok := node.(*ast.CompositeLit); ok {
			if boundaryType, found := functionalBoundaryTypeOf(literal.Type, imports); found && !allowedBoundaryType(boundaryType.name) {
				violations = append(violations, newFunctionalBoundaryViolation(
					relativePath, fileSet.Position(literal.Pos()).Line, boundaryType.functionalBoundaryImport,
					boundaryType.path+"."+boundaryType.name,
				))
			}
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		qualifier, ok := selector.X.(*ast.Ident)
		if !ok {
			return true
		}
		boundaryImport, importedCall := imports[qualifier.Name]
		if importedCall {
			if allowedBoundarySymbol(boundaryImport.path, selector.Sel.Name) {
				return true
			}
			violations = append(violations, newFunctionalBoundaryViolation(
				relativePath, fileSet.Position(call.Pos()).Line, boundaryImport,
				boundaryImport.path+"."+selector.Sel.Name,
			))
			return true
		}
		boundaryType, typedCall := functionalBoundaryTypedIdentifier(parsed, typedIdentifiers, call.Pos(), qualifier.Name)
		if !typedCall {
			return true
		}
		violations = append(violations, newFunctionalBoundaryViolation(
			relativePath, fileSet.Position(call.Pos()).Line, boundaryType.functionalBoundaryImport,
			boundaryType.path+"."+boundaryType.name+"."+selector.Sel.Name,
		))
		return true
	})
	return violations, nil
}

func newFunctionalBoundaryViolation(relativePath string, line int, boundaryImport functionalBoundaryImport, symbol string) FunctionalBoundaryViolation {
	return FunctionalBoundaryViolation{
		File: filepath.ToSlash(relativePath), Line: line,
		Boundary: boundaryImport.boundary, Symbol: symbol,
	}
}

func functionalBoundaryTypedIdentifiers(file *ast.File, imports map[string]functionalBoundaryImport) map[*ast.FuncDecl]map[string]functionalBoundaryType {
	functions := make(map[*ast.FuncDecl]map[string]functionalBoundaryType)
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok {
			continue
		}
		identifiers := make(map[string]functionalBoundaryType)
		for _, fields := range []*ast.FieldList{function.Recv, function.Type.Params, function.Type.Results} {
			addFunctionalBoundaryFields(identifiers, fields, imports)
		}
		ast.Inspect(function.Body, func(node ast.Node) bool {
			declaration, ok := node.(*ast.DeclStmt)
			if !ok {
				return true
			}
			general, ok := declaration.Decl.(*ast.GenDecl)
			if !ok {
				return true
			}
			for _, specification := range general.Specs {
				value, ok := specification.(*ast.ValueSpec)
				if !ok || value.Type == nil {
					continue
				}
				addFunctionalBoundaryNames(identifiers, value.Names, value.Type, imports)
			}
			return true
		})
		functions[function] = identifiers
	}
	return functions
}

func addFunctionalBoundaryFields(identifiers map[string]functionalBoundaryType, fields *ast.FieldList, imports map[string]functionalBoundaryImport) {
	if fields == nil {
		return
	}
	for _, field := range fields.List {
		addFunctionalBoundaryNames(identifiers, field.Names, field.Type, imports)
	}
}

func addFunctionalBoundaryNames(identifiers map[string]functionalBoundaryType, names []*ast.Ident, expression ast.Expr, imports map[string]functionalBoundaryImport) {
	boundaryType, found := functionalBoundaryTypeOf(expression, imports)
	if !found || allowedBoundaryType(boundaryType.name) {
		return
	}
	for _, name := range names {
		identifiers[name.Name] = boundaryType
	}
}

func functionalBoundaryTypedIdentifier(file *ast.File, identifiers map[*ast.FuncDecl]map[string]functionalBoundaryType, position token.Pos, name string) (functionalBoundaryType, bool) {
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || position < function.Pos() || position > function.End() {
			continue
		}
		boundaryType, found := identifiers[function][name]
		return boundaryType, found
	}
	return functionalBoundaryType{}, false
}

func functionalBoundaryTypeOf(expression ast.Expr, imports map[string]functionalBoundaryImport) (functionalBoundaryType, bool) {
	for {
		switch typed := expression.(type) {
		case *ast.StarExpr:
			expression = typed.X
		case *ast.ArrayType:
			expression = typed.Elt
		default:
			selector, ok := expression.(*ast.SelectorExpr)
			if !ok {
				return functionalBoundaryType{}, false
			}
			qualifier, ok := selector.X.(*ast.Ident)
			if !ok {
				return functionalBoundaryType{}, false
			}
			boundaryImport, ok := imports[qualifier.Name]
			if !ok {
				return functionalBoundaryType{}, false
			}
			return functionalBoundaryType{functionalBoundaryImport: boundaryImport, name: selector.Sel.Name}, true
		}
	}
}

func functionalBoundaryImports(file *ast.File) map[string]functionalBoundaryImport {
	imports := make(map[string]functionalBoundaryImport)
	for _, specification := range file.Imports {
		importPath, err := strconv.Unquote(specification.Path.Value)
		if err != nil {
			continue
		}
		boundary := prohibitedFunctionalBoundary(importPath)
		if boundary == "" {
			continue
		}
		name := defaultFunctionalImportName(importPath)
		if specification.Name != nil {
			name = specification.Name.Name
		}
		if name != "_" && name != "." {
			imports[name] = functionalBoundaryImport{path: importPath, boundary: boundary}
		}
	}
	return imports
}

func defaultFunctionalImportName(importPath string) string {
	if importPath == repositoryImportPrefix+"pkg/transports/http" {
		return "api"
	}
	return filepath.Base(importPath)
}

func prohibitedFunctionalBoundary(importPath string) string {
	if !strings.HasPrefix(importPath, repositoryImportPrefix) {
		return ""
	}
	path := strings.TrimPrefix(importPath, repositoryImportPrefix)
	switch {
	case strings.Contains(path, "poller"):
		return "poller"
	case strings.Contains(path, "supervisor"):
		return "supervisor"
	case path == "pkg/service" || strings.HasPrefix(path, "pkg/service/"):
		return "service"
	case path == "pkg/factory/runtime" || strings.HasPrefix(path, "pkg/factory/runtime/") || path == "pkg/runtimehost":
		return "runtime"
	case path == "pkg/transports/http":
		return "handler"
	case strings.Contains(path, "repository") || strings.Contains(path, "persistence") || strings.Contains(path, "runtimepersist"):
		return "repository"
	case path == "pkg/replay" || strings.Contains(path, "/recording"):
		return "recorder"
	case strings.Contains(path, "/projections"):
		return "projection"
	default:
		return ""
	}
}

func allowedBoundarySymbol(importPath, symbol string) bool {
	// Public types are not calls. These constructors produce configuration or
	// option values only and do not expose a live product implementation.
	if strings.HasPrefix(symbol, "New") &&
		(strings.HasSuffix(symbol, "Config") || strings.HasSuffix(symbol, "Option") || strings.HasSuffix(symbol, "Options")) {
		return true
	}
	// Replay conversion and projection helpers produce public assertion data;
	// they do not construct or drive the product runtime.
	if importPath == repositoryImportPrefix+"pkg/replay" &&
		(symbol == "RuntimeConfigFromGeneratedFactory" || strings.Contains(symbol, "ArtifactFrom")) {
		return true
	}
	return false
}

func allowedBoundaryType(name string) bool {
	return strings.HasSuffix(name, "Config") || strings.HasSuffix(name, "Option") || strings.HasSuffix(name, "Options")
}
