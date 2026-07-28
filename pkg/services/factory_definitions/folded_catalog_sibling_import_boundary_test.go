package factorydefinitions_test

import (
	"os/exec"
	"strings"
	"testing"
)

var foldedCatalogPublicSiblingImports = []string{
	factoryDefinitionsOwnerPrefix + "/namedpaths",
	factoryDefinitionsOwnerPrefix + "/namedfactories",
	factoryDefinitionsOwnerPrefix + "/persistence",
	factoryDefinitionsOwnerPrefix + "/resource",
}

// TestNonOwnerProductionPackages_DoNotImportFoldedCatalogPublicSiblings seals
// pss-cln-def-fold-catalog-006: production packages outside the Factory
// Definitions owner must not import transitional namedpaths/namedfactories/
// persistence/resource shims as the catalog implementation home.
func TestNonOwnerProductionPackages_DoNotImportFoldedCatalogPublicSiblings(t *testing.T) {
	t.Parallel()

	cmd := exec.Command(
		"go",
		"list",
		"-f",
		"{{.ImportPath}} {{join .Imports \" \"}}",
		"./...",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list repository packages: %v\n%s", err, output)
	}

	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		packagePath := fields[0]
		if strings.HasSuffix(packagePath, "_test") {
			continue
		}
		if packagePath == factoryDefinitionsOwnerPrefix ||
			strings.HasPrefix(packagePath, factoryDefinitionsOwnerPrefix+"/") {
			continue
		}
		for _, importPath := range fields[1:] {
			for _, foldedImport := range foldedCatalogPublicSiblingImports {
				if importPath != foldedImport &&
					!strings.HasPrefix(importPath, foldedImport+"/") {
					continue
				}
				t.Fatalf(
					"%s must not import folded catalog public sibling %s; use %s root contracts or owner wire/catalog construction",
					packagePath,
					importPath,
					factoryDefinitionsOwnerPrefix,
				)
			}
		}
	}
}
