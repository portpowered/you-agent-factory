package http_test

import (
	"os/exec"
	"strings"
	"testing"
)

func TestPackageBoundary_DoesNotImportWorkInternals(t *testing.T) {
	t.Helper()

	packagePath := "github.com/portpowered/infinite-you/pkg/services/work/transports/http"
	forbidden := "github.com/portpowered/infinite-you/pkg/services/work/internal"
	assertPackageDirectImportsForbidden(t, packagePath, []string{forbidden})
}

func assertPackageDirectImportsForbidden(t *testing.T, packagePath string, forbiddenRoots []string) {
	t.Helper()

	cmd := exec.Command("go", "list", "-f", "{{.Imports}}", packagePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list imports for %s: %v\n%s", packagePath, err, output)
	}
	imports := strings.Fields(strings.Trim(string(output), "[]"))
	for _, importPath := range imports {
		for _, forbidden := range forbiddenRoots {
			if importPath == forbidden || strings.HasPrefix(importPath, forbidden+"/") {
				t.Fatalf(
					"%s must not import forbidden ownership %s; found direct import %s",
					packagePath,
					forbidden,
					importPath,
				)
			}
		}
	}
}
