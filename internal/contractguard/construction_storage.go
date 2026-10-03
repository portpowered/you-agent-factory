package contractguard

import (
	"go/ast"
	"go/token"
	"slices"
)

// Storage is declaration-based, not field-name or pointer-type requiredness.
// Different writes to the same result field retain debt rather than selecting
// an arbitrary branch or claiming flow-sensitive equivalence.
func (index constructionIndex) constructionRequiredFields(decl constructionDeclaration, constructor ConstructionConstructor, required map[*ast.Object]string) map[ConstructionSymbol]map[string]string {
	fields := make(map[ConstructionSymbol]map[string]string)
	if decl.function.Body == nil {
		return fields
	}
	p := constructionGuardProvenance{source: decl.source, required: required, mutations: constructionGuardMutations(decl.function.Body)}
	ast.Inspect(decl.function.Body, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.CompositeLit:
			typ, resolved := constructionResultSymbol(value.Type, decl.source)
			if resolved && slices.Contains(constructor.Results, typ) {
				index.constructionLiteralFields(fields, typ, value, p)
			}
		case *ast.AssignStmt:
			for position, left := range value.Lhs {
				selector, ok := left.(*ast.SelectorExpr)
				if !ok || len(value.Lhs) != len(value.Rhs) {
					continue
				}
				typ, resolved := constructionStoredType(selector.X, decl.source, map[*ast.Object]bool{})
				if !resolved || !slices.Contains(constructor.Results, typ) {
					continue
				}
				rule := p.origin(value.Rhs[position], map[*ast.Object]bool{})
				if constructionStorageMutated(selector.X, p.mutations, map[*ast.Object]bool{}) && rule != "" {
					rule = "unresolved-required-dependency-guard"
				}
				mergeConstructionField(fields, typ, selector.Sel.Name, rule)
			}
		}
		return true
	})
	return fields
}

func (index constructionIndex) constructionLiteralFields(fields map[ConstructionSymbol]map[string]string, typ ConstructionSymbol, literal *ast.CompositeLit, p constructionGuardProvenance) {
	names := index.constructionStructFieldNames(typ)
	for position, element := range literal.Elts {
		name, expression := "", element
		if keyed, ok := element.(*ast.KeyValueExpr); ok {
			if ident, ok := keyed.Key.(*ast.Ident); ok {
				name, expression = ident.Name, keyed.Value
			}
		} else if position < len(names) {
			name = names[position]
		}
		if name != "" {
			mergeConstructionField(fields, typ, name, p.origin(expression, map[*ast.Object]bool{}))
		}
	}
}

func mergeConstructionField(fields map[ConstructionSymbol]map[string]string, typ ConstructionSymbol, name, rule string) {
	if fields[typ] == nil {
		fields[typ] = make(map[string]string)
	}
	prior, exists := fields[typ][name]
	if exists && prior != rule {
		rule = "unresolved-required-dependency-guard"
	}
	fields[typ][name] = rule
}

func (index constructionIndex) constructionStructFieldNames(typ ConstructionSymbol) []string {
	decl := index.declarations[typ]
	if decl.typeSpec == nil {
		return nil
	}
	structure, ok := decl.typeSpec.Type.(*ast.StructType)
	if !ok {
		return nil
	}
	var names []string
	for _, field := range structure.Fields.List {
		if len(field.Names) == 0 {
			names = append(names, constructionEmbeddedFieldName(field.Type))
		}
		for _, name := range field.Names {
			names = append(names, name.Name)
		}
	}
	return names
}

func constructionEmbeddedFieldName(expr ast.Expr) string {
	switch value := expr.(type) {
	case *ast.SelectorExpr:
		return value.Sel.Name
	case *ast.StarExpr:
		return constructionEmbeddedFieldName(value.X)
	case *ast.IndexExpr:
		return constructionEmbeddedFieldName(value.X)
	case *ast.IndexListExpr:
		return constructionEmbeddedFieldName(value.X)
	default:
		return constructionReceiver(expr)
	}
}

// Resolve only explicit declared types, allocations and local aliases. A
// function call is not evidence that an arbitrary returned value is the owner.
func constructionStoredType(expr ast.Expr, source *constructionSource, visited map[*ast.Object]bool) (ConstructionSymbol, bool) {
	switch value := expr.(type) {
	case *ast.ParenExpr:
		return constructionStoredType(value.X, source, visited)
	case *ast.UnaryExpr:
		if value.Op == token.AND {
			return constructionStoredType(value.X, source, visited)
		}
	case *ast.CompositeLit:
		return constructionResultSymbol(value.Type, source)
	case *ast.CallExpr:
		if name, ok := value.Fun.(*ast.Ident); ok && name.Name == "new" && name.Obj == nil && len(value.Args) == 1 {
			return constructionResultSymbol(value.Args[0], source)
		}
	case *ast.Ident:
		if value.Obj == nil || visited[value.Obj] {
			return ConstructionSymbol{}, false
		}
		visited[value.Obj] = true
		switch declaration := value.Obj.Decl.(type) {
		case *ast.Field:
			return constructionResultSymbol(declaration.Type, source)
		case *ast.ValueSpec:
			if declaration.Type != nil {
				return constructionResultSymbol(declaration.Type, source)
			}
			for position, name := range declaration.Names {
				if name.Obj == value.Obj && len(declaration.Names) == len(declaration.Values) {
					return constructionStoredType(declaration.Values[position], source, visited)
				}
			}
		case *ast.AssignStmt:
			return constructionStoredType(constructionAssignedValue(value.Obj, declaration.Lhs, declaration.Rhs), source, visited)
		}
	}
	return ConstructionSymbol{}, false
}

func constructionStorageMutated(expr ast.Expr, mutations map[*ast.Object]bool, visited map[*ast.Object]bool) bool {
	switch value := expr.(type) {
	case *ast.ParenExpr:
		return constructionStorageMutated(value.X, mutations, visited)
	case *ast.Ident:
		if value.Obj == nil || visited[value.Obj] {
			return false
		}
		visited[value.Obj] = true
		if mutations[value.Obj] {
			return true
		}
		switch declaration := value.Obj.Decl.(type) {
		case *ast.AssignStmt:
			return constructionStorageMutated(constructionAssignedValue(value.Obj, declaration.Lhs, declaration.Rhs), mutations, visited)
		case *ast.ValueSpec:
			if len(declaration.Names) == 1 && len(declaration.Values) == 1 {
				return constructionStorageMutated(declaration.Values[0], mutations, visited)
			}
		}
	}
	return false
}

func constructionReceiverAlias(expr ast.Expr, receiver *ast.Object, visited map[*ast.Object]bool) bool {
	switch value := expr.(type) {
	case *ast.ParenExpr:
		return constructionReceiverAlias(value.X, receiver, visited)
	case *ast.Ident:
		if value.Obj == nil || receiver == nil || visited[value.Obj] {
			return false
		}
		if value.Obj == receiver {
			return true
		}
		visited[value.Obj] = true
		switch declaration := value.Obj.Decl.(type) {
		case *ast.AssignStmt:
			return constructionReceiverAlias(constructionAssignedValue(value.Obj, declaration.Lhs, declaration.Rhs), receiver, visited)
		case *ast.ValueSpec:
			if len(declaration.Names) == 1 && len(declaration.Values) == 1 {
				return constructionReceiverAlias(declaration.Values[0], receiver, visited)
			}
		}
	}
	return false
}
