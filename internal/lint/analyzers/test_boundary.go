package analyzers

import (
	"go/ast"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// TestBoundary keeps test assertions and support from executing another
// service's policy. Work normalization and functional transport composition
// are zero-debt ownership rules.
var TestBoundary = &analysis.Analyzer{
	Name: "testboundary",
	Doc:  "enforce service policy ownership in tests and reusable test support",
	Run:  runTestBoundary,
}

func runTestBoundary(pass *analysis.Pass) (any, error) {
	unit, ok := unitKey(pass)
	if !ok {
		return nil, nil
	}
	support := under(unit, "internal/testutil") || under(unit, "tests/functional/internal/support")
	for _, file := range pass.Files {
		if ast.IsGenerated(file) || (!support && !strings.HasSuffix(pass.Fset.Position(file.Pos()).Filename, "_test.go")) {
			continue
		}
		inspectTestBoundary(pass, file, unit)
	}
	return nil, nil
}

func functionalTransportComposition(importPath, symbol string) bool {
	if importPath == modulePrefix+"pkg/services/work/transports/cli/submit" && symbol == "NewSubmit" {
		return true
	}
	if !strings.HasPrefix(importPath, modulePrefix+"pkg/transports/") ||
		importPath == modulePrefix+"pkg/transports/http/generated" ||
		importPath == modulePrefix+"pkg/transports/http/client" {
		return false
	}
	return strings.HasPrefix(symbol, "New") || strings.HasPrefix(symbol, "Build") || strings.HasPrefix(symbol, "Create")
}

func inspectTestBoundary(pass *analysis.Pass, file *ast.File, unit string) {
	called := testBoundaryCalls(file)
	ast.Inspect(file, func(node ast.Node) bool {
		id, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		obj := pass.TypesInfo.Uses[id]
		if obj == nil || obj.Pkg() == nil || obj.Parent() != obj.Pkg().Scope() {
			return true
		}
		fn, isFunction := obj.(*types.Func)
		if isFunction && !under(strings.TrimSuffix(unit, "_test"), "pkg/services/work") &&
			fn.Pkg().Path() == modulePrefix+"pkg/services/work" && fn.Name() == "NormalizeWorkRequest" {
			pass.Reportf(id.Pos(), "test-work-normalization: %s -> pkg/services/work.NormalizeWorkRequest; assert the consumer owner's public contract, relocate normalization scenarios to pkg/services/work, or exercise root.BuildProcess", unit)
		}
		if under(unit, "tests/functional") && testBoundaryCallable(obj, called[id]) && functionalTransportComposition(obj.Pkg().Path(), obj.Name()) {
			pass.Reportf(id.Pos(), "test-functional-transport-composition: %s -> %s.%s; exercise customer behavior through root.BuildProcess instead of constructing a handwritten transport", unit, strings.TrimPrefix(obj.Pkg().Path(), modulePrefix), obj.Name())
		}
		return true
	})
}

func testBoundaryCallable(obj types.Object, called bool) bool {
	if called {
		return true
	}
	switch obj.(type) {
	case *types.Func:
		return true
	case *types.Var:
		_, signature := obj.Type().Underlying().(*types.Signature)
		return signature
	}
	return false
}

func testBoundaryCalls(file *ast.File) map[*ast.Ident]bool {
	called := make(map[*ast.Ident]bool)
	ast.Inspect(file, func(node ast.Node) bool {
		if call, ok := node.(*ast.CallExpr); ok {
			switch fun := ast.Unparen(call.Fun).(type) {
			case *ast.Ident:
				called[fun] = true
			case *ast.SelectorExpr:
				called[fun.Sel] = true
			}
		}
		return true
	})
	return called
}
