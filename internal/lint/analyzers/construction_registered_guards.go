package analyzers

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"
)

// Guard provenance follows declared required parameters, classified receivers
// and constructor result fields through same-package helper calls.
func scanRegisteredConstructionGuards(pass *analysis.Pass, registry ConstructionRegistry, helpers *registeredGuardHelpers,
	stored map[ConstructionSymbol]map[*types.Var]string,
	add func(ConstructionSymbol, ConstructionSymbol, ConstructionConstructor, string, token.Pos),
) {
	for _, constructor := range registry.Constructors {
		origins := helpers.requiredOrigins(constructor, registry.Types, stored[constructor.Symbol])
		for _, fn := range helpers.functions {
			caller := registeredConstructionSymbol(pass.TypesInfo.Defs[fn.Name])
			p := helpers.context(fn, origins[caller], constructor, stored[constructor.Symbol])
			p.scan(fn.Body, func(rule string, pos token.Pos) { add(caller, constructor.Symbol, constructor, rule, pos) })
		}
	}
}

type registeredGuardOrigins struct {
	pass           *analysis.Pass
	values         registeredValues
	required       map[types.Object]string
	assertions     map[types.Object]registeredGuardAssertion
	receiver       *types.Var
	fields         map[*types.Var]string
	fieldMutations map[*types.Var]bool
	helpers        *registeredGuardHelpers
	activeHelpers  map[ConstructionSymbol]bool
}

type registeredGuardAssertion struct {
	expr   *ast.TypeAssertExpr
	status bool
}

func registeredGuardAssertions(pass *analysis.Pass, body ast.Node) map[types.Object]registeredGuardAssertion {
	assertions := map[types.Object]registeredGuardAssertion{}
	ast.Inspect(body, func(node ast.Node) bool {
		assignment, ok := node.(*ast.AssignStmt)
		if !ok || assignment.Tok != token.DEFINE || len(assignment.Lhs) != 2 || len(assignment.Rhs) != 1 {
			return true
		}
		assertion, ok := assignment.Rhs[0].(*ast.TypeAssertExpr)
		if ok {
			for i, left := range assignment.Lhs {
				if id, ok := left.(*ast.Ident); ok && pass.TypesInfo.Defs[id] != nil {
					assertions[pass.TypesInfo.Defs[id]] = registeredGuardAssertion{expr: assertion, status: i == 1}
				}
			}
		}
		return true
	})
	return assertions
}

func (p registeredGuardOrigins) origin(expr ast.Expr, status bool, visited map[types.Object]bool) string {
	switch expr := expr.(type) {
	case *ast.ParenExpr:
		return p.origin(expr.X, status, visited)
	case *ast.TypeAssertExpr:
		if !status {
			return p.origin(expr.X, false, visited)
		}
	case *ast.UnaryExpr:
		if status && expr.Op == token.NOT {
			return p.origin(expr.X, true, visited)
		}
	case *ast.BinaryExpr:
		if status && (expr.Op == token.EQL || expr.Op == token.NEQ) {
			for _, pair := range [][2]ast.Expr{{expr.X, expr.Y}, {expr.Y, expr.X}} {
				if id, ok := pair[0].(*ast.Ident); ok && (p.pass.TypesInfo.ObjectOf(id) == types.Universe.Lookup("true") || p.pass.TypesInfo.ObjectOf(id) == types.Universe.Lookup("false")) {
					return p.origin(pair[1], true, visited)
				}
			}
		}
	case *ast.CallExpr:
		if !status && p.helpers != nil {
			return p.helperOrigin(expr)
		}
		if status {
			for _, arg := range expr.Args {
				if p.origin(arg, true, map[types.Object]bool{}) != "" {
					return "unresolved-required-dependency-guard"
				}
			}
		}
	case *ast.Ident:
		return p.objectOrigin(p.pass.TypesInfo.ObjectOf(expr), status, visited)
	case *ast.SelectorExpr:
		if !status {
			selection := p.pass.TypesInfo.Selections[expr]
			if selection != nil && selection.Kind() == types.FieldVal && p.receiverAlias(expr.X, map[types.Object]bool{}) {
				field := selection.Obj().(*types.Var)
				rule := p.fields[field]
				if rule != "" && (p.fieldMutations[field] || p.storageMutated(expr.X, map[types.Object]bool{})) {
					return "unresolved-required-dependency-guard"
				}
				return rule
			}
		}
	}
	return ""
}

func (p registeredGuardOrigins) objectOrigin(obj types.Object, status bool, visited map[types.Object]bool) string {
	if obj == nil || visited[obj] {
		return ""
	}
	visited[obj] = true
	defer delete(visited, obj)
	rule := ""
	if !status {
		rule = p.required[obj]
	}
	if assertion, ok := p.assertions[obj]; ok {
		if status == assertion.status {
			rule = p.origin(assertion.expr.X, false, visited)
			if status && rule != "" && rule != "unresolved-required-dependency-guard" {
				rule = "required-dependency-assertion-guard"
			}
		}
	} else if rule == "" {
		if call := p.values.tuples[obj]; call != nil && !status && p.helpers != nil {
			rule = p.helperOrigin(call)
		} else {
			rule = p.origin(p.values.initial[obj], status, visited)
		}
	}
	if rule != "" && p.values.mutated[obj] {
		return "unresolved-required-dependency-guard"
	}
	return rule
}

func (p registeredGuardOrigins) scan(body ast.Node, add func(string, token.Pos)) {
	ast.Inspect(body, func(node ast.Node) bool {
		var condition ast.Expr
		switch stmt := node.(type) {
		case *ast.IfStmt:
			condition = stmt.Cond
		case *ast.ForStmt:
			condition = stmt.Cond
		}
		if condition != nil {
			ast.Inspect(condition, func(node ast.Node) bool {
				expr, ok := node.(ast.Expr)
				if !ok {
					return true
				}
				if rule := p.origin(expr, true, map[types.Object]bool{}); rule != "" {
					add(rule, expr.Pos())
					return false
				}
				_, call := expr.(*ast.CallExpr)
				return !call // Computing status in a called closure is not a status guard.
			})
		}
		if cmp, ok := node.(*ast.BinaryExpr); ok && (cmp.Op == token.EQL || cmp.Op == token.NEQ) {
			for _, pair := range [][2]ast.Expr{{cmp.X, cmp.Y}, {cmp.Y, cmp.X}} {
				if id, ok := pair[0].(*ast.Ident); ok && p.pass.TypesInfo.ObjectOf(id) == types.Universe.Lookup("nil") {
					if rule := p.origin(pair[1], false, map[types.Object]bool{}); rule != "" {
						add(rule, cmp.Pos())
					}
				}
			}
		}
		return true
	})
}
