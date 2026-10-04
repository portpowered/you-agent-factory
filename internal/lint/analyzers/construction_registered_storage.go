package analyzers

import (
	"go/ast"
	"go/token"
	"go/types"
	"slices"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// Field identity comes from compiler declarations, including grouped and
// embedded fields. Conflicting writes retain debt rather than choosing a branch.
func registeredConstructionStorage(pass *analysis.Pass, registry ConstructionRegistry, values registeredValues) map[ConstructionSymbol]map[*types.Var]string {
	stored := map[ConstructionSymbol]map[*types.Var]string{}
	for _, file := range pass.Files {
		if ast.IsGenerated(file) || strings.HasSuffix(pass.Fset.Position(file.Pos()).Filename, "_test.go") {
			continue
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			obj := pass.TypesInfo.Defs[fn.Name].(*types.Func)
			for _, constructor := range registry.Constructors {
				if constructor.Symbol != registeredConstructionSymbol(obj) {
					continue
				}
				p := registeredGuardOrigins{pass: pass, values: values, required: map[types.Object]string{},
					assertions: registeredGuardAssertions(pass, fn.Body)}
				for _, param := range constructor.RequiredParameters {
					p.required[obj.Type().(*types.Signature).Params().At(param.Index)] = "required-dependency-guard"
				}
				storage := registeredFieldStorage{origins: p, constructor: constructor, fields: map[*types.Var]string{}}
				ast.Inspect(fn.Body, func(node ast.Node) bool {
					switch n := node.(type) {
					case *ast.CompositeLit:
						storage.literal(n)
					case *ast.AssignStmt:
						storage.assignment(n)
					}
					return true
				})
				stored[constructor.Symbol] = storage.fields
			}
		}
	}
	return stored
}

type registeredFieldStorage struct {
	origins     registeredGuardOrigins
	constructor ConstructionConstructor
	fields      map[*types.Var]string
}

func (s registeredFieldStorage) merge(field *types.Var, expr ast.Expr, mutated bool) {
	rule := s.origins.origin(expr, false, map[types.Object]bool{})
	if prior, exists := s.fields[field]; (exists && prior != rule) || (mutated && rule != "") {
		rule = "unresolved-required-dependency-guard"
	}
	s.fields[field] = rule
}

func (s registeredFieldStorage) literal(literal *ast.CompositeLit) {
	pass := s.origins.pass
	if !registeredStorageResult(pass.TypesInfo.TypeOf(literal), s.constructor) {
		return
	}
	structure, ok := pass.TypesInfo.TypeOf(literal).Underlying().(*types.Struct)
	if !ok {
		return
	}
	for i, element := range literal.Elts {
		if keyed, ok := element.(*ast.KeyValueExpr); ok {
			if id, ok := keyed.Key.(*ast.Ident); ok {
				if field, ok := pass.TypesInfo.ObjectOf(id).(*types.Var); ok {
					s.merge(field, keyed.Value, false)
				}
			}
		} else if i < structure.NumFields() {
			s.merge(structure.Field(i), element, false)
		}
	}
}

func (s registeredFieldStorage) assignment(assignment *ast.AssignStmt) {
	if len(assignment.Lhs) != len(assignment.Rhs) {
		return
	}
	pass := s.origins.pass
	for i, left := range assignment.Lhs {
		selector, ok := left.(*ast.SelectorExpr)
		if !ok || !registeredStorageResult(pass.TypesInfo.TypeOf(selector.X), s.constructor) {
			continue
		}
		selection := pass.TypesInfo.Selections[selector]
		if selection != nil && selection.Kind() == types.FieldVal {
			s.merge(selection.Obj().(*types.Var), assignment.Rhs[i], s.origins.storageMutated(selector.X, map[types.Object]bool{}))
		}
	}
}

func registeredStorageResult(typ types.Type, constructor ConstructionConstructor) bool {
	if typ == nil {
		return false
	}
	typ = types.Unalias(typ)
	if pointer, ok := typ.(*types.Pointer); ok {
		typ = types.Unalias(pointer.Elem())
	}
	named, ok := typ.(*types.Named)
	return ok && slices.Contains(constructor.Results, registeredConstructionSymbol(named.Origin().Obj()))
}

func (p registeredGuardOrigins) receiverAlias(expr ast.Expr, visited map[types.Object]bool) bool {
	switch n := expr.(type) {
	case *ast.ParenExpr:
		return p.receiverAlias(n.X, visited)
	case *ast.Ident:
		obj := p.pass.TypesInfo.ObjectOf(n)
		if obj == nil || visited[obj] || p.receiver == nil {
			return false
		}
		if obj == p.receiver {
			return true
		}
		visited[obj] = true
		return p.receiverAlias(p.values.initial[obj], visited)
	}
	return false
}

func (p registeredGuardOrigins) storageMutated(expr ast.Expr, visited map[types.Object]bool) bool {
	switch n := expr.(type) {
	case *ast.ParenExpr:
		return p.storageMutated(n.X, visited)
	case *ast.Ident:
		obj := p.pass.TypesInfo.ObjectOf(n)
		if obj == nil || visited[obj] {
			return false
		}
		visited[obj] = true
		return p.values.mutated[obj] || p.storageMutated(p.values.initial[obj], visited)
	}
	return false
}

func (p registeredGuardOrigins) mutatedFields(body ast.Node) map[*types.Var]bool {
	mutated := map[*types.Var]bool{}
	mark := func(expr ast.Expr) {
		selector, ok := expr.(*ast.SelectorExpr)
		if !ok || !p.receiverAlias(selector.X, map[types.Object]bool{}) {
			return
		}
		if selection := p.pass.TypesInfo.Selections[selector]; selection != nil && selection.Kind() == types.FieldVal {
			mutated[selection.Obj().(*types.Var)] = true
		}
	}
	ast.Inspect(body, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.AssignStmt:
			for _, left := range n.Lhs {
				mark(left)
			}
		case *ast.UnaryExpr:
			if n.Op == token.AND {
				mark(n.X)
			}
		}
		return true
	})
	return mutated
}
