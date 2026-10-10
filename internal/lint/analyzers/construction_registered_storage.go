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
func registeredConstructionStorage(pass *analysis.Pass, registry ConstructionRegistry, helpers *registeredGuardHelpers) map[ConstructionSymbol]map[*types.Var]string {
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
				origins := helpers.requiredOrigins(constructor, registry.Types, nil)
				fields := map[*types.Var]string{}
				for _, helper := range helpers.functions {
					required := origins[registeredConstructionSymbol(pass.TypesInfo.Defs[helper.Name])]
					if helper != fn && !registeredStorageHelper(pass, helper, required) {
						continue
					}
					p := helpers.context(helper, required, constructor, nil)
					storage := registeredFieldStorage{origins: p, constructor: constructor, fields: fields}
					ast.Inspect(helper.Body, func(node ast.Node) bool {
						if _, closure := node.(*ast.FuncLit); closure {
							return false
						}
						switch n := node.(type) {
						case *ast.CompositeLit:
							storage.literal(n)
						case *ast.AssignStmt:
							storage.assignment(n)
						}
						return true
					})
				}
				stored[constructor.Symbol] = fields
			}
		}
	}
	return stored
}

func registeredStorageHelper(pass *analysis.Pass, fn *ast.FuncDecl, required map[types.Object]string) bool {
	params := pass.TypesInfo.Defs[fn.Name].Type().(*types.Signature).Params()
	for i := 0; i < params.Len(); i++ {
		if required[params.At(i)] != "" {
			return true
		}
	}
	return false
}

// Extend storage identity only from concrete values on authored return paths.
// Implementing an interface alone never makes an unrelated type required.
// Keep this compilation-unit summary separate from immutable registry metadata.
func (h *registeredGuardHelpers) storageResults(registry ConstructionRegistry) ConstructionRegistry {
	registry.Constructors = slices.Clone(registry.Constructors)
	for i, constructor := range registry.Constructors {
		fn := h.bySymbol[constructor.Symbol]
		if fn == nil {
			continue
		}
		results := slices.Clone(constructor.Results)
		sig := h.pass.TypesInfo.Defs[fn.Name].Type().(*types.Signature)
		var targets []types.Type
		for j := 0; j < sig.Results().Len(); j++ {
			if registeredStorageResult(sig.Results().At(j).Type(), constructor) {
				targets = append(targets, sig.Results().At(j).Type())
			}
		}
		h.returnedStorageTypes(fn, targets, map[ConstructionSymbol]bool{}, func(typ types.Type) {
			if pointer, ok := types.Unalias(typ).(*types.Pointer); ok {
				typ = pointer.Elem()
			}
			named, ok := types.Unalias(typ).(*types.Named)
			if !ok {
				return
			}
			if _, ok := named.Underlying().(*types.Struct); !ok {
				return
			}
			symbol := registeredConstructionSymbol(named.Origin().Obj())
			if !slices.Contains(results, symbol) {
				results = append(results, symbol)
			}
		})
		registry.Constructors[i].Results = results
	}
	return registry
}

func (h *registeredGuardHelpers) returnedStorageTypes(fn *ast.FuncDecl, targets []types.Type, active map[ConstructionSymbol]bool, add func(types.Type)) {
	symbol := registeredConstructionSymbol(h.pass.TypesInfo.Defs[fn.Name])
	if active[symbol] {
		return
	}
	active[symbol] = true
	defer delete(active, symbol)
	sig := h.pass.TypesInfo.Defs[fn.Name].Type().(*types.Signature)
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		if _, closure := node.(*ast.FuncLit); closure {
			return false
		}
		if returned, ok := node.(*ast.ReturnStmt); ok {
			for i, expr := range returned.Results {
				if len(returned.Results) == sig.Results().Len() && !slices.ContainsFunc(targets, func(target types.Type) bool {
					return types.AssignableTo(sig.Results().At(i).Type(), target)
				}) {
					continue
				}
				h.returnedStorageType(expr, targets, active, map[types.Object]bool{}, add)
			}
		}
		return true
	})
}

func (h *registeredGuardHelpers) returnedStorageType(expr ast.Expr, targets []types.Type, active map[ConstructionSymbol]bool, visited map[types.Object]bool, add func(types.Type)) {
	if expr == nil {
		return
	}
	switch n := ast.Unparen(expr).(type) {
	case *ast.CompositeLit:
		add(h.pass.TypesInfo.TypeOf(n))
	case *ast.UnaryExpr:
		if n.Op == token.AND {
			h.returnedStorageType(n.X, targets, active, visited, add)
		}
	case *ast.Ident:
		obj := h.pass.TypesInfo.ObjectOf(n)
		if obj != nil && !visited[obj] && !h.values.mutated[obj] {
			visited[obj] = true
			h.returnedStorageType(h.values.initial[obj], targets, active, visited, add)
		}
	case *ast.CallExpr:
		if fn := h.bySymbol[h.values.resolve(h.pass, n.Fun, map[types.Object]bool{})]; fn != nil {
			h.returnedStorageTypes(fn, targets, active, add)
		} else if id, ok := n.Fun.(*ast.Ident); ok && h.pass.TypesInfo.ObjectOf(id) == types.Universe.Lookup("new") {
			add(h.pass.TypesInfo.TypeOf(n))
		}
	}
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
