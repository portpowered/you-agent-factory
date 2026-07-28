package work_test

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"
)

const (
	transitionalWorkServiceImportPath = "github.com/portpowered/infinite-you/pkg/services/work/service"
	workOwnerPrefix                   = "github.com/portpowered/infinite-you/pkg/services/work"
)

// TestProductionPackagesOutsideOwnerDoNotImportTransitionalWorkService seals
// pss-cln-work-fold-service-004: production construction and peer imports must
// use work/wire or the published work root contract, not the DEL-WORK
// transitional service/ compile shim.
func TestProductionPackagesOutsideOwnerDoNotImportTransitionalWorkService(t *testing.T) {
	t.Parallel()

	var violations []string
	for _, packagePath := range listPackagesOutsideWorkOwner(t) {
		for _, dep := range listTransitiveWorkServiceDeps(t, packagePath) {
			if !isForbiddenTransitionalWorkServiceImport(dep) {
				continue
			}
			violations = append(
				violations,
				fmt.Sprintf(
					"%s must not depend on transitional %s; construct through work/wire or depend on the work root only (found %s)",
					packagePath,
					transitionalWorkServiceImportPath,
					dep,
				),
			)
		}
	}
	if len(violations) > 0 {
		t.Fatalf("forbidden transitional work/service imports:\n%s", strings.Join(violations, "\n"))
	}
}

func listPackagesOutsideWorkOwner(t *testing.T) []string {
	t.Helper()

	cmd := exec.Command("go", "list", "./...")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list repository packages: %v\n%s", err, output)
	}

	var packages []string
	for _, packagePath := range strings.Fields(string(output)) {
		if strings.HasPrefix(packagePath, workOwnerPrefix) {
			continue
		}
		if strings.HasSuffix(packagePath, "_test") {
			continue
		}
		packages = append(packages, packagePath)
	}
	return packages
}

func isForbiddenTransitionalWorkServiceImport(importPath string) bool {
	return importPath == transitionalWorkServiceImportPath ||
		strings.HasPrefix(importPath, transitionalWorkServiceImportPath+"/")
}

func listTransitiveWorkServiceDeps(t *testing.T, packagePath string) []string {
	t.Helper()

	cmd := exec.Command(
		"go",
		"list",
		"-deps",
		"-f",
		"{{if not .Standard}}{{.ImportPath}}{{end}}",
		packagePath,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list deps for %s: %v\n%s", packagePath, err, output)
	}
	return strings.Fields(string(output))
}
