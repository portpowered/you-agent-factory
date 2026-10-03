package contractguard

import "go/ast"

// A concrete declared result establishes method identity, not the returned
// instance's requiredness. Keep this separate from constructor storage tracing.
// Interface results, tuples and opaque function values remain outside this rule.
func constructionMethodReceiverType(expr ast.Expr, source *constructionSource, visited map[*ast.Object]bool) (ConstructionSymbol, bool) {
	if typ, resolved := constructionStoredType(expr, source, map[*ast.Object]bool{}); resolved {
		return typ, true
	}
	switch value := expr.(type) {
	case *ast.ParenExpr:
		return constructionMethodReceiverType(value.X, source, visited)
	case *ast.Ident:
		if value.Obj != nil && !visited[value.Obj] && !source.mutations[value.Obj] {
			visited[value.Obj] = true
			return constructionMethodReceiverType(constructionValueInitializer(value.Obj), source, visited)
		}
	case *ast.CallExpr:
		symbol, resolved := resolveConstructionValue(value.Fun, source, visited)
		decl := source.declarations[symbol]
		if !resolved || decl.function == nil {
			break
		}
		results := constructionParameterFields(decl.function.Type.Results)
		if len(results) == 1 {
			return constructionResultSymbol(results[0].Type, decl.source)
		}
	}
	return ConstructionSymbol{}, false
}
