package factorysessions_test

import (
	"os/exec"
	"strings"
	"testing"
)

const (
	modulePrefix      = "github.com/portpowered/infinite-you/"
	workRoot          = modulePrefix + "pkg/services/work"
	factorySessionsRoot = modulePrefix + "pkg/services/factory_sessions"
)

// TestProductionPackagesImportWorkRootOnly seals CUT-SES-WORK story 001:
// Factory Sessions production packages may depend on Work only through the
// service root contract, not nested Work implementation or legacy helper paths.
func TestProductionPackagesImportWorkRootOnly(t *testing.T) {
	t.Parallel()

	for _, pkg := range listFactorySessionsPackages(t) {
		pkg := pkg
		t.Run(shortFactorySessionsPackageName(pkg), func(t *testing.T) {
			t.Parallel()
			assertProductionImportsUseWorkRootOnly(t, pkg)
		})
	}
}

func listFactorySessionsPackages(t *testing.T) []string {
	t.Helper()

	cmd := exec.Command("go", "list", factorySessionsRoot+"/...")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list factory_sessions packages: %v\n%s", err, output)
	}
	return strings.Fields(string(output))
}

func assertProductionImportsUseWorkRootOnly(t *testing.T, packagePath string) {
	t.Helper()

	cmd := exec.Command("go", "list", "-f", "{{join .Imports \"\\n\"}}", packagePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list imports for %s: %v\n%s", packagePath, err, output)
	}
	for _, importPath := range strings.Fields(string(output)) {
		if isForbiddenFactorySessionsWorkImport(importPath) {
			t.Fatalf(
				"%s production import %s is forbidden; use %s for Work surfaces",
				packagePath,
				importPath,
				workRoot,
			)
		}
	}
}

func isForbiddenFactorySessionsWorkImport(importPath string) bool {
	if importPath == workRoot {
		return false
	}
	if strings.HasPrefix(importPath, workRoot+"/") {
		return true
	}
	legacyWorkRoots := []string{
		modulePrefix + "pkg/work",
		modulePrefix + "pkg/workcontent",
		modulePrefix + "pkg/workgraph",
		modulePrefix + "pkg/workquery",
	}
	for _, legacyRoot := range legacyWorkRoots {
		if importPath == legacyRoot || strings.HasPrefix(importPath, legacyRoot+"/") {
			return true
		}
	}
	return false
}

func shortFactorySessionsPackageName(packagePath string) string {
	if strings.HasPrefix(packagePath, factorySessionsRoot) {
		rest := strings.TrimPrefix(packagePath, factorySessionsRoot)
		if rest == "" {
			return "factory_sessions"
		}
		return strings.TrimPrefix(rest, "/")
	}
	return packagePath
}
