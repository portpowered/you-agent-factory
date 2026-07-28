package factorydefinitions_test

import (
	"os/exec"
	"strings"
	"testing"
)

const (
	modulePrefix          = "github.com/portpowered/infinite-you/"
	factoryRuntimeRoot    = modulePrefix + "pkg/services/factory_runtime"
	factoryDefinitionsRoot = modulePrefix + "pkg/services/factory_definitions"
)

// TestProductionPackagesImportFactoryRuntimeRootOnly seals CUT-DEF-RUN story 001:
// Factory Definitions production packages may depend on Factory Runtime only
// through the service root contract, not nested Runtime implementation, Petri,
// or legacy helper paths.
func TestProductionPackagesImportFactoryRuntimeRootOnly(t *testing.T) {
	t.Parallel()

	for _, pkg := range listFactoryDefinitionsPackages(t) {
		pkg := pkg
		t.Run(shortFactoryDefinitionsPackageName(pkg), func(t *testing.T) {
			t.Parallel()
			assertProductionImportsUseRuntimeRootOnly(t, pkg)
		})
	}
}

func listFactoryDefinitionsPackages(t *testing.T) []string {
	t.Helper()

	cmd := exec.Command("go", "list", factoryDefinitionsRoot+"/...")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list factory definitions packages: %v\n%s", err, output)
	}
	return strings.Fields(string(output))
}

func assertProductionImportsUseRuntimeRootOnly(t *testing.T, packagePath string) {
	t.Helper()

	cmd := exec.Command("go", "list", "-f", "{{join .Imports \"\\n\"}}", packagePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list imports for %s: %v\n%s", packagePath, err, output)
	}
	for _, importPath := range strings.Fields(string(output)) {
		if isForbiddenFactoryDefinitionsRuntimeImport(importPath) {
			t.Fatalf(
				"%s production import %s is forbidden; use %s or a Definitions-owned injected port backed by Runtime root capability",
				packagePath,
				importPath,
				factoryRuntimeRoot,
			)
		}
	}
}

func isForbiddenFactoryDefinitionsRuntimeImport(importPath string) bool {
	if importPath == factoryRuntimeRoot {
		return false
	}
	if strings.HasPrefix(importPath, factoryRuntimeRoot+"/") {
		return true
	}
	if importPath == modulePrefix+"pkg/factory" ||
		strings.HasPrefix(importPath, modulePrefix+"pkg/factory/") {
		return true
	}
	if strings.HasPrefix(importPath, modulePrefix+"pkg/services/factory_runtime/internal/orchestrators/petri") {
		return true
	}
	return false
}

func shortFactoryDefinitionsPackageName(packagePath string) string {
	if strings.HasPrefix(packagePath, factoryDefinitionsRoot) {
		rest := strings.TrimPrefix(packagePath, factoryDefinitionsRoot)
		if rest == "" {
			return "factory_definitions"
		}
		return strings.TrimPrefix(rest, "/")
	}
	return packagePath
}
