package factorydefinitions_test

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"
)

const transitionalDefinitionImport = "github.com/portpowered/infinite-you/pkg/services/factory_definitions/definition"

// TestNonOwnerPackages_DoNotImportTransitionalDefinitionShim seals
// pss-cln-def-fold-composition-003: only the Factory Definitions owner may
// import the transitional public definition/ compile shim; production peers and
// peer integration tests must use factory_definitions root or factory_definitions/wire.
func TestNonOwnerPackages_DoNotImportTransitionalDefinitionShim(t *testing.T) {
	t.Parallel()

	var violations []string
	for _, packagePath := range listPackagesOutsideFactoryDefinitionsOwner(t) {
		for _, dep := range listTransitiveDefinitionShimDeps(t, packagePath) {
			if !isForbiddenTransitionalDefinitionImport(dep) {
				continue
			}
			violations = append(
				violations,
				fmt.Sprintf(
					"%s must not depend on transitional %s; use %s or %s",
					packagePath,
					transitionalDefinitionImport,
					factoryDefinitionsOwnerPrefix,
					factoryDefinitionsOwnerPrefix+"/wire",
				),
			)
		}
	}
	if len(violations) > 0 {
		t.Fatalf("forbidden transitional definition/ imports:\n%s", strings.Join(violations, "\n"))
	}
}

func listPackagesOutsideFactoryDefinitionsOwner(t *testing.T) []string {
	t.Helper()

	cmd := exec.Command("go", "list", "./...")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list repository packages: %v\n%s", err, output)
	}

	var packages []string
	for _, packagePath := range strings.Fields(string(output)) {
		if strings.HasPrefix(packagePath, factoryDefinitionsOwnerPrefix) {
			continue
		}
		if strings.HasSuffix(packagePath, "_test") {
			continue
		}
		packages = append(packages, packagePath)
	}
	return packages
}

func isForbiddenTransitionalDefinitionImport(importPath string) bool {
	return importPath == transitionalDefinitionImport ||
		strings.HasPrefix(importPath, transitionalDefinitionImport+"/")
}

func listTransitiveDefinitionShimDeps(t *testing.T, packagePath string) []string {
	t.Helper()

	cmd := exec.Command(
		"go",
		"list",
		"-test",
		"-deps",
		"-f",
		"{{if not .Standard}}{{.ImportPath}}{{end}}",
		packagePath,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list test deps for %s: %v\n%s", packagePath, err, output)
	}
	return strings.Fields(string(output))
}
