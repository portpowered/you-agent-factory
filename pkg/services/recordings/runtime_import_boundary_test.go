package recordings_test

import (
	"os/exec"
	"strings"
	"testing"
)

const (
	modulePrefix       = "github.com/portpowered/infinite-you/"
	factoryRuntimeRoot = modulePrefix + "pkg/services/factory_runtime"
	recordingsRoot     = modulePrefix + "pkg/services/recordings"
)

// TestProductionPackagesImportFactoryRuntimeRootOnly seals CUT-REC-RUN story 001:
// Recordings production packages may depend on Factory Runtime only through the
// service root contract, not nested Runtime implementation or legacy helper paths.
func TestProductionPackagesImportFactoryRuntimeRootOnly(t *testing.T) {
	t.Parallel()

	for _, pkg := range listRecordingsPackages(t) {
		pkg := pkg
		t.Run(shortRecordingsPackageName(pkg), func(t *testing.T) {
			t.Parallel()
			assertProductionImportsUseRuntimeRootOnly(t, pkg)
		})
	}
}

func listRecordingsPackages(t *testing.T) []string {
	t.Helper()

	cmd := exec.Command("go", "list", recordingsRoot+"/...")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list recordings packages: %v\n%s", err, output)
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
		if isForbiddenRecordingsRuntimeImport(importPath) {
			t.Fatalf(
				"%s production import %s is forbidden; use %s for Factory Runtime surfaces",
				packagePath,
				importPath,
				factoryRuntimeRoot,
			)
		}
	}
}

func isForbiddenRecordingsRuntimeImport(importPath string) bool {
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
	if strings.HasPrefix(importPath, modulePrefix+"pkg/transports/mapping/factory") {
		return true
	}
	return false
}

func shortRecordingsPackageName(packagePath string) string {
	if strings.HasPrefix(packagePath, recordingsRoot) {
		rest := strings.TrimPrefix(packagePath, recordingsRoot)
		if rest == "" {
			return "recordings"
		}
		return strings.TrimPrefix(rest, "/")
	}
	return packagePath
}
