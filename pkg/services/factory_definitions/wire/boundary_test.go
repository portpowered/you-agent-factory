package wire_test

import (
	"os/exec"
	"strings"
	"testing"
)

const (
	factoryDefinitionsWirePackage = "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"
	transitionalServiceImport     = "github.com/portpowered/infinite-you/pkg/services/factory_definitions/service"
	transitionalDefinitionImport  = "github.com/portpowered/infinite-you/pkg/services/factory_definitions/definition"
)

func TestWire_DoesNotImportTransitionalServiceShim(t *testing.T) {
	t.Parallel()

	assertWireDoesNotImport(t, transitionalServiceImport,
		"must construct from factory_definitions/internal, not transitional service shim")
}

func TestWire_DoesNotImportTransitionalDefinitionShim(t *testing.T) {
	t.Parallel()

	assertWireDoesNotImport(t, transitionalDefinitionImport,
		"must construct from factory_definitions/internal lifecycle composition, not public definition shim")
}

func assertWireDoesNotImport(t *testing.T, forbiddenImport, message string) {
	t.Helper()

	cmd := exec.Command("go", "list", "-f", "{{.Imports}}", factoryDefinitionsWirePackage)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list imports for %s: %v\n%s", factoryDefinitionsWirePackage, err, output)
	}

	imports := strings.Fields(strings.Trim(string(output), "[]"))
	for _, importPath := range imports {
		if importPath == forbiddenImport ||
			strings.HasPrefix(importPath, forbiddenImport+"/") {
			t.Fatalf(
				"%s %s; found %s",
				factoryDefinitionsWirePackage,
				message,
				importPath,
			)
		}
	}
}
