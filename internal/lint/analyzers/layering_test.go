package analyzers

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
	"golang.org/x/tools/go/packages"
)

// useFixtures points the analyzers at the m/ fixture module and a fixture
// baseline for the duration of one test. Tests using these analyzer globals
// remain serialized; this is fixture isolation, not application serialization.
func useFixtures(t *testing.T, entries ...string) {
	t.Helper()
	oldPrefix, oldBaseline := modulePrefix, baseline
	modulePrefix = "m/"
	listed := ""
	for _, entry := range entries {
		listed += entry + "\n"
	}
	baseline = func() map[string]struct{} { return parseBaseline(listed) }
	t.Cleanup(func() { modulePrefix, baseline = oldPrefix, oldBaseline })
}

func TestLayeringReportsEachRule(t *testing.T) {
	useFixtures(t,
		"service-subpackage|pkg/services/listed|pkg/services/b/sub",
		"service-subpackage|pkg/services/stale|pkg/services/b/sub",
	)
	analysistest.Run(t, analysistest.TestData(), Layering,
		"m/pkg/services/a", "m/pkg/services/c", "m/pkg/services/d", "m/pkg/services/listed", "m/pkg/services/stale",
		"m/pkg/platform/p", "m/pkg/initializer/i",
	)
}

func TestBehaviorReportsEachRule(t *testing.T) {
	useFixtures(t)
	analysistest.Run(t, analysistest.TestData(), Behavior,
		"m/pkg/initializer/ctor", "m/pkg/transports/mapping/mp",
	)
}

func TestEmbeddedBaselineParses(t *testing.T) {
	if entries := parseBaseline(baselineText); entries == nil {
		t.Fatal("baseline must parse")
	}
}

func TestLayeringPackageFamilies(t *testing.T) {
	useFixtures(t)
	packages := []string{"m/pkg/unapprovedfamily", "m/pkg/generatedexperimental"}
	for _, family := range []string{"config", "initializer", "internal", "platform", "root", "services", "transports", "wire"} {
		packages = append(packages, "m/pkg/"+family+"/familyfixture")
	}
	analysistest.Run(t, analysistest.TestData(), Layering, packages...)
}

func TestGeneratedOnlyRoots(t *testing.T) {
	useFixtures(t)
	analysistest.Run(t, analysistest.TestData(), Layering,
		"m/pkg/transports/http/client", "m/pkg/transports/http/generated", "m/pkg/transports/http/generated/sub")
}

func TestLayeringConstructedServiceEdges(t *testing.T) {
	useFixtures(t)
	analysistest.Run(t, analysistest.TestData(), Layering,
		"m/pkg/services/edgesnamed", "m/pkg/services/edgesblank", "m/pkg/services/edgesdot",
		"m/pkg/services/edges", "m/pkg/services/edges/sub", "m/pkg/services/edgestest")
}

func TestIgnoredImports(t *testing.T) {
	useFixtures(t)
	// analysistest does not read want comments in IgnoredFiles for custom
	// analyzers. Use the real package loader and observe emitted diagnostics.
	loaded, err := packages.Load(&packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax | packages.NeedTypes,
		Env:  append(os.Environ(), "GOPATH="+analysistest.TestData(), "GO111MODULE=off", "GOWORK=off"),
	}, "m/pkg/services/ignoredimports")
	if err != nil || len(loaded) != 1 || packages.PrintErrors(loaded) != 0 {
		t.Fatalf("load fixture: %v", err)
	}
	pkg := loaded[0]
	var diagnostics []analysis.Diagnostic
	pass := &analysis.Pass{
		Analyzer: Layering, Fset: pkg.Fset, Files: pkg.Syntax, Pkg: pkg.Types,
		IgnoredFiles: pkg.IgnoredFiles, OtherFiles: pkg.OtherFiles, ReadFile: os.ReadFile,
		Report: func(d analysis.Diagnostic) { diagnostics = append(diagnostics, d) },
	}
	if _, err := runLayering(pass); err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 2 {
		t.Fatalf("diagnostics=%v, want two excluded import violations", diagnostics)
	}
	for i, rule := range []string{"constructed-service-edges", "service-subpackage"} {
		position := pkg.Fset.Position(diagnostics[i].Pos)
		if filepath.Base(position.Filename) != "excluded.go" || position.Line != 8-i || !strings.HasPrefix(diagnostics[i].Message, rule+":") {
			t.Fatalf("diagnostic at %v: %s", position, diagnostics[i].Message)
		}
	}
}

func TestIgnoredAndOtherFilesPositions(t *testing.T) {
	useFixtures(t)
	for _, kind := range []string{"ignored", "other"} {
		t.Run(kind, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "base.go", "package sample", 0)
			if err != nil {
				t.Fatal(err)
			}
			name := filepath.Join(t.TempDir(), "excluded.go")
			reads := 0
			var diagnostics []analysis.Diagnostic
			pass := &analysis.Pass{
				Analyzer: Layering, Fset: fset, Files: []*ast.File{file},
				Pkg: types.NewPackage("m/pkg/services/sample", "sample"),
				ReadFile: func(path string) ([]byte, error) {
					reads++
					if path != name {
						t.Fatalf("read %q, want %q", path, name)
					}
					// Invalid declarations prove only imports are parsed.
					return []byte("package sample\nimport _ \"m/pkg/services/edges\"\nfunc !invalid"), nil
				},
				Report: func(d analysis.Diagnostic) { diagnostics = append(diagnostics, d) },
			}
			if kind == "ignored" {
				pass.IgnoredFiles = []string{name, "ignored.txt"}
				pass.OtherFiles = []string{name} // repeated compiler inputs are read once
			} else {
				pass.OtherFiles = []string{name, "other.txt"}
			}
			if _, err := runLayering(pass); err != nil {
				t.Fatal(err)
			}
			if reads != 1 || len(diagnostics) != 1 {
				t.Fatalf("reads=%d diagnostics=%v, want one import violation", reads, diagnostics)
			}
			position := fset.Position(diagnostics[0].Pos)
			if position.Filename != name || position.Line != 2 || !strings.Contains(diagnostics[0].Message, "constructed-service-edges|pkg/services/sample|pkg/services/edges") {
				t.Fatalf("diagnostic at %v: %s", position, diagnostics[0].Message)
			}
		})
	}
}
