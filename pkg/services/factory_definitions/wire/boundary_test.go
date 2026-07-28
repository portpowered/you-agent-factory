package wire_test

import (
	"os/exec"
	"strings"
	"testing"
)

const (
	factoryDefinitionsWirePackage = "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"
	transitionalServiceImport     = "github.com/portpowered/infinite-you/pkg/services/factory_definitions/service"
)

func TestWire_DoesNotImportTransitionalServiceShim(t *testing.T) {
	t.Parallel()

	cmd := exec.Command("go", "list", "-f", "{{.Imports}}", factoryDefinitionsWirePackage)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list imports for %s: %v\n%s", factoryDefinitionsWirePackage, err, output)
	}

	imports := strings.Fields(strings.Trim(string(output), "[]"))
	for _, importPath := range imports {
		if importPath == transitionalServiceImport ||
			strings.HasPrefix(importPath, transitionalServiceImport+"/") {
			t.Fatalf(
				"%s must construct from factory_definitions/internal, not transitional service shim; found %s",
				factoryDefinitionsWirePackage,
				importPath,
			)
		}
	}
}
