package analyzers

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"io/fs"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/packagedfactorycatalog"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
)

const catalogJS = "/* @you-factory-meta\n{\"name\":\"@you/example\",\"id\":\"example\",\"version\":1}\n*/\nreturn {};\n"

func cleanCatalogSnapshot(t *testing.T) catalogSnapshot {
	t.Helper()
	return catalogSnapshotForFactory(t, "example")
}

func catalogSnapshotForFactory(t *testing.T, name string) catalogSnapshot {
	t.Helper()
	snapshot := catalogSnapshot{
		"factories/" + name + "/factory.js": {data: []byte(strings.ReplaceAll(catalogJS, "example", name)), mode: 0o644},
		"schemas/factory.schema.json":       {data: []byte(`{"$id":"urn:catalog-fixture","type":"object"}`), mode: 0o644},
	}
	catalog, err := packagedfactorycatalog.BuildCatalog(context.Background(), snapshot, "factories", "schemas/factory.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range catalog.Files {
		snapshot[name] = catalogFile{data: data, mode: 0o644}
	}
	return snapshot
}

func TestPackagedFactoryCatalogAnalysistest(t *testing.T) {
	// Other analyzer fixtures replace modulePrefix; keep compiler fixture runs
	// serialized with them. Captured catalog data itself is invocation-local.
	snapshot := cleanCatalogSnapshot(t)
	snapshot["generated/manifest.json"] = catalogFile{data: []byte("{}"), mode: 0o644}
	delete(snapshot, "generated/factories/example/factory.yaml")
	snapshot["generated/unexpected file.json"] = catalogFile{data: []byte("{}"), mode: 0o644}
	analysistest.Run(t, analysistest.TestData(), packagedFactoryCatalogSnapshot(snapshot, nil),
		"github.com/portpowered/infinite-you/internal/lint/analyzers", "github.com/portpowered/infinite-you/internal/catalogunrelated")
}

func TestPackagedFactoryCatalogDriftAndReadOnly(t *testing.T) {
	t.Parallel()
	example := cleanCatalogSnapshot(t)
	second := catalogSnapshotForFactory(t, "second")
	exampleConversion := factorydefinitions.SerializedFactoryConfigInput(example["generated/factories/example/factory.json"].data).Path()
	secondConversion := factorydefinitions.SerializedFactoryConfigInput(second["generated/factories/second/factory.json"].data).Path()
	for _, tc := range []struct {
		name   string
		mutate func(catalogSnapshot)
		want   []string
	}{
		{"clean", func(catalogSnapshot) {}, nil},
		{"manifest", func(s catalogSnapshot) { s["generated/manifest.json"] = catalogFile{data: []byte("{}"), mode: 0o644} }, []string{"stale: generated/manifest.json"}},
		{"JSON", func(s catalogSnapshot) {
			s["generated/factories/example/factory.json"] = catalogFile{data: []byte("{}"), mode: 0o644}
		}, []string{"stale: generated/factories/example/factory.json"}},
		{"YAML", func(s catalogSnapshot) {
			s["generated/factories/example/factory.yaml"] = catalogFile{data: []byte("{}"), mode: 0o644}
		}, []string{"stale: generated/factories/example/factory.yaml"}},
		{"serialized conversion", func(s catalogSnapshot) {
			s[exampleConversion] = catalogFile{data: []byte("{}"), mode: 0o644}
		}, []string{"stale: " + exampleConversion}},
		{"missing conversion", func(s catalogSnapshot) { delete(s, exampleConversion) }, []string{"missing: " + exampleConversion}},
		{"conversion symlink", func(s catalogSnapshot) { s[exampleConversion] = catalogFile{mode: fs.ModeSymlink} }, []string{"non-regular: " + exampleConversion}},
		{"missing", func(s catalogSnapshot) { delete(s, "generated/manifest.json") }, []string{"missing: generated/manifest.json"}},
		{"all outputs missing", func(s catalogSnapshot) {
			for name := range s {
				if strings.HasPrefix(name, "generated/") {
					delete(s, name)
				}
			}
		}, []string{"missing: generated/README.md", "missing: generated/factories/example/factory.json", "missing: generated/factories/example/factory.yaml", "missing: generated/manifest.json", "missing: " + exampleConversion}},
		{"added Factory", func(s catalogSnapshot) {
			s["factories/second/factory.js"] = catalogFile{data: []byte(strings.ReplaceAll(catalogJS, "example", "second")), mode: 0o644}
		}, []string{"missing: generated/factories/second/factory.json", "missing: generated/factories/second/factory.yaml", "missing: " + secondConversion, "stale: generated/manifest.json"}},
		{"renamed Factory", func(s catalogSnapshot) {
			delete(s, "factories/example/factory.js")
			s["factories/second/factory.js"] = catalogFile{data: []byte(strings.ReplaceAll(catalogJS, "example", "second")), mode: 0o644}
		}, []string{"missing: generated/factories/second/factory.json", "missing: generated/factories/second/factory.yaml", "missing: " + secondConversion, "stale: generated/manifest.json", "unexpected: generated/factories/example/factory.json", "unexpected: generated/factories/example/factory.yaml", "unexpected: " + exampleConversion}},
		{"extra", func(s catalogSnapshot) { s["generated/nested/a b"] = catalogFile{mode: 0o644} }, []string{"unexpected: generated/nested/a b"}},
		{"notice prose", func(s catalogSnapshot) {
			s["generated/README.md"] = catalogFile{data: []byte("Edited prose"), mode: 0o644}
		}, nil},
		{"missing notice", func(s catalogSnapshot) { delete(s, "generated/README.md") }, []string{"missing: generated/README.md"}},
		{"notice symlink", func(s catalogSnapshot) { s["generated/README.md"] = catalogFile{mode: fs.ModeSymlink} }, []string{"non-regular: generated/README.md"}},
		{"output directory", func(s catalogSnapshot) { s["generated/manifest.json"] = catalogFile{mode: fs.ModeDir} }, []string{"non-regular: generated/manifest.json"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			snapshot := cleanCatalogSnapshot(t)
			tc.mutate(snapshot)
			before := packagedFactoryCatalogSnapshot(snapshot, nil).Name
			if got := catalogFindings(snapshot, nil); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("findings = %v, want %v", got, tc.want)
			}
			if after := packagedFactoryCatalogSnapshot(snapshot, nil).Name; before != after {
				t.Fatal("analysis mutated catalog input")
			}
		})
	}
}

func TestPackagedFactoryCatalogProjectionFailures(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		mutate func(catalogSnapshot)
		want   string
	}{
		{"missing schema", func(s catalogSnapshot) { delete(s, "schemas/factory.schema.json") }, "schema"},
		{"malformed schema", func(s catalogSnapshot) {
			s["schemas/factory.schema.json"] = catalogFile{data: []byte("{"), mode: 0o644}
		}, "schema"},
		{"schema ID", func(s catalogSnapshot) {
			s["schemas/factory.schema.json"] = catalogFile{data: []byte(`{"type":"object"}`), mode: 0o644}
		}, "$id"},
		{"metadata", func(s catalogSnapshot) {
			s["factories/example/factory.js"] = catalogFile{data: []byte("return {};"), mode: 0o644}
		}, "@you-factory-meta"},
		{"ambiguous", func(s catalogSnapshot) {
			s["factories/example/factory.yaml"] = catalogFile{data: []byte("{}"), mode: 0o644}
		}, "exactly one"},
		{"unsupported", func(s catalogSnapshot) { s["factories/example/factory.txt"] = catalogFile{mode: 0o644} }, "unsupported"},
		{"missing root", func(s catalogSnapshot) { s["factories/empty/note.md"] = catalogFile{mode: 0o644} }, "no root Factory"},
		{"nonregular root", func(s catalogSnapshot) { s["factories/example/factory.js"] = catalogFile{mode: fs.ModeSymlink} }, "not a regular file"},
		{"duplicate identity", func(s catalogSnapshot) { s["factories/second/factory.js"] = s["factories/example/factory.js"] }, "duplicate"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			snapshot := cleanCatalogSnapshot(t)
			tc.mutate(snapshot)
			findings := catalogFindings(snapshot, nil)
			if len(findings) != 1 || !strings.Contains(findings[0], "projection failed:") || !strings.Contains(findings[0], tc.want) {
				t.Fatalf("findings = %v, want projection failure containing %q", findings, tc.want)
			}
		})
	}
}

func TestPackagedFactoryCatalogSnapshotIdentity(t *testing.T) {
	t.Parallel()
	source := cleanCatalogSnapshot(t)
	first := packagedFactoryCatalogSnapshot(source, nil)
	if first.Name != packagedFactoryCatalogSnapshot(source, nil).Name {
		t.Fatal("identical snapshots changed cache identity")
	}
	for _, change := range []func(catalogSnapshot){
		func(s catalogSnapshot) { s["factories/example/factory.js"].data[0] = 'x' },
		func(s catalogSnapshot) { delete(s, "generated/manifest.json") },
		func(s catalogSnapshot) { s["generated/extra"] = catalogFile{mode: 0o644} },
		func(s catalogSnapshot) {
			f := s["generated/README.md"]
			f.mode = fs.ModeSymlink
			s["generated/README.md"] = f
		},
	} {
		next := cleanCatalogSnapshot(t)
		change(next)
		if first.Name == packagedFactoryCatalogSnapshot(next, nil).Name {
			t.Fatal("changed catalog input retained cache identity")
		}
	}
	if first.Name == packagedFactoryCatalogSnapshot(source, errors.New("read failed")).Name {
		t.Fatal("input error retained clean cache identity")
	}
	source["factories/example/factory.js"].data[0] = 'x'
	if first.Name == packagedFactoryCatalogSnapshot(source, nil).Name {
		t.Fatal("captured snapshot aliases caller bytes")
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "source.go", "package analyzers", 0)
	if err != nil {
		t.Fatal(err)
	}
	pass := &analysis.Pass{Fset: fset, Files: []*ast.File{file}, Pkg: types.NewPackage("github.com/portpowered/infinite-you/internal/lint/analyzers", "analyzers"), Report: func(d analysis.Diagnostic) { t.Errorf("caller mutation affected captured analyzer: %s", d.Message) }}
	if _, err := first.Run(pass); err != nil {
		t.Fatal(err)
	}
}

func TestPackagedFactoryCatalogInventoryBoundary(t *testing.T) {
	t.Parallel()
	for _, inventory := range []string{"../outside\x00", catalogPackagePath + "../outside\x00", catalogPackagePath + "generated/..\\..\\outside\x00", catalogPackagePath + "generated/file:stream\x00", catalogPackagePath + "package.json\x00", catalogPackagePath + "generated/file"} {
		calls := 0
		_, err := readCatalogSnapshot([]byte(inventory), func(string) (catalogFile, error) { calls++; return catalogFile{}, nil })
		if err == nil || calls != 0 {
			t.Fatalf("unsafe inventory %q read %d files, error %v", inventory, calls, err)
		}
	}
	data := []byte("space-safe")
	snapshot, err := readCatalogSnapshot([]byte(catalogPackagePath+"generated/a b\x00"+catalogPackagePath+"generated/deleted\x00"), func(name string) (catalogFile, error) {
		if strings.HasSuffix(name, "deleted") {
			return catalogFile{}, os.ErrNotExist
		}
		return catalogFile{data: data, mode: 0o644}, nil
	})
	if err != nil || len(snapshot) != 1 || string(snapshot["generated/a b"].data) != "space-safe" {
		t.Fatalf("snapshot = %v, error %v", snapshot, err)
	}
	data[0] = 'x'
	if string(snapshot["generated/a b"].data) != "space-safe" {
		t.Fatal("snapshot aliases IO bytes")
	}
	_, err = readCatalogSnapshot([]byte(catalogPackagePath+"generated/unreadable\x00"), func(string) (catalogFile, error) { return catalogFile{}, errors.New("denied") })
	if err == nil || !strings.Contains(err.Error(), "generated/unreadable") {
		t.Fatalf("read error = %v", err)
	}
	if got := catalogFindings(nil, errors.New("git unavailable")); len(got) != 1 || !strings.Contains(got[0], "git unavailable") {
		t.Fatalf("input failure = %v", got)
	}
}
