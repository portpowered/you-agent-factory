package packagedfactorycatalog_test

import (
	"os/exec"
	"strings"
	"testing"
)

const (
	packagedFactoryCatalogPackage = "github.com/portpowered/infinite-you/internal/packagedfactorycatalog"
	transitionalValidationImport  = "github.com/portpowered/infinite-you/pkg/services/factory_definitions/validation"
)

func TestPackagedFactoryCatalog_DoesNotImportTransitionalValidationShim(t *testing.T) {
	t.Parallel()

	cmd := exec.Command("go", "list", "-f", "{{.Imports}}", packagedFactoryCatalogPackage)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list imports for %s: %v\n%s", packagedFactoryCatalogPackage, err, output)
	}

	imports := strings.Fields(strings.Trim(string(output), "[]"))
	for _, importPath := range imports {
		if importPath == transitionalValidationImport ||
			strings.HasPrefix(importPath, transitionalValidationImport+"/") {
			t.Fatalf(
				"%s must use factory_definitions root validation contracts, not transitional validation shim; found %s",
				packagedFactoryCatalogPackage,
				importPath,
			)
		}
	}
}
