package providersessions_test

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"
)

const (
	transitionalServiceImportPath = "github.com/portpowered/infinite-you/pkg/services/provider_sessions/service"
	providerSessionsOwnerPrefix   = "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
)

// TestProductionPackagesOutsideOwnerDoNotImportTransitionalService seals
// pss-cln-pses-fold-service-004: production construction and peer imports must
// use provider_sessions/wire or the published provider_sessions root contract,
// not the DEL-PSES transitional service/ compile shim.
func TestProductionPackagesOutsideOwnerDoNotImportTransitionalService(t *testing.T) {
	t.Parallel()

	var violations []string
	for _, packagePath := range listPackagesOutsideProviderSessionsOwner(t) {
		for _, dep := range listTransitiveDeps(t, packagePath) {
			if !isForbiddenTransitionalServiceImport(dep) {
				continue
			}
			violations = append(
				violations,
				fmt.Sprintf(
					"%s must not depend on transitional %s; construct through provider_sessions/wire or depend on the provider_sessions root only (found %s)",
					packagePath,
					transitionalServiceImportPath,
					dep,
				),
			)
		}
	}
	if len(violations) > 0 {
		t.Fatalf("forbidden transitional service imports:\n%s", strings.Join(violations, "\n"))
	}
}

func listPackagesOutsideProviderSessionsOwner(t *testing.T) []string {
	t.Helper()

	cmd := exec.Command("go", "list", "./...")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list repository packages: %v\n%s", err, output)
	}

	var packages []string
	for _, packagePath := range strings.Fields(string(output)) {
		if strings.HasPrefix(packagePath, providerSessionsOwnerPrefix) {
			continue
		}
		if strings.HasSuffix(packagePath, "_test") {
			continue
		}
		packages = append(packages, packagePath)
	}
	return packages
}

func isForbiddenTransitionalServiceImport(importPath string) bool {
	return importPath == transitionalServiceImportPath ||
		strings.HasPrefix(importPath, transitionalServiceImportPath+"/")
}
