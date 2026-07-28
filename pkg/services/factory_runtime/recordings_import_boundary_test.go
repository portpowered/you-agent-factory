package factory_test

import (
	"os/exec"
	"strings"
	"testing"
)

const (
	factoryRuntimeRoot = "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	recordingsRoot     = "github.com/portpowered/infinite-you/pkg/services/recordings"
)

// TestProductionPackagesImportRecordingsRootOnly seals CUT-RUN-REC story 002:
// Factory Runtime production packages may depend on Recordings only through the
// service root contract, not nested Recordings implementation packages or
// transitional public dirs.
func TestProductionPackagesImportRecordingsRootOnly(t *testing.T) {
	t.Parallel()

	for _, pkg := range listFactoryRuntimePackages(t) {
		pkg := pkg
		t.Run(shortFactoryRuntimePackageName(pkg), func(t *testing.T) {
			t.Parallel()
			assertProductionImportsUseRecordingsRootOnly(t, pkg)
		})
	}
}

func listFactoryRuntimePackages(t *testing.T) []string {
	t.Helper()

	cmd := exec.Command("go", "list", factoryRuntimeRoot+"/...")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list factory runtime packages: %v\n%s", err, output)
	}
	return strings.Fields(string(output))
}

func assertProductionImportsUseRecordingsRootOnly(t *testing.T, packagePath string) {
	t.Helper()

	cmd := exec.Command("go", "list", "-f", "{{join .Imports \"\\n\"}}", packagePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list imports for %s: %v\n%s", packagePath, err, output)
	}
	for _, importPath := range strings.Fields(string(output)) {
		if isForbiddenFactoryRuntimeRecordingsImport(importPath) {
			t.Fatalf(
				"%s production import %s is forbidden; use %s for Recordings surfaces",
				packagePath,
				importPath,
				recordingsRoot,
			)
		}
	}
}

func isForbiddenFactoryRuntimeRecordingsImport(importPath string) bool {
	if importPath == recordingsRoot {
		return false
	}
	return strings.HasPrefix(importPath, recordingsRoot+"/")
}

func shortFactoryRuntimePackageName(packagePath string) string {
	if strings.HasPrefix(packagePath, factoryRuntimeRoot) {
		rest := strings.TrimPrefix(packagePath, factoryRuntimeRoot)
		if rest == "" {
			return "factory_runtime"
		}
		return strings.TrimPrefix(rest, "/")
	}
	return packagePath
}
