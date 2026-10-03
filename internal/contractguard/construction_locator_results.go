package contractguard

import "go/ast"

// Only collaborator result positions can establish lookup. Named result writes
// need control-flow analysis to prove their value at a naked return; retain
// explicit debt when any write carries required storage, including closures.
func constructionGetterReturnOrigin(returned *ast.ReturnStmt, results map[int]*ast.Ident, p constructionGuardProvenance, function *ast.FuncDecl) string {
	origin := ""
	for position, name := range results {
		candidate := ""
		switch {
		case len(returned.Results) == constructionResultCount(function):
			candidate = p.origin(returned.Results[position], map[*ast.Object]bool{})
		case len(returned.Results) == 0 && name != nil:
			candidate = p.origin(name, map[*ast.Object]bool{})
		case len(returned.Results) == 1:
			// A tuple-producing helper has no per-result summary yet.
			if p.origin(returned.Results[0], map[*ast.Object]bool{}) != "" {
				candidate = "unresolved-required-dependency-guard"
			}
		}
		if candidate == "unresolved-required-dependency-guard" {
			return candidate
		}
		if candidate != "" {
			origin = candidate
		}
	}
	return origin
}

// Named results have no immutable initializer. Preserve their possible required
// origin through subsequent local writes using a finite, monotonic debt map.
// This intentionally does not claim a value at any particular control-flow edge.
func constructionGetterAssignedOrigins(function *ast.FuncDecl, results map[int]*ast.Ident, p constructionGuardProvenance) map[*ast.Object]string {
	named := make(map[*ast.Object]bool)
	for _, name := range results {
		if name != nil {
			named[name.Obj] = true
		}
	}
	p.required = make(map[*ast.Object]string)
	if len(named) == 0 {
		return p.required
	}
	for changed := true; changed; {
		changed = false
		ast.Inspect(function.Body, func(node ast.Node) bool {
			left, right := constructionGetterWrite(node)
			for position, target := range left {
				if target.Obj == nil || p.required[target.Obj] != "" || len(right) == 0 {
					continue
				}
				value := right[0]
				if len(left) == len(right) {
					value = right[position]
				}
				origin := p.origin(value, map[*ast.Object]bool{})
				if origin != "" && (named[target.Obj] || origin == "unresolved-required-dependency-guard") {
					p.required[target.Obj] = "unresolved-required-dependency-guard"
					changed = true
				}
			}
			return true
		})
	}
	return p.required
}

func constructionGetterWrite(node ast.Node) ([]*ast.Ident, []ast.Expr) {
	switch write := node.(type) {
	case *ast.ValueSpec:
		return write.Names, write.Values
	case *ast.AssignStmt:
		names := make([]*ast.Ident, len(write.Lhs))
		for position, left := range write.Lhs {
			names[position], _ = left.(*ast.Ident)
			if names[position] == nil {
				names[position] = &ast.Ident{}
			}
		}
		return names, write.Rhs
	}
	return nil, nil
}
