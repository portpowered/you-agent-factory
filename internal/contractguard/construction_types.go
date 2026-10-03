package contractguard

import (
	"go/ast"
	"strings"
)

func constructionTypeExpr(expr ast.Expr, source *constructionSource) string {
	switch value := expr.(type) {
	case *ast.Ident:
		if constructionBuiltin(value.Name) {
			return value.Name
		}
		return source.importPath + "." + value.Name
	case *ast.SelectorExpr:
		if ident, ok := value.X.(*ast.Ident); ok && source.imports[ident.Name] != "" {
			return source.imports[ident.Name] + "." + value.Sel.Name
		}
	case *ast.StarExpr:
		return "*" + constructionTypeExpr(value.X, source)
	case *ast.Ellipsis:
		return "..." + constructionTypeExpr(value.Elt, source)
	case *ast.ArrayType:
		if value.Len == nil {
			return "[]" + constructionTypeExpr(value.Elt, source)
		}
		if length, ok := value.Len.(*ast.BasicLit); ok {
			return "[" + length.Value + "]" + constructionTypeExpr(value.Elt, source)
		}
	case *ast.MapType:
		return "map[" + constructionTypeExpr(value.Key, source) + "]" + constructionTypeExpr(value.Value, source)
	case *ast.ChanType:
		prefix := "chan "
		if value.Dir == ast.RECV {
			prefix = "<-chan "
		}
		if value.Dir == ast.SEND {
			prefix = "chan<- "
		}
		return prefix + constructionTypeExpr(value.Value, source)
	case *ast.FuncType:
		result := "func(" + strings.Join(constructionFieldTypes(value.Params, source), ", ") + ")"
		outputs := constructionFieldTypes(value.Results, source)
		if len(outputs) == 1 {
			return result + " " + outputs[0]
		}
		if len(outputs) > 1 {
			return result + " (" + strings.Join(outputs, ", ") + ")"
		}
		return result
	case *ast.InterfaceType:
		if len(value.Methods.List) == 0 {
			return "interface{}"
		}
	case *ast.ParenExpr:
		return constructionTypeExpr(value.X, source)
	}
	return "<unresolved-type>"
}

func constructionBuiltin(name string) bool {
	switch name {
	case "bool", "string", "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64", "uintptr", "byte", "rune", "float32", "float64", "complex64", "complex128", "error", "any":
		return true
	default:
		return false
	}
}

func constructionFieldTypes(fields *ast.FieldList, source *constructionSource) []string {
	var types []string
	if fields == nil {
		return types
	}
	for _, field := range fields.List {
		count := max(1, len(field.Names))
		for range count {
			types = append(types, constructionTypeExpr(field.Type, source))
		}
	}
	return types
}

func constructionResultSymbol(expr ast.Expr, source *constructionSource) (ConstructionSymbol, bool) {
	if pointer, ok := expr.(*ast.StarExpr); ok {
		return constructionResultSymbol(pointer.X, source)
	}
	switch value := expr.(type) {
	case *ast.Ident:
		if !constructionBuiltin(value.Name) {
			return ConstructionSymbol{ImportPath: source.importPath, Name: value.Name}, true
		}
	case *ast.SelectorExpr:
		if ident, ok := value.X.(*ast.Ident); ok && source.imports[ident.Name] != "" {
			return ConstructionSymbol{ImportPath: source.imports[ident.Name], Name: value.Sel.Name}, true
		}
	}
	return ConstructionSymbol{}, false
}
