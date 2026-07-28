package factorydefinitions_test

import (
	"os/exec"
	"strings"
	"testing"
)

const compilationPackageRoot = "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation"

var ownerProductionCompileLoadPackages = []string{
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/capture",
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout/authoredlayout",
}

var transitionalCompileLoadImportRoots = []string{
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/loading",
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/loadedsource",
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/runtimeconfig",
}

func TestOwnerProductionCompileLoadImports_UseCompilationOwnedPackages(t *testing.T) {
	t.Parallel()

	for _, packagePath := range ownerProductionCompileLoadPackages {
		packagePath := packagePath
		t.Run(packagePath, func(t *testing.T) {
			t.Parallel()
			assertPackageImportsCompilationOwnedCompileLoad(t, packagePath)
			for _, forbidden := range transitionalCompileLoadImportRoots {
				assertPackageDoesNotImport(t, packagePath, forbidden)
			}
		})
	}
}

func assertPackageImportsCompilationOwnedCompileLoad(t *testing.T, packagePath string) {
	t.Helper()

	imports := listPackageImports(t, packagePath)
	for _, required := range []string{
		compilationPackageRoot + "/runtimeconfig",
	} {
		found := false
		for _, importPath := range imports {
			if importPath == required || strings.HasPrefix(importPath, required+"/") {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("%s must import compilation-owned compile/load package %s; imports = %v", packagePath, required, imports)
		}
	}
}

func listPackageImports(t *testing.T, packagePath string) []string {
	t.Helper()

	cmd := exec.Command("go", "list", "-f", "{{.Imports}}", packagePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list imports for %s: %v\n%s", packagePath, err, output)
	}
	return strings.Fields(strings.Trim(string(output), "[]"))
}
