package automations_test

import (
	"os/exec"
	"strings"
	"testing"
)

const (
	modulePrefix              = "github.com/portpowered/infinite-you/"
	automationsRoot           = modulePrefix + "pkg/services/automations"
	functionalAutomationsRoot = modulePrefix + "tests/functional/automations"
)

var forbiddenFunctionalAutomationsImports = []string{
	automationsRoot + "/internal",
	automationsRoot + "/wire",
	automationsRoot + "/service",
}

// TestFunctionalAutomationsPackageUsesPublicProcessImportsOnly seals
// FUN-automations story 006: Automations functional proofs construct the process
// only through root.BuildProcess / shared functional support and must not import
// automations/internal, automations/wire, or deleted automations/service.
func TestFunctionalAutomationsPackageUsesPublicProcessImportsOnly(t *testing.T) {
	t.Parallel()

	for _, pkg := range listFunctionalAutomationsPackages(t) {
		pkg := pkg
		t.Run(shortFunctionalAutomationsPackageName(pkg), func(t *testing.T) {
			t.Parallel()
			assertFunctionalAutomationsImportsArePublic(t, pkg)
		})
	}
}

func listFunctionalAutomationsPackages(t *testing.T) []string {
	t.Helper()

	cmd := exec.Command("go", "list", functionalAutomationsRoot+"/...")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list functional automations packages: %v\n%s", err, output)
	}
	return strings.Fields(string(output))
}

func assertFunctionalAutomationsImportsArePublic(t *testing.T, packagePath string) {
	t.Helper()

	cmd := exec.Command("go", "list", "-f", "{{join .Imports \"\\n\"}}", packagePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list imports for %s: %v\n%s", packagePath, err, output)
	}
	for _, importPath := range strings.Fields(string(output)) {
		for _, forbidden := range forbiddenFunctionalAutomationsImports {
			if importPath == forbidden || strings.HasPrefix(importPath, forbidden+"/") {
				t.Fatalf(
					"%s must not import %s; use root.BuildProcess and published Automations contracts",
					packagePath,
					importPath,
				)
			}
		}
	}
}

func shortFunctionalAutomationsPackageName(packagePath string) string {
	if strings.HasPrefix(packagePath, functionalAutomationsRoot) {
		rest := strings.TrimPrefix(packagePath, functionalAutomationsRoot)
		if rest == "" {
			return "automations"
		}
		return strings.TrimPrefix(rest, "/")
	}
	return packagePath
}
