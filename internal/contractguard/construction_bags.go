package contractguard

import "go/ast"

// A required record that contains a classified collaborator is a dependency
// bag. Direct collaborators and explicitly classified domain/state/resource
// records retain their semantics; names and pointer-ness are not evidence.
func (index constructionIndex) scanConstructionDependencyBags(registry ConstructionRegistry) []ConstructionFinding {
	var findings []ConstructionFinding
	for _, constructor := range registry.Constructors {
		decl := index.declarations[constructor.Symbol]
		parameters := constructionParameterFields(decl.function.Type.Params)
		for _, required := range constructor.RequiredParameters {
			field := parameters[required.Index]
			if !index.constructionDependencyBag(field.Type, decl.source, registry, map[ConstructionSymbol]bool{}) {
				continue
			}
			findings = append(findings, ConstructionFinding{
				FilePath: decl.source.path, Line: decl.source.set.Position(field.Pos()).Line,
				Caller: constructor.Symbol, Callee: constructor.Symbol,
				CapabilitySet: constructor.CapabilitySet, Mode: constructionSetMode(constructor.CapabilitySet, registry),
				Rule: "required-dependency-bag",
			})
		}
	}
	return findings
}

func constructionParameterFields(fields *ast.FieldList) []*ast.Field {
	var parameters []*ast.Field
	if fields != nil {
		for _, field := range fields.List {
			for range max(1, len(field.Names)) {
				parameters = append(parameters, field)
			}
		}
	}
	return parameters
}

func constructionSetMode(name string, registry ConstructionRegistry) ConstructionMode {
	for _, set := range registry.CapabilitySets {
		if set.Name == name {
			return set.Mode
		}
	}
	return ConstructionReport
}

func (index constructionIndex) constructionDependencyBag(expr ast.Expr, source *constructionSource, registry ConstructionRegistry, visited map[ConstructionSymbol]bool) bool {
	if mapping, ok := expr.(*ast.MapType); ok {
		return index.constructionDependencyBag(mapping.Key, source, registry, visited) ||
			index.constructionDependencyBag(mapping.Value, source, registry, visited)
	}
	if nested := constructionContainerElement(expr); nested != nil {
		return index.constructionDependencyBag(nested, source, registry, visited)
	}
	if record, ok := expr.(*ast.StructType); ok {
		return index.constructionBagFields(record, source, registry, visited)
	}
	symbol, resolved := constructionResultSymbol(expr, source)
	if !resolved || visited[symbol] {
		return false
	}
	// Any explicit kind is authoritative. In particular a constructed service
	// itself is not a bag merely because it stores its injected collaborators.
	for _, typ := range registry.Types {
		if typ.Symbol == symbol {
			return false
		}
	}
	decl := index.declarations[symbol]
	if decl.typeSpec == nil {
		return false
	}
	visited[symbol] = true
	defer delete(visited, symbol)
	return index.constructionDependencyBag(decl.typeSpec.Type, decl.source, registry, visited)
}

func (index constructionIndex) constructionBagFields(record *ast.StructType, source *constructionSource, registry ConstructionRegistry, visited map[ConstructionSymbol]bool) bool {
	for _, field := range record.Fields.List {
		if index.constructionBagCollaborator(field.Type, source, registry, map[ConstructionSymbol]bool{}) ||
			index.constructionDependencyBag(field.Type, source, registry, visited) {
			return true
		}
	}
	return false
}

// Follow container and authored alias/defined-type chains inside a record.
// Defined records inherit their underlying fields; unlike method resolution,
// their dependency storage is unchanged by defining a new named type.
func (index constructionIndex) constructionBagCollaborator(expr ast.Expr, source *constructionSource, registry ConstructionRegistry, visited map[ConstructionSymbol]bool) bool {
	if mapping, ok := expr.(*ast.MapType); ok {
		return index.constructionBagCollaborator(mapping.Key, source, registry, visited) ||
			index.constructionBagCollaborator(mapping.Value, source, registry, visited)
	}
	if nested := constructionContainerElement(expr); nested != nil {
		return index.constructionBagCollaborator(nested, source, registry, visited)
	}
	symbol, resolved := constructionResultSymbol(expr, source)
	if !resolved || visited[symbol] {
		return false
	}
	for _, typ := range registry.Types {
		if typ.Symbol == symbol {
			return typ.Kind == ConstructionBehavior || typ.Kind == ConstructionEffect
		}
	}
	decl := index.declarations[symbol]
	if decl.typeSpec == nil {
		return false
	}
	visited[symbol] = true
	defer delete(visited, symbol)
	return index.constructionBagCollaborator(decl.typeSpec.Type, decl.source, registry, visited)
}

func constructionContainerElement(expr ast.Expr) ast.Expr {
	switch value := expr.(type) {
	case *ast.StarExpr:
		return value.X
	case *ast.ParenExpr:
		return value.X
	case *ast.ArrayType:
		return value.Elt
	case *ast.Ellipsis:
		return value.Elt
	case *ast.MapType:
		return value.Value
	case *ast.ChanType:
		return value.Value
	}
	return nil
}
