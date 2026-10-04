package contractguard

import (
	"go/ast"
	"go/token"
)

// Summarize only a single unnamed function result. Explicit alternatives must
// agree on one authored identity; nested closure returns belong to that closure.
func constructionProviderReturn(signature *ast.FuncType, body *ast.BlockStmt, source *constructionSource, visiting map[ast.Node]bool) constructionCallable {
	if body == nil || visiting[body] || signature.Results == nil || len(signature.Results.List) != 1 {
		return constructionCallable{}
	}
	result := signature.Results.List[0]
	if _, callable := result.Type.(*ast.FuncType); !callable || len(result.Names) != 0 {
		return constructionCallable{}
	}
	visiting[body] = true
	defer delete(visiting, body)
	var identity constructionCallable
	found, unknown := false, false
	ast.Inspect(body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		returned, ok := node.(*ast.ReturnStmt)
		if !ok {
			return true
		}
		if len(returned.Results) != 1 {
			unknown = true
			return false
		}
		candidate := constructionProviderCallable(returned.Results[0], source, visiting)
		if candidate.source == nil || (found && (identity.symbol != candidate.symbol || identity.closure != candidate.closure)) {
			unknown = true
		}
		identity, found = candidate, true
		return false
	})
	if !found || unknown {
		return constructionCallable{}
	}
	return identity
}

// Local callable aliases may be called, returned or transferred to another
// stable local alias. Callback transfer, address escape and nonlocal storage
// cannot prove the initializer remains the invoked identity.
func constructionProviderValueEscaped(object *ast.Object, source *constructionSource, visiting map[*ast.Object]bool) bool {
	if visiting[object] {
		return true
	}
	visiting[object] = true
	defer delete(visiting, object)
	aliases, definitions := constructionValueBindings(source.file)
	allowed := make(map[ast.Expr]bool)
	ast.Inspect(source.file, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.CallExpr:
			markConstructionCallee(allowed, value.Fun)
		case *ast.ReturnStmt:
			for _, result := range value.Results {
				markConstructionCallee(allowed, result)
			}
		case *ast.AssignStmt:
			if len(value.Lhs) == 1 && len(value.Rhs) == 1 {
				if name, ok := value.Lhs[0].(*ast.Ident); ok && name.Name == "_" {
					markConstructionCallee(allowed, value.Rhs[0])
				}
			}
		}
		return true
	})
	for target, initializer := range aliases {
		if target == object || source.mutations[target] || constructionProviderPackageObject(target, source) {
			continue
		}
		// Only inspect aliases which actually transfer this binding.
		parts := make(map[ast.Expr]bool)
		markConstructionCallee(parts, initializer)
		for part := range parts {
			if name, ok := part.(*ast.Ident); ok && name.Obj == object && !constructionProviderValueEscaped(target, source, visiting) {
				markConstructionCallee(allowed, initializer)
			}
		}
	}
	escaped := false
	ast.Inspect(source.file, func(node ast.Node) bool {
		if name, ok := node.(*ast.Ident); ok && name.Obj == object && !definitions[name] && !allowed[name] {
			escaped = true
		}
		return !escaped
	})
	return escaped
}

func constructionProviderPackageObject(object *ast.Object, source *constructionSource) bool {
	for _, candidate := range source.packageSources {
		if candidate.file.Scope.Lookup(object.Name) == object {
			return true
		}
	}
	return false
}

// A called variable without a stable authored function or closure cannot
// establish an acyclic provider path. Calling a parameter is dispatch debt,
// even when one caller happens to pass a known callback. Types and builtins
// are not variable calls and must not acquire debt from their spelling.
func constructionProviderCallableDebt(expr ast.Expr, source *constructionSource) bool {
	if parenthesized, ok := expr.(*ast.ParenExpr); ok {
		return constructionProviderCallableDebt(parenthesized.X, source)
	}
	if _, returned := expr.(*ast.CallExpr); returned {
		// The caller first tries a bounded return summary. An unsupported or
		// conflicting identity still cannot establish an acyclic path.
		return true
	}
	if _, selector := expr.(*ast.SelectorExpr); selector {
		// A concrete method or package declaration has qualified identity.
		// Function fields and interface/opaque method dispatch require storage
		// or implementation provenance before they can establish a safe path.
		_, resolved := resolveConstructionCall(expr, source)
		return !resolved
	}
	ident, ok := expr.(*ast.Ident)
	if !ok {
		return false
	}
	object, _, safe := constructionProviderValue(ident, source)
	if object == nil || object.Kind != ast.Var {
		// An unsafe package value still has its declaration in the index.
		for _, candidate := range source.packageSources {
			found := candidate.file.Scope.Lookup(ident.Name)
			if found != nil && found.Kind == ast.Var && (ident.Obj == nil || ident.Obj == found) {
				return true
			}
		}
		return false
	}
	symbol, resolved := resolveConstructionCall(expr, source)
	return !safe || !resolved || source.declarations[symbol].function == nil
}

// Package values need their declaring source's imports and a package-wide
// write check. A local shadow must never fall through to a package binding.
func constructionProviderValue(ident *ast.Ident, source *constructionSource) (*ast.Object, *constructionSource, bool) {
	object, owner := ident.Obj, source
	var packageObject *ast.Object
	for _, candidate := range source.packageSources {
		found := candidate.file.Scope.Lookup(ident.Name)
		if found == nil || (object != nil && object != found) {
			continue
		}
		if packageObject != nil {
			return nil, nil, false
		}
		packageObject, owner = found, candidate
	}
	if packageObject != nil {
		object = packageObject
		if object.Kind != ast.Var || constructionValueInitializer(object) == nil {
			return nil, nil, false
		}
		if constructionProviderPackageValueChanged(object, owner) {
			return nil, nil, false
		}
	}
	return object, owner, object != nil && !owner.mutations[object]
}

// Do not choose a package initializer after any authored write or address
// escape. Cross-file unresolved identifiers resolve only in their own package;
// selectors and local shadows cannot write this binding by name.
func constructionProviderPackageValueChanged(object *ast.Object, source *constructionSource) bool {
	changed := false
	matches := func(expr ast.Expr) bool {
		for {
			parenthesized, ok := expr.(*ast.ParenExpr)
			if !ok {
				break
			}
			expr = parenthesized.X
		}
		ident, ok := expr.(*ast.Ident)
		return ok && (ident.Obj == object || (ident.Obj == nil && ident.Name == object.Name))
	}
	for _, candidate := range source.packageSources {
		ast.Inspect(candidate.file, func(node ast.Node) bool {
			switch value := node.(type) {
			case *ast.AssignStmt:
				for _, target := range value.Lhs {
					changed = changed || matches(target)
				}
			case *ast.RangeStmt:
				changed = changed || matches(value.Key) || matches(value.Value)
			case *ast.UnaryExpr:
				changed = changed || (value.Op == token.AND && matches(value.X))
			}
			return !changed
		})
		if changed {
			return true
		}
	}
	return false
}
