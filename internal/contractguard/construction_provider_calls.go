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

// A focused allowance cannot authorize a recursive construction path. Follow
// resolved authored calls within the provider package; merely declaring a
// closure or referencing an uncalled helper does not establish such a path.
func constructionRecursiveProvider(source *constructionSource, caller ConstructionSymbol, allowances []ConstructionAllowance) bool {
	approved := false
	for _, allowance := range allowances {
		approved = approved || (allowance.Kind == "focused-provider" && allowance.Caller == caller && allowance.FilePath == source.path)
	}
	if !approved {
		return false
	}
	visited := make(map[ConstructionSymbol]bool)
	pending := []ConstructionSymbol{caller}
	for len(pending) > 0 {
		symbol := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if visited[symbol] {
			continue
		}
		visited[symbol] = true
		decl := source.declarations[symbol]
		if decl.function == nil || decl.function.Body == nil {
			continue
		}
		for _, callee := range constructionProviderCallees(decl) {
			if callee == caller {
				return true
			}
			if callee.ImportPath == caller.ImportPath && !visited[callee] {
				pending = append(pending, callee)
			}
		}
	}
	return false
}

func constructionProviderCallees(decl constructionDeclaration) []ConstructionSymbol {
	var callees []ConstructionSymbol
	type bodySource struct {
		body   ast.Node
		source *constructionSource
	}
	pending := []bodySource{{decl.function.Body, decl.source}}
	visited := make(map[*ast.FuncLit]bool)
	for len(pending) > 0 {
		body := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		ast.Inspect(body.body, func(node ast.Node) bool {
			if _, closure := node.(*ast.FuncLit); closure {
				return false
			}
			if call, ok := node.(*ast.CallExpr); ok {
				if symbol, resolved := resolveConstructionCall(call.Fun, body.source); resolved {
					callees = append(callees, symbol)
				}
				closure, closureSource := constructionProviderClosure(call.Fun, body.source, map[*ast.Object]bool{})
				if closure != nil && !visited[closure] {
					visited[closure] = true
					pending = append(pending, bodySource{closure.Body, closureSource})
				}
			}
			return true
		})
	}
	return callees
}

// Calling an immutable local function value establishes a closure edge. Merely
// passing or storing the value does not; mutable and opaque values need further
// classification rather than selecting their initializer as current behavior.
func constructionProviderClosure(expr ast.Expr, source *constructionSource, visited map[*ast.Object]bool) (*ast.FuncLit, *constructionSource) {
	switch value := expr.(type) {
	case *ast.FuncLit:
		return value, source
	case *ast.ParenExpr:
		return constructionProviderClosure(value.X, source, visited)
	case *ast.Ident:
		object, owner, safe := constructionProviderValue(value, source)
		if safe && !visited[object] {
			visited[object] = true
			return constructionProviderClosure(constructionValueInitializer(object), owner, visited)
		}
	}
	return nil, nil
}
