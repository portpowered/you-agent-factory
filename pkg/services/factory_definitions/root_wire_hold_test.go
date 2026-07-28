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

	// PostAutoDELDefinitionsWireRetargetFollowUp names the Automations-leased
	// follow-up that must retarget root pkg/wire construction to
	// factory_definitions/wire after AUTO-DEL completes.
	PostAutoDELDefinitionsWireRetargetFollowUp = "post-AUTO-DEL: retarget root pkg/wire Factory Definitions construction from factory_definitions/service to factory_definitions/wire only (factory_definition_service_provider.go, cli_commands.go, profiles.go, session_runtime_providers.go)"
)

var transitionalServiceForbiddenImports = []string{
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/authoredlayout",
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/loading",
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/namedfactories",
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/portableconfig",
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

func TestRootPkgWire_StillImportsTransitionalServiceResidual(t *testing.T) {
	t.Parallel()

	imports := packageImports(t, rootPkgWireImport)
	if !importsContain(imports, transitionalServiceImport) {
		t.Fatalf(
			"%s still imports %s under the Automations-leased root wire hold; follow-up %q",
			rootPkgWireImport,
			transitionalServiceImport,
			PostAutoDELDefinitionsWireRetargetFollowUp,
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
