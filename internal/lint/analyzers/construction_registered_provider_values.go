package analyzers

import (
	"go/ast"
	"go/types"
)

// Local callable transfer is bounded to calls, returns, discarded values and
// immutable local aliases. Package bindings keep the legacy write/address gate.
func (h *registeredGuardHelpers) providerValueEscaped(obj types.Object, visiting map[types.Object]bool) bool {
	if obj.Parent() == h.pass.Pkg.Scope() {
		return false
	}
	if visiting[obj] {
		return true
	}
	visiting[obj] = true
	defer delete(visiting, obj)
	allowed := map[ast.Expr]bool{}
	h.markProviderTransfers(allowed)
	h.markProviderAliases(obj, visiting, allowed)
	for id, used := range h.pass.TypesInfo.Uses {
		if used == obj && !allowed[id] {
			return true
		}
	}
	return false
}

func (h *registeredGuardHelpers) markProviderTransfers(allowed map[ast.Expr]bool) {
	for _, file := range h.pass.Files {
		ast.Inspect(file, func(node ast.Node) bool {
			switch n := node.(type) {
			case *ast.CallExpr:
				markRegisteredCallee(allowed, n.Fun)
			case *ast.ReturnStmt:
				for _, result := range n.Results {
					markRegisteredCallee(allowed, result)
				}
			case *ast.AssignStmt:
				if len(n.Lhs) == 1 && len(n.Rhs) == 1 {
					if id, ok := n.Lhs[0].(*ast.Ident); ok && id.Name == "_" {
						markRegisteredCallee(allowed, n.Rhs[0])
					}
				}
			}
			return true
		})
	}
}

func (h *registeredGuardHelpers) markProviderAliases(obj types.Object, visiting map[types.Object]bool, allowed map[ast.Expr]bool) {
	for target, initializer := range h.values.initial {
		if target == obj || target.Parent() == h.pass.Pkg.Scope() || h.values.mutated[target] {
			continue
		}
		parts := map[ast.Expr]bool{}
		markRegisteredCallee(parts, initializer)
		for part := range parts {
			if id, ok := part.(*ast.Ident); ok && h.pass.TypesInfo.ObjectOf(id) == obj && !h.providerValueEscaped(target, visiting) {
				markRegisteredCallee(allowed, initializer)
			}
		}
	}
}
