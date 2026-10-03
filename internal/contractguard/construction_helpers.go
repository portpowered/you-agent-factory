package contractguard

import "go/ast"

// Only the asserted value inherits collaborator provenance. The comma-ok
// boolean is domain information, even when both names share one declaration.
func constructionAssignedValue(object *ast.Object, left, right []ast.Expr) ast.Expr {
	for position, expression := range left {
		name, ok := expression.(*ast.Ident)
		if !ok || name.Obj != object {
			continue
		}
		if len(left) == len(right) {
			return right[position]
		}
		if position == 0 && len(right) == 1 {
			if assertion, ok := right[0].(*ast.TypeAssertExpr); ok {
				return assertion
			}
		}
	}
	return nil
}

// Summaries follow same-package authored function returns, never execute them,
// and retain debt at recursion or an unavailable/ambiguous declaration. A
// helper that always returns unrelated domain data does not become required.
func (p constructionGuardProvenance) helperOrigin(call *ast.CallExpr) string {
	if p.source == nil {
		return ""
	}
	required := make(map[*ast.Object]string)
	mutations := make(map[*ast.Object]bool)
	symbol, resolved := resolveConstructionCall(call.Fun, p.source)
	decl := p.source.declarations[symbol]
	parameters := constructionFunctionParameters(decl.function)
	dependent := false
	for position, argument := range call.Args {
		rule := p.origin(argument, map[*ast.Object]bool{})
		if rule == "" {
			continue
		}
		dependent = true
		if position < len(parameters) && parameters[position] != nil {
			required[parameters[position]] = rule
		}
	}
	if !dependent {
		return ""
	}
	if !resolved || symbol.ImportPath != p.source.importPath || decl.function == nil ||
		decl.function.Body == nil || p.helpers[decl.function] || len(parameters) != len(call.Args) ||
		constructionResultCount(decl.function) != 1 {
		return "unresolved-required-dependency-guard"
	}
	helpers := make(map[*ast.FuncDecl]bool, len(p.helpers)+1)
	for function := range p.helpers {
		helpers[function] = true
	}
	helpers[decl.function] = true
	for object, mutated := range constructionGuardMutations(decl.function.Body) {
		mutations[object] = mutations[object] || mutated
	}
	summary := constructionGuardProvenance{source: decl.source, required: required, mutations: mutations, helpers: helpers}
	return summary.returnOrigin(decl.function)
}

func constructionFunctionParameters(function *ast.FuncDecl) []*ast.Object {
	var parameters []*ast.Object
	if function == nil || function.Type.Params == nil {
		return parameters
	}
	for _, field := range function.Type.Params.List {
		if len(field.Names) == 0 {
			parameters = append(parameters, nil)
		}
		for _, name := range field.Names {
			parameters = append(parameters, name.Obj)
		}
	}
	return parameters
}

func constructionResultCount(function *ast.FuncDecl) int {
	count := 0
	if function.Type.Results != nil {
		for _, result := range function.Type.Results.List {
			count += max(1, len(result.Names))
		}
	}
	return count
}

func (p constructionGuardProvenance) returnOrigin(function *ast.FuncDecl) string {
	var rules []string
	ast.Inspect(function.Body, func(node ast.Node) bool {
		if _, closure := node.(*ast.FuncLit); closure {
			return false
		}
		if statement, ok := node.(*ast.ReturnStmt); ok {
			rule := "unresolved-required-dependency-guard"
			if len(statement.Results) == 1 {
				rule = p.origin(statement.Results[0], map[*ast.Object]bool{})
			}
			rules = append(rules, rule)
		}
		return true
	})
	if len(rules) == 0 {
		return "unresolved-required-dependency-guard"
	}
	for _, rule := range rules {
		if rule != rules[0] {
			return "unresolved-required-dependency-guard"
		}
	}
	return rules[0]
}

// Requiredness flows only from registered parameters or their stored fields,
// not from a helper's names or parameter types. The finite object graph reaches
// a fixed point; recursion cannot create new origins or unbounded traversal.
func (index constructionIndex) constructionHelperRequired(decl constructionDeclaration, required map[*ast.Object]string, fields map[ConstructionSymbol]map[string]string) map[*ast.FuncDecl]map[*ast.Object]string {
	origins := map[*ast.FuncDecl]map[*ast.Object]string{decl.function: required}
	for changed := true; changed; {
		changed = false
		for _, candidate := range index.declarations {
			function := candidate.function
			if function == nil || function.Body == nil || candidate.source.importPath != decl.source.importPath {
				continue
			}
			provenance := constructionGuardProvenance{source: candidate.source, required: origins[function],
				mutations: constructionGuardMutations(function.Body)}
			if function.Recv != nil {
				field := function.Recv.List[0]
				if len(field.Names) == 1 {
					provenance.receiver = field.Names[0].Obj
				}
				provenance.fields = fields[ConstructionSymbol{ImportPath: candidate.source.importPath, Name: constructionReceiver(field.Type)}]
				provenance.fieldMutations = constructionGuardFieldMutations(function.Body, provenance.receiver)
			}
			ast.Inspect(function.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if ok && index.propagateConstructionRequired(call, provenance, origins) {
					changed = true
				}
				return true
			})
		}
	}
	return origins
}

func (index constructionIndex) propagateConstructionRequired(call *ast.CallExpr, p constructionGuardProvenance, origins map[*ast.FuncDecl]map[*ast.Object]string) bool {
	symbol, resolved := resolveConstructionCall(call.Fun, p.source)
	decl := index.declarations[symbol]
	if !resolved || symbol.ImportPath != p.source.importPath || decl.function == nil || decl.function.Body == nil {
		return false
	}
	parameters := constructionFunctionParameters(decl.function)
	if len(parameters) != len(call.Args) {
		return false
	}
	changed := false
	for position, argument := range call.Args {
		rule := p.origin(argument, map[*ast.Object]bool{})
		object := parameters[position]
		if rule == "" || object == nil {
			continue
		}
		if origins[decl.function] == nil {
			origins[decl.function] = make(map[*ast.Object]string)
		}
		prior := origins[decl.function][object]
		if prior == "" || (prior != rule && rule == "unresolved-required-dependency-guard") {
			origins[decl.function][object] = rule
			changed = true
		}
	}
	return changed
}
