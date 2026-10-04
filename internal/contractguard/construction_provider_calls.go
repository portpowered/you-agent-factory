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
func constructionRecursiveProvider(source *constructionSource, caller ConstructionSymbol, allowances []ConstructionAllowance) (bool, bool) {
	approved := false
	for _, allowance := range allowances {
		approved = approved || (allowance.Kind == "focused-provider" && allowance.Caller == caller && allowance.FilePath == source.path)
	}
	if !approved {
		return false, false
	}
	unresolved := false
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
		callees, debt := constructionProviderCallees(decl)
		unresolved = unresolved || debt
		for _, callee := range callees {
			if callee == caller {
				return true, unresolved
			}
			if callee.ImportPath == caller.ImportPath && !visited[callee] {
				pending = append(pending, callee)
			}
		}
	}
	return false, unresolved
}

func constructionProviderCallees(decl constructionDeclaration) ([]ConstructionSymbol, bool) {
	unresolved := false
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
				callable := constructionProviderCallable(call.Fun, body.source, map[ast.Node]bool{})
				if callable.symbol.Name != "" {
					callees = append(callees, callable.symbol)
				}
				closure := callable.closure
				if closure == nil && callable.symbol.Name == "" {
					unresolved = unresolved || constructionProviderCallableDebt(call.Fun, body.source)
				}
				if closure != nil && !visited[closure] {
					visited[closure] = true
					pending = append(pending, bodySource{closure.Body, callable.source})
				}
			}
			return true
		})
	}
	return callees, unresolved
}

// Identity is authored declaration/closure identity, never body equivalence.
type constructionCallable struct {
	symbol  ConstructionSymbol
	closure *ast.FuncLit
	source  *constructionSource
}

// The visiting set covers expressions, bindings and return bodies. Re-entering
// any node leaves identity unknown; sibling returns have independent paths.
func constructionProviderCallable(expr ast.Expr, source *constructionSource, visiting map[ast.Node]bool) constructionCallable {
	if expr == nil || visiting[expr] {
		return constructionCallable{}
	}
	visiting[expr] = true
	defer delete(visiting, expr)
	switch value := expr.(type) {
	case *ast.FuncLit:
		return constructionCallable{closure: value, source: source}
	case *ast.ParenExpr:
		return constructionProviderCallable(value.X, source, visiting)
	case *ast.Ident:
		object, owner, safe := constructionProviderValue(value, source)
		if object != nil && object.Kind == ast.Var {
			if safe && (constructionProviderPackageObject(object, owner) || !constructionProviderValueEscaped(object, owner, map[*ast.Object]bool{})) {
				return constructionProviderCallable(constructionValueInitializer(object), owner, visiting)
			}
			return constructionCallable{}
		}
	case *ast.CallExpr:
		producer := constructionProviderCallable(value.Fun, source, visiting)
		if producer.closure != nil {
			return constructionProviderReturn(producer.closure.Type, producer.closure.Body, producer.source, visiting)
		}
		decl := source.declarations[producer.symbol]
		if decl.function != nil {
			return constructionProviderReturn(decl.function.Type, decl.function.Body, decl.source, visiting)
		}
		return constructionCallable{}
	}
	symbol, resolved := resolveConstructionCall(expr, source)
	decl := source.declarations[symbol]
	if resolved && symbol.ImportPath == source.importPath && symbol.Receiver == "" && decl.function != nil && decl.function.Body != nil {
		return constructionCallable{symbol: symbol, source: decl.source}
	}
	return constructionCallable{}
}
