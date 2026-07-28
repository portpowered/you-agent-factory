package provider_sessions_test

import (
	"os/exec"
	"strings"
	"testing"
)

const (
	modulePrefix                     = "github.com/portpowered/infinite-you/"
	providerSessionsRoot             = modulePrefix + "pkg/services/provider_sessions"
	functionalProviderSessionsRoot   = modulePrefix + "tests/functional/provider_sessions"
)

var forbiddenFunctionalProviderSessionsImports = []string{
	providerSessionsRoot + "/internal",
	providerSessionsRoot + "/service",
}

// TestFunctionalProviderSessionsPackageUsesPublicProcessImportsOnly seals
// pss-fun-provider-sessions-004: Provider Sessions functional proofs construct
// the process only through root.BuildProcess / shared functional support and
// must not import provider_sessions/internal or deleted provider_sessions/service.
func TestFunctionalProviderSessionsPackageUsesPublicProcessImportsOnly(t *testing.T) {
	t.Parallel()

	for _, pkg := range listFunctionalProviderSessionsPackages(t) {
		pkg := pkg
		t.Run(shortFunctionalProviderSessionsPackageName(pkg), func(t *testing.T) {
			t.Parallel()
			assertFunctionalProviderSessionsImportsArePublic(t, pkg)
		})
	}
}

func listFunctionalProviderSessionsPackages(t *testing.T) []string {
	t.Helper()

	cmd := exec.Command("go", "list", functionalProviderSessionsRoot+"/...")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list functional provider sessions packages: %v\n%s", err, output)
	}
	return strings.Fields(string(output))
}

func assertFunctionalProviderSessionsImportsArePublic(t *testing.T, packagePath string) {
	t.Helper()

	cmd := exec.Command("go", "list", "-f", "{{join .Imports \"\\n\"}}", packagePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list imports for %s: %v\n%s", packagePath, err, output)
	}
	for _, importPath := range strings.Fields(string(output)) {
		for _, forbidden := range forbiddenFunctionalProviderSessionsImports {
			if importPath == forbidden || strings.HasPrefix(importPath, forbidden+"/") {
				t.Fatalf(
					"%s must not import %s; use root.BuildProcess and published Provider Sessions contracts",
					packagePath,
					importPath,
				)
			}
		}
	}
}

func shortFunctionalProviderSessionsPackageName(packagePath string) string {
	if strings.HasPrefix(packagePath, functionalProviderSessionsRoot) {
		rest := strings.TrimPrefix(packagePath, functionalProviderSessionsRoot)
		if rest == "" {
			return "provider_sessions"
		}
		return strings.TrimPrefix(rest, "/")
	}
	return packagePath
}
