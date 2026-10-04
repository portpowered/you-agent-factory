package analyzers

import (
	"fmt"
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
	var found []violation
	support := under(unit, "internal/testutil") || under(unit, "tests/functional/internal/support")
	for _, file := range pass.Files {
		if ast.IsGenerated(file) || (!support && !strings.HasSuffix(pass.Fset.Position(file.Pos()).Filename, "_test.go")) {
			continue
		}
		found = append(found, inspectTestBoundary(pass, file, unit)...)
	}
	reportWithBaseline(pass, unit, setOf("test-cross-owner-policy"), countedTestPolicy(found), true, testPolicyBaseline(pass, unit))
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

func inspectTestBoundary(pass *analysis.Pass, file *ast.File, unit string) []violation {
	var found []violation
	if under(unit, "pkg/transports") {
		inspectTestWorkCallbacks(pass, file, unit)
	}
	if under(unit, "pkg/transports/http") {
		inspectTestEngineLiterals(pass, file, unit)
	}
	called := testBoundaryCalls(file)
	path := timingFile(unit, pass.Fset.Position(file.Pos()).Filename)
	ast.Inspect(file, func(node ast.Node) bool {
		id, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		obj := pass.TypesInfo.Uses[id]
		if obj == nil || obj.Pkg() == nil || obj.Parent() != obj.Pkg().Scope() {
			return true
		}
		if v := crossOwnerTestPolicyViolation(pass, id, obj, unit, called[id]); v != nil {
			found = append(found, *v)
		}
		reportTestComposition(pass, id, obj, unit, path, called[id])
		fn, isFunction := obj.(*types.Func)
		if isFunction && !under(strings.TrimSuffix(unit, "_test"), "pkg/services/work") &&
			fn.Pkg().Path() == modulePrefix+"pkg/services/work" && fn.Name() == "NormalizeWorkRequest" {
			pass.Reportf(id.Pos(), "test-work-normalization: %s -> pkg/services/work.NormalizeWorkRequest; assert the consumer owner's public contract, relocate normalization scenarios to pkg/services/work, or exercise root.BuildProcess", unit)
		}
		return true
	})
	return found
}

// Exact service operations remain owned by their service even in reusable
// test support. Wire is the canonical composition boundary.
var crossOwnerTestPolicy = map[string]map[string]bool{
	"pkg/services/factory_definitions": {
		"MapDir": true, "NamedFactoriesRoot": true, "ResolveCurrentDir": true, "WriteCurrentPointer": true,
	},
	"pkg/services/factory_definitions/internal/services/catalog/namedpaths": {
		"MapDir": true, "NamedFactoriesRoot": true, "ResolveCurrentDir": true, "WriteCurrentPointer": true,
	},
	"pkg/services/operator_settings": {
		"DefaultConfigPath": true, "ResolveFromHomeWithEnvironment": true,
	},
	"pkg/services/workers": {"LoadMockWorkersConfig": true},
}

func crossOwnerTestPolicyViolation(pass *analysis.Pass, id *ast.Ident, obj types.Object, unit string, called bool) *violation {
	path := strings.TrimPrefix(obj.Pkg().Path(), modulePrefix)
	if !crossOwnerTestPolicy[path][obj.Name()] || !testBoundaryCallable(obj, called) {
		return nil
	}
	owner := strings.Split(path, "/")[2]
	caller := strings.TrimSuffix(unit, "_test")
	if under(caller, "pkg/services/"+owner) || under(caller, "pkg/wire") {
		return nil
	}
	return &violation{rule: "test-cross-owner-policy", importer: unit, importee: timingFile(unit, pass.Fset.Position(id.Pos()).Filename) + "#" + path + "." + obj.Name(), pos: id.Pos(), hint: "move policy assertions to pkg/services/" + owner + " or exercise the customer process boundary"}
}

// Counts preserve exact source/symbol debt: another call changes the key.
func countedTestPolicy(found []violation) []violation {
	counts := map[string]int{}
	for _, v := range found {
		counts[v.key()]++
	}
	for i := range found {
		found[i].importee += fmt.Sprintf("::count=%d", counts[found[i].key()])
	}
	return found
}

func testPolicyBaseline(pass *analysis.Pass, unit string) map[string]struct{} {
	listed := baseline()
	selected, ignored := map[string]bool{}, map[string]bool{}
	hasTests := false
	for _, file := range pass.Files {
		name := serviceSource(pass, unit, file)
		selected[name] = true
		hasTests = hasTests || strings.HasSuffix(name, "_test.go")
	}
	for _, name := range pass.IgnoredFiles {
		ignored[sourceName(unit, name)] = true
	}
	for key := range listed {
		parts := strings.SplitN(key, "|", 3)
		if len(parts) != 3 || parts[0] != "test-cross-owner-policy" || parts[1] != unit {
			continue
		}
		name, _, _ := strings.Cut(parts[2], "#")
		if !selected[name] && (ignored[name] || (strings.HasSuffix(name, "_test.go") && !hasTests)) {
			delete(listed, key)
		}
	}
	return listed
}

// HTTP tests consume detached service results instead of recreating engine
// projections. Resolve aliases and dot imports through the compiler's types.
func inspectTestEngineLiterals(pass *analysis.Pass, file *ast.File, unit string) {
	ast.Inspect(file, func(node ast.Node) bool {
		literal, ok := node.(*ast.CompositeLit)
		if !ok {
			return true
		}
		typ := pass.TypesInfo.TypeOf(literal)
		if typ == nil {
			return true
		}
		named, ok := types.Unalias(typ).(*types.Named)
		if !ok || named.Obj().Pkg() == nil || named.Obj().Pkg().Path() != modulePrefix+"pkg/services/factory_runtime" {
			return true
		}
		switch named.Obj().Name() {
		case "EngineStateSnapshot", "Net", "PetriMarkingSnapshot", "RuntimeToken", "RuntimeTokenColor":
			pass.Reportf(literal.Pos(), "test-http-engine-literal: %s -> pkg/services/factory_runtime.%s; use detached service-root results instead of implementing engine policy in HTTP tests", unit, named.Obj().Name())
		}
		return true
	})
}

// A transport fake may forward the Work role to an injected callback, but
// must not implement parsing, normalization or response policy in that role.
func inspectTestWorkCallbacks(pass *analysis.Pass, file *ast.File, unit string) {
	for _, declaration := range file.Decls {
		fn, ok := declaration.(*ast.FuncDecl)
		if ok && fn.Name.Name == "PrepareInvocationInput" && !strictTestWorkCallback(fn) {
			pass.Reportf(fn.Name.Pos(), "test-work-invocation-policy: %s -> pkg/services/work.PrepareInvocationInput; forward to an injected callback instead of implementing Work policy in a transport fake", unit)
		}
	}
}

func strictTestWorkCallback(fn *ast.FuncDecl) bool {
	if fn.Recv == nil || fn.Body == nil || len(fn.Body.List) != 1 ||
		len(fn.Recv.List) != 1 || len(fn.Recv.List[0].Names) != 1 {
		return false
	}
	returned, ok := fn.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(returned.Results) != 1 {
		return false
	}
	call, ok := returned.Results[0].(*ast.CallExpr)
	if !ok {
		return false
	}
	callback, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	receiver, ok := callback.X.(*ast.Ident)
	if !ok || receiver.Name != fn.Recv.List[0].Names[0].Name {
		return false
	}
	for _, argument := range call.Args {
		if _, direct := argument.(*ast.Ident); !direct {
			return false
		}
	}
	return true
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

// Reviewed command inventory/parity proofs retain their exact source allowance.
// A neighboring file or descendant package is not the reviewed proof.
var reviewedTransportRootProcessTests = map[string]bool{
	"pkg/transports/cli/baseline/goal_failure_process_test.go":  true,
	"pkg/transports/cli/baseline/root_process_external_test.go": true,
	"pkg/transports/cli/baseline/root_process_test.go":          true,
	"pkg/transports/cli/clicontract/root_process_test.go":       true,
	"pkg/transports/cli/cliinputs/root_process_test.go":         true,
	"pkg/transports/cli/commandidentity/root_process_test.go":   true,
}

func testProcessComposition(unit, path, importPath, symbol string) string {
	unit = strings.TrimSuffix(unit, "_test")
	if importPath == modulePrefix+"pkg/root" && symbol == "BuildProcess" {
		if under(unit, "pkg/services") {
			return "test-alternate-customer-composition"
		}
		if under(unit, "pkg/transports") && !reviewedTransportRootProcessTests[path] {
			return "test-customer-process-under-transport"
		}
	}
	if importPath == modulePrefix+"internal/builtcliacceptance" && symbol == "NewHarness" && under(unit, "pkg") {
		return "test-alternate-customer-composition"
	}
	return ""
}

func reportTestComposition(pass *analysis.Pass, id *ast.Ident, obj types.Object, unit, path string, called bool) {
	if !testBoundaryCallable(obj, called) {
		return
	}
	if rule := testProcessComposition(unit, path, obj.Pkg().Path(), obj.Name()); rule != "" {
		pass.Reportf(id.Pos(), "%s: %s -> %s.%s; move customer process scenarios to tests/functional and keep owner tests component-isolated", rule, unit, strings.TrimPrefix(obj.Pkg().Path(), modulePrefix), obj.Name())
	}
	if under(unit, "tests/functional") && functionalTransportComposition(obj.Pkg().Path(), obj.Name()) {
		pass.Reportf(id.Pos(), "test-functional-transport-composition: %s -> %s.%s; exercise customer behavior through root.BuildProcess instead of constructing a handwritten transport", unit, strings.TrimPrefix(obj.Pkg().Path(), modulePrefix), obj.Name())
	}
}
