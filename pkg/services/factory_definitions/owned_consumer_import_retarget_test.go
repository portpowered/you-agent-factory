package factorydefinitions_test

import (
	"os/exec"
	"strings"
	"testing"
)

// Owner-local packages that previously depended on the contracts mega-barrel for
// Definition-owned vocabulary must import the service root instead.
var ownedConsumerPackages = []string{
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/definition",
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/impl",
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/editable",
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/transports/http",
}

const contractsBarrelImport = "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"
const ownerInternalContractsImport = "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/contracts"

func TestOwnedConsumers_DoNotImportContractsMegaBarrel(t *testing.T) {
	t.Parallel()

	for _, packagePath := range ownedConsumerPackages {
		packagePath := packagePath
		t.Run(packagePath, func(t *testing.T) {
			t.Parallel()
			assertPackageDoesNotImport(t, packagePath, contractsBarrelImport)
			assertPackageDoesNotImport(t, packagePath, ownerInternalContractsImport)
		})
	}
}

func assertPackageDoesNotImport(t *testing.T, packagePath, forbiddenImport string) {
	t.Helper()

	cmd := exec.Command("go", "list", "-f", "{{.Imports}}", packagePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list imports for %s: %v\n%s", packagePath, err, output)
	}

	imports := strings.Fields(strings.Trim(string(output), "[]"))
	for _, importPath := range imports {
		if importPath == forbiddenImport || strings.HasPrefix(importPath, forbiddenImport+"/") {
			t.Fatalf("%s must not import contracts mega-barrel %s; found %s", packagePath, forbiddenImport, importPath)
		}
	}
}
