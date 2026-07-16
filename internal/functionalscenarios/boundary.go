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

// CheckFunctionalTestBoundaries statically checks Go sources under
// tests/functional. It does not type-check, execute tests, or contact a live
// service. Approved client, contract, composition, and external-edge packages
// are excluded before implementation-use inspection.
func CheckFunctionalTestBoundaries(repositoryRoot string) error {
	_, err := CheckFunctionalTestBoundariesReport(repositoryRoot)
	return err
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

type functionalBoundaryIdentifiers struct {
	implementations map[string]functionalBoundaryType
	localTypes      map[string]string
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

	parents := functionalBoundaryParents(parsed)
	typedIdentifiers := functionalBoundaryTypedIdentifiers(parsed, imports)
	structFields := functionalBoundaryStructFields(parsed, imports)
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
			selector, isSelector := node.(*ast.SelectorExpr)
			if !isSelector || !functionalBoundaryValueReference(selector, parents) {
				return true
			}
			qualifier, isQualifier := selector.X.(*ast.Ident)
			boundaryImport, imported := imports[qualifierName(qualifier, isQualifier)]
			if !imported || allowedBoundarySymbol(boundaryImport.path, selector.Sel.Name) {
				return true
			}
			violations = append(violations, newFunctionalBoundaryViolation(
				relativePath, fileSet.Position(selector.Pos()).Line, boundaryImport,
				boundaryImport.path+"."+selector.Sel.Name,
			))
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		qualifier, isQualifier := selector.X.(*ast.Ident)
		boundaryImport, importedCall := imports[qualifierName(qualifier, isQualifier)]
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
		boundaryType, typedCall := functionalBoundaryReceiverType(
			parsed, typedIdentifiers, structFields, call.Pos(), selector.X,
		)
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

func qualifierName(qualifier *ast.Ident, ok bool) string {
	if !ok {
		return ""
	}
	return qualifier.Name
}

func functionalBoundaryParents(root ast.Node) map[ast.Node]ast.Node {
	parents := make(map[ast.Node]ast.Node)
	var stack []ast.Node
	ast.Inspect(root, func(node ast.Node) bool {
		if node == nil {
			stack = stack[:len(stack)-1]
			return false
		}
		if len(stack) > 0 {
			parents[node] = stack[len(stack)-1]
		}
		stack = append(stack, node)
		return true
	})
	return parents
}

func functionalBoundaryValueReference(selector *ast.SelectorExpr, parents map[ast.Node]ast.Node) bool {
	for node := ast.Node(selector); parents[node] != nil; node = parents[node] {
		parent := parents[node]
		switch typed := parent.(type) {
		case *ast.CallExpr:
			return typed.Fun != node
		case *ast.AssignStmt:
			return slices.Contains(typed.Rhs, node.(ast.Expr))
		case *ast.ValueSpec:
			return slices.Contains(typed.Values, node.(ast.Expr))
		case *ast.ReturnStmt:
			return slices.Contains(typed.Results, node.(ast.Expr))
		case *ast.KeyValueExpr:
			return typed.Value == node
		case *ast.CompositeLit:
			return typed.Type != node
		case *ast.ParenExpr, *ast.IndexExpr, *ast.IndexListExpr:
			continue
		default:
			return false
		}
	}
	return false
}

func newFunctionalBoundaryViolation(relativePath string, line int, boundaryImport functionalBoundaryImport, symbol string) FunctionalBoundaryViolation {
	return FunctionalBoundaryViolation{
		File: filepath.ToSlash(relativePath), Line: line,
		Boundary: boundaryImport.boundary, Symbol: symbol,
	}
}

func functionalBoundaryTypedIdentifiers(file *ast.File, imports map[string]functionalBoundaryImport) map[*ast.FuncDecl]functionalBoundaryIdentifiers {
	functions := make(map[*ast.FuncDecl]functionalBoundaryIdentifiers)
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok {
			continue
		}
		identifiers := functionalBoundaryIdentifiers{
			implementations: make(map[string]functionalBoundaryType),
			localTypes:      make(map[string]string),
		}
		for _, fields := range []*ast.FieldList{function.Recv, function.Type.Params, function.Type.Results} {
			addFunctionalBoundaryFields(identifiers.implementations, fields, imports)
			addFunctionalLocalTypeFields(identifiers.localTypes, fields)
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
				addFunctionalBoundaryNames(identifiers.implementations, value.Names, value.Type, imports)
				addFunctionalLocalTypeNames(identifiers.localTypes, value.Names, value.Type)
			}
			return true
		})
		functions[function] = identifiers
	}
	return functions
}

func addFunctionalLocalTypeFields(identifiers map[string]string, fields *ast.FieldList) {
	if fields == nil {
		return
	}
	for _, field := range fields.List {
		addFunctionalLocalTypeNames(identifiers, field.Names, field.Type)
	}
}

func addFunctionalLocalTypeNames(identifiers map[string]string, names []*ast.Ident, expression ast.Expr) {
	for star, ok := expression.(*ast.StarExpr); ok; star, ok = expression.(*ast.StarExpr) {
		expression = star.X
	}
	typeName, ok := expression.(*ast.Ident)
	if !ok {
		return
	}
	for _, name := range names {
		identifiers[name.Name] = typeName.Name
	}
}

func functionalBoundaryStructFields(file *ast.File, imports map[string]functionalBoundaryImport) map[string]map[string]functionalBoundaryType {
	structs := make(map[string]map[string]functionalBoundaryType)
	for _, declaration := range file.Decls {
		general, ok := declaration.(*ast.GenDecl)
		if !ok || general.Tok != token.TYPE {
			continue
		}
		for _, specification := range general.Specs {
			typeSpec, ok := specification.(*ast.TypeSpec)
			structure, isStruct := typeSpec.Type.(*ast.StructType)
			if !ok || !isStruct {
				continue
			}
			fields := make(map[string]functionalBoundaryType)
			for _, field := range structure.Fields.List {
				boundaryType, found := functionalBoundaryTypeOf(field.Type, imports)
				if !found || allowedBoundaryType(boundaryType.name) {
					continue
				}
				for _, name := range field.Names {
					fields[name.Name] = boundaryType
				}
			}
			structs[typeSpec.Name.Name] = fields
		}
	}
	return structs
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

func functionalBoundaryReceiverType(file *ast.File, identifiers map[*ast.FuncDecl]functionalBoundaryIdentifiers, structFields map[string]map[string]functionalBoundaryType, position token.Pos, expression ast.Expr) (functionalBoundaryType, bool) {
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || position < function.Pos() || position > function.End() {
			continue
		}
		scope := identifiers[function]
		if identifier, direct := expression.(*ast.Ident); direct {
			boundaryType, found := scope.implementations[identifier.Name]
			return boundaryType, found
		}
		selector, nested := expression.(*ast.SelectorExpr)
		if !nested {
			return functionalBoundaryType{}, false
		}
		root, rooted := selector.X.(*ast.Ident)
		if !rooted {
			return functionalBoundaryType{}, false
		}
		localType, found := scope.localTypes[root.Name]
		if !found {
			return functionalBoundaryType{}, false
		}
		boundaryType, found := structFields[localType][selector.Sel.Name]
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
