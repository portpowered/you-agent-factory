package analyzers

import (
	"go/ast"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// Only compiler-resolved, authored declarations in this compilation unit have
// bodies. Imported/opaque returns retain debt when an argument is required.
type registeredGuardHelpers struct {
	pass      *analysis.Pass
	values    registeredValues
	functions []*ast.FuncDecl
	bySymbol  map[ConstructionSymbol]*ast.FuncDecl
}

func registeredConstructionHelpers(pass *analysis.Pass, values registeredValues) *registeredGuardHelpers {
	h := &registeredGuardHelpers{pass: pass, values: values, bySymbol: map[ConstructionSymbol]*ast.FuncDecl{}}
	for _, file := range pass.Files {
		if ast.IsGenerated(file) || strings.HasSuffix(pass.Fset.Position(file.Pos()).Filename, "_test.go") {
			continue
		}
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Body != nil {
				h.functions = append(h.functions, fn)
				h.bySymbol[registeredConstructionSymbol(pass.TypesInfo.Defs[fn.Name])] = fn
			}
		}
	}
	return h
}

func (h *registeredGuardHelpers) context(fn *ast.FuncDecl, required map[types.Object]string, constructor ConstructionConstructor, fields map[*types.Var]string) registeredGuardOrigins {
	sig := h.pass.TypesInfo.Defs[fn.Name].Type().(*types.Signature)
	p := registeredGuardOrigins{pass: h.pass, values: h.values, required: required,
		assertions: registeredGuardAssertions(h.pass, fn.Body), receiver: sig.Recv(), helpers: h}
	if sig.Recv() != nil && registeredStorageResult(sig.Recv().Type(), constructor) {
		p.fields = fields
	}
	p.fieldMutations = p.mutatedFields(fn.Body)
	return p
}

// Required parameter identities reach a finite fixed point. Recursion cannot
// invent a new origin; unresolved origins monotonically replace proved ones.
func (h *registeredGuardHelpers) requiredOrigins(constructor ConstructionConstructor, classified []ConstructionType, fields map[*types.Var]string) map[ConstructionSymbol]map[types.Object]string {
	origins := map[ConstructionSymbol]map[types.Object]string{}
	for _, fn := range h.functions {
		obj := h.pass.TypesInfo.Defs[fn.Name]
		symbol := registeredConstructionSymbol(obj)
		sig := obj.Type().(*types.Signature)
		origins[symbol] = map[types.Object]string{}
		if constructor.Symbol == symbol {
			for _, param := range constructor.RequiredParameters {
				origins[symbol][sig.Params().At(param.Index)] = "required-dependency-guard"
			}
		}
		if sig.Recv() != nil && registeredStorageResult(sig.Recv().Type(), constructor) && registeredProhibitedKind(constructor, classified) {
			origins[symbol][sig.Recv()] = "required-receiver-guard"
		}
	}
	for changed := true; changed; {
		changed = false
		for _, fn := range h.functions {
			p := h.context(fn, origins[registeredConstructionSymbol(h.pass.TypesInfo.Defs[fn.Name])], constructor, fields)
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				if call, ok := node.(*ast.CallExpr); ok && h.propagate(call, p, origins) {
					changed = true
				}
				return true
			})
		}
	}
	return origins
}

func (h *registeredGuardHelpers) propagate(call *ast.CallExpr, p registeredGuardOrigins, origins map[ConstructionSymbol]map[types.Object]string) bool {
	symbol := h.values.resolve(h.pass, call.Fun, map[types.Object]bool{})
	fn := h.bySymbol[symbol]
	if fn == nil {
		return false
	}
	params := h.pass.TypesInfo.Defs[fn.Name].Type().(*types.Signature).Params()
	if params.Len() != len(call.Args) {
		return false
	}
	changed := false
	for i, arg := range call.Args {
		rule := p.origin(arg, false, map[types.Object]bool{})
		obj := params.At(i)
		prior := origins[symbol][obj]
		if rule != "" && (prior == "" || (prior != rule && rule == "unresolved-required-dependency-guard")) {
			origins[symbol][obj] = rule
			changed = true
		}
	}
	return changed
}

func (p registeredGuardOrigins) helperOrigin(call *ast.CallExpr) string {
	h := p.helpers
	symbol := h.values.resolve(h.pass, call.Fun, map[types.Object]bool{})
	fn := h.bySymbol[symbol]
	required := map[types.Object]string{}
	dependent := false
	for i, arg := range call.Args {
		rule := p.origin(arg, false, map[types.Object]bool{})
		if rule == "" {
			continue
		}
		dependent = true
		if fn != nil {
			params := h.pass.TypesInfo.Defs[fn.Name].Type().(*types.Signature).Params()
			if i < params.Len() {
				required[params.At(i)] = rule
			}
		}
	}
	if !dependent {
		return ""
	}
	if fn == nil || p.activeHelpers[symbol] {
		return "unresolved-required-dependency-guard"
	}
	sig := h.pass.TypesInfo.Defs[fn.Name].Type().(*types.Signature)
	if sig.Params().Len() != len(call.Args) || sig.Results().Len() != 1 {
		return "unresolved-required-dependency-guard"
	}
	active := map[ConstructionSymbol]bool{symbol: true}
	for key := range p.activeHelpers {
		active[key] = true
	}
	summary := registeredGuardOrigins{pass: h.pass, values: h.values, required: required,
		assertions: registeredGuardAssertions(h.pass, fn.Body), helpers: h, activeHelpers: active}
	return summary.returnOrigin(fn.Body)
}

func (p registeredGuardOrigins) returnOrigin(body *ast.BlockStmt) string {
	var rules []string
	ast.Inspect(body, func(node ast.Node) bool {
		if _, closure := node.(*ast.FuncLit); closure {
			return false
		}
		if stmt, ok := node.(*ast.ReturnStmt); ok {
			rule := "unresolved-required-dependency-guard"
			if len(stmt.Results) == 1 {
				rule = p.origin(stmt.Results[0], false, map[types.Object]bool{})
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
