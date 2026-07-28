package factorydefinitions_test

import (
	"os/exec"
	"strings"
	"testing"
)

const (
	wireFactoryDefinitionsPackage = "github.com/portpowered/infinite-you/pkg/wire/factorydefinitions"
	transitionalValidationImport  = "github.com/portpowered/infinite-you/pkg/services/factory_definitions/validation"
)

func TestWireFactoryDefinitions_DoesNotImportTransitionalValidationShim(t *testing.T) {
	t.Parallel()

	cmd := exec.Command("go", "list", "-f", "{{.Imports}}", wireFactoryDefinitionsPackage)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list imports for %s: %v\n%s", wireFactoryDefinitionsPackage, err, output)
	}

	imports := strings.Fields(strings.Trim(string(output), "[]"))
	for _, importPath := range imports {
		if importPath == transitionalValidationImport ||
			strings.HasPrefix(importPath, transitionalValidationImport+"/") {
			t.Fatalf(
				"%s must bind Factory Definitions validation through root contracts, not transitional validation shim; found %s",
				wireFactoryDefinitionsPackage,
				importPath,
			)
		}
	}
}
