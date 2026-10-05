package analyzers

import (
	"errors"
	"go/ast"
	"go/token"
	"go/types"
	"reflect"
	"sort"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
	"golang.org/x/tools/go/packages"
)

// Existing fixture globals require serialization; the production snapshot does
// not share this state between plugin invocations.
func TestWireSelectionResolvedCallsAndLiterals(t *testing.T) {
	useFixtures(t)
	directory, cleanup, err := analysistest.WriteFiles(map[string]string{
		"m/pkg/platform/leaf/leaf.go": `package unusualname
type Local struct{}
func NewLocal() Local { return Local{} }
func OnlyReference() {}
func CommentOnly() {}
`,
		"m/pkg/platform/process/process.go": `package process
func NewParentOwnedStdio() {}
`,
		"m/pkg/wire/wire.go": `package wire
import alias "m/pkg/platform/leaf"
import p "m/pkg/platform/process"
var reference = alias.OnlyReference
var stdio = p.NewParentOwnedStdio
// alias.CommentOnly() does not select a leaf.
var value = alias.Local{}
func selectLeaf() {
 _ = alias.NewLocal()
 alias := struct { NewLocal func() }{func(){}}
 alias.NewLocal()
}
`,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)
	analyzer := &analysis.Analyzer{
		Name: "wireselectiontest", Doc: "observe resolved canonical choices",
		Run: func(pass *analysis.Pass) (any, error) {
			var selected []string
			for symbol := range typedWireSelections(pass.Files[0], pass.TypesInfo) {
				selected = append(selected, symbol)
			}
			sort.Strings(selected)
			want := []string{"m/pkg/platform/leaf.Local", "m/pkg/platform/leaf.NewLocal", "m/pkg/platform/process.NewParentOwnedStdio"}
			if !reflect.DeepEqual(selected, want) {
				t.Errorf("selections=%v, want %v", selected, want)
			}
			return nil, nil
		},
	}
	analysistest.Run(t, directory, analyzer, "m/pkg/wire")
}

func TestWireSelectionSourceExclusions(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		want bool
	}{
		{"pkg/wire/wire.go", true},
		{"pkg/wire/child/provider.go", true},
		{"pkg/wire/wire_gen.go", false},
		{"pkg/wire/provider_test.go", false},
	} {
		if got := wireSelectionSource(tc.name); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestWireSelectionCompilerMetadataFailsClosed(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		loaded []*packages.Package
		want   string
	}{
		{nil, "missing canonical Wire compiler sources"},
		{[]*packages.Package{{PkgPath: modulePrefix + "pkg/wire", Errors: []packages.Error{{Msg: "dependency unavailable"}}}}, "dependency unavailable"},
		{[]*packages.Package{{PkgPath: modulePrefix + "pkg/wirelookalike"}}, "unexpected Wire compiler owner"},
		{[]*packages.Package{{PkgPath: modulePrefix + "pkg/wire"}}, "missing Wire syntax/types"},
	} {
		selected, identity, err := collectWireSelections(tc.loaded, "configuration")
		if err == nil || !strings.Contains(err.Error(), tc.want) || selected != nil || identity != "configuration" {
			t.Fatalf("metadata failure must retain configuration and reject selections: %v %q %v", selected, identity, err)
		}
	}
}

func TestWireSelectionCacheIdentityAndIsolation(t *testing.T) {
	t.Parallel()
	selected := map[string]bool{"m/pkg/platform/clock.Real": true, "m/pkg/platform/filesystem.Local": true}
	first := wireSelectionSnapshot(selected, "source-v1/goos-linux", nil)
	stable := wireSelectionSnapshot(map[string]bool{"m/pkg/platform/filesystem.Local": true, "m/pkg/platform/clock.Real": true}, "source-v1/goos-linux", nil)
	if first.Name != stable.Name {
		t.Fatal("map insertion order changed snapshot identity")
	}
	delete(selected, "m/pkg/platform/clock.Real")
	for _, changed := range []*analysis.Analyzer{
		wireSelectionSnapshot(selected, "source-v1/goos-linux", nil),
		wireSelectionSnapshot(selected, "source-v2/goos-linux", nil),
		wireSelectionSnapshot(selected, "source-v1/goos-windows", nil),
		wireSelectionSnapshot(selected, "source-v1/goos-linux", errors.New("missing compiler input")),
	} {
		if first.Name == changed.Name {
			t.Fatal("changed selection, source, configuration or error reused cached success")
		}
	}
	if first.Name != stable.Name {
		t.Fatal("caller mutation altered existing snapshot")
	}
}

func TestWireSelectionMetadataFailureDiagnostic(t *testing.T) {
	t.Parallel()
	fset := token.NewFileSet()
	file := fset.AddFile("snapshot.go", -1, 20)
	position := file.Pos(1)
	analyzer := wireSelectionSnapshot(nil, "missing", errors.New("missing canonical Wire compiler sources"))
	var diagnostics []analysis.Diagnostic
	pass := &analysis.Pass{
		Analyzer: analyzer, Fset: fset,
		Pkg:    types.NewPackage(modulePrefix+"internal/lint/analyzers", "analyzers"),
		Files:  []*ast.File{{Package: position}},
		Report: func(d analysis.Diagnostic) { diagnostics = append(diagnostics, d) },
	}
	if _, err := analyzer.Run(pass); err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 1 || diagnostics[0].Message != "wire-selection-metadata: missing canonical Wire compiler sources; restore canonical Wire compiler inputs" {
		t.Fatalf("missing metadata must fail with remediation: %v", diagnostics)
	}
	pass.Pkg = types.NewPackage(modulePrefix+"pkg/platform/clock", "clock")
	if _, err := analyzer.Run(pass); err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 1 {
		t.Fatal("metadata failure repeated for each leaf package")
	}
}
