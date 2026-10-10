package analyzers

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// RegisteredConstruction migrates qualified construction calls and references.
// Report mode never enables an owner.
var RegisteredConstruction = registeredConstructionAnalyzer(RepositoryConstructionRegistry())

type ConstructionFinding struct {
	FilePath      string
	Line          int
	Caller        ConstructionSymbol
	Callee        ConstructionSymbol
	CapabilitySet string
	Mode          ConstructionMode
	Rule          string
}

func registeredConstructionAnalyzer(registry ConstructionRegistry) *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:       "registeredconstruction",
		Doc:        "resolve classified construction calls and unresolved references with go/types",
		ResultType: reflect.TypeOf([]ConstructionFinding{}),
		FactTypes:  []analysis.Fact{new(registeredConstructionKindFact), new(registeredGetterFact)},
		Run:        func(pass *analysis.Pass) (any, error) { return runRegisteredConstruction(pass, registry) },
	}
}

func runRegisteredConstruction(pass *analysis.Pass, registry ConstructionRegistry) (any, error) {
	unit, ok := unitKey(pass)
	findings := []ConstructionFinding{}
	if !ok {
		return findings, nil
	}
	if err := validateRegisteredConstruction(pass, registry); err != nil {
		pass.Reportf(pass.Files[0].Package, "construction-metadata: %s", err)
		return findings, nil
	}
	registry = registeredUnlistedConstructors(pass, registry)
	values := registeredConstructionValues(pass)
	helpers := registeredConstructionHelpers(pass, values)
	registry = helpers.storageResults(registry)
	var blocking []violation
	add := func(caller, callee ConstructionSymbol, constructor ConstructionConstructor, rule string, pos token.Pos) {
		mode := ConstructionReport
		for _, set := range registry.CapabilitySets {
			if set.Name == constructor.CapabilitySet {
				mode = set.Mode
			}
		}
		position := pass.Fset.Position(pos)
		finding := ConstructionFinding{
			FilePath: unit + "/" + filepath.Base(position.Filename), Line: position.Line,
			Caller: caller, Callee: callee, CapabilitySet: constructor.CapabilitySet, Mode: mode, Rule: rule,
		}
		findings = append(findings, finding)
		if mode == ConstructionEnforce {
			blocking = append(blocking, violation{rule: rule, importer: unit,
				importee: caller.String() + "->" + callee.String(), pos: pos,
				hint: fmt.Sprintf("set=%s mode=%s; inject the classified collaborator through its focused provider", constructor.CapabilitySet, mode)})
		}
	}
	typeset := scanRegisteredConstructionBags(pass, registry, add)
	stored := registeredConstructionStorage(pass, registry, helpers)
	scanRegisteredConstructionGuards(pass, registry, helpers, stored, add)
	getters := registeredConstructionGetters(pass, registry, helpers, stored, typeset)
	for _, file := range pass.Files {
		if ast.IsGenerated(file) || strings.HasSuffix(pass.Fset.Position(file.Pos()).Filename, "_test.go") {
			continue
		}
		for _, decl := range file.Decls {
			caller := ConstructionSymbol{ImportPath: pass.Pkg.Path(), Name: "<package>"}
			if fn, ok := decl.(*ast.FuncDecl); ok {
				caller = registeredConstructionSymbol(pass.TypesInfo.Defs[fn.Name])
			}
			approved := registeredCompositionCaller(caller, caller)
			recursive, debt := false, false
			var indirect map[*ast.CallExpr]bool
			if approved {
				recursive, debt = helpers.providerPath(caller)
				indirect = registeredIndirectProviderCalls(decl)
			}
			called := map[ast.Expr]bool{}
			markRegisteredWireProviders(pass, values, decl, caller, called)
			scanRegisteredCalls(pass, registry, values, getters, decl, caller, called, indirect, recursive, debt, add)
			scanRegisteredReferences(pass, registry, values, getters, decl, caller, called, add)
		}
	}
	reportAgainstBaseline(pass, unit, setOf("registered-construction", "unresolved-construction-reference", "unresolved-focused-provider-dispatch", "required-dependency-bag",
		"required-dependency-guard", "required-receiver-guard", "required-dependency-assertion-guard", "unresolved-required-dependency-guard",
		"service-getter-locator", "unresolved-service-getter-locator", "unresolved-service-getter-reference"), blocking, false)
	return findings, nil
}

func scanRegisteredCalls(pass *analysis.Pass, registry ConstructionRegistry, values registeredValues,
	getters map[ConstructionSymbol]registeredGetterFact, decl ast.Decl, caller ConstructionSymbol,
	called map[ast.Expr]bool, indirect map[*ast.CallExpr]bool, recursive, debt bool,
	add func(ConstructionSymbol, ConstructionSymbol, ConstructionConstructor, string, token.Pos),
) {
	ast.Inspect(decl, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		markRegisteredCallee(called, call.Fun)
		callee := values.resolve(pass, call.Fun, map[types.Object]bool{})
		if getter, ok := getters[callee]; ok {
			add(caller, callee, getter.Constructor, getter.Rule, call.Pos())
		}
		for _, constructor := range registry.Constructors {
			if constructor.Symbol != callee || !registeredProhibitedKind(constructor, registry.Types) {
				continue
			}
			allowed := registeredCompositionCaller(caller, callee)
			rule := registeredCallRule(allowed, indirect[call], recursive, debt)
			if rule == "" {
				continue
			}
			add(caller, callee, constructor, rule, call.Pos())
		}
		return true
	})
}

func scanRegisteredReferences(pass *analysis.Pass, registry ConstructionRegistry, values registeredValues,
	getters map[ConstructionSymbol]registeredGetterFact, decl ast.Decl, caller ConstructionSymbol,
	called map[ast.Expr]bool, add func(ConstructionSymbol, ConstructionSymbol, ConstructionConstructor, string, token.Pos),
) {
	ast.Inspect(decl, func(node ast.Node) bool {
		expr, ok := node.(ast.Expr)
		if !ok {
			return true
		}
		if id, ok := expr.(*ast.Ident); ok && pass.TypesInfo.Defs[id] != nil {
			return false // Declarations introduce names; they do not reference constructors.
		}
		if called[expr] {
			_, selector := expr.(*ast.SelectorExpr)
			return !selector // An invoked closure still contains references in its body.
		}
		if values.safe[expr] && values.resolve(pass, expr, map[types.Object]bool{}) != (ConstructionSymbol{}) {
			return false
		}
		callee := values.resolve(pass, expr, map[types.Object]bool{})
		if getter, ok := getters[callee]; ok {
			add(caller, callee, getter.Constructor, "unresolved-service-getter-reference", expr.Pos())
			return false
		}
		for _, constructor := range registry.Constructors {
			if constructor.Symbol == callee && registeredProhibitedKind(constructor, registry.Types) {
				add(caller, callee, constructor, "unresolved-construction-reference", expr.Pos())
				return false
			}
		}
		return true
	})
}

func registeredProhibitedKind(constructor ConstructionConstructor, classified []ConstructionType) bool {
	return slices.ContainsFunc(classified, func(t ConstructionType) bool {
		return slices.Contains(constructor.Results, t.Symbol) && (t.Kind == ConstructionBehavior || t.Kind == ConstructionEffect)
	})
}

func registeredConstructionSymbol(obj types.Object) ConstructionSymbol {
	if obj == nil || obj.Pkg() == nil {
		return ConstructionSymbol{}
	}
	symbol := ConstructionSymbol{ImportPath: obj.Pkg().Path(), Name: obj.Name()}
	if fn, ok := obj.(*types.Func); ok {
		fn = fn.Origin()
		if recv := fn.Type().(*types.Signature).Recv(); recv != nil {
			typ := types.Unalias(recv.Type())
			if pointer, ok := typ.(*types.Pointer); ok {
				typ = types.Unalias(pointer.Elem())
			}
			if named, ok := typ.(*types.Named); ok {
				symbol.Receiver = named.Origin().Obj().Name()
			}
		}
	}
	return symbol
}

func markRegisteredCallee(marked map[ast.Expr]bool, expr ast.Expr) {
	marked[expr] = true
	switch expr := expr.(type) {
	case *ast.ParenExpr:
		markRegisteredCallee(marked, expr.X)
	case *ast.IndexExpr:
		markRegisteredCallee(marked, expr.X)
	case *ast.IndexListExpr:
		markRegisteredCallee(marked, expr.X)
	}
}

func registeredCallRule(allowed, indirect, recursive, debt bool) string {
	if allowed && !indirect && !recursive && !debt {
		return ""
	}
	if allowed && debt && !recursive && !indirect {
		return "unresolved-focused-provider-dispatch"
	}
	return "registered-construction"
}
