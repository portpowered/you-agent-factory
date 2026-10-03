package contractguard

import (
	"go/ast"
	"go/token"
)

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
