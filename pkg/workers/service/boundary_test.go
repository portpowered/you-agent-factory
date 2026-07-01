package service

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPackage_DoesNotImportForbiddenDependencies(t *testing.T) {
	forbidden := []string{
		"github.com/portpowered/infinite-you/pkg/service",
		"github.com/portpowered/infinite-you/pkg/factorysessions",
	}

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}

	fset := token.NewFileSet()
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(".", entry.Name())
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("ParseFile(%s): %v", path, err)
		}
		for _, imp := range file.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			for _, blocked := range forbidden {
				if importPath == blocked {
					t.Fatalf("%s imports forbidden package %s", path, blocked)
				}
			}
		}
	}
}
