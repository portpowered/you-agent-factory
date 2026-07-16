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

type functionalBoundaryDotImport struct {
	functionalBoundaryImport
	line int
}

type functionalBoundaryType struct {
	functionalBoundaryImport
	name string
}

type functionalBoundaryIdentifiers struct {
	implementations map[string]functionalBoundaryType
	localTypes      map[string]ast.Expr
}

func checkFunctionalBoundaryFile(repositoryRoot, path string) ([]FunctionalBoundaryViolation, error) {
	fileSet := token.NewFileSet()
	parsed, err := parser.ParseFile(fileSet, path, nil, parser.SkipObjectResolution)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", filepath.ToSlash(path), err)
	}
	imports := functionalBoundaryImports(parsed)
	dotImports := functionalBoundaryDotImports(parsed, fileSet)
	if len(imports) == 0 && len(dotImports) == 0 {
		return nil, nil
	}
	relativePath, err := filepath.Rel(repositoryRoot, path)
	if err != nil {
		return nil, fmt.Errorf("resolve functional test path %s: %w", filepath.ToSlash(path), err)
	}

	parents := functionalBoundaryParents(parsed)
	typeDeclarations := functionalBoundaryTypeDeclarations(parsed)
	typedIdentifiers := functionalBoundaryTypedIdentifiers(parsed, imports, typeDeclarations)
	structFields := functionalBoundaryStructFields(parsed, imports)
	violations := make([]FunctionalBoundaryViolation, 0, len(dotImports))
	for _, dotImport := range dotImports {
		violations = append(violations, newFunctionalBoundaryViolation(
			relativePath, dotImport.line, dotImport.functionalBoundaryImport,
			dotImport.path+" (dot import)",
		))
	}
	ast.Inspect(parsed, func(node ast.Node) bool {
		if literal, ok := node.(*ast.CompositeLit); ok {
			if boundaryType, found := functionalBoundaryTypeOf(literal.Type, imports, typeDeclarations); found && !allowedBoundaryType(boundaryType.name) {
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
			if boundaryImport, imported := functionalBoundaryImportedSelector(selector, imports); imported {
				if !allowedBoundarySymbol(boundaryImport.path, selector.Sel.Name) {
					violations = append(violations, newFunctionalBoundaryViolation(relativePath, fileSet.Position(selector.Pos()).Line, boundaryImport, boundaryImport.path+"."+selector.Sel.Name))
				}
				return true
			}
			if boundaryType, found := functionalBoundaryTypeOf(selector.X, imports, typeDeclarations); found && !allowedBoundaryType(boundaryType.name) {
				violations = append(violations, newFunctionalBoundaryViolation(relativePath, fileSet.Position(selector.Pos()).Line, boundaryType.functionalBoundaryImport, boundaryType.path+"."+boundaryType.name+"."+selector.Sel.Name))
			}
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		boundaryImport, importedCall := functionalBoundaryImportedSelector(selector, imports)
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
			parsed, typedIdentifiers, structFields, typeDeclarations, imports, call.Pos(), selector.X,
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

func functionalBoundaryImportedSelector(selector *ast.SelectorExpr, imports map[string]functionalBoundaryImport) (functionalBoundaryImport, bool) {
	qualifier, isQualifier := selector.X.(*ast.Ident)
	boundaryImport, imported := imports[qualifierName(qualifier, isQualifier)]
	return boundaryImport, imported
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

func functionalBoundaryTypedIdentifiers(file *ast.File, imports map[string]functionalBoundaryImport, typeDeclarations map[string]ast.Expr) map[*ast.FuncDecl]functionalBoundaryIdentifiers {
	functions := make(map[*ast.FuncDecl]functionalBoundaryIdentifiers)
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok {
			continue
		}
		identifiers := functionalBoundaryIdentifiers{
			implementations: make(map[string]functionalBoundaryType),
			localTypes:      make(map[string]ast.Expr),
		}
		for _, fields := range []*ast.FieldList{function.Recv, function.Type.Params, function.Type.Results} {
			addFunctionalBoundaryFields(identifiers.implementations, fields, imports, typeDeclarations)
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
				addFunctionalBoundaryNames(identifiers.implementations, value.Names, value.Type, imports, typeDeclarations)
				addFunctionalLocalTypeNames(identifiers.localTypes, value.Names, value.Type)
			}
			return true
		})
		functions[function] = identifiers
	}
	return functions
}

func addFunctionalLocalTypeFields(identifiers map[string]ast.Expr, fields *ast.FieldList) {
	if fields == nil {
		return
	}
	for _, field := range fields.List {
		addFunctionalLocalTypeNames(identifiers, field.Names, field.Type)
	}
}

func addFunctionalLocalTypeNames(identifiers map[string]ast.Expr, names []*ast.Ident, expression ast.Expr) {
	for _, name := range names {
		identifiers[name.Name] = expression
	}
}

func functionalBoundaryTypeDeclarations(file *ast.File) map[string]ast.Expr {
	declarations := make(map[string]ast.Expr)
	for _, declaration := range file.Decls {
		general, ok := declaration.(*ast.GenDecl)
		if !ok || general.Tok != token.TYPE {
			continue
		}
		for _, specification := range general.Specs {
			if typeSpec, ok := specification.(*ast.TypeSpec); ok {
				declarations[typeSpec.Name.Name] = typeSpec.Type
			}
		}
	}
	return declarations
}

func functionalBoundaryStructFields(file *ast.File, imports map[string]functionalBoundaryImport) map[string]map[string]ast.Expr {
	structs := make(map[string]map[string]ast.Expr)
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
			fields := make(map[string]ast.Expr)
			for _, field := range structure.Fields.List {
				for _, name := range field.Names {
					fields[name.Name] = field.Type
				}
			}
			structs[typeSpec.Name.Name] = fields
		}
	}
	return structs
}

func addFunctionalBoundaryFields(identifiers map[string]functionalBoundaryType, fields *ast.FieldList, imports map[string]functionalBoundaryImport, typeDeclarations map[string]ast.Expr) {
	if fields == nil {
		return
	}
	for _, field := range fields.List {
		addFunctionalBoundaryNames(identifiers, field.Names, field.Type, imports, typeDeclarations)
	}
}

func addFunctionalBoundaryNames(identifiers map[string]functionalBoundaryType, names []*ast.Ident, expression ast.Expr, imports map[string]functionalBoundaryImport, typeDeclarations map[string]ast.Expr) {
	boundaryType, found := functionalBoundaryTypeOf(expression, imports, typeDeclarations)
	if !found || allowedBoundaryType(boundaryType.name) {
		return
	}
	for _, name := range names {
		identifiers[name.Name] = boundaryType
	}
}

func functionalBoundaryReceiverType(file *ast.File, identifiers map[*ast.FuncDecl]functionalBoundaryIdentifiers, structFields map[string]map[string]ast.Expr, typeDeclarations map[string]ast.Expr, imports map[string]functionalBoundaryImport, position token.Pos, expression ast.Expr) (functionalBoundaryType, bool) {
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || position < function.Pos() || position > function.End() {
			continue
		}
		scope := identifiers[function]
		return functionalBoundaryExpressionType(expression, scope, structFields, typeDeclarations, imports)
	}
	return functionalBoundaryType{}, false
}

func functionalBoundaryExpressionType(expression ast.Expr, scope functionalBoundaryIdentifiers, structFields map[string]map[string]ast.Expr, typeDeclarations map[string]ast.Expr, imports map[string]functionalBoundaryImport) (functionalBoundaryType, bool) {
	if boundaryType, found := functionalBoundaryTypeOf(expression, imports, typeDeclarations); found && !allowedBoundaryType(boundaryType.name) {
		return boundaryType, true
	}
	switch typed := expression.(type) {
	case *ast.Ident:
		if boundaryType, found := scope.implementations[typed.Name]; found {
			return boundaryType, true
		}
		declaredType, found := scope.localTypes[typed.Name]
		if !found {
			return functionalBoundaryType{}, false
		}
		return functionalBoundaryExpressionType(declaredType, scope, structFields, typeDeclarations, imports)
	case *ast.ParenExpr:
		return functionalBoundaryExpressionType(typed.X, scope, structFields, typeDeclarations, imports)
	case *ast.IndexExpr:
		return functionalBoundaryExpressionType(typed.X, scope, structFields, typeDeclarations, imports)
	case *ast.IndexListExpr:
		return functionalBoundaryExpressionType(typed.X, scope, structFields, typeDeclarations, imports)
	case *ast.SelectorExpr:
		localType, found := functionalBoundaryLocalTypeName(typed.X, scope, structFields, typeDeclarations)
		if !found {
			return functionalBoundaryType{}, false
		}
		fieldType, found := structFields[localType][typed.Sel.Name]
		if !found {
			return functionalBoundaryType{}, false
		}
		return functionalBoundaryExpressionType(fieldType, scope, structFields, typeDeclarations, imports)
	default:
		return functionalBoundaryType{}, false
	}
}

func functionalBoundaryLocalTypeName(expression ast.Expr, scope functionalBoundaryIdentifiers, structFields map[string]map[string]ast.Expr, typeDeclarations map[string]ast.Expr) (string, bool) {
	switch typed := expression.(type) {
	case *ast.Ident:
		declaredType, found := scope.localTypes[typed.Name]
		if !found {
			return "", false
		}
		return functionalBoundaryNamedType(declaredType, typeDeclarations)
	case *ast.ParenExpr:
		return functionalBoundaryLocalTypeName(typed.X, scope, structFields, typeDeclarations)
	case *ast.IndexExpr:
		return functionalBoundaryLocalTypeName(typed.X, scope, structFields, typeDeclarations)
	case *ast.IndexListExpr:
		return functionalBoundaryLocalTypeName(typed.X, scope, structFields, typeDeclarations)
	case *ast.SelectorExpr:
		parentType, found := functionalBoundaryLocalTypeName(typed.X, scope, structFields, typeDeclarations)
		if !found {
			return "", false
		}
		fieldType, found := structFields[parentType][typed.Sel.Name]
		if !found {
			return "", false
		}
		return functionalBoundaryNamedType(fieldType, typeDeclarations)
	default:
		return "", false
	}
}

func functionalBoundaryNamedType(expression ast.Expr, typeDeclarations map[string]ast.Expr) (string, bool) {
	for {
		switch typed := expression.(type) {
		case *ast.StarExpr:
			expression = typed.X
		case *ast.ArrayType:
			expression = typed.Elt
		case *ast.MapType:
			expression = typed.Value
		case *ast.ParenExpr:
			expression = typed.X
		case *ast.Ident:
			declaration, declared := typeDeclarations[typed.Name]
			if !declared {
				return typed.Name, true
			}
			if _, structType := declaration.(*ast.StructType); structType {
				return typed.Name, true
			}
			expression = declaration
		default:
			return "", false
		}
	}
}

func functionalBoundaryTypeOf(expression ast.Expr, imports map[string]functionalBoundaryImport, typeDeclarations map[string]ast.Expr) (functionalBoundaryType, bool) {
	seenNames := make(map[string]bool)
	for {
		switch typed := expression.(type) {
		case *ast.StarExpr:
			expression = typed.X
		case *ast.ArrayType:
			expression = typed.Elt
		case *ast.MapType:
			expression = typed.Value
		case *ast.ParenExpr:
			expression = typed.X
		case *ast.Ident:
			if seenNames[typed.Name] {
				return functionalBoundaryType{}, false
			}
			seenNames[typed.Name] = true
			declaration, found := typeDeclarations[typed.Name]
			if !found {
				return functionalBoundaryType{}, false
			}
			expression = declaration
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

// functionalBoundaryDotImports rejects dot imports from implementation
// packages. Their unqualified exported symbols cannot be safely distinguished
// from local declarations without type-checking the fixture package, and the
// import itself bypasses the customer-boundary policy.
func functionalBoundaryDotImports(file *ast.File, fileSet *token.FileSet) []functionalBoundaryDotImport {
	var imports []functionalBoundaryDotImport
	for _, specification := range file.Imports {
		if specification.Name == nil || specification.Name.Name != "." {
			continue
		}
		importPath, err := strconv.Unquote(specification.Path.Value)
		if err != nil {
			continue
		}
		boundary := prohibitedFunctionalBoundary(importPath)
		if boundary == "" {
			continue
		}
		imports = append(imports, functionalBoundaryDotImport{
			functionalBoundaryImport: functionalBoundaryImport{path: importPath, boundary: boundary},
			line:                     fileSet.Position(specification.Pos()).Line,
		})
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
