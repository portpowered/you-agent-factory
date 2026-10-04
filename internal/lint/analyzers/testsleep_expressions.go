package analyzers

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"time"

	"golang.org/x/tools/go/analysis"
)

func timingKind(pass *analysis.Pass, node ast.Node) string {
	switch node := node.(type) {
	case *ast.CallExpr:
		return timingCallKind(pass, node)
	case *ast.BinaryExpr:
		switch node.Op {
		case token.LSS, token.LEQ, token.GTR, token.GEQ:
			if (timingElapsed(pass, node.X) && literalTimingDuration(pass, node.Y)) ||
				(timingElapsed(pass, node.Y) && literalTimingDuration(pass, node.X)) {
				return "elapsed"
			}
		}
	}
	return ""
}

func timingCallKind(pass *analysis.Pass, call *ast.CallExpr) string {
	pkg, name := timingSymbol(pass, call.Fun)
	if pkg == "time" {
		switch name {
		case "Sleep":
			return "sleep"
		case "After", "NewTimer", "AfterFunc":
			if len(call.Args) > 0 && shortTimingDuration(pass, call.Args[0]) {
				return "deadline"
			}
		}
	}
	if pkg == "context" && len(call.Args) == 2 {
		if name == "WithTimeout" && shortTimingDuration(pass, call.Args[1]) {
			return "deadline"
		}
		if name == "WithDeadline" && shortTimingDeadline(pass, call.Args[1]) {
			return "deadline"
		}
	}
	return ""
}

// Uses resolves aliases and dot imports without mistaking a shadowing local
// object for a package function. Method objects also retain package identity.
func timingSymbol(pass *analysis.Pass, expr ast.Expr) (string, string) {
	var id *ast.Ident
	switch expr := expr.(type) {
	case *ast.Ident:
		id = expr
	case *ast.SelectorExpr:
		id = expr.Sel
	default:
		return "", ""
	}
	object := pass.TypesInfo.Uses[id]
	if object == nil || object.Pkg() == nil {
		return "", ""
	}
	return object.Pkg().Path(), object.Name()
}

func shortTimingDuration(pass *analysis.Pass, expr ast.Expr) bool {
	if !literalTimingDuration(pass, expr) {
		return false
	}
	value := pass.TypesInfo.Types[expr].Value
	if value == nil {
		return false
	}
	nanos, ok := constant.Int64Val(constant.ToInt(value))
	return ok && nanos <= int64(5*time.Second)
}

func shortTimingDeadline(pass *analysis.Pass, expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok || len(call.Args) != 1 {
		return false
	}
	pkg, name := timingSymbol(pass, call.Fun)
	return pkg == "time" && name == "Add" && shortTimingDuration(pass, call.Args[0])
}

func timingElapsed(pass *analysis.Pass, expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	pkg, name := timingSymbol(pass, call.Fun)
	return pkg == "time" && (name == "Since" || (name == "Sub" && len(call.Args) == 1))
}

// Named constants and variables remain nonliteral failure ceilings. Compiler
// constant folding supplies arithmetic precision only after this syntax guard.
func literalTimingDuration(pass *analysis.Pass, expr ast.Expr) bool {
	switch expr := expr.(type) {
	case *ast.ParenExpr:
		return literalTimingDuration(pass, expr.X)
	case *ast.BasicLit:
		return expr.Kind == token.INT || expr.Kind == token.FLOAT
	case *ast.SelectorExpr, *ast.Ident:
		pkg, name := timingSymbol(pass, expr)
		return pkg == "time" && setOf("Nanosecond", "Microsecond", "Millisecond", "Second", "Minute", "Hour")[name]
	case *ast.CallExpr:
		pkg, name := timingSymbol(pass, expr.Fun)
		_, conversion := pass.TypesInfo.Types[expr.Fun].Type.(*types.Named)
		return conversion && pkg == "time" && name == "Duration" && len(expr.Args) == 1 && literalTimingDuration(pass, expr.Args[0])
	case *ast.BinaryExpr:
		switch expr.Op {
		case token.MUL, token.ADD, token.SUB, token.QUO:
			return literalTimingDuration(pass, expr.X) && literalTimingDuration(pass, expr.Y)
		}
	}
	return false
}
