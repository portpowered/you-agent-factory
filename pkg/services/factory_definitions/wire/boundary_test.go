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

var transitionalSnapshotPackages = []string{
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/portableconfig",
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/snapshotcapture",
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/editable",
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/replayconfig",
}

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

func TestWire_DoesNotImportTransitionalSnapshotShims(t *testing.T) {
	t.Parallel()

	cmd := exec.Command("go", "list", "-f", "{{.Imports}}", factoryDefinitionsWirePackage)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list imports for %s: %v\n%s", factoryDefinitionsWirePackage, err, output)
	}

	imports := strings.Fields(strings.Trim(string(output), "[]"))
	for _, importPath := range imports {
		for _, forbidden := range transitionalSnapshotPackages {
			if importPath == forbidden || strings.HasPrefix(importPath, forbidden+"/") {
				t.Fatalf(
					"%s must construct from snapshots_portability internals, not transitional shim %s",
					factoryDefinitionsWirePackage,
					importPath,
				)
			}
		}
	}
}
