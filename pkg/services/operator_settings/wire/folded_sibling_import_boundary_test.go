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
	operatorSettingsFoldedIdentityInventoryImport =
		"github.com/portpowered/infinite-you/pkg/services/operator_settings/identityinventory"
	operatorSettingsFoldedTestlinkImport =
		"github.com/portpowered/infinite-you/pkg/services/operator_settings/testlink"
	operatorSettingsFoldedTestprovidersImport =
		"github.com/portpowered/infinite-you/pkg/services/operator_settings/testproviders"
)

var operatorSettingsFoldedSiblingImports = []string{
	operatorSettingsFoldedIdentityInventoryImport,
	operatorSettingsFoldedTestlinkImport,
	operatorSettingsFoldedTestprovidersImport,
}

// TestProductionPackagesOutsideOwnerDoNotImportFoldedSiblingShims proves folded
// Operator Settings siblings remain owner-private until DEL-SET and no production
// package outside the owner imports identityinventory, testlink, or testproviders.
func TestProductionPackagesOutsideOwnerDoNotImportFoldedSiblingShims(t *testing.T) {
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
			assertGoSourceDoesNotImportFoldedSiblingShims(t, path)
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", scanRoot, err)
		}
	}
}

// TestOwnerProductionPackagesOutsideFoldedDestinationsDoNotImportTransitionalSiblingShims
// seals pss-cln-set-legacy-packages story 003: only delete-ready transitional shims
// may import the folded public sibling paths as their implementation home.
func TestOwnerProductionPackagesOutsideFoldedDestinationsDoNotImportTransitionalSiblingShims(t *testing.T) {
	t.Parallel()

	repoRoot := testutil.MustRepoRoot(t)
	ownerRoot := filepath.Join(repoRoot, filepath.FromSlash(operatorSettingsOwnerRelative))
	allowedImporterRoots := []string{
		filepath.Join(ownerRoot, "identityinventory"),
		filepath.Join(ownerRoot, "testlink"),
		filepath.Join(ownerRoot, "testproviders"),
	}

	err := filepath.WalkDir(ownerRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		for _, allowedRoot := range allowedImporterRoots {
			if path == allowedRoot || strings.HasPrefix(path, allowedRoot+string(filepath.Separator)) {
				return nil
			}
		}
		assertGoSourceDoesNotImportFoldedSiblingShims(t, path)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", ownerRoot, err)
	}
}

func assertGoSourceDoesNotImportFoldedSiblingShims(t *testing.T, path string) {
	t.Helper()

	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, path, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	for _, importSpec := range file.Imports {
		importPath := strings.Trim(importSpec.Path.Value, `"`)
		for _, forbidden := range operatorSettingsFoldedSiblingImports {
			if importPath == forbidden || strings.HasPrefix(importPath, forbidden+"/") {
				t.Fatalf(
					"%s imports forbidden transitional package %s; construct Operator Settings through pkg/services/operator_settings/wire and private owner destinations",
					path,
					importPath,
				)
			}
		}
	}
}
