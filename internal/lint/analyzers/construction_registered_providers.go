package analyzers

import (
	"go/ast"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// Wire registers direct declarations as data for generated composition. A
// similarly named local callable has no authority to retain constructors.
func markRegisteredWireProviders(pass *analysis.Pass, values registeredValues, decl ast.Decl,
	caller ConstructionSymbol, marked map[ast.Expr]bool,
) {
	ast.Inspect(decl, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		wire := values.resolve(pass, call.Fun, map[types.Object]bool{})
		if wire.ImportPath != "github.com/google/wire" || wire.Receiver != "" ||
			(wire.Name != "NewSet" && wire.Name != "Build") {
			return true
		}
		boundary := caller
		boundary.Name = "Provide"
		for _, arg := range call.Args {
			provider := values.resolve(pass, arg, map[types.Object]bool{})
			if provider != (ConstructionSymbol{}) && registeredCompositionCaller(boundary, provider) {
				markRegisteredCallee(marked, arg)
			}
		}
		return true
	})
}

// Composition belongs to free functions in the canonical Wire package or the
// callee's owning service Wire package. Compare compiler declaration identities,
// not filenames, import aliases, or a list of exempt callers. The body must also
// pass the synchronous, acyclic, resolved-dispatch checks below.
func registeredCompositionCaller(caller, callee ConstructionSymbol) bool {
	providerName := serviceConstructorName(caller.Name) ||
		(strings.HasPrefix(caller.Name, "provide") && serviceConstructorName("Provide"+strings.TrimPrefix(caller.Name, "provide")))
	if caller.Receiver != "" || !providerName {
		return false
	}
	prefix, remainder, ok := strings.Cut(caller.ImportPath, "/pkg/")
	if !ok {
		return false
	}
	if remainder == "wire" {
		return true
	}
	parts := strings.Split(remainder, "/")
	if len(parts) != 3 || parts[0] != "services" || parts[2] != "wire" {
		return false
	}
	owner := prefix + "/pkg/services/" + parts[1]
	return callee.ImportPath == owner || strings.HasPrefix(callee.ImportPath, owner+"/")
}

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
