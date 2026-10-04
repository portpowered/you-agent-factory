package analyzers

import (
	"go/ast"
	"go/types"
)

// Provider paths contain only compiled same-package bodies. Closure declarations
// are not edges until invoked; recursion takes precedence over dispatch debt.
func (h *registeredGuardHelpers) providerPath(caller ConstructionSymbol) (recursive, debt bool) {
	fn := h.bySymbol[caller]
	if fn == nil {
		return false, false
	}
	pending := []ast.Node{fn.Body}
	visited := map[ast.Node]bool{}
	for len(pending) != 0 {
		body := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if visited[body] {
			continue
		}
		visited[body] = true
		ast.Inspect(body, func(node ast.Node) bool {
			if _, closure := node.(*ast.FuncLit); closure {
				return false
			}
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			symbol := h.values.resolve(h.pass, call.Fun, map[types.Object]bool{})
			callable := h.providerCallable(call.Fun, map[ast.Node]bool{})
			if callable.symbol != (ConstructionSymbol{}) {
				symbol = callable.symbol
			}
			if symbol == caller {
				recursive = true
			}
			if target := h.bySymbol[symbol]; target != nil {
				pending = append(pending, target.Body)
			}
			if callable.closure != nil {
				pending = append(pending, callable.closure.Body)
			} else if callable.symbol == (ConstructionSymbol{}) {
				debt = debt || h.providerCallableDebt(call.Fun)
			}
			return true
		})
	}
	return recursive, debt
}

func registeredIndirectProviderCalls(body ast.Node) map[*ast.CallExpr]bool {
	indirect := map[*ast.CallExpr]bool{}
	ast.Inspect(body, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.FuncLit:
			ast.Inspect(n.Body, func(node ast.Node) bool {
				if call, ok := node.(*ast.CallExpr); ok {
					indirect[call] = true
				}
				return true
			})
			return false
		case *ast.DeferStmt:
			indirect[n.Call] = true
		case *ast.GoStmt:
			indirect[n.Call] = true
		}
		return true
	})
	return indirect
}

func (h *registeredGuardHelpers) providerCallableDebt(expr ast.Expr) bool {
	if paren, ok := expr.(*ast.ParenExpr); ok {
		return h.providerCallableDebt(paren.X)
	}
	if tv := h.pass.TypesInfo.Types[expr]; tv.IsType() {
		return false
	}
	switch expr := expr.(type) {
	case *ast.CallExpr:
		return true
	case *ast.Ident:
		obj, variable := h.pass.TypesInfo.ObjectOf(expr).(*types.Var)
		if !variable {
			return false
		}
		// An immutable alias to an imported declaration is a known call even
		// though its body is outside this package's bounded return summaries.
		fn := h.values.resolveFunction(h.pass, expr, map[types.Object]bool{})
		return fn == nil || registeredInterfaceDispatch(fn) ||
			h.providerValueEscaped(obj, map[types.Object]bool{})
	case *ast.SelectorExpr:
		selection := h.pass.TypesInfo.Selections[expr]
		if selection == nil {
			_, variable := h.pass.TypesInfo.ObjectOf(expr.Sel).(*types.Var)
			return variable
		}
		if selection.Kind() == types.FieldVal {
			return true
		}
		// A struct can promote an interface method. Its selected declaration's
		// receiver, rather than the outer struct receiver, owns dispatch.
		return registeredInterfaceDispatch(selection.Obj().(*types.Func))
	}
	return false
}

func registeredInterfaceDispatch(fn *types.Func) bool {
	receiver := fn.Type().(*types.Signature).Recv()
	if receiver == nil {
		return false
	}
	_, iface := receiver.Type().Underlying().(*types.Interface)
	return iface
}
