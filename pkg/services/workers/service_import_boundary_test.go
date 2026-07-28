package workers_test

import (
	"os/exec"
	"strings"
	"testing"
)

const (
	modulePrefix               = "github.com/portpowered/infinite-you/"
	workersOwnerPrefix         = modulePrefix + "pkg/services/workers"
	transitionalWorkersService = modulePrefix + "pkg/services/workers/service"
	workersInternalPrefix      = workersOwnerPrefix + "/internal"
)

// TestProductionPackagesOutsideWorkersOwnerDoNotImportTransitionalServiceShim
// seals the folded Workers construction boundary: only owner-local packages may
// depend on the transitional workers/service compile shim until DEL-WRK.
func TestProductionPackagesOutsideWorkersOwnerDoNotImportTransitionalServiceShim(t *testing.T) {
	t.Parallel()

	for _, packagePath := range listModulePackages(t) {
		if strings.HasPrefix(packagePath, workersOwnerPrefix) {
			continue
		}
		for _, importPath := range listDirectImports(t, packagePath) {
			if importPath == transitionalWorkersService ||
				strings.HasPrefix(importPath, transitionalWorkersService+"/") {
				t.Fatalf(
					"%s must not import transitional Workers service shim %s; construct through %spkg/services/workers/wire",
					packagePath,
					importPath,
					modulePrefix,
				)
			}
			if importPath == workersInternalPrefix ||
				strings.HasPrefix(importPath, workersInternalPrefix+"/") {
				t.Fatalf(
					"%s must not import moved Workers internal helper %s; construct through %spkg/services/workers/wire or published Workers root contracts",
					packagePath,
					importPath,
					modulePrefix,
				)
			}
		}
	}
}

func listModulePackages(t *testing.T) []string {
	t.Helper()

	cmd := exec.Command("go", "list", modulePrefix+"...")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list module packages: %v\n%s", err, output)
	}
	return strings.Fields(string(output))
}

func listDirectImports(t *testing.T, packagePath string) []string {
	t.Helper()

	cmd := exec.Command("go", "list", "-f", "{{join .Imports \"\\n\"}}", packagePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list imports for %s: %v\n%s", packagePath, err, output)
	}
	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "\n")
}
