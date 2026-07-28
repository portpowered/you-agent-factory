package factorydefinitions_test

import (
	"os/exec"
	"strings"
	"testing"
)

const (
	retiredContractsMegaBarrelImport = "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"
	internalContractsImport          = "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/contracts"
)

// productionPeerPackages must not import the deleted contracts mega-barrel path.
// Owner-local implementation may use internal/contracts only inside factory_definitions.
var productionPeerPackages = []string{
	"github.com/portpowered/infinite-you/pkg/services/recordings",
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime",
	"github.com/portpowered/infinite-you/pkg/services/workers",
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factoryeventprojection",
}

func TestProductionPeers_DoNotImportDeletedContractsMegaBarrel(t *testing.T) {
	t.Parallel()

	for _, packagePath := range productionPeerPackages {
		packagePath := packagePath
		t.Run(packagePath, func(t *testing.T) {
			t.Parallel()
			assertPackageDoesNotImportContractsMegaBarrel(t, packagePath)
		})
	}
}

func assertPackageDoesNotImportContractsMegaBarrel(t *testing.T, packagePath string) {
	t.Helper()

	cmd := exec.Command("go", "list", "-f", "{{.Imports}}", packagePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list imports for %s: %v\n%s", packagePath, err, output)
	}

	imports := strings.Fields(strings.Trim(string(output), "[]"))
	for _, importPath := range imports {
		if importPath == retiredContractsMegaBarrelImport || strings.HasPrefix(importPath, retiredContractsMegaBarrelImport+"/") {
			t.Fatalf("%s must not import deleted contracts mega-barrel %s", packagePath, importPath)
		}
		if importPath == internalContractsImport || strings.HasPrefix(importPath, internalContractsImport+"/") {
			t.Fatalf("%s must not import owner-internal contracts %s; use pkg/services/factory_definitions root", packagePath, importPath)
		}
	}
}
