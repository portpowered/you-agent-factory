package packagedfactorycatalog_test

import (
	"os/exec"
	"strings"
	"testing"
)

const (
	packagedFactoryCatalogPackage = "github.com/portpowered/infinite-you/internal/packagedfactorycatalog"
	transitionalValidationImport  = "github.com/portpowered/infinite-you/pkg/services/factory_definitions/validation"
	factoryDefinitionsWireImport  = "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"
)

func TestPackagedFactoryCatalog_DoesNotImportTransitionalValidationShim(t *testing.T) {
	t.Parallel()

	assertPackagedFactoryCatalogDoesNotImport(t, transitionalValidationImport, "transitional validation shim")
}

func TestPackagedFactoryCatalog_DoesNotImportFactoryDefinitionsWireComposition(t *testing.T) {
	t.Parallel()

	cmd := exec.Command("go", "list", "-f", "{{.Imports}}", packagedFactoryCatalogPackage)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list imports for %s: %v\n%s", packagedFactoryCatalogPackage, err, output)
	}

	imports := strings.Fields(strings.Trim(string(output), "[]"))
	for _, importPath := range imports {
		if importPath == factoryDefinitionsWireImport {
			t.Fatalf(
				"%s must use factory_definitions/wire/validation, not %s composition; found %s",
				packagedFactoryCatalogPackage,
				factoryDefinitionsWireImport,
				importPath,
			)
		}
	}
}

func assertPackagedFactoryCatalogDoesNotImport(t *testing.T, forbiddenImport, label string) {
	t.Helper()

	cmd := exec.Command("go", "list", "-f", "{{.Imports}}", packagedFactoryCatalogPackage)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list imports for %s: %v\n%s", packagedFactoryCatalogPackage, err, output)
	}

	imports := strings.Fields(strings.Trim(string(output), "[]"))
	for _, importPath := range imports {
		if importPath == forbiddenImport ||
			strings.HasPrefix(importPath, forbiddenImport+"/") {
			t.Fatalf(
				"%s must use factory_definitions root validation contracts, not %s; found %s",
				packagedFactoryCatalogPackage,
				label,
				importPath,
			)
		}
	}
}
