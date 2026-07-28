package wire_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
)

const (
	operatorSettingsOwnerRelative = "pkg/services/operator_settings"
	operatorSettingsServicewireImport =
		"github.com/portpowered/infinite-you/pkg/services/operator_settings/servicewire"
)

// TestProductionPackagesOutsideOwnerDoNotImportServicewire proves the folded
// Operator Settings shape: transitional servicewire remains owner-private until
// DEL-SET and no production package outside the owner imports it.
func TestProductionPackagesOutsideOwnerDoNotImportServicewire(t *testing.T) {
	t.Parallel()

	repoRoot := testutil.MustRepoRoot(t)
	ownerRoot := filepath.Join(repoRoot, filepath.FromSlash(operatorSettingsOwnerRelative))

	for _, scanRoot := range []string{"pkg", "cmd"} {
		scanRoot := filepath.Join(repoRoot, scanRoot)
		err := filepath.WalkDir(scanRoot, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				if path == ownerRoot || strings.HasPrefix(path, ownerRoot+string(filepath.Separator)) {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				return nil
			}
			assertGoSourceDoesNotImportServicewire(t, path)
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", scanRoot, err)
		}
	}
}

func assertGoSourceDoesNotImportServicewire(t *testing.T, path string) {
	t.Helper()

	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, path, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	for _, importSpec := range file.Imports {
		importPath := strings.Trim(importSpec.Path.Value, `"`)
		if importPath == operatorSettingsServicewireImport ||
			strings.HasPrefix(importPath, operatorSettingsServicewireImport+"/") {
			t.Fatalf(
				"%s imports forbidden transitional package %s; construct Operator Settings through pkg/services/operator_settings/wire",
				path,
				importPath,
			)
		}
	}
}
