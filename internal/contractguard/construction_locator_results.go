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
			if value, ok := returned.Results[position].(*ast.Ident); ok && name != nil && value.Obj == name.Obj {
				candidate = constructionNamedGetterOrigin(name, p, function.Body)
			}
		case len(returned.Results) == 0 && name != nil:
			candidate = constructionNamedGetterOrigin(name, p, function.Body)
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

func constructionNamedGetterOrigin(name *ast.Ident, p constructionGuardProvenance, body ast.Node) string {
	origin := ""
	ast.Inspect(body, func(node ast.Node) bool {
		assignment, ok := node.(*ast.AssignStmt)
		if !ok {
			return true
		}
		value := constructionAssignedValue(name.Obj, assignment.Lhs, assignment.Rhs)
		if value == nil && len(assignment.Rhs) == 1 {
			for _, left := range assignment.Lhs {
				if target, ok := left.(*ast.Ident); ok && target.Obj == name.Obj {
					value = assignment.Rhs[0]
				}
			}
		}
		if p.origin(value, map[*ast.Object]bool{}) != "" {
			origin = "unresolved-required-dependency-guard"
		}
		return true
	})
	return origin
}
