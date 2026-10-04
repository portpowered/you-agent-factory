package analyzers

import (
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestConstructionDurableRules(t *testing.T) {
	useFixtures(t)
	analysistest.Run(t, analysistest.TestData(), DurableConstruction,
		"m/pkg/runtime/durable", "m/pkg/runtime/dotdurable",
		"m/pkg/transports/durable", "m/pkg/transports/durableexternal",
		"m/pkg/services/factory_sessions/internal/execution",
		"m/pkg/services/factory_sessions/internal/execution/runtimepersist",
		"m/pkg/services/factory_sessions/internal/execution/livechild",
		"m/pkg/services/factory_runtime/internal/services/orchestration/javascript/durable",
		"m/pkg/services/factory_sessions/internal/services/durable_execution/internal/service",
		"m/pkg/initializer/application", "m/pkg/transports/cli/mcp",
	)
}

func TestConstructionTaggedDurableRule(t *testing.T) {
	useFixtures(t)
	t.Setenv("GOFLAGS", "-tags=backendconformance")
	analysistest.Run(t, analysistest.TestData(), DurableConstruction, "m/pkg/runtime/durabletagged")
}

// The retired import cannot be a valid compiled Go fixture: it violates the
// compiler's internal-package boundary and its real package has been deleted.
// Exercise the allowed ImportsOnly edge instead, with an intentionally invalid
// excluded call body; this must not claim type checking of that body.
func TestConstructionExcludedRetiredProviderImport(t *testing.T) {
	useFixtures(t)
	for _, unit := range []string{
		"pkg/services/factory_sessions/internal/execution/livechild",
		"pkg/services/factory_runtime/internal/services/orchestration/javascript/runtime",
	} {
		t.Run(unit, func(t *testing.T) {
			filename := filepath.Join(t.TempDir(), "excluded_windows.go")
			text := "package fixture\nimport \"m/" + retiredLiveChildProvider + "\"\nfunc invalid( {"
			if err := os.WriteFile(filename, []byte(text), 0o600); err != nil {
				t.Fatal(err)
			}
			fset := token.NewFileSet()
			compiled := fset.AddFile("compiled.go", -1, 20)
			var diagnostics []analysis.Diagnostic
			pass := &analysis.Pass{
				Fset: fset, Pkg: types.NewPackage("m/"+unit, "fixture"),
				Files:        []*ast.File{{Package: compiled.Pos(0)}},
				IgnoredFiles: []string{filename}, OtherFiles: []string{filename},
				Report: func(d analysis.Diagnostic) { diagnostics = append(diagnostics, d) },
			}
			if _, err := runDurableConstruction(pass); err != nil {
				t.Fatal(err)
			}
			if len(diagnostics) != 1 || !strings.Contains(diagnostics[0].Message, retiredLiveChildProvider) {
				t.Fatalf("diagnostics = %#v, want one retired provider import", diagnostics)
			}
			position := fset.Position(diagnostics[0].Pos)
			if position.Filename != filename || position.Line != 2 {
				t.Fatalf("position = %v, want excluded import at line 2", position)
			}
		})
	}
}

func TestConstructionExactBaselineAndStaleEntry(t *testing.T) {
	useFixtures(t,
		"durable-runtime-construction|pkg/runtime/durablelisted|NewJavaScriptRuntimeService",
		"durable-runtime-construction|pkg/runtime/durablestale|NewJavaScriptRuntimeService",
	)
	analysistest.Run(t, analysistest.TestData(), DurableConstruction,
		"m/pkg/runtime/durablelisted", "m/pkg/runtime/durablestale",
	)
}
