package analyzers

import (
	"go/ast"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// FunctionalShape preserves functional source ownership using compiler inputs.
var FunctionalShape = &analysis.Analyzer{
	Name: "functionalshape",
	Doc:  "enforce domain and subsection ownership of compiled functional sources",
	Run:  runFunctionalShape,
}

var functionalShapeRules = setOf("functional-test-missing-subsection", "functional-test-unclassified-domain", "deprecated-runtime-api-file", "deprecated-runtime-api-test")

var functionalDomains = setOf("transport", "workers", "orchestration", "workstations", "work", "sessions", "factory", "factory_definitions", "providers", "provider_sessions", "operator_settings", "events", "recordings", "models", "guards", "resources", "observability", "product", "resilience")

func runFunctionalShape(pass *analysis.Pass) (any, error) {
	unit, ok := unitKey(pass)
	if !ok {
		return nil, nil
	}
	path := strings.TrimSuffix(unit, "_test")
	var found []violation
	if under(path, "tests/functional") && !under(path, "tests/functional/internal/support") {
		parts := strings.Split(strings.TrimPrefix(path, "tests/functional/"), "/")
		domain := parts[0]
		for _, file := range pass.Files {
			name := serviceSource(pass, unit, file)
			add := func(rule, target string) {
				found = append(found, violation{rule: rule, importer: unit, importee: target, pos: file.Package, hint: "move this source to its durable functional domain/subsection owner"})
			}
			switch {
			case domain == "runtime_api":
				add("deprecated-runtime-api-file", name)
				if !ast.IsGenerated(file) {
					for _, declaration := range file.Decls {
						fn, ok := declaration.(*ast.FuncDecl)
						if !ok || fn.Recv != nil || !strings.HasPrefix(fn.Name.Name, "Test") || genuineTestMain(pass, fn) {
							continue
						}
						found = append(found, violation{rule: "deprecated-runtime-api-test", importer: unit, importee: name + "#" + fn.Name.Name, pos: fn.Pos(), hint: "move this scenario to its durable owner"})
					}
				}
			case path == "tests/functional" || len(parts) < 2:
				add("functional-test-missing-subsection", name)
			case !functionalDomains[domain]:
				add("functional-test-unclassified-domain", name)
			}
		}
	}
	reportWithBaseline(pass, unit, functionalShapeRules, found, true, functionalShapeBaseline(pass, unit))
	return nil, nil
}

func genuineTestMain(pass *analysis.Pass, fn *ast.FuncDecl) bool {
	if fn.Name.Name != "TestMain" || fn.Recv != nil || pass.TypesInfo == nil {
		return false
	}
	obj, ok := pass.TypesInfo.Defs[fn.Name].(*types.Func)
	if !ok {
		return false
	}
	sig := obj.Type().(*types.Signature)
	if sig.TypeParams().Len() != 0 || sig.Variadic() || sig.Params().Len() != 1 || sig.Results().Len() != 0 {
		return false
	}
	ptr, ok := sig.Params().At(0).Type().(*types.Pointer)
	if !ok {
		return false
	}
	named, ok := types.Unalias(ptr.Elem()).(*types.Named)
	return ok && named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == "testing" && named.Obj().Name() == "M"
}

// Ordinary compilation units omit test files. Tag/platform-excluded files are
// judged by their selecting configuration, never parsed by this analyzer.
func functionalShapeBaseline(pass *analysis.Pass, unit string) map[string]struct{} {
	listed := serviceShapeBaseline(pass, unit)
	selected := map[string]bool{}
	for _, file := range pass.Files {
		selected[serviceSource(pass, unit, file)] = true
	}
	ignored := map[string]bool{}
	for _, name := range pass.IgnoredFiles {
		// IgnoredFiles uses compiler filenames; only the basename is needed.
		ignored[sourceName(unit, name)] = true
	}
	hasTests := false
	for name := range selected {
		hasTests = hasTests || strings.HasSuffix(name, "_test.go")
	}
	for key := range listed {
		parts := strings.SplitN(key, "|", 3)
		if len(parts) != 3 || parts[1] != unit || !functionalShapeRules[parts[0]] {
			continue
		}
		name, _, _ := strings.Cut(parts[2], "#")
		if ignored[name] || (strings.HasSuffix(name, "_test.go") && !hasTests) {
			delete(listed, key)
		}
	}
	return listed
}
