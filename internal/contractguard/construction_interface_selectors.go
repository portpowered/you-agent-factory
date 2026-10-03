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
	contract, ok := declaration.typeSpec.Type.(*ast.InterfaceType)
	if !ok {
		return constructionInterfaceExpressionSelector(declaration.typeSpec.Type, name, declaration.source, source, visited)
	}
	known := true
	for _, field := range contract.Methods.List {
		if len(field.Names) != 0 {
			for _, method := range field.Names {
				if method.Name == name && (ast.IsExported(name) || owner.ImportPath == source.importPath) {
					return true, true
				}
			}
			continue
		}
		matched, complete := constructionInterfaceExpressionSelector(field.Type, name, declaration.source, source, visited)
		if matched {
			return true, true
		}
		known = known && complete
	}
	return false, known
}

// Predeclared interfaces have known method sets. Authored declarations with
// the same name take precedence, including declarations in another source file.
func constructionInterfaceExpressionSelector(expr ast.Expr, name string, declaringSource, source *constructionSource, visited map[ConstructionSymbol]bool) (bool, bool) {
	if ident, ok := expr.(*ast.Ident); ok {
		symbol := ConstructionSymbol{ImportPath: declaringSource.importPath, Name: ident.Name}
		if source.declarations[symbol].typeSpec != nil {
			return constructionInterfaceSelector(symbol, name, source, visited)
		}
		switch ident.Name {
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
