package packagedfactorycatalog_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/portpowered/infinite-you/internal/packagedfactorycatalog"
	packagedfactories "github.com/portpowered/infinite-you/packages/packaged-factories"
)

func TestBuildCatalogIsByteDeterministicAndComplete(t *testing.T) {
	t.Parallel()

	first, err := packagedfactorycatalog.BuildCatalog(
		context.Background(),
		packagedfactories.Source(),
		"factories",
		testSchemaPath,
	)
	if err != nil {
		t.Fatalf("BuildCatalog(first): %v", err)
	}
	second, err := packagedfactorycatalog.BuildCatalog(
		context.Background(),
		packagedfactories.Source(),
		"factories",
		testSchemaPath,
	)
	if err != nil {
		t.Fatalf("BuildCatalog(second): %v", err)
	}
	if !reflect.DeepEqual(first.Files, second.Files) {
		t.Fatal("identical source produced different catalog bytes")
	}
	if len(first.Files) != 30 {
		t.Fatalf("outputs = %d, want 30 (fourteen pairs, manifest, notice)", len(first.Files))
	}
	if !strings.HasPrefix(
		string(first.Files["generated/README.md"]),
		"<!-- Code generated ",
	) {
		t.Fatal("generated content notice is missing the repository convention")
	}
}

func TestBuildCatalogIsIndependentOfInventoryInsertionOrder(t *testing.T) {
	t.Parallel()

	schemaPayload, err := fs.ReadFile(packagedfactories.Source(), testSchemaPath)
	if err != nil {
		t.Fatal(err)
	}
	firstSource := validInventoryFS(
		factoryFile("factories/zeta/factory.json", "@you/zeta", "id-zeta"),
		factoryFile("factories/alpha/factory.json", "@you/alpha", "id-alpha"),
	)
	secondSource := validInventoryFS(
		factoryFile("factories/alpha/factory.json", "@you/alpha", "id-alpha"),
		factoryFile("factories/zeta/factory.json", "@you/zeta", "id-zeta"),
	)
	firstSource[testSchemaPath] = &fstest.MapFile{Data: schemaPayload}
	secondSource[testSchemaPath] = &fstest.MapFile{Data: schemaPayload}

	first, err := packagedfactorycatalog.BuildCatalog(
		context.Background(),
		firstSource,
		"factories",
		testSchemaPath,
	)
	if err != nil {
		t.Fatalf("BuildCatalog(first): %v", err)
	}
	second, err := packagedfactorycatalog.BuildCatalog(
		context.Background(),
		secondSource,
		"factories",
		testSchemaPath,
	)
	if err != nil {
		t.Fatalf("BuildCatalog(second): %v", err)
	}
	if !reflect.DeepEqual(first.Files, second.Files) {
		t.Fatal("inventory insertion order changed complete catalog bytes")
	}
}

func TestBuildCatalogDetectsChangingSerializerAndOutputSet(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		next func(call int) map[string][]byte
		want string
	}{
		{
			name: "bytes",
			next: func(call int) map[string][]byte {
				return map[string][]byte{"generated/example/factory.json": []byte{byte(call)}}
			},
			want: "bytes changed at generated/example/factory.json",
		},
		{
			name: "set",
			next: func(call int) map[string][]byte {
				return map[string][]byte{
					"generated/example/factory.json":            []byte("same"),
					"generated/extra-" + string(rune('0'+call)): []byte("extra"),
				}
			},
			want: "output set changed at generated/extra-1",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			calls := 0
			_, err := packagedfactorycatalog.BuildCatalogWithDependencies(
				context.Background(),
				fstest.MapFS{},
				"factories",
				"schema.json",
				packagedfactorycatalog.BuildCatalogDependencies{
					BuildOnce: func(context.Context, fs.FS, string, string) (map[string][]byte, error) {
						calls++
						return test.next(calls), nil
					},
				},
			)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestBuildCatalogRejectsNormalizedAndParentOutputCollisions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		files map[string][]byte
		want  string
	}{
		{
			name: "normalized",
			files: map[string][]byte{
				"generated/Example/factory.json": []byte("one"),
				"generated/example/factory.json": []byte("two"),
			},
			want: "normalized output targets conflict",
		},
		{
			name: "parent file",
			files: map[string][]byte{
				"generated/example":              []byte("file"),
				"generated/example/factory.json": []byte("child"),
			},
			want: "conflicts with parent file",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := packagedfactorycatalog.BuildCatalogWithDependencies(
				context.Background(),
				fstest.MapFS{},
				"factories",
				"schema.json",
				packagedfactorycatalog.BuildCatalogDependencies{
					BuildOnce: func(context.Context, fs.FS, string, string) (map[string][]byte, error) {
						return test.files, nil
					},
				},
			)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestReplaceGeneratedDirectoryReplacesWholeCatalogAndPreservesOnPlanningFailure(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	oldPath := filepath.Join(root, "generated", "old.json")
	if err := os.MkdirAll(filepath.Dir(oldPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(oldPath, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	invalid := packagedfactorycatalog.Catalog{Files: map[string][]byte{
		"outside/generated.json": []byte("bad"),
	}}
	if err := packagedfactorycatalog.ReplaceGeneratedDirectory(root, invalid); err == nil {
		t.Fatal("expected invalid output plan to fail")
	}
	if payload, err := os.ReadFile(oldPath); err != nil || string(payload) != "old" {
		t.Fatalf("planning failure changed existing catalog: payload=%q err=%v", payload, err)
	}

	next := packagedfactorycatalog.Catalog{Files: map[string][]byte{
		"generated/manifest.json":  []byte("manifest\n"),
		"generated/a/factory.json": []byte("json\n"),
		"generated/a/factory.yaml": []byte("yaml\n"),
		"generated/README.md":      []byte("notice\n"),
	}}
	if err := packagedfactorycatalog.ReplaceGeneratedDirectory(root, next); err != nil {
		t.Fatalf("ReplaceGeneratedDirectory: %v", err)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("old mixed-catalog artifact remains: %v", err)
	}
	for target, want := range next.Files {
		payload, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(target)))
		if err != nil || string(payload) != string(want) {
			t.Fatalf("%s = %q, %v; want %q", target, payload, err, want)
		}
	}
}

func TestReplaceGeneratedDirectoryPreservesCatalogOnStagingFailure(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	oldPath := filepath.Join(root, "generated", "old.json")
	if err := os.MkdirAll(filepath.Dir(oldPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(oldPath, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	calls := 0
	err := packagedfactorycatalog.ReplaceGeneratedDirectoryWithDependencies(
		root,
		packagedfactorycatalog.Catalog{Files: map[string][]byte{
			"generated/manifest.json": []byte("manifest"),
			"generated/README.md":     []byte("notice"),
		}},
		packagedfactorycatalog.ReplaceCatalogDependencies{
			WriteFile: func(path string, payload []byte, mode fs.FileMode) error {
				calls++
				if calls == 2 {
					return errors.New("controlled write failure")
				}
				return os.WriteFile(path, payload, mode)
			},
		},
	)
	if err == nil || !strings.Contains(err.Error(), "controlled write failure") {
		t.Fatalf("error = %v", err)
	}
	if payload, readErr := os.ReadFile(oldPath); readErr != nil || string(payload) != "old" {
		t.Fatalf("staging failure changed existing catalog: payload=%q err=%v", payload, readErr)
	}
}
