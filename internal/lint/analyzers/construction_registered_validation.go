package analyzers

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"path"
	"slices"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// Each compilation unit validates declarations it owns. Compiler-invalid
// selectors and aliases are rejected by Go before the analyzer runs.
func validateRegisteredConstruction(pass *analysis.Pass, registry ConstructionRegistry) error {
	sets, err := validateRegisteredSets(registry)
	if err != nil {
		return err
	}
	classified, err := validateRegisteredTypes(pass, registry, sets)
	if err != nil {
		return err
	}
	_, err = validateRegisteredConstructors(pass, registry, sets, classified)
	if err != nil {
		return err
	}
	return nil
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

func validateRegisteredSets(registry ConstructionRegistry) (map[string]bool, error) {
	sets := map[string]bool{}
	for _, set := range registry.CapabilitySets {
		if set.Name == "" || strings.TrimSpace(set.OwnerTask) == "" || (set.Mode != ConstructionReport && set.Mode != ConstructionEnforce) {
			return nil, fmt.Errorf("invalid capability set %q", set.Name)
		}
		if sets[set.Name] {
			return nil, fmt.Errorf("duplicate capability set %q", set.Name)
		}
		sets[set.Name] = true
	}
	return sets, nil
}

func validateRegisteredTypes(pass *analysis.Pass, registry ConstructionRegistry, sets map[string]bool) (map[ConstructionSymbol]ConstructionType, error) {
	classified := map[ConstructionSymbol]ConstructionType{}
	for _, typ := range registry.Types {
		if !validRegisteredSymbol(typ.Symbol) || !sets[typ.CapabilitySet] {
			return nil, fmt.Errorf("invalid type metadata %s", typ.Symbol)
		}
		if _, exists := classified[typ.Symbol]; exists {
			return nil, fmt.Errorf("duplicate type %s", typ.Symbol)
		}
		switch typ.Kind {
		case ConstructionBehavior, ConstructionEffect, ConstructionState, ConstructionResource, ConstructionDomain:
		default:
			return nil, fmt.Errorf("invalid construction kind %s", typ.Symbol)
		}
		classified[typ.Symbol] = typ
		if typ.Symbol.ImportPath == pass.Pkg.Path() {
			if _, ok := pass.Pkg.Scope().Lookup(typ.Symbol.Name).(*types.TypeName); !ok {
				return nil, fmt.Errorf("missing type declaration %s", typ.Symbol)
			}
		}
	}
	return classified, nil
}

func validateRegisteredConstructors(pass *analysis.Pass, registry ConstructionRegistry, sets map[string]bool, classified map[ConstructionSymbol]ConstructionType) (map[ConstructionSymbol]bool, error) {
	constructors := map[ConstructionSymbol]bool{}
	for _, constructor := range registry.Constructors {
		if !validRegisteredSymbol(constructor.Symbol) || !sets[constructor.CapabilitySet] {
			return nil, fmt.Errorf("invalid constructor metadata %s", constructor.Symbol)
		}
		if constructors[constructor.Symbol] {
			return nil, fmt.Errorf("duplicate constructor %s", constructor.Symbol)
		}
		constructors[constructor.Symbol] = true
		for _, result := range constructor.Results {
			if typ, ok := classified[result]; !ok || typ.CapabilitySet != constructor.CapabilitySet {
				return nil, fmt.Errorf("constructor result requires matching type classification %s", result)
			}
		}
		if constructor.Symbol.ImportPath != pass.Pkg.Path() {
			continue
		}
		fn := registeredConstructorDeclaration(pass.Pkg, constructor.Symbol)
		if fn == nil {
			return nil, fmt.Errorf("missing constructor declaration %s", constructor.Symbol)
		}
		if err := validateRegisteredSignature(fn, constructor); err != nil {
			return nil, err
		}
	}
	return constructors, nil
}

// A classified result stays governed when an owner adds another constructor.
// Inspect compiler objects in the selected source, never dependency source or
// a second repository inventory. Explicit signatures remain authoritative.
func registeredUnlistedConstructors(pass *analysis.Pass, registry ConstructionRegistry) ConstructionRegistry {
	registry.Constructors = slices.Clone(registry.Constructors)
	seen := map[ConstructionSymbol]bool{}
	for _, constructor := range registry.Constructors {
		seen[constructor.Symbol] = true
	}
	for _, file := range pass.Files {
		if ast.IsGenerated(file) || strings.HasSuffix(pass.Fset.Position(file.Pos()).Filename, "_test.go") {
			continue
		}
		ast.Inspect(file, func(node ast.Node) bool {
			id, ok := node.(*ast.Ident)
			if !ok {
				return true
			}
			fn, ok := pass.TypesInfo.ObjectOf(id).(*types.Func)
			if !ok || !serviceConstructorName(fn.Name()) {
				return true
			}
			fn = fn.Origin()
			symbol := registeredConstructionSymbol(fn)
			if seen[symbol] {
				return true
			}
			seen[symbol] = true
			if constructor, ok := registeredResultConstructor(fn, registry); ok {
				registry.Constructors = append(registry.Constructors, constructor)
			}
			return true
		})
	}
	return registry
}

func registeredResultConstructor(fn *types.Func, registry ConstructionRegistry) (ConstructionConstructor, bool) {
	constructor := ConstructionConstructor{Symbol: registeredConstructionSymbol(fn)}
	enforced := map[string]bool{}
	for _, set := range registry.CapabilitySets {
		enforced[set.Name] = set.Mode == ConstructionEnforce
	}
	sig := fn.Type().(*types.Signature)
	for i := 0; i < sig.Results().Len(); i++ {
		if typ, ok := registeredClassifiedType(sig.Results().At(i).Type(), registry.Types); ok &&
			(typ.Kind == ConstructionBehavior || typ.Kind == ConstructionEffect) {
			if constructor.CapabilitySet == "" || enforced[typ.CapabilitySet] {
				constructor.CapabilitySet = typ.CapabilitySet
			}
			constructor.Results = append(constructor.Results, typ.Symbol)
		}
	}
	for i := 0; i < sig.Params().Len(); i++ {
		param := sig.Params().At(i)
		if typ, ok := registeredClassifiedType(param.Type(), registry.Types); ok &&
			(typ.Kind == ConstructionBehavior || typ.Kind == ConstructionEffect) {
			constructor.RequiredParameters = append(constructor.RequiredParameters, ConstructionParameter{
				Index: i, TypeExpr: types.TypeString(param.Type(), func(pkg *types.Package) string { return pkg.Path() }),
			})
		}
	}
	return constructor, constructor.CapabilitySet != ""
}

func registeredClassifiedType(typ types.Type, classified []ConstructionType) (ConstructionType, bool) {
	typ = types.Unalias(typ)
	if pointer, ok := typ.(*types.Pointer); ok {
		typ = types.Unalias(pointer.Elem())
	}
	if named, ok := typ.(*types.Named); ok {
		symbol := registeredConstructionSymbol(named.Origin().Obj())
		for _, entry := range classified {
			if entry.Symbol == symbol {
				return entry, true
			}
		}
	}
	return ConstructionType{}, false
}
