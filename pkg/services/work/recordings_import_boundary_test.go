package work_test

import (
	"os/exec"
	"strings"
	"testing"
)

const (
	modulePrefix   = "github.com/portpowered/infinite-you/"
	workRoot       = modulePrefix + "pkg/services/work"
	recordingsRoot = modulePrefix + "pkg/services/recordings"
)

// TestProductionPackagesImportRecordingsRootOnly seals CUT-WORK-REC story 001:
// Work production packages may depend on Recordings only through the service
// root contract, not nested Recordings implementation packages.
func TestProductionPackagesImportRecordingsRootOnly(t *testing.T) {
	t.Parallel()

	for _, pkg := range listWorkPackages(t) {
		pkg := pkg
		t.Run(shortWorkPackageName(pkg), func(t *testing.T) {
			t.Parallel()
			assertProductionImportsUseRecordingsRootOnly(t, pkg)
		})
	}
}

func listWorkPackages(t *testing.T) []string {
	t.Helper()

	cmd := exec.Command("go", "list", workRoot+"/...")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list work packages: %v\n%s", err, output)
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
		if isForbiddenWorkRecordingsImport(importPath) {
			t.Fatalf(
				"%s production import %s is forbidden; use %s for Recordings surfaces",
				packagePath,
				importPath,
				recordingsRoot,
			)
		}
	}
}

func isForbiddenWorkRecordingsImport(importPath string) bool {
	if importPath == recordingsRoot {
		return false
	}
	return strings.HasPrefix(importPath, recordingsRoot+"/")
}

func shortWorkPackageName(packagePath string) string {
	if strings.HasPrefix(packagePath, workRoot) {
		rest := strings.TrimPrefix(packagePath, workRoot)
		if rest == "" {
			return "work"
		}
		return strings.TrimPrefix(rest, "/")
	}
	return packagePath
}
