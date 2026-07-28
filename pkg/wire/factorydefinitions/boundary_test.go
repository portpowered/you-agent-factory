package factorydefinitions_test

import (
	"os/exec"
	"strings"
	"testing"
)

const wireFactoryDefinitionsPackage = "github.com/portpowered/infinite-you/pkg/wire/factorydefinitions"

var transitionalSnapshotPackages = []string{
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/portableconfig",
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/snapshotcapture",
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/editable",
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/replayconfig",
}

func TestWireFactoryDefinitions_DoesNotImportTransitionalSnapshotShims(t *testing.T) {
	t.Parallel()

	cmd := exec.Command("go", "list", "-f", "{{.Imports}}", wireFactoryDefinitionsPackage)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list imports for %s: %v\n%s", wireFactoryDefinitionsPackage, err, output)
	}

	imports := strings.Fields(strings.Trim(string(output), "[]"))
	for _, importPath := range imports {
		for _, forbidden := range transitionalSnapshotPackages {
			if importPath == forbidden || strings.HasPrefix(importPath, forbidden+"/") {
				t.Fatalf(
					"%s must construct from snapshots_portability internals, not transitional shim %s",
					wireFactoryDefinitionsPackage,
					importPath,
				)
			}
		}
	}
}
