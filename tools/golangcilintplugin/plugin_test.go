package golangcilintplugin

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"strings"
	"testing"

	"github.com/golangci/plugin-module-register/register"
	"golang.org/x/tools/go/analysis"
)

func TestPluginRecordingReadDiagnostic(t *testing.T) {
	instance, err := New(nil)
	if err != nil {
		t.Fatal(err)
	}
	rules, err := instance.BuildAnalyzers()
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "request.go", "package sample\nfunc Request() { reader.LoadWorkerRecording() }", 0)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{Uses: map[*ast.Ident]types.Object{}}
	owner := types.NewPackage("github.com/portpowered/infinite-you/pkg/services/recordings", "recordings")
	fn := types.NewFunc(token.NoPos, owner, "LoadWorkerRecording", types.NewSignatureType(nil, nil, nil, nil, nil, false))
	ast.Inspect(file, func(node ast.Node) bool {
		if id, ok := node.(*ast.Ident); ok && id.Name == fn.Name() {
			info.Uses[id] = fn
		}
		return true
	})
	var diagnostics []analysis.Diagnostic
	for _, rule := range rules {
		if rule.Name != "recordingreads" {
			continue
		}
		_, err = rule.Run(&analysis.Pass{Analyzer: rule, Fset: fset, Files: []*ast.File{file}, TypesInfo: info,
			Pkg:    types.NewPackage("github.com/portpowered/infinite-you/pkg/sample", "sample"),
			Report: func(d analysis.Diagnostic) { diagnostics = append(diagnostics, d) }})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(diagnostics) != 1 || !strings.Contains(diagnostics[0].Message, "Request#LoadWorkerRecording") {
		t.Fatalf("plugin lost typed recording-read diagnostic: %v", diagnostics)
	}
}

func TestPluginPackagedFactorySourceDiagnostic(t *testing.T) {
	instance, err := New(nil)
	if err != nil {
		t.Fatal(err)
	}
	rules, err := instance.BuildAnalyzers()
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "source.go", "package sample\nvar definition = `name: '@you/plugin'`\n", 0)
	if err != nil {
		t.Fatal(err)
	}
	var diagnostics []analysis.Diagnostic
	for _, rule := range rules {
		if rule.Name != "packagedfactorysource" {
			continue
		}
		_, err = rule.Run(&analysis.Pass{Analyzer: rule, Fset: fset, Files: []*ast.File{file}, Pkg: types.NewPackage("github.com/portpowered/infinite-you/internal/sample", "sample"), Report: func(d analysis.Diagnostic) { diagnostics = append(diagnostics, d) }})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(diagnostics) != 1 || !strings.Contains(diagnostics[0].Message, "@you/plugin") || fset.Position(diagnostics[0].Pos).Line != 2 {
		t.Fatalf("plugin lost source diagnostic: %v", diagnostics)
	}
}

func TestPluginConfiguration(t *testing.T) {
	t.Parallel()
	constructor, err := register.GetPlugin("repolint")
	if err != nil {
		t.Fatal(err)
	}
	ordinary, err := constructor(map[string]any{"defer-stale": []string{"layering"}})
	if err != nil {
		t.Fatal(err)
	}
	strict, err := constructor(nil)
	if err != nil {
		t.Fatal(err)
	}
	if strict.GetLoadMode() != register.LoadModeTypesInfo {
		t.Fatal("plugin must request type information")
	}
	ordinaryRules, _ := ordinary.BuildAnalyzers()
	strictRules, _ := strict.BuildAnalyzers()
	if len(ordinaryRules) == 0 || len(ordinaryRules) != len(strictRules) {
		t.Fatal("configuration must retain the same nonempty rule set")
	}
	for _, rules := range [][]*analysis.Analyzer{ordinaryRules, strictRules} {
		var boundary, wire *analysis.Analyzer
		for _, rule := range rules {
			if rule.Name == "packageboundary" {
				boundary = rule
			}
			if strings.HasPrefix(rule.Name, "wireselection_") {
				wire = rule
			}
		}
		if boundary == nil || wire == nil || len(boundary.Requires) != 1 || boundary.Requires[0] != wire {
			t.Fatal("boundary must consume its own invocation Wire snapshot")
		}
	}
	for i, rule := range strictRules {
		if rule.Flags.Lookup("check-stale").Value.String() != "true" {
			t.Fatalf("strict %s lost stale checking", rule.Name)
		}
		want := "true"
		if rule.Name == "layering" {
			want = "false"
		}
		if ordinaryRules[i].Flags.Lookup("check-stale").Value.String() != want {
			t.Fatalf("ordinary %s: wrong stale setting", rule.Name)
		}
		if err := rule.Flags.Set("check-stale", "false"); err != nil {
			t.Fatal(err)
		}
		if ordinaryRules[i].Flags.Lookup("check-stale").Value.String() != want {
			t.Fatalf("%s settings leak between invocations", rule.Name)
		}
	}
}

func TestPluginRejectsInvalidSettings(t *testing.T) {
	t.Parallel()
	for _, raw := range []any{
		map[string]any{"unknown": true},
		map[string]any{"defer-stale": "layering"},
		map[string]any{"defer-stale": []string{"missing"}},
		map[string]any{"defer-stale": []string{"layering", "layering"}},
	} {
		if _, err := New(raw); err == nil {
			t.Fatalf("invalid settings accepted: %v", raw)
		}
	}
}
