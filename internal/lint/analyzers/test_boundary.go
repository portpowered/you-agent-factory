package analyzers

import (
	"go/ast"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// TestBoundary keeps test assertions and support from executing another
// service's policy. Work normalization is a zero-debt ownership rule.
var TestBoundary = &analysis.Analyzer{
	Name: "testboundary",
	Doc:  "enforce service policy ownership in tests and reusable test support",
	Run:  runTestBoundary,
}

func runTestBoundary(pass *analysis.Pass) (any, error) {
	unit, ok := unitKey(pass)
	if !ok || under(strings.TrimSuffix(unit, "_test"), "pkg/services/work") {
		return nil, nil
	}
	support := under(unit, "internal/testutil") || under(unit, "tests/functional/internal/support")
	for _, file := range pass.Files {
		if ast.IsGenerated(file) || (!support && !strings.HasSuffix(pass.Fset.Position(file.Pos()).Filename, "_test.go")) {
			continue
		}
		ast.Inspect(file, func(node ast.Node) bool {
			id, ok := node.(*ast.Ident)
			if !ok {
				return true
			}
			fn, ok := pass.TypesInfo.Uses[id].(*types.Func)
			if ok && fn.Pkg() != nil && fn.Parent() == fn.Pkg().Scope() &&
				fn.Pkg().Path() == modulePrefix+"pkg/services/work" && fn.Name() == "NormalizeWorkRequest" {
				pass.Reportf(id.Pos(), "test-work-normalization: %s -> pkg/services/work.NormalizeWorkRequest; assert the consumer owner's public contract, relocate normalization scenarios to pkg/services/work, or exercise root.BuildProcess", unit)
			}
			return true
		})
	}
	return nil, nil
}
