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

type functionalBoundaryFunction struct {
	functionalBoundaryImport
	symbol     string
	resultType string
}

type functionalBoundaryIdentifiers struct {
	types     map[string]functionalBoundaryType
	functions map[string]functionalBoundaryFunction
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

	identifiers := functionalBoundaryIdentifiersByFunction(parsed, imports)
	var violations []FunctionalBoundaryViolation
	ast.Inspect(parsed, func(node ast.Node) bool {
		if literal, ok := node.(*ast.CompositeLit); ok {
			if boundaryType, found := functionalBoundaryTypeOf(literal.Type, imports); found {
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
		if identifier, ok := call.Fun.(*ast.Ident); ok {
			boundaryFunction, found := functionalBoundaryFunctionIdentifier(parsed, identifiers, call.Pos(), identifier.Name)
			if found {
				violations = append(violations, newFunctionalBoundaryViolation(
					relativePath, fileSet.Position(call.Pos()).Line, boundaryFunction.functionalBoundaryImport,
					boundaryFunction.path+"."+boundaryFunction.symbol,
				))
			}
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
			violations = append(violations, newFunctionalBoundaryViolation(
				relativePath, fileSet.Position(call.Pos()).Line, boundaryImport,
				boundaryImport.path+"."+selector.Sel.Name,
			))
			return true
		}
		boundaryType, typedCall := functionalBoundaryTypedIdentifier(parsed, identifiers, call.Pos(), qualifier.Name)
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

func functionalBoundaryIdentifiersByFunction(file *ast.File, imports map[string]functionalBoundaryImport) map[*ast.FuncDecl]functionalBoundaryIdentifiers {
	functions := make(map[*ast.FuncDecl]functionalBoundaryIdentifiers)
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok {
			continue
		}
		identifiers := functionalBoundaryIdentifiers{
			types:     make(map[string]functionalBoundaryType),
			functions: make(map[string]functionalBoundaryFunction),
		}
		for _, fields := range []*ast.FieldList{function.Recv, function.Type.Params, function.Type.Results} {
			addFunctionalBoundaryFields(identifiers.types, fields, imports)
		}
		ast.Inspect(function.Body, func(node ast.Node) bool {
			switch statement := node.(type) {
			case *ast.DeclStmt:
				general, ok := statement.Decl.(*ast.GenDecl)
				if !ok {
					return true
				}
				for _, specification := range general.Specs {
					value, ok := specification.(*ast.ValueSpec)
					if !ok {
						continue
					}
					if value.Type != nil {
						addFunctionalBoundaryNames(identifiers.types, value.Names, value.Type, imports)
					}
					addFunctionalBoundaryInferredValues(identifiers, value.Names, value.Values, imports)
				}
			case *ast.AssignStmt:
				names := make([]*ast.Ident, 0, len(statement.Lhs))
				for _, expression := range statement.Lhs {
					identifier, ok := expression.(*ast.Ident)
					if !ok {
						return true
					}
					names = append(names, identifier)
				}
				addFunctionalBoundaryInferredValues(identifiers, names, statement.Rhs, imports)
			}
			return true
		})
		functions[function] = identifiers
	}
	return functions
}

func addFunctionalBoundaryInferredValues(
	identifiers functionalBoundaryIdentifiers,
	names []*ast.Ident,
	values []ast.Expr,
	imports map[string]functionalBoundaryImport,
) {
	if len(names) != len(values) {
		return
	}
	for index, value := range values {
		if boundaryType, found := functionalBoundaryTypeFromValue(value, imports, identifiers.functions); found {
			identifiers.types[names[index].Name] = boundaryType
		}
		if boundaryFunction, found := functionalBoundaryFunctionOf(value, imports); found {
			identifiers.functions[names[index].Name] = boundaryFunction
		}
	}
}

func functionalBoundaryTypeFromValue(
	expression ast.Expr,
	imports map[string]functionalBoundaryImport,
	functions map[string]functionalBoundaryFunction,
) (functionalBoundaryType, bool) {
	if literal, ok := expression.(*ast.CompositeLit); ok {
		return functionalBoundaryTypeOf(literal.Type, imports)
	}
	call, ok := expression.(*ast.CallExpr)
	if !ok {
		return functionalBoundaryType{}, false
	}
	if selector, ok := call.Fun.(*ast.SelectorExpr); ok {
		qualifier, ok := selector.X.(*ast.Ident)
		if !ok {
			return functionalBoundaryType{}, false
		}
		boundaryImport, found := imports[qualifier.Name]
		if !found {
			return functionalBoundaryType{}, false
		}
		return functionalBoundaryType{functionalBoundaryImport: boundaryImport, name: inferredBoundaryResultType(selector.Sel.Name)}, true
	}
	identifier, ok := call.Fun.(*ast.Ident)
	if !ok {
		return functionalBoundaryType{}, false
	}
	boundaryFunction, found := functions[identifier.Name]
	if !found {
		return functionalBoundaryType{}, false
	}
	return functionalBoundaryType{functionalBoundaryImport: boundaryFunction.functionalBoundaryImport, name: boundaryFunction.resultType}, true
}

func functionalBoundaryFunctionOf(expression ast.Expr, imports map[string]functionalBoundaryImport) (functionalBoundaryFunction, bool) {
	selector, ok := expression.(*ast.SelectorExpr)
	if !ok {
		return functionalBoundaryFunction{}, false
	}
	qualifier, ok := selector.X.(*ast.Ident)
	if !ok {
		return functionalBoundaryFunction{}, false
	}
	boundaryImport, found := imports[qualifier.Name]
	if !found {
		return functionalBoundaryFunction{}, false
	}
	return functionalBoundaryFunction{
		functionalBoundaryImport: boundaryImport,
		symbol:                   selector.Sel.Name,
		resultType:               inferredBoundaryResultType(selector.Sel.Name),
	}, true
}

func inferredBoundaryResultType(symbol string) string {
	return strings.TrimPrefix(symbol, "New")
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
	if !found {
		return
	}
	for _, name := range names {
		identifiers[name.Name] = boundaryType
	}
}

func functionalBoundaryTypedIdentifier(file *ast.File, identifiers map[*ast.FuncDecl]functionalBoundaryIdentifiers, position token.Pos, name string) (functionalBoundaryType, bool) {
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || position < function.Pos() || position > function.End() {
			continue
		}
		boundaryType, found := identifiers[function].types[name]
		return boundaryType, found
	}
	return functionalBoundaryType{}, false
}

func functionalBoundaryFunctionIdentifier(file *ast.File, identifiers map[*ast.FuncDecl]functionalBoundaryIdentifiers, position token.Pos, name string) (functionalBoundaryFunction, bool) {
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || position < function.Pos() || position > function.End() {
			continue
		}
		boundaryFunction, found := identifiers[function].functions[name]
		return boundaryFunction, found
	}
	return functionalBoundaryFunction{}, false
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
	case path == "pkg/wire" || strings.HasPrefix(path, "pkg/wire/"):
		return "composition"
	case path == "pkg/transports/mapping":
		return "API surface"
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
	case path == "pkg/replay" || path == "pkg/factory/replay" || strings.HasPrefix(path, "pkg/factory/replay/") || strings.Contains(path, "/recording"):
		return "recorder"
	case strings.Contains(path, "/projections"):
		return "projection"
	default:
		return ""
	}
}
