package factorydefinitions_test

import (
	"os/exec"
	"strings"
	"testing"
)

const (
	transitionalServiceImport = "github.com/portpowered/infinite-you/pkg/services/factory_definitions/service"
	factoryDefinitionsInternalImport = "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal"
	factoryDefinitionsPeerImport       = "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	rootPkgWireImport                  = "github.com/portpowered/infinite-you/pkg/wire"
	factoryDefinitionsWireImport       = "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"
)

var transitionalServiceForbiddenImports = []string{
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout/authoredlayout",
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/loading",
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog/namedfactories",
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/portableconfig",
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire",
}

func TestTransitionalServiceShim_ForwardsFromInternalOnly(t *testing.T) {
	t.Parallel()

	imports := packageImports(t, transitionalServiceImport)

	hasInternal := false
	hasPeer := false
	for _, importPath := range imports {
		switch {
		case importPath == factoryDefinitionsInternalImport ||
			strings.HasPrefix(importPath, factoryDefinitionsInternalImport+"/"):
			hasInternal = true
		case importPath == factoryDefinitionsPeerImport:
			hasPeer = true
		default:
			for _, forbidden := range transitionalServiceForbiddenImports {
				if importPath == forbidden || strings.HasPrefix(importPath, forbidden+"/") {
					t.Fatalf(
						"%s must remain a transitional compile shim over internal; found forbidden import %s",
						transitionalServiceImport,
						importPath,
					)
				}
			}
		}
	}

	if !hasInternal {
		t.Fatalf("%s must forward to %s", transitionalServiceImport, factoryDefinitionsInternalImport)
	}
	if !hasPeer {
		t.Fatalf("%s must keep peer contracts on %s", transitionalServiceImport, factoryDefinitionsPeerImport)
	}
}

func TestRootPkgWire_DoesNotImportTransitionalServiceResidual(t *testing.T) {
	t.Parallel()

	imports := packageImports(t, rootPkgWireImport)
	if importsContain(imports, transitionalServiceImport) {
		t.Fatalf(
			"%s must construct Factory Definitions through %s, not transitional service shim %s",
			rootPkgWireImport,
			factoryDefinitionsWireImport,
			transitionalServiceImport,
		)
	}
}

func packageImports(t *testing.T, packagePath string) []string {
	t.Helper()

	cmd := exec.Command("go", "list", "-f", "{{.Imports}}", packagePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list imports for %s: %v\n%s", packagePath, err, output)
	}

	return strings.Fields(strings.Trim(string(output), "[]"))
}

func importsContain(imports []string, target string) bool {
	for _, importPath := range imports {
		if importPath == target || strings.HasPrefix(importPath, target+"/") {
			return true
		}
	}
	return false
}
