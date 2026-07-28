package compilation_test

import (
	"os/exec"
	"strings"
	"testing"
)

const compilationRuntimeTestsPackage = "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/runtimetests"

var transitionalCompileLoadImports = []string{
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/loading",
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/loadedsource",
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/runtimeconfig",
}

func TestRuntimeTests_DoNotImportTransitionalCompileLoadPackages(t *testing.T) {
	t.Parallel()

	cmd := exec.Command("go", "list", "-f", "{{join .Imports \"\\n\"}}", compilationRuntimeTestsPackage)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list imports for %s: %v\n%s", compilationRuntimeTestsPackage, err, output)
	}
	for _, importPath := range strings.Fields(string(output)) {
		for _, forbidden := range transitionalCompileLoadImports {
			if importPath == forbidden || strings.HasPrefix(importPath, forbidden+"/") {
				t.Fatalf(
					"%s must exercise compilation-owned compile/load behavior; found transitional import %s",
					compilationRuntimeTestsPackage,
					importPath,
				)
			}
		}
	}
}
