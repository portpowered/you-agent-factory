package factory_visualization_test

import (
	"os/exec"
	"strings"
	"testing"
)

// TestProductionPackagesImportFactoryRuntimeRootOnly seals CUT-VIS-RUN story 001:
// Factory Visualization production packages may depend on Factory Runtime only
// through the service root contract, not nested Runtime implementation, Petri,
// or legacy helper paths.
func TestProductionPackagesImportFactoryRuntimeRootOnly(t *testing.T) {
	t.Parallel()

	for _, pkg := range listFactoryVisualizationPackages(t) {
		pkg := pkg
		t.Run(shortFactoryVisualizationPackageName(pkg), func(t *testing.T) {
			t.Parallel()
			assertProductionImportsUseRuntimeRootOnly(t, pkg)
		})
	}
}

func assertProductionImportsUseRuntimeRootOnly(t *testing.T, packagePath string) {
	t.Helper()

	cmd := exec.Command("go", "list", "-f", "{{join .Imports \"\\n\"}}", packagePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list imports for %s: %v\n%s", packagePath, err, output)
	}
	for _, importPath := range strings.Fields(string(output)) {
		if isForbiddenFactoryVisualizationRuntimeImport(importPath) {
			t.Fatalf(
				"%s production import %s is forbidden; use %s for Factory Runtime surfaces",
				packagePath,
				importPath,
				factoryRuntimeRoot,
			)
		}
	}
}

func isForbiddenFactoryVisualizationRuntimeImport(importPath string) bool {
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
	if strings.HasPrefix(importPath, modulePrefix+"pkg/transports/mapping/factory") {
		return true
	}
	return false
}
