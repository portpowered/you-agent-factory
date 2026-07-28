package factory_visualization_test

import (
	"os/exec"
	"strings"
	"testing"
)

const recordingsRoot = modulePrefix + "pkg/services/recordings"

// TestProductionPackagesImportRecordingsRootOnly seals CUT-VIS-REC story 001:
// Factory Visualization production packages may depend on Recordings only through
// the service root contract, not nested Recordings implementation paths.
func TestProductionPackagesImportRecordingsRootOnly(t *testing.T) {
	t.Parallel()

	for _, pkg := range listFactoryVisualizationPackages(t) {
		pkg := pkg
		t.Run(shortFactoryVisualizationPackageName(pkg), func(t *testing.T) {
			t.Parallel()
			assertProductionImportsUseRecordingsRootOnly(t, pkg)
		})
	}
}

func assertProductionImportsUseRecordingsRootOnly(t *testing.T, packagePath string) {
	t.Helper()

	cmd := exec.Command("go", "list", "-f", "{{join .Imports \"\\n\"}}", packagePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list imports for %s: %v\n%s", packagePath, err, output)
	}
	for _, importPath := range strings.Fields(string(output)) {
		if isForbiddenVisualizationRecordingsImport(importPath) {
			t.Fatalf(
				"%s production import %s is forbidden; use %s for Recordings surfaces",
				packagePath,
				importPath,
				recordingsRoot,
			)
		}
	}
}

func isForbiddenVisualizationRecordingsImport(importPath string) bool {
	if importPath == recordingsRoot {
		return false
	}
	return strings.HasPrefix(importPath, recordingsRoot+"/")
}
