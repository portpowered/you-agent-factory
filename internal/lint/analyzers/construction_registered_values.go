package analyzers

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"
)

type registeredValues struct {
	initial map[types.Object]ast.Expr
	mutated map[types.Object]bool
	safe    map[ast.Expr]bool
}

func registeredConstructionValues(pass *analysis.Pass) registeredValues {
	v := registeredValues{initial: map[types.Object]ast.Expr{}, mutated: map[types.Object]bool{}, safe: map[ast.Expr]bool{}}
	called := map[ast.Expr]bool{}
	for _, file := range pass.Files {
		ast.Inspect(file, func(node ast.Node) bool {
			switch n := node.(type) {
			case *ast.CallExpr:
				markRegisteredCallee(called, n.Fun)
			case *ast.ValueSpec:
				if len(n.Names) == len(n.Values) {
					for i, name := range n.Names {
						v.initial[pass.TypesInfo.Defs[name]] = n.Values[i]
					}
				}
			case *ast.AssignStmt:
				for i, left := range n.Lhs {
					if id, ok := left.(*ast.Ident); ok {
						if obj := pass.TypesInfo.Defs[id]; obj != nil && n.Tok.String() == ":=" && len(n.Lhs) == len(n.Rhs) {
							v.initial[obj] = n.Rhs[i]
						} else if pass.TypesInfo.Defs[id] == nil {
							v.mutated[pass.TypesInfo.ObjectOf(id)] = true
						}
					}
				}
			case *ast.UnaryExpr:
				if id, ok := n.X.(*ast.Ident); ok && n.Op.String() == "&" {
					v.mutated[pass.TypesInfo.ObjectOf(id)] = true
				}
			case *ast.RangeStmt:
				for _, expr := range []ast.Expr{n.Key, n.Value} {
					if id, ok := expr.(*ast.Ident); ok {
						v.mutated[pass.TypesInfo.ObjectOf(id)] = true
					}
				}
			}
			return true
		})
	}
	uses := map[types.Object][]*ast.Ident{}
	for id, obj := range pass.TypesInfo.Uses {
		uses[obj] = append(uses[obj], id)
	}
	owners := map[ast.Expr]types.Object{}
	for obj, expr := range v.initial {
		owners[expr] = obj
	}
	var safeObject func(types.Object, map[types.Object]bool) bool
	safeObject = func(obj types.Object, visited map[types.Object]bool) bool {
		if v.mutated[obj] || visited[obj] || len(uses[obj]) == 0 {
			return false
		}
		visited[obj] = true
		defer delete(visited, obj)
		for _, id := range uses[obj] {
			if called[id] {
				continue
			}
			if owner := owners[id]; owner != nil && safeObject(owner, visited) {
				continue
			}
			return false
		}
		return true
	}
	for obj, expr := range v.initial {
		if safeObject(obj, map[types.Object]bool{}) {
			v.safe[expr] = true
		}
	}
	return v
}

func (v registeredValues) resolve(pass *analysis.Pass, expr ast.Expr, visited map[types.Object]bool) ConstructionSymbol {
	switch expr := expr.(type) {
	case *ast.Ident:
		obj := pass.TypesInfo.ObjectOf(expr)
		if _, ok := obj.(*types.Func); ok {
			return registeredConstructionSymbol(obj)
		}
		if obj != nil && !visited[obj] && !v.mutated[obj] {
			visited[obj] = true
			return v.resolve(pass, v.initial[obj], visited)
		}
	case *ast.SelectorExpr:
		if selection := pass.TypesInfo.Selections[expr]; selection != nil {
			if _, ok := selection.Obj().(*types.Func); ok {
				return registeredConstructionSymbol(selection.Obj())
			}
		} else if fn, ok := pass.TypesInfo.ObjectOf(expr.Sel).(*types.Func); ok {
			return registeredConstructionSymbol(fn)
		}
	case *ast.ParenExpr:
		return v.resolve(pass, expr.X, visited)
	case *ast.IndexExpr:
		return v.resolve(pass, expr.X, visited)
	case *ast.IndexListExpr:
		return v.resolve(pass, expr.X, visited)
	}
	return ConstructionSymbol{}
}
