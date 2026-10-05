package analyzers

import (
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPackagedFactorySourceLiterals(t *testing.T) {
	useFixtures(t)
	analysistest.Run(t, analysistest.TestData(), PackagedFactorySource, "m/internal/packagedsource")
}

func TestPackagedFactorySourceExclusions(t *testing.T) {
	for _, unit := range []string{"factory", packagedSourceBoundary + "/new", "internal/tests/sub", "internal/TestData", "examples/demo", "internal\\fixtures\\demo", ".git/a", ".artifacts/a", "coverage/a", "dist/a", "node_modules/a", "vendor/a"} {
		if !packagedSourceExcluded(unit) {
			t.Errorf("not excluded: %s", unit)
		}
	}
	for _, unit := range []string{"internal/mytests", "factoryextra", "internal/fixture", "pkg/services/factory_definitions"} {
		if packagedSourceExcluded(unit) {
			t.Errorf("wrongly excluded: %s", unit)
		}
	}
}

func packagedSourceDiagnostics(t *testing.T, analyzer *analysis.Analyzer, unit string) []analysis.Diagnostic {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "source.go", "package sample\n", 0)
	if err != nil {
		t.Fatal(err)
	}
	var diagnostics []analysis.Diagnostic
	_, err = analyzer.Run(&analysis.Pass{Analyzer: analyzer, Fset: fset, Files: []*ast.File{file}, Pkg: types.NewPackage(modulePrefix+unit, "sample"), Report: func(d analysis.Diagnostic) { diagnostics = append(diagnostics, d) }})
	if err != nil {
		t.Fatal(err)
	}
	return diagnostics
}

func packagedSourceFixture(name, id string) string {
	return `{"name":"` + name + `","id":"` + id + `","workTypes":[],"resources":[],"workers":[],"workstations":[]}`
}

func packagedSourceMetadata(t *testing.T, names []string, extra map[string]any) []byte {
	t.Helper()
	metadata := map[string]any{"Dir": filepath.Join(t.TempDir(), "owner"), "ImportPath": modulePrefix + packagedSourceUnit, "EmbedFiles": names}
	for key, value := range extra {
		metadata[key] = value
	}
	data, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestPackagedFactorySourceCompilerMetadata(t *testing.T) {
	for _, test := range []struct {
		name      string
		names     []string
		extra     map[string]any
		readError bool
		want      string
	}{
		{"empty", nil, nil, false, "no authored"},
		{"escape", []string{"factories/a/factory.json", "../outside"}, nil, false, "unsafe"},
		{"backslash", []string{`factories\a\factory.json`}, nil, false, "unsafe"},
		{"absolute", []string{"/outside"}, nil, false, "unsafe"},
		{"drive", []string{"C:/outside"}, nil, false, "unsafe"},
		{"duplicate", []string{"factories/a/factory.json", "factories/a/factory.json"}, nil, false, "duplicate"},
		{"owner", []string{"factories/a/factory.json"}, map[string]any{"ImportPath": "other"}, false, "owner"},
		{"directory", nil, map[string]any{"Dir": "relative"}, false, "directory"},
		{"incomplete", nil, map[string]any{"Error": map[string]string{"Err": "missing embed"}}, false, "missing embed"},
		{"dependency", nil, map[string]any{"DepsErrors": []map[string]string{{"Err": "broken dependency"}}}, false, "broken dependency"},
		{"unreadable", []string{"factories/a/factory.json"}, nil, true, "permission"},
	} {
		t.Run(test.name, func(t *testing.T) {
			reads := 0
			metadata := packagedSourceMetadata(t, test.names, test.extra)
			_, err := packagedCompilerSnapshot(metadata, func(string) ([]byte, error) { reads++; return nil, errors.New("permission") })
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("got %v, want %s", err, test.want)
			}
			if !test.readError && reads != 0 {
				t.Fatalf("unsafe input performed %d reads", reads)
			}
		})
	}
	if _, err := packagedCompilerSnapshot([]byte("{"), nil); err == nil {
		t.Fatal("malformed metadata accepted")
	}
}

func TestPackagedFactorySourceCatalogSnapshots(t *testing.T) {
	for _, test := range []struct {
		name  string
		files map[string]string
		want  string
	}{
		{"json", map[string]string{"factories/new/factory.json": packagedSourceFixture("@you/new", "new")}, ""},
		{"yaml", map[string]string{"factories/new/factory.yaml": "name: '@you/new'\nid: new\nworkTypes: []\nresources: []\nworkers: []\nworkstations: []\n"}, ""},
		{"yml", map[string]string{"factories/new/factory.yml": "name: '@you/new'\nid: new\nworkTypes: []\nresources: []\nworkers: []\nworkstations: []\n"}, ""},
		{"missing-asset", map[string]string{"factories/new/factory.json": `{"name":"@you/new","id":"new","workTypes":[],"resources":[],"workers":[{"name":"worker","type":"AGENT_WORKER","promptFile":"prompts/missing.md"}],"workstations":[]}`}, "missing.md"},
		{"owned-asset", map[string]string{"factories/new/factory.json": `{"name":"@you/new","id":"new","workTypes":[],"resources":[],"workers":[{"name":"worker","type":"AGENT_WORKER","promptFile":"prompts/worker.md"}],"workstations":[]}`, "factories/new/prompts/worker.md": "Owned prompt.\n"}, ""},
		{"invalid-slug", map[string]string{"factories/ new/factory.json": packagedSourceFixture("@you/new", "new")}, "invalid directory slug"},
		{"js", map[string]string{"factories/new/factory.js": "/* @you-factory-meta\n{\"name\":\"@you/new\",\"id\":\"new\",\"version\":1}\n*/\nexport default {};"}, ""},
		{"missing-root", map[string]string{"factories/new/asset.txt": "text"}, "no root Factory"},
		{"duplicate-root", map[string]string{"factories/new/factory.json": "{}", "factories/new/factory.yml": "{}"}, "2 root Factory"},
		{"unsupported", map[string]string{"factories/new/factory.JSON": "{}"}, "unsupported"},
		{"malformed", map[string]string{"factories/new/factory.json": "{"}, "decode"},
		{"unknown-field", map[string]string{"factories/new/factory.json": `{"name":"@you/new","id":"new","unknown":true}`}, "unknown"},
		{"missing-name", map[string]string{"factories/new/factory.json": packagedSourceFixture("", "new")}, "name"},
		{"missing-id", map[string]string{"factories/new/factory.json": packagedSourceFixture("@you/new", "")}, "project/id"},
		{"duplicate-name", map[string]string{"factories/a/factory.json": packagedSourceFixture("@you/same", "a"), "factories/b/factory.json": packagedSourceFixture("@you/same", "b")}, "duplicate public"},
		{"duplicate-id", map[string]string{"factories/a/factory.json": packagedSourceFixture("@you/a", "same"), "factories/b/factory.json": packagedSourceFixture("@you/b", "same")}, "duplicate Factory project"},
		{"duplicate-slug", map[string]string{"factories/A/factory.json": packagedSourceFixture("@you/a", "a"), "factories/a/factory.json": packagedSourceFixture("@you/b", "b")}, "duplicate directory"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var names []string
			for name := range test.files {
				names = append(names, name)
			}
			metadata := packagedSourceMetadata(t, names, nil)
			snapshot, err := packagedCompilerSnapshot(metadata, func(name string) ([]byte, error) {
				for key, value := range test.files {
					if strings.HasSuffix(filepath.ToSlash(name), key) {
						return []byte(value), nil
					}
				}
				return nil, errors.New("unexpected read")
			})
			if err != nil {
				t.Fatal(err)
			}
			analyzer := packagedSourceInputsSnapshot(metadata, snapshot, nil)
			diagnostics := packagedSourceDiagnostics(t, analyzer, packagedCatalogReportUnit)
			if test.want == "" {
				if len(diagnostics) != 0 {
					t.Fatal(diagnostics)
				}
				return
			}
			if len(diagnostics) != 1 || !strings.Contains(diagnostics[0].Message, test.want) {
				t.Fatalf("got %v, want %s", diagnostics, test.want)
			}
			if len(packagedSourceDiagnostics(t, analyzer, "internal/other")) != 0 {
				t.Fatal("catalog diagnostic leaked to another unit")
			}
		})
	}
}

func TestPackagedFactorySourceCompilerMetadataCacheIsolation(t *testing.T) {
	metadata := packagedSourceMetadata(t, []string{"factories/new/factory.json"}, nil)
	payload := []byte(packagedSourceFixture("@you/new", "new"))
	read := func(string) ([]byte, error) { return payload, nil }
	clean, err := packagedCompilerSnapshot(metadata, read)
	if err != nil {
		t.Fatal(err)
	}
	first := packagedSourceInputsSnapshot(metadata, clean, nil)
	payload[0] = '!'
	bad, err := packagedCompilerSnapshot(metadata, read)
	if err != nil {
		t.Fatal(err)
	}
	second := packagedSourceInputsSnapshot(metadata, bad, nil)
	if first.Name == second.Name {
		t.Fatal("non-Go change did not invalidate cache identity")
	}
	if len(packagedSourceDiagnostics(t, first, packagedCatalogReportUnit)) != 0 {
		t.Fatal("snapshot mutated")
	}
	if len(packagedSourceDiagnostics(t, second, packagedCatalogReportUnit)) != 1 {
		t.Fatal("changed input not rejected")
	}
	failed := packagedSourceInputsSnapshot(metadata, nil, errors.New("compiler query failed"))
	if len(packagedSourceDiagnostics(t, failed, packagedCatalogReportUnit)) != 1 {
		t.Fatal("metadata error failed open")
	}
}
