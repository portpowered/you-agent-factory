package analyzers

import (
	"go/ast"
	"go/types"
)

// Identity is a compiler declaration or authored closure, not body equivalence.
type registeredCallable struct {
	symbol  ConstructionSymbol
	closure *ast.FuncLit
}

func (h *registeredGuardHelpers) providerCallable(expr ast.Expr, visiting map[ast.Node]bool) registeredCallable {
	if expr == nil || visiting[expr] {
		return registeredCallable{}
	}
	visiting[expr] = true
	defer delete(visiting, expr)
	switch expr := expr.(type) {
	case *ast.FuncLit:
		return registeredCallable{closure: expr}
	case *ast.ParenExpr:
		return h.providerCallable(expr.X, visiting)
	case *ast.IndexExpr:
		return h.providerCallable(expr.X, visiting)
	case *ast.IndexListExpr:
		return h.providerCallable(expr.X, visiting)
	case *ast.Ident:
		obj := h.pass.TypesInfo.ObjectOf(expr)
		if _, variable := obj.(*types.Var); variable {
			return h.providerVariable(obj, visiting)
		}
	case *ast.CallExpr:
		producer := h.providerCallable(expr.Fun, visiting)
		if producer.closure != nil {
			return h.providerReturn(producer.closure.Type, producer.closure.Body, visiting)
		}
		if fn := h.bySymbol[producer.symbol]; fn != nil {
			return h.providerReturn(fn.Type, fn.Body, visiting)
		}
		return registeredCallable{}
	}
	symbol := h.values.resolve(h.pass, expr, map[types.Object]bool{})
	if symbol.ImportPath == h.pass.Pkg.Path() && symbol.Receiver == "" && h.bySymbol[symbol] != nil {
		return registeredCallable{symbol: symbol}
	}
	return registeredCallable{}
}

// Only one unnamed literal function result is summarized. Every explicit return
// must select the same identity. Named/tuple/mixed/cyclic results retain debt.
func (h *registeredGuardHelpers) providerReturn(signature *ast.FuncType, body *ast.BlockStmt, visiting map[ast.Node]bool) registeredCallable {
	if body == nil || visiting[body] || signature.Results == nil || len(signature.Results.List) != 1 {
		return registeredCallable{}
	}
	result := signature.Results.List[0]
	if _, callable := result.Type.(*ast.FuncType); !callable || len(result.Names) != 0 {
		return registeredCallable{}
	}
	visiting[body] = true
	defer delete(visiting, body)
	var identity registeredCallable
	found, unknown := false, false
	ast.Inspect(body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		if returned, ok := node.(*ast.ReturnStmt); ok {
			if len(returned.Results) != 1 {
				unknown = true
				return false
			}
			candidate := h.providerCallable(returned.Results[0], visiting)
			if candidate == (registeredCallable{}) || (found && candidate != identity) {
				unknown = true
			}
			identity, found = candidate, true
			return false
		}
		return true
	})
	if !found || unknown {
		return registeredCallable{}
	}
	return identity
}

func (h *registeredGuardHelpers) providerVariable(obj types.Object, visiting map[ast.Node]bool) registeredCallable {
	if !h.values.mutated[obj] && !h.providerValueEscaped(obj, map[types.Object]bool{}) {
		return h.providerCallable(h.values.initial[obj], visiting)
	}
	return registeredCallable{}
}
