package contractguard

import (
	"go/ast"
	"go/token"
)

// A concrete declared result establishes method identity, not the returned
// instance's requiredness. Keep this separate from constructor storage tracing.
// Interface implementation selection and opaque function values remain outside this rule.
func constructionMethodReceiverType(expr ast.Expr, source *constructionSource, visited map[*ast.Object]bool) (ConstructionSymbol, bool) {
	if typ, resolved := constructionStoredType(expr, source, map[*ast.Object]bool{}); resolved {
		return typ, true
	}
	switch value := expr.(type) {
	case *ast.ParenExpr:
		return constructionMethodReceiverType(value.X, source, visited)
	case *ast.StarExpr:
		return constructionMethodReceiverType(value.X, source, visited)
	case *ast.UnaryExpr:
		if value.Op == token.AND {
			return constructionMethodReceiverType(value.X, source, visited)
		}
	case *ast.Ident:
		if value.Obj != nil && !visited[value.Obj] {
			// Assignment changes the instance, not the binding's declared Go type.
			// Function-value resolution still requires an immutable callee.
			visited[value.Obj] = true
			if initializer := constructionValueInitializer(value.Obj); initializer != nil {
				return constructionMethodReceiverType(initializer, source, visited)
			}
			return constructionTupleReceiverType(value.Obj, source, visited)
		}
	case *ast.CallExpr:
		return constructionDeclaredReceiverResult(value, 0, 1, source, visited)
	}
	return ConstructionSymbol{}, false
}

// Bind a tuple's exact declared result position to its local AST object. This
// establishes type identity only; it does not summarize any returned value.
func constructionTupleReceiverType(object *ast.Object, source *constructionSource, visited map[*ast.Object]bool) (ConstructionSymbol, bool) {
	var names []ast.Expr
	var values []ast.Expr
	switch declaration := object.Decl.(type) {
	case *ast.AssignStmt:
		names, values = declaration.Lhs, declaration.Rhs
	case *ast.ValueSpec:
		for _, name := range declaration.Names {
			names = append(names, name)
		}
		values = declaration.Values
	}
	if len(names) < 2 || len(values) != 1 {
		return ConstructionSymbol{}, false
	}
	expression := values[0]
	for {
		parentheses, ok := expression.(*ast.ParenExpr)
		if !ok {
			break
		}
		expression = parentheses.X
	}
	call, ok := expression.(*ast.CallExpr)
	if !ok {
		return ConstructionSymbol{}, false
	}
	for position, name := range names {
		if ident, ok := name.(*ast.Ident); ok && ident.Obj == object {
			return constructionDeclaredReceiverResult(call, position, len(names), source, visited)
		}
	}
	return ConstructionSymbol{}, false
}

func constructionDeclaredReceiverResult(call *ast.CallExpr, position, count int, source *constructionSource, visited map[*ast.Object]bool) (ConstructionSymbol, bool) {
	symbol, resolved := resolveConstructionValue(call.Fun, source, visited)
	decl := source.declarations[symbol]
	if !resolved || decl.function == nil {
		return ConstructionSymbol{}, false
	}
	results := constructionParameterFields(decl.function.Type.Results)
	if len(results) != count || position >= len(results) {
		return ConstructionSymbol{}, false
	}
	result := results[position].Type
	for pointer, ok := result.(*ast.StarExpr); ok; pointer, ok = result.(*ast.StarExpr) {
		result = pointer.X
	}
	if name, ok := result.(*ast.Ident); ok && name.Obj != nil {
		if _, parameter := name.Obj.Decl.(*ast.Field); parameter {
			return ConstructionSymbol{}, false
		}
	}
	return constructionResultSymbol(results[position].Type, decl.source)
}
