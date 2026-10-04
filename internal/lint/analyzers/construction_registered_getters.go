package analyzers

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"
)

// Export only the bounded getter observation, never dependency source bodies.
// Importers resolve the method object (including promotions) through go/types.
type registeredGetterFact struct {
	Constructor ConstructionConstructor
	Rule        string
}

func (*registeredGetterFact) AFact()           {}
func (f *registeredGetterFact) String() string { return "construction-getter=" + f.Rule }

func registeredConstructionGetters(pass *analysis.Pass, registry ConstructionRegistry, helpers *registeredGuardHelpers,
	stored map[ConstructionSymbol]map[*types.Var]string,
	typeset registeredBagTypes,
) map[ConstructionSymbol]registeredGetterFact {
	getters := map[ConstructionSymbol]registeredGetterFact{}
	for _, fact := range pass.AllObjectFacts() {
		if getter, ok := fact.Fact.(*registeredGetterFact); ok {
			getters[registeredConstructionSymbol(fact.Object)] = *getter
		}
	}
	for _, constructor := range registry.Constructors {
		if !registeredProhibitedKind(constructor, registry.Types) {
			continue
		}
		for _, fn := range helpers.functions {
			obj := pass.TypesInfo.Defs[fn.Name].(*types.Func)
			sig := obj.Type().(*types.Signature)
			if sig.Recv() == nil || sig.Params().Len() != 0 || !registeredStorageResult(sig.Recv().Type(), constructor) {
				continue
			}
			results := map[int]bool{}
			for i := 0; i < sig.Results().Len(); i++ {
				if typeset.collaborator(sig.Results().At(i).Type(), map[types.Type]bool{}) {
					results[i] = true
				}
			}
			if len(results) == 0 {
				continue
			}
			p := helpers.context(fn, map[types.Object]string{}, constructor, stored[constructor.Symbol])
			p.getterAssignedOrigins(fn.Body)
			rule := p.getterReturnRule(fn.Body, sig, results)
			if rule != "" {
				symbol := registeredConstructionSymbol(obj)
				if getters[symbol].Rule == "unresolved-service-getter-locator" {
					rule = "unresolved-service-getter-locator"
				}
				fact := registeredGetterFact{Constructor: constructor, Rule: rule}
				getters[symbol] = fact
				pass.ExportObjectFact(obj, &fact)
			}
		}
	}
	return getters
}

// Writes to named results and mutable aliases retain debt. Propagating only
// unresolved origins terminates even when assignments form cycles.
func (p registeredGuardOrigins) getterAssignedOrigins(body *ast.BlockStmt) {
	for changed := true; changed; {
		changed = false
		ast.Inspect(body, func(node ast.Node) bool {
			assignment, ok := node.(*ast.AssignStmt)
			if !ok {
				return true
			}
			for i, left := range assignment.Lhs {
				id, ok := left.(*ast.Ident)
				if !ok {
					continue
				}
				obj := p.pass.TypesInfo.ObjectOf(id)
				if !p.values.mutated[obj] || p.required[obj] != "" {
					continue
				}
				rule := ""
				if len(assignment.Lhs) == len(assignment.Rhs) {
					rule = p.origin(assignment.Rhs[i], false, map[types.Object]bool{})
				} else if len(assignment.Rhs) == 1 {
					rule = p.origin(assignment.Rhs[0], false, map[types.Object]bool{})
				}
				if rule != "" {
					p.required[obj] = "unresolved-required-dependency-guard"
					changed = true
				}
			}
			return true
		})
	}
}

func (p registeredGuardOrigins) getterReturnRule(body *ast.BlockStmt, sig *types.Signature, results map[int]bool) string {
	rule := ""
	ast.Inspect(body, func(node ast.Node) bool {
		if _, closure := node.(*ast.FuncLit); closure {
			return false
		}
		returned, ok := node.(*ast.ReturnStmt)
		if !ok {
			return true
		}
		for i := range results {
			origin := ""
			if len(returned.Results) == 0 {
				origin = p.objectOrigin(sig.Results().At(i), false, map[types.Object]bool{})
			} else if len(returned.Results) == sig.Results().Len() {
				origin = p.origin(returned.Results[i], false, map[types.Object]bool{})
			} else if len(returned.Results) == 1 {
				origin = p.origin(returned.Results[0], false, map[types.Object]bool{})
				if origin != "" {
					origin = "unresolved-required-dependency-guard"
				}
			}
			if origin == "unresolved-required-dependency-guard" {
				rule = "unresolved-service-getter-locator"
			} else if origin != "" && rule == "" {
				rule = "service-getter-locator"
			}
		}
		return true
	})
	return rule
}
