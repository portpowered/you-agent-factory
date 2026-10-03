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
		underlying, resolved := constructionResultSymbol(declaration.typeSpec.Type, declaration.source)
		if !resolved {
			return false, false
		}
		return constructionInterfaceSelector(underlying, name, source, visited)
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
		embedded, resolved := constructionResultSymbol(field.Type, declaration.source)
		if !resolved {
			known = false
			continue
		}
		matched, complete := constructionInterfaceSelector(embedded, name, source, visited)
		if matched {
			return true, true
		}
		known = known && complete
	}
	return false, known
}
