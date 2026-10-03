package contractguard

import (
	"fmt"
	"go/token"
	"path"
	"slices"
	"strings"
)

func (index constructionIndex) validate(registry ConstructionRegistry) error {
	sets := make(map[string]ConstructionCapabilitySet)
	for _, set := range registry.CapabilitySets {
		if set.Name == "" || set.OwnerTask == "" || (set.Mode != ConstructionReport && set.Mode != ConstructionEnforce) {
			return fmt.Errorf("construction registry: invalid capability set %q", set.Name)
		}
		if _, exists := sets[set.Name]; exists {
			return fmt.Errorf("construction registry: duplicate capability set %q", set.Name)
		}
		sets[set.Name] = set
	}
	if err := index.validateTypes(registry.Types, sets); err != nil {
		return err
	}
	constructors := make(map[ConstructionSymbol]bool)
	for _, constructor := range registry.Constructors {
		if constructors[constructor.Symbol] {
			return index.metadataError(constructor.Symbol, "duplicate constructor")
		}
		constructors[constructor.Symbol] = true
		if _, exists := sets[constructor.CapabilitySet]; !exists {
			return index.metadataError(constructor.Symbol, "unknown capability set")
		}
		if err := index.validateConstructor(constructor, registry.Types); err != nil {
			return err
		}
	}
	return index.validateAllowances(registry.Allowances, constructors)
}

func (index constructionIndex) validateTypes(types []ConstructionType, sets map[string]ConstructionCapabilitySet) error {
	seen := make(map[ConstructionSymbol]bool)
	for _, typ := range types {
		if !validConstructionSymbol(typ.Symbol) || index.declarations[typ.Symbol].typeSpec == nil {
			return index.metadataError(typ.Symbol, "missing or ambiguous type declaration")
		}
		if seen[typ.Symbol] {
			return index.metadataError(typ.Symbol, "duplicate type")
		}
		seen[typ.Symbol] = true
		if _, exists := sets[typ.CapabilitySet]; !exists {
			return index.metadataError(typ.Symbol, "unknown capability set")
		}
		switch typ.Kind {
		case ConstructionBehavior, ConstructionEffect, ConstructionState, ConstructionResource, ConstructionDomain:
		default:
			return index.metadataError(typ.Symbol, "invalid construction kind")
		}
	}
	return nil
}

func (index constructionIndex) validateConstructor(constructor ConstructionConstructor, types []ConstructionType) error {
	decl := index.declarations[constructor.Symbol]
	if !validConstructionSymbol(constructor.Symbol) || decl.function == nil {
		return index.metadataError(constructor.Symbol, "missing or ambiguous constructor declaration")
	}
	parameters := constructionFieldTypes(decl.function.Type.Params, decl.source)
	seen := make(map[int]bool)
	for _, param := range constructor.RequiredParameters {
		if param.Index < 0 || param.Index >= len(parameters) || seen[param.Index] {
			return index.metadataError(constructor.Symbol, "invalid or duplicate parameter index")
		}
		seen[param.Index] = true
		actual := parameters[param.Index]
		if strings.Contains(actual, "<unresolved-type>") || actual != param.TypeExpr {
			return index.metadataError(constructor.Symbol, "parameter type mismatch or unresolved type")
		}
	}
	var results []ConstructionSymbol
	if decl.function.Type.Results != nil {
		for _, result := range decl.function.Type.Results.List {
			if symbol, ok := constructionResultSymbol(result.Type, decl.source); ok {
				for range max(1, len(result.Names)) {
					results = append(results, symbol)
				}
			} else if constructionTypeExpr(result.Type, decl.source) != "error" {
				return index.metadataError(constructor.Symbol, "unclassified constructor result")
			}
		}
	}
	if len(results) == 0 || !slices.Equal(results, constructor.Results) {
		return index.metadataError(constructor.Symbol, "result type mismatch")
	}
	for _, result := range results {
		if !slices.ContainsFunc(types, func(typ ConstructionType) bool {
			return typ.Symbol == result && typ.CapabilitySet == constructor.CapabilitySet
		}) {
			return index.metadataError(result, "constructor result requires matching type classification")
		}
	}
	return nil
}

func validConstructionSymbol(symbol ConstructionSymbol) bool {
	return symbol.ImportPath != "" && !strings.ContainsAny(symbol.ImportPath, "*?\\ \t\n") && path.Clean(symbol.ImportPath) == symbol.ImportPath &&
		token.IsIdentifier(symbol.Name) && (symbol.Receiver == "" || token.IsIdentifier(symbol.Receiver))
}

func (index constructionIndex) metadataError(symbol ConstructionSymbol, reason string) error {
	file := "<missing>"
	if decl := index.declarations[symbol]; decl.source != nil {
		file = decl.source.path
	}
	return fmt.Errorf("construction registry: %s %s: %s", file, symbol, reason)
}
