package analyzers

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// Each compilation unit validates declarations it owns. Compiler-invalid
// selectors and aliases are rejected by Go before the analyzer runs.
func validateRegisteredConstruction(pass *analysis.Pass, registry ConstructionRegistry) error {
	sets := map[string]bool{}
	for _, set := range registry.CapabilitySets {
		if set.Name == "" || strings.TrimSpace(set.OwnerTask) == "" || (set.Mode != ConstructionReport && set.Mode != ConstructionEnforce) {
			return fmt.Errorf("invalid capability set %q", set.Name)
		}
		if sets[set.Name] {
			return fmt.Errorf("duplicate capability set %q", set.Name)
		}
		sets[set.Name] = true
	}
	classified := map[ConstructionSymbol]ConstructionType{}
	for _, typ := range registry.Types {
		if !validRegisteredSymbol(typ.Symbol) || !sets[typ.CapabilitySet] {
			return fmt.Errorf("invalid type metadata %s", typ.Symbol)
		}
		if _, exists := classified[typ.Symbol]; exists {
			return fmt.Errorf("duplicate type %s", typ.Symbol)
		}
		switch typ.Kind {
		case ConstructionBehavior, ConstructionEffect, ConstructionState, ConstructionResource, ConstructionDomain:
		default:
			return fmt.Errorf("invalid construction kind %s", typ.Symbol)
		}
		classified[typ.Symbol] = typ
		if typ.Symbol.ImportPath == pass.Pkg.Path() {
			if _, ok := pass.Pkg.Scope().Lookup(typ.Symbol.Name).(*types.TypeName); !ok {
				return fmt.Errorf("missing type declaration %s", typ.Symbol)
			}
		}
	}
	constructors := map[ConstructionSymbol]bool{}
	for _, constructor := range registry.Constructors {
		if !validRegisteredSymbol(constructor.Symbol) || !sets[constructor.CapabilitySet] {
			return fmt.Errorf("invalid constructor metadata %s", constructor.Symbol)
		}
		if constructors[constructor.Symbol] {
			return fmt.Errorf("duplicate constructor %s", constructor.Symbol)
		}
		constructors[constructor.Symbol] = true
		for _, result := range constructor.Results {
			if typ, ok := classified[result]; !ok || typ.CapabilitySet != constructor.CapabilitySet {
				return fmt.Errorf("constructor result requires matching type classification %s", result)
			}
		}
		if constructor.Symbol.ImportPath != pass.Pkg.Path() {
			continue
		}
		fn := registeredConstructorDeclaration(pass.Pkg, constructor.Symbol)
		if fn == nil {
			return fmt.Errorf("missing constructor declaration %s", constructor.Symbol)
		}
		if err := validateRegisteredSignature(fn, constructor); err != nil {
			return err
		}
	}
	seen := map[string]bool{}
	for _, a := range registry.Allowances {
		key := a.FilePath + "|" + a.Caller.String() + "|" + a.Callee.String()
		if seen[key] {
			return fmt.Errorf("duplicate allowance %s", key)
		}
		seen[key] = true
		if !validRegisteredSymbol(a.Caller) || !validRegisteredSymbol(a.Callee) || !constructors[a.Callee] ||
			a.FilePath == "." || path.IsAbs(a.FilePath) || path.Clean(a.FilePath) != a.FilePath ||
			strings.ContainsAny(a.FilePath, "*?\\:") || strings.HasPrefix(a.FilePath, "../") ||
			strings.TrimSpace(a.OwnerTask) == "" || strings.TrimSpace(a.Reason) == "" {
			return fmt.Errorf("invalid exact allowance %s", key)
		}
		switch a.Kind {
		case "focused-provider", "boundary-normalization", "leaf-effect", "scoped-view":
		default:
			return fmt.Errorf("invalid allowance kind %s", key)
		}
		if a.Caller.ImportPath == pass.Pkg.Path() {
			if err := validateRegisteredAllowance(pass, a); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateRegisteredAllowance(pass *analysis.Pass, allowance ConstructionAllowance) error {
	values := registeredConstructionValues(pass)
	for _, file := range pass.Files {
		filename := pass.Fset.Position(file.Pos()).Filename
		if ast.IsGenerated(file) || strings.HasSuffix(filename, "_test.go") {
			continue
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || registeredConstructionSymbol(pass.TypesInfo.Defs[fn.Name]) != allowance.Caller {
				continue
			}
			unit, _ := unitKey(pass)
			if fn.Body == nil || unit+"/"+filepath.Base(filename) != allowance.FilePath {
				break
			}
			found := false
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				if call, ok := node.(*ast.CallExpr); ok {
					found = found || values.resolve(pass, call.Fun, map[types.Object]bool{}) == allowance.Callee
				}
				return true
			})
			if !found {
				return fmt.Errorf("stale allowance callee %s", allowance.Callee)
			}
			return nil
		}
	}
	return fmt.Errorf("stale allowance caller/path %s", allowance.Caller)
}

func registeredConstructorDeclaration(pkg *types.Package, symbol ConstructionSymbol) *types.Func {
	if symbol.Receiver == "" {
		fn, _ := pkg.Scope().Lookup(symbol.Name).(*types.Func)
		return fn
	}
	obj, ok := pkg.Scope().Lookup(symbol.Receiver).(*types.TypeName)
	if !ok {
		return nil
	}
	objMethod, _, _ := types.LookupFieldOrMethod(types.NewPointer(obj.Type()), true, pkg, symbol.Name)
	fn, _ := objMethod.(*types.Func)
	if fn == nil || registeredConstructionSymbol(fn) != symbol {
		return nil
	}
	return fn
}

func validateRegisteredSignature(fn *types.Func, constructor ConstructionConstructor) error {
	sig := fn.Type().(*types.Signature)
	seen := map[int]bool{}
	for _, param := range constructor.RequiredParameters {
		if param.Index < 0 || param.Index >= sig.Params().Len() || seen[param.Index] {
			return fmt.Errorf("invalid or duplicate parameter index %s", constructor.Symbol)
		}
		seen[param.Index] = true
		actual := types.TypeString(sig.Params().At(param.Index).Type(), func(pkg *types.Package) string { return pkg.Path() })
		if actual != param.TypeExpr {
			return fmt.Errorf("parameter type mismatch %s: got %s, want %s", constructor.Symbol, actual, param.TypeExpr)
		}
	}
	var results []ConstructionSymbol
	for i := 0; i < sig.Results().Len(); i++ {
		typ := types.Unalias(sig.Results().At(i).Type())
		if types.Identical(typ, types.Universe.Lookup("error").Type()) {
			continue
		}
		if pointer, ok := typ.(*types.Pointer); ok {
			typ = types.Unalias(pointer.Elem())
		}
		named, ok := typ.(*types.Named)
		if !ok {
			return fmt.Errorf("unclassified constructor result %s", constructor.Symbol)
		}
		results = append(results, registeredConstructionSymbol(named.Origin().Obj()))
	}
	if len(results) == 0 || !slices.Equal(results, constructor.Results) {
		return fmt.Errorf("result type mismatch %s", constructor.Symbol)
	}
	return nil
}

func validRegisteredSymbol(symbol ConstructionSymbol) bool {
	return symbol.ImportPath != "" && !strings.ContainsAny(symbol.ImportPath, "*?\\ \t\n") && path.Clean(symbol.ImportPath) == symbol.ImportPath &&
		token.IsIdentifier(symbol.Name) && (symbol.Receiver == "" || token.IsIdentifier(symbol.Receiver))
}
