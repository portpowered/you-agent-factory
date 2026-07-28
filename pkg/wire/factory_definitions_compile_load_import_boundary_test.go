package wire

import (
	"os/exec"
	"strings"
	"testing"
)

const (
	compilationPackageRoot = modulePrefix + "pkg/services/factory_definitions/internal/services/compilation"
	factoryDefinitionsWire = modulePrefix + "pkg/services/factory_definitions/wire"
	wireFactoryDefinitions   = modulePrefix + "pkg/wire/factorydefinitions"
)

var transitionalCompileLoadImportRoots = []string{
	modulePrefix + "pkg/services/factory_definitions/loading",
	modulePrefix + "pkg/services/factory_definitions/loadedsource",
	modulePrefix + "pkg/services/factory_definitions/runtimeconfig",
}

var wireCompileLoadPackages = []string{
	rootWirePackage,
	wireFactoryDefinitions,
	factoryDefinitionsWire,
}

// TestWireCompileLoadImports_DoNotUseTransitionalPublicPackages seals root and
// Factory Definitions wire composition on compilation-owned compile/load packages.
func TestWireCompileLoadImports_DoNotUseTransitionalPublicPackages(t *testing.T) {
	t.Parallel()

	for _, packagePath := range wireCompileLoadPackages {
		packagePath := packagePath
		t.Run(packagePath, func(t *testing.T) {
			t.Parallel()
			for _, forbidden := range transitionalCompileLoadImportRoots {
				assertWirePackageDoesNotImport(t, packagePath, forbidden)
			}
		})
	}
}

// TestRootWireDependsOnFactoryDefinitionsWireBridge proves application composition
// reaches Factory Definitions compile/load construction through the owner wire bridge.
func TestRootWireDependsOnFactoryDefinitionsWireBridge(t *testing.T) {
	t.Parallel()

	found := false
	for _, dep := range listWirePackageImports(t, rootWirePackage) {
		if dep == factoryDefinitionsWire || strings.HasPrefix(dep, factoryDefinitionsWire+"/") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf(
			"%s must depend on %s for Factory Definitions compile/load construction",
			rootWirePackage,
			factoryDefinitionsWire,
		)
	}
}

// TestFactoryDefinitionsWireCompileLoadImports_UseCompilationOwnedPackages proves
// owner wire composition binds compile/load ports through compilation-owned packages.
func TestFactoryDefinitionsWireCompileLoadImports_UseCompilationOwnedPackages(t *testing.T) {
	t.Parallel()

	assertWirePackageImportsCompilationOwnedCompileLoad(t, factoryDefinitionsWire)
}

func assertWirePackageImportsCompilationOwnedCompileLoad(t *testing.T, packagePath string) {
	t.Helper()

	imports := listWirePackageImports(t, packagePath)
	for _, required := range []string{
		compilationPackageRoot + "/loading",
		compilationPackageRoot + "/loadedsource",
	} {
		found := false
		for _, importPath := range imports {
			if importPath == required || strings.HasPrefix(importPath, required+"/") {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf(
				"%s must import compilation-owned compile/load package %s; imports = %v",
				packagePath,
				required,
				imports,
			)
		}
	}
}

func assertWirePackageDoesNotImport(t *testing.T, packagePath, forbiddenImport string) {
	t.Helper()

	for _, importPath := range listWirePackageImports(t, packagePath) {
		if importPath == forbiddenImport || strings.HasPrefix(importPath, forbiddenImport+"/") {
			t.Fatalf(
				"%s must not import transitional compile/load package %s; found %s",
				packagePath,
				forbiddenImport,
				importPath,
			)
		}
	}
}

func listWirePackageImports(t *testing.T, packagePath string) []string {
	t.Helper()

	cmd := exec.Command("go", "list", "-f", "{{.Imports}}", packagePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list imports for %s: %v\n%s", packagePath, err, output)
	}
	return strings.Fields(strings.Trim(string(output), "[]"))
}
