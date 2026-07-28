package mcp_test

import (
	"os/exec"
	"strings"
	"testing"
)

func TestPackageBoundary_DoesNotImportFactoryRuntimeInternal(t *testing.T) {
	t.Parallel()

	forbidden := "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal"
	for _, packagePath := range []string{
		"github.com/portpowered/infinite-you/pkg/services/factory_runtime/transports/mcp",
	} {
		packagePath := packagePath
		t.Run(packagePath, func(t *testing.T) {
			t.Parallel()
			assertPackageDirectImportsForbidden(t, packagePath, []string{forbidden})
		})
	}
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
				t.Fatalf("%s must not import forbidden ownership %s; found direct import %s", packagePath, forbidden, importPath)
			}
		}
	}
}
