package contractguard

import "go/ast"

func constructionValueMutations(body ast.Node) map[*ast.Object]bool {
	mutations := constructionGuardMutations(body)
	ast.Inspect(body, func(node ast.Node) bool {
		if loop, ok := node.(*ast.RangeStmt); ok {
			for _, expression := range []ast.Expr{loop.Key, loop.Value} {
				if ident, ok := expression.(*ast.Ident); ok && ident.Obj != nil && ident.Obj.Decl != loop {
					mutations[ident.Obj] = true
				}
			}
		}
		return true
	})
	return mutations
}

func constructionValueInitializer(object *ast.Object) ast.Expr {
	if object == nil {
		return nil
	}
	switch declaration := object.Decl.(type) {
	case *ast.AssignStmt:
		return constructionAssignedValue(object, declaration.Lhs, declaration.Rhs)
	case *ast.ValueSpec:
		if len(declaration.Names) == len(declaration.Values) {
			for position, name := range declaration.Names {
				if name.Obj == object {
					return declaration.Values[position]
				}
			}
		}
	}
	return nil
}

// Resolve declared receiver types and aliases, never a method's spelling alone.
// Promoted methods and interface implementation selection need further proof.
func resolveConstructionMethod(selector *ast.SelectorExpr, source *constructionSource, visited map[*ast.Object]bool) (ConstructionSymbol, bool) {
	typ, resolved := constructionMethodReceiverType(selector.X, source, visited)
	if !resolved && constructionIsTypeExpression(selector.X, source) {
		typ, resolved = constructionResultSymbol(constructionMethodTypeExpression(selector.X), source)
	}
	if !resolved {
		return ConstructionSymbol{}, false
	}
	typ, resolved = constructionCanonicalType(typ, source.declarations, map[ConstructionSymbol]bool{})
	if !resolved {
		return ConstructionSymbol{}, false
	}
	method := ConstructionSymbol{ImportPath: typ.ImportPath, Receiver: typ.Name, Name: selector.Sel.Name}
	return method, source.declarations[method].function != nil
}

func constructionMethodTypeExpression(expr ast.Expr) ast.Expr {
	if parentheses, ok := expr.(*ast.ParenExpr); ok {
		return constructionMethodTypeExpression(parentheses.X)
	}
	return expr
}

func constructionIsTypeExpression(expr ast.Expr, source *constructionSource) bool {
	switch value := expr.(type) {
	case *ast.ParenExpr:
		return constructionIsTypeExpression(value.X, source)
	case *ast.StarExpr:
		return constructionIsTypeExpression(value.X, source)
	case *ast.Ident:
		return value.Obj == nil || value.Obj.Kind == ast.Typ
	case *ast.SelectorExpr:
		name, ok := value.X.(*ast.Ident)
		return ok && name.Obj == nil && source.imports[name.Name] != ""
	}
	return false
}

func constructionCanonicalType(symbol ConstructionSymbol, declarations map[ConstructionSymbol]constructionDeclaration, visited map[ConstructionSymbol]bool) (ConstructionSymbol, bool) {
	declaration := declarations[symbol]
	if visited[symbol] || declaration.typeSpec == nil {
		return ConstructionSymbol{}, false
	}
	if !declaration.typeSpec.Assign.IsValid() {
		return symbol, true
	}
	visited[symbol] = true
	target, resolved := constructionResultSymbol(declaration.typeSpec.Type, declaration.source)
	if !resolved {
		return ConstructionSymbol{}, false
	}
	return constructionCanonicalType(target, declarations, visited)
}

// A local immutable constructor value is fully covered only when all its uses
// are calls or transfers to other fully covered local aliases. Escapes, writes,
// package storage and unused references retain the existing classification debt.
func constructionSafeValueReferences(body ast.Node, source *constructionSource, callees map[ast.Expr]bool) map[ast.Expr]bool {
	aliases, definitions := constructionValueBindings(body)
	safe := make(map[*ast.Object]bool)
	transfers := make(map[ast.Expr]*ast.Object)
	uses := make(map[*ast.Object]bool)
	for object, expression := range aliases {
		_, resolved := resolveConstructionCall(expression, source)
		safe[object] = resolved && !source.mutations[object]
		parts := make(map[ast.Expr]bool)
		markConstructionCallee(parts, expression)
		for part := range parts {
			transfers[part] = object
		}
	}
	ast.Inspect(body, func(node ast.Node) bool {
		if ident, ok := node.(*ast.Ident); ok && !definitions[ident] && (callees[ident] || transfers[ident] != nil) {
			uses[ident.Obj] = true
		}
		return true
	})
	for object := range safe {
		safe[object] = safe[object] && uses[object]
	}
	for changed := true; changed; {
		changed = false
		ast.Inspect(body, func(node ast.Node) bool {
			ident, ok := node.(*ast.Ident)
			if !ok || !safe[ident.Obj] || definitions[ident] || callees[ident] {
				return true
			}
			if !safe[transfers[ident]] {
				safe[ident.Obj], changed = false, true
			}
			return true
		})
	}
	covered := make(map[ast.Expr]bool)
	for ident, definition := range definitions {
		if definition {
			covered[ident] = true
		}
	}
	for object, expression := range aliases {
		if safe[object] {
			markConstructionCallee(covered, expression)
		}
	}
	return covered
}

func constructionValueBindings(body ast.Node) (map[*ast.Object]ast.Expr, map[*ast.Ident]bool) {
	aliases := make(map[*ast.Object]ast.Expr)
	definitions := make(map[*ast.Ident]bool)
	ast.Inspect(body, func(node ast.Node) bool {
		ident, ok := node.(*ast.Ident)
		if !ok || ident.Obj == nil || ident.Obj.Kind != ast.Var {
			return true
		}
		expression := constructionValueInitializer(ident.Obj)
		if expression != nil {
			aliases[ident.Obj] = expression
			definitions[ident] = constructionValueDefinition(ident)
		}
		return true
	})
	return aliases, definitions
}

func constructionValueDefinition(ident *ast.Ident) bool {
	switch declaration := ident.Obj.Decl.(type) {
	case *ast.AssignStmt:
		for _, expression := range declaration.Lhs {
			if expression == ident {
				return true
			}
		}
	case *ast.ValueSpec:
		for _, name := range declaration.Names {
			if name == ident {
				return true
			}
		}
	}
	return false
}
