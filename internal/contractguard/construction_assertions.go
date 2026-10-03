package contractguard

import (
	"go/ast"
	"go/token"
	"slices"
)

func constructionHasResult(constructor ConstructionConstructor, symbol ConstructionSymbol) bool {
	return slices.Contains(constructor.Results, symbol)
}

// Assertion status is separate from the asserted collaborator value. Merely
// computing, recording or returning status does not constitute a guard.
func constructionAssertionGuards(condition ast.Expr, p constructionGuardProvenance, caller ConstructionSymbol, constructor ConstructionConstructor, registry ConstructionRegistry) []ConstructionFinding {
	var findings []ConstructionFinding
	if condition == nil {
		return findings
	}
	ast.Inspect(condition, func(node ast.Node) bool {
		expression, ok := node.(ast.Expr)
		if !ok {
			return true
		}
		rule := p.assertionStatusOrigin(expression, map[*ast.Object]bool{})
		if rule == "" {
			if _, call := expression.(*ast.CallExpr); call {
				return false
			}
			return true
		}
		mode := ConstructionReport
		for _, set := range registry.CapabilitySets {
			if set.Name == constructor.CapabilitySet {
				mode = set.Mode
			}
		}
		findings = append(findings, ConstructionFinding{FilePath: p.source.path,
			Line: p.source.set.Position(expression.Pos()).Line, Caller: caller,
			Callee: constructor.Symbol, CapabilitySet: constructor.CapabilitySet, Mode: mode, Rule: rule})
		return false
	})
	return findings
}

func (p constructionGuardProvenance) assertionStatusOrigin(expression ast.Expr, visited map[*ast.Object]bool) string {
	switch value := expression.(type) {
	case *ast.ParenExpr:
		return p.assertionStatusOrigin(value.X, visited)
	case *ast.UnaryExpr:
		if value.Op == token.NOT {
			return p.assertionStatusOrigin(value.X, visited)
		}
	case *ast.CallExpr:
		for _, argument := range value.Args {
			if p.assertionStatusOrigin(argument, map[*ast.Object]bool{}) != "" {
				return "unresolved-required-dependency-guard"
			}
		}
	case *ast.BinaryExpr:
		if value.Op == token.EQL || value.Op == token.NEQ {
			if constructionBooleanLiteral(value.X) {
				return p.assertionStatusOrigin(value.Y, visited)
			}
			if constructionBooleanLiteral(value.Y) {
				return p.assertionStatusOrigin(value.X, visited)
			}
		}
	case *ast.Ident:
		if value.Obj == nil || visited[value.Obj] {
			return ""
		}
		visited[value.Obj] = true
		rule := ""
		switch declaration := value.Obj.Decl.(type) {
		case *ast.AssignStmt:
			if len(declaration.Lhs) == 2 && len(declaration.Rhs) == 1 {
				status, ok := declaration.Lhs[1].(*ast.Ident)
				assertion, asserted := declaration.Rhs[0].(*ast.TypeAssertExpr)
				if ok && asserted && status.Obj == value.Obj {
					rule = p.origin(assertion.X, map[*ast.Object]bool{})
					if rule != "" && rule != "unresolved-required-dependency-guard" {
						rule = "required-dependency-assertion-guard"
					}
				}
			} else {
				rule = p.assertionStatusOrigin(constructionAssignedValue(value.Obj, declaration.Lhs, declaration.Rhs), visited)
			}
		case *ast.ValueSpec:
			if len(declaration.Names) == 1 && len(declaration.Values) == 1 {
				rule = p.assertionStatusOrigin(declaration.Values[0], visited)
			}
		}
		if rule != "" && p.mutations[value.Obj] {
			return "unresolved-required-dependency-guard"
		}
		return rule
	}
	return ""
}

func constructionBooleanLiteral(expression ast.Expr) bool {
	name, ok := expression.(*ast.Ident)
	return ok && name.Obj == nil && (name.Name == "true" || name.Name == "false")
}
