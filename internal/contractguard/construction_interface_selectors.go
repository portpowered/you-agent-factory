package contractguard

import "go/ast"

// Interface methods occupy the embedding's selector depth, even when the
// interface itself embeds another interface. This proves only whether a named
// selector competes with a concrete method, never implementation identity.
func constructionInterfaceSelector(owner ConstructionSymbol, name string, source *constructionSource, visited map[ConstructionSymbol]bool) (bool, bool) {
	declaration := source.declarations[owner]
	if visited[owner] || declaration.typeSpec == nil {
		return false, false
	}
	visited[owner] = true
	defer delete(visited, owner)
	return constructionInterfaceExpressionSelector(declaration.typeSpec.Type, name, declaration.source, source, visited)
}

// Predeclared interfaces have known method sets. Authored declarations with
// the same name take precedence, including declarations in another source file.
func constructionInterfaceExpressionSelector(expr ast.Expr, name string, declaringSource, source *constructionSource, visited map[ConstructionSymbol]bool) (bool, bool) {
	switch value := expr.(type) {
	case *ast.ParenExpr:
		return constructionInterfaceExpressionSelector(value.X, name, declaringSource, source, visited)
	case *ast.InterfaceType:
		return constructionInterfaceMethodsSelector(value, name, declaringSource, source, visited)
	case *ast.Ident:
		symbol := ConstructionSymbol{ImportPath: declaringSource.importPath, Name: value.Name}
		if source.declarations[symbol].typeSpec != nil {
			return constructionInterfaceSelector(symbol, name, source, visited)
		}
		switch value.Name {
		case "any":
			return false, true
		case "error":
			return name == "Error", true
		}
	}
	symbol, resolved := constructionResultSymbol(expr, declaringSource)
	if !resolved {
		return false, false
	}
	return constructionInterfaceSelector(symbol, name, source, visited)
}

func constructionInterfaceMethodsSelector(contract *ast.InterfaceType, name string, declaringSource, source *constructionSource, visited map[ConstructionSymbol]bool) (bool, bool) {
	known := true
	for _, field := range contract.Methods.List {
		if len(field.Names) != 0 {
			for _, method := range field.Names {
				if method.Name == name && (ast.IsExported(name) || declaringSource.importPath == source.importPath) {
					return true, true
				}
			}
			continue
		}
		matched, complete := constructionInterfaceExpressionSelector(field.Type, name, declaringSource, source, visited)
		if matched {
			return true, true
		}
		known = known && complete
	}
	return false, known
}
