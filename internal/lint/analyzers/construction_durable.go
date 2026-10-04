package analyzers

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/types/typeutil"
)

// DurableConstruction enforces durable execution and persistence ownership using the
// compilation unit's resolved declarations. The staged T20 registry migration
// remains separate from these always-enforced durable rules.
var DurableConstruction = &analysis.Analyzer{
	Name: "durableconstruction",
	Doc:  "enforce durable execution, persistence, canonical event and live-child construction ownership",
	Run:  runDurableConstruction,
}

var durableCompositionCalls = setOf(
	"BuildInvocationBootstrap", "NewExecutionService", "NewFakeServiceFromContractFixtures", "ProjectPersistence",
)

var durableCanonicalCalls = setOf(
	"AppendDispatchInterruptedEvent", "BuildCanonicalRuntimeSessionEvents", "MapCanonicalRuntimeSessionEvents",
)

var durableCompositionFiles = setOf(
	"pkg/initializer/application/entrypoints.go",
	"pkg/services/factory_sessions/internal/execution/service.go",
	"pkg/services/factory_sessions/internal/services/durable_execution/internal/service/construction.go",
)

var durablePersistenceFiles = setOf(
	"pkg/transports/cli/mcp/serve_runtime_resume_smoke_test.go",
	"pkg/services/factory_sessions/transports/cli/session/smoke/resume_smoke_test.go",
	"pkg/services/factory_sessions/internal/execution/service.go",
	"pkg/services/factory_sessions/internal/execution/runtimepersist/store.go",
	"pkg/services/factory_sessions/transports/mcp/execution_test.go",
)

const durableExecutionRoot = "pkg/services/factory_sessions/internal/execution"
const retiredLiveChildProvider = "pkg/services/workers/internal/providercompat"

var durableRuleNames = func() map[string]bool {
	names := setOf("durable-application-composition", "durable-canonical-events", "durable-live-child-provider",
		"durable-runtime-construction", "durable-persistence-construction", "durable-persistence-boolean")
	// Insert after iterating: new entries in an iterated map may be visited too.
	tests := make([]string, 0, len(names))
	for rule := range names {
		tests = append(tests, rule+"-test")
	}
	for _, rule := range tests {
		names[rule] = true
	}
	return names
}()

func runDurableConstruction(pass *analysis.Pass) (any, error) {
	unit, ok := unitKey(pass)
	if !ok {
		return nil, nil
	}
	var found []violation
	seen := map[string]bool{}
	hasTests := false
	for _, file := range pass.Files {
		if ast.IsGenerated(file) {
			continue
		}
		filename := pass.Fset.Position(file.Pos()).Filename
		test := strings.HasSuffix(filename, "_test.go")
		hasTests = hasTests || test
		if test && !under(strings.TrimSuffix(unit, "_test"), "pkg/transports") {
			continue // Preserve the legacy exclusion for non-transport test doubles.
		}
		// The compiler gives us the package path and file identity; no repository
		// root discovery or authored-source indexing is needed.
		relative := strings.TrimSuffix(unit, "_test") + "/" + filepath.Base(filename)
		add := func(rule, target, hint string, pos token.Pos) {
			if test {
				rule += "-test"
			}
			finding := violation{rule: rule, importer: unit, importee: target, pos: pos, hint: hint}
			if !seen[finding.key()] {
				seen[finding.key()] = true
				found = append(found, finding)
			}
		}
		durableFileFindings(pass, file, relative, test, add)
	}
	durableExcludedImports(pass, unit, &found)
	reportAgainstBaseline(pass, unit, durableRuleNames, found, hasTests)
	return nil, nil
}

func durableLiveChild(path string) bool {
	return under(path, durableExecutionRoot+"/livechild") ||
		under(path, "pkg/services/factory_runtime/internal/services/orchestration/javascript")
}

func durableFileFindings(pass *analysis.Pass, file *ast.File, relative string, test bool, add addFinding) {
	durableImportFindings(file, relative, add)
	ast.Inspect(file, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.CallExpr:
			if object := typeutil.Callee(pass.TypesInfo, value); object != nil {
				durableCallFindings(object.Name(), relative, test, value.Pos(), add)
			}
		case *ast.KeyValueExpr:
			if key, ok := value.Key.(*ast.Ident); ok {
				if field, ok := pass.TypesInfo.Uses[key].(*types.Var); ok && field.IsField() && field.Name() == "PersistSessions" {
					add("durable-persistence-boolean", "PersistSessions",
						"inject a persistence store or explicit disabled policy instead of a boolean", value.Pos())
				}
			}
		case *ast.CompositeLit:
			if named, ok := types.Unalias(pass.TypesInfo.TypeOf(value)).(*types.Named); ok &&
				named.Obj().Name() == "DirectoryStore" && !durablePersistenceFiles[relative] {
				add("durable-persistence-construction", "DirectoryStore literal",
					"construct durable persistence at the approved application composition boundary", value.Pos())
			}
		}
		return true
	})
}

func durableImportFindings(file *ast.File, relative string, add addFinding) {
	if durableLiveChild(relative) {
		for _, spec := range file.Imports {
			path, err := strconv.Unquote(spec.Path.Value)
			if err == nil && relImport(path) == retiredLiveChildProvider {
				add("durable-live-child-provider", retiredLiveChildProvider,
					"route production live-child provider execution through pkg/services/workers", spec.Pos())
			}
		}
	}
}

// Only imports of tag-excluded files are readable without compiler types.
// Retired provider imports are still rejected, but excluded call bodies are
// deliberately left to the native platform/tag-union compiler invocation.
func durableExcludedImports(pass *analysis.Pass, unit string, found *[]violation) {
	importer := strings.TrimSuffix(unit, "_test")
	if !durableLiveChild(importer) {
		return
	}
	names := append(append([]string(nil), pass.IgnoredFiles...), pass.OtherFiles...)
	seen := map[string]bool{}
	for _, filename := range names {
		if seen[filename] || !strings.HasSuffix(filename, ".go") || strings.HasSuffix(filename, "_test.go") {
			continue
		}
		seen[filename] = true
		file, err := parser.ParseFile(pass.Fset, filename, nil, parser.ImportsOnly|parser.ParseComments)
		if err != nil {
			pass.Reportf(pass.Files[0].Package, "durable-live-child-provider: cannot read excluded imports in %s: %v", filename, err)
			continue
		}
		if ast.IsGenerated(file) {
			continue
		}
		durableImportFindings(file, importer+"/"+filepath.Base(filename), func(rule, target, hint string, pos token.Pos) {
			*found = append(*found, violation{rule: rule, importer: unit, importee: target, pos: pos, hint: hint})
		})
	}
}

// Durable policy deliberately keeps the legacy by-declaration-name bans. A
// retired constructor name stays forbidden even if its declaring package moves.
func durableCallFindings(name, relative string, test bool, pos token.Pos, add addFinding) {
	if durableCompositionCalls[name] && !test && !durableCompositionFiles[relative] {
		add("durable-application-composition", name,
			"construct application collaborators in pkg/wire and inject them into the transport", pos)
	}
	if durableCanonicalCalls[name] && !under(relative, durableExecutionRoot) {
		add("durable-canonical-events", name,
			"route canonical Factory Events through the factory_sessions execution recorder and persistence owner", pos)
	}
	if name == "Infer" && durableLiveChild(relative) {
		add("durable-live-child-provider", name,
			"route production live-child provider invocation through pkg/services/workers", pos)
	}
	if name == "NewJavaScriptRuntimeService" && relative != durableExecutionRoot+"/service.go" {
		add("durable-runtime-construction", name, "construct durable execution in an approved composition owner", pos)
	}
	if (name == "NewLazyProjectStore" || name == "DirForProjectRoot") && !durablePersistenceFiles[relative] {
		add("durable-persistence-construction", name,
			"resolve and construct durable persistence at the approved application composition boundary", pos)
	}
}
