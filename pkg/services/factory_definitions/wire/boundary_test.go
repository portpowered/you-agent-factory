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
	transitionalValidationImport  = "github.com/portpowered/infinite-you/pkg/services/factory_definitions/validation"
)

var transitionalSnapshotPackages = []string{
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/portableconfig",
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/snapshotcapture",
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/editable",
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/replayconfig",
}

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

func TestWire_DoesNotImportTransitionalValidationShim(t *testing.T) {
	t.Parallel()

	assertWireDoesNotImport(t, transitionalValidationImport,
		"must construct from factory_definitions root validation contracts, not transitional validation shim")
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
