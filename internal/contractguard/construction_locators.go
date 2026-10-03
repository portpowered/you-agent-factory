package contractguard

import "go/ast"

type constructionServiceGetter struct {
	set  string
	rule string
}

// Identify getters by required storage and classified return contracts, never
// by their names. Parameterized views and domain/resource results are separate
// operations; this bounded rule only covers zero-argument peer lookup.
func (index constructionIndex) constructionServiceGetters(registry ConstructionRegistry) map[ConstructionSymbol]constructionServiceGetter {
	getters := make(map[ConstructionSymbol]constructionServiceGetter)
	for _, constructor := range registry.Constructors {
		if !constructionProhibitedKind(constructor, registry.Types) {
			continue
		}
		decl := index.declarations[constructor.Symbol]
		required := constructionRequiredObjects(decl.function, constructor.RequiredParameters)
		fields := index.constructionRequiredFields(decl, constructor, required)
		for symbol, method := range index.declarations {
			owner := ConstructionSymbol{ImportPath: symbol.ImportPath, Name: symbol.Receiver}
			function := method.function
			if symbol.Receiver == "" || fields[owner] == nil || function == nil || function.Body == nil ||
				len(constructionParameterFields(function.Type.Params)) != 0 {
				continue
			}
			results := index.constructionCollaboratorResults(function, method.source, registry)
			if len(results) == 0 {
				continue
			}
			var receiver *ast.Object
			if names := function.Recv.List[0].Names; len(names) == 1 {
				receiver = names[0].Obj
			}
			provenance := constructionGuardProvenance{source: method.source, receiver: receiver, fields: fields[owner],
				mutations: constructionGuardMutations(function.Body), fieldMutations: constructionGuardFieldMutations(function.Body, receiver)}
			provenance.required = constructionGetterAssignedOrigins(function, results, provenance)
			ast.Inspect(function.Body, func(node ast.Node) bool {
				if _, nested := node.(*ast.FuncLit); nested {
					return false
				}
				if returned, ok := node.(*ast.ReturnStmt); ok {
					origin := constructionGetterReturnOrigin(returned, results, provenance, function)
					if origin != "" {
						rule := "service-getter-locator"
						if origin == "unresolved-required-dependency-guard" || getters[symbol].rule == "unresolved-service-getter-locator" {
							rule = "unresolved-service-getter-locator"
						}
						getters[symbol] = constructionServiceGetter{set: constructor.CapabilitySet, rule: rule}
					}
				}
				return true
			})
		}
	}
	return getters
}

func (index constructionIndex) constructionCollaboratorResults(function *ast.FuncDecl, source *constructionSource, registry ConstructionRegistry) map[int]*ast.Ident {
	results := make(map[int]*ast.Ident)
	position := 0
	if function.Type.Results == nil {
		return results
	}
	for _, field := range function.Type.Results.List {
		collaborator := index.constructionBagCollaborator(field.Type, source, registry, map[ConstructionSymbol]bool{})
		for offset := range max(1, len(field.Names)) {
			if collaborator {
				var name *ast.Ident
				if len(field.Names) != 0 {
					name = field.Names[offset]
				}
				results[position] = name
			}
			position++
		}
	}
	return results
}

func scanConstructionServiceGetters(source *constructionSource, caller ConstructionSymbol, body ast.Node, getters map[ConstructionSymbol]constructionServiceGetter, registry ConstructionRegistry) []ConstructionFinding {
	var findings []ConstructionFinding
	if len(getters) == 0 {
		return findings
	}
	callees := make(map[ast.Expr]bool)
	ast.Inspect(body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		markConstructionCallee(callees, call.Fun)
		symbol, resolved := resolveConstructionCall(call.Fun, source)
		if getter := getters[symbol]; resolved && getter.set != "" {
			findings = append(findings, ConstructionFinding{FilePath: source.path, Line: source.set.Position(call.Pos()).Line,
				Caller: caller, Callee: symbol, CapabilitySet: getter.set, Mode: constructionSetMode(getter.set, registry), Rule: getter.rule})
		}
		return true
	})
	for expression := range constructionSafeValueReferences(body, source, callees) {
		callees[expression] = true
	}
	ast.Inspect(body, func(node ast.Node) bool {
		expression, ok := node.(ast.Expr)
		if !ok {
			return true
		}
		_, selector := expression.(*ast.SelectorExpr)
		if !callees[expression] {
			symbol, resolved := resolveConstructionCall(expression, source)
			if getter := getters[symbol]; resolved && getter.set != "" {
				findings = append(findings, ConstructionFinding{FilePath: source.path, Line: source.set.Position(expression.Pos()).Line,
					Caller: caller, Callee: symbol, CapabilitySet: getter.set, Mode: constructionSetMode(getter.set, registry), Rule: "unresolved-service-getter-reference"})
			}
		}
		return !selector
	})
	return findings
}
