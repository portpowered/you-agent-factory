package contractguard

import "go/ast"

// A focused provider constructs synchronously in its own body. Its allowance
// does not authorize deferred/asynchronous calls or factories inside closures.
// Arguments to defer/go calls are still evaluated in the provider body.
func constructionIndirectProviderCalls(body ast.Node) map[*ast.CallExpr]bool {
	indirect := make(map[*ast.CallExpr]bool)
	ast.Inspect(body, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.FuncLit:
			ast.Inspect(value.Body, func(nested ast.Node) bool {
				if call, ok := nested.(*ast.CallExpr); ok {
					indirect[call] = true
				}
				return true
			})
			return false
		case *ast.DeferStmt:
			indirect[value.Call] = true
		case *ast.GoStmt:
			indirect[value.Call] = true
		}
		return true
	})
	return indirect
}
