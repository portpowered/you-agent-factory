package contractguard

import "go/ast"

// Search one embedding depth at a time, as Go selector lookup does. Two paths
// to the same declaration still make a selector ambiguous. Saturate path counts
// at two and discard previously searched depths to bound diamonds and cycles.
func constructionPromotedMethod(typ ConstructionSymbol, name string, source *constructionSource) (ConstructionSymbol, bool) {
	level := map[ConstructionSymbol]int{typ: 1}
	visited := make(map[ConstructionSymbol]bool)
	for len(level) != 0 {
		next := make(map[ConstructionSymbol]int)
		matches := 0
		unknown, unknownEmbedding := false, false
		var selected ConstructionSymbol
		for owner, paths := range level {
			method := ConstructionSymbol{ImportPath: owner.ImportPath, Receiver: owner.Name, Name: name}
			if declaration, exists := source.declarations[method]; exists && (ast.IsExported(name) || owner.ImportPath == source.importPath) {
				if declaration.function == nil {
					return ConstructionSymbol{}, false
				}
				matches += paths
				selected = method
				continue
			}
			if matched, known := constructionInterfaceSelector(owner, name, source, map[ConstructionSymbol]bool{}); known {
				if matched {
					matches += paths
				}
				continue
			}
			structure, declaringSource, known := constructionUnderlyingStruct(owner, source.declarations, map[ConstructionSymbol]bool{})
			if structure == nil {
				// An unavailable or interface embedding might hide a deeper method.
				unknown = unknown || !known
				continue
			}
			for _, field := range structure.Fields.List {
				if constructionSelectorField(field, name) {
					matches += paths
				}
				if len(field.Names) != 0 {
					continue
				}
				embedded, resolved := constructionResultSymbol(field.Type, declaringSource)
				if resolved {
					embedded, resolved = constructionCanonicalType(embedded, source.declarations, map[ConstructionSymbol]bool{})
				}
				if !resolved {
					unknownEmbedding = true
					continue
				}
				if !visited[embedded] {
					next[embedded] = min(2, next[embedded]+paths)
				}
			}
		}
		if matches != 0 {
			return selected, !unknown && matches == 1 && selected.Name != ""
		}
		if unknown || unknownEmbedding {
			return ConstructionSymbol{}, false
		}
		for owner := range level {
			visited[owner] = true
			delete(next, owner)
		}
		level = next
	}
	return ConstructionSymbol{}, false
}

func constructionSelectorField(field *ast.Field, name string) bool {
	if len(field.Names) == 0 {
		return constructionEmbeddedFieldName(field.Type) == name
	}
	for _, fieldName := range field.Names {
		if fieldName.Name == name {
			return true
		}
	}
	return false
}

// Defined struct types retain their fields and embedded-method promotion, but
// do not inherit methods declared on the underlying named type itself.
func constructionUnderlyingStruct(typ ConstructionSymbol, declarations map[ConstructionSymbol]constructionDeclaration, visited map[ConstructionSymbol]bool) (*ast.StructType, *constructionSource, bool) {
	declaration := declarations[typ]
	if visited[typ] || declaration.typeSpec == nil {
		return nil, nil, false
	}
	visited[typ] = true
	if structure, ok := declaration.typeSpec.Type.(*ast.StructType); ok {
		return structure, declaration.source, true
	}
	switch underlying := declaration.typeSpec.Type.(type) {
	case *ast.Ident:
		if constructionBuiltin(underlying.Name) {
			return nil, declaration.source, true
		}
	case *ast.ArrayType, *ast.MapType, *ast.ChanType, *ast.FuncType:
		return nil, declaration.source, true
	}
	underlying, resolved := constructionResultSymbol(declaration.typeSpec.Type, declaration.source)
	if !resolved {
		return nil, nil, false
	}
	return constructionUnderlyingStruct(underlying, declarations, visited)
}
