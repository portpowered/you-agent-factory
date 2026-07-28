package compilation_test

import (
	"os/exec"
	"strings"
	"testing"
)

const compilationOwnerRoot = "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation"

var compilationCodecAllowedTransportImportPackages = []string{
	compilationOwnerRoot + "/canonical",
}

func TestCodecBoundary_ProductionCompilationPackagesDoNotImportTransportMappingDirectly(t *testing.T) {
	t.Parallel()

	packages := []string{
		compilationOwnerRoot,
		compilationOwnerRoot + "/wire",
		compilationOwnerRoot + "/internal/service",
		compilationOwnerRoot + "/loading",
		compilationOwnerRoot + "/loadedsource",
		compilationOwnerRoot + "/runtimeconfig",
	}
	for _, packagePath := range packages {
		assertPackageDoesNotImportTransportMappingUnlessAllowed(t, packagePath)
	}
}

func assertPackageDoesNotImportTransportMappingUnlessAllowed(t *testing.T, packagePath string) {
	t.Helper()

	for _, allowed := range compilationCodecAllowedTransportImportPackages {
		if packagePath == allowed {
			return
		}
	}

	cmd := exec.Command("go", "list", "-f", "{{.Imports}}", packagePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list imports for %s: %v\n%s", packagePath, err, output)
	}
	imports := strings.Fields(strings.Trim(string(output), "[]"))
	for _, importPath := range imports {
		if importPath == "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig" ||
			strings.HasPrefix(importPath, "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig/") {
			t.Fatalf("%s must not import transport-mapping factoryconfig directly; bind codecs through compilation/canonical", packagePath)
		}
	}
}
