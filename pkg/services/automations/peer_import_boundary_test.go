package automations_test

import (
	"os/exec"
	"strings"
	"testing"
)

const (
	modulePrefix    = "github.com/portpowered/infinite-you/"
	automationsRoot = modulePrefix + "pkg/services/automations"
)

var productionPeerScanRoots = []string{
	modulePrefix + "pkg/services/...",
	modulePrefix + "pkg/root/...",
	modulePrefix + "pkg/wire",
	modulePrefix + "pkg/wire/...",
}

// allowedPeerAutomationsWireImporters documents the sole application injector
// seam that may reach Automations wire during root.BuildProcess composition.
var allowedPeerAutomationsWireImporters = map[string]struct{}{
	modulePrefix + "pkg/wire": {},
}

// TestProductionPeersImportAutomationsRootOnly seals FUN-automations story 006:
// production composition reaches Automations through the published service root,
// not deleted automations/service or peer-facing automations/internal imports.
func TestProductionPeersImportAutomationsRootOnly(t *testing.T) {
	t.Parallel()

	for _, pkg := range listProductionPeerPackages(t) {
		pkg := pkg
		t.Run(shortAutomationsPeerPackageName(pkg), func(t *testing.T) {
			t.Parallel()
			assertProductionPeerImportsUseAutomationsRootOnly(t, pkg)
		})
	}
}

func listProductionPeerPackages(t *testing.T) []string {
	t.Helper()

	seen := make(map[string]struct{})
	var packages []string
	for _, root := range productionPeerScanRoots {
		cmd := exec.Command("go", "list", root)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("go list %s: %v\n%s", root, err, output)
		}
		for _, pkg := range strings.Fields(string(output)) {
			if pkg == automationsRoot || strings.HasPrefix(pkg, automationsRoot+"/") {
				continue
			}
			if _, ok := seen[pkg]; ok {
				continue
			}
			seen[pkg] = struct{}{}
			packages = append(packages, pkg)
		}
	}
	return packages
}

func assertProductionPeerImportsUseAutomationsRootOnly(t *testing.T, packagePath string) {
	t.Helper()

	cmd := exec.Command("go", "list", "-f", "{{join .Imports \"\\n\"}}", packagePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list imports for %s: %v\n%s", packagePath, err, output)
	}
	for _, importPath := range strings.Fields(string(output)) {
		if isForbiddenProductionPeerAutomationsImport(packagePath, importPath) {
			t.Fatalf(
				"%s production import %s is forbidden; use %s or root.BuildProcess composition",
				packagePath,
				importPath,
				automationsRoot,
			)
		}
	}
}

func isForbiddenProductionPeerAutomationsImport(importerPackage, importPath string) bool {
	if importPath == automationsRoot {
		return false
	}
	if strings.HasPrefix(importPath, automationsRoot+"/internal") ||
		strings.HasPrefix(importPath, automationsRoot+"/service") {
		return true
	}
	if strings.HasPrefix(importPath, automationsRoot+"/wire") {
		_, allowed := allowedPeerAutomationsWireImporters[importerPackage]
		return !allowed
	}
	return false
}

func shortAutomationsPeerPackageName(packagePath string) string {
	prefixes := []string{
		modulePrefix + "pkg/services/",
		modulePrefix + "pkg/root/",
		modulePrefix + "pkg/wire",
	}
	for _, prefix := range prefixes {
		if strings.HasPrefix(packagePath, prefix) {
			rest := strings.TrimPrefix(packagePath, prefix)
			if rest == "" {
				return strings.TrimPrefix(prefix, modulePrefix)
			}
			return rest
		}
	}
	return packagePath
}
