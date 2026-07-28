package factorysessions_test

import (
	"os/exec"
	"strings"
	"testing"
)

const factoryRuntimeImportRoot = "github.com/portpowered/infinite-you/pkg/services/factory_runtime"

// TestProductionPackagesImportFactoryRuntimeOnlyThroughRoot seals the CUT-SES-RUN
// consumer edge: Factory Sessions production packages must depend on Factory Runtime
// only through the published service root, not nested implementation packages.
func TestProductionPackagesImportFactoryRuntimeOnlyThroughRoot(t *testing.T) {
	t.Parallel()

	cmd := exec.Command(
		"go",
		"list",
		"-f",
		"{{.ImportPath}} {{join .Imports \" \"}}",
		"github.com/portpowered/infinite-you/pkg/services/factory_sessions/...",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list factory_sessions packages: %v\n%s", err, output)
	}

	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 1 {
			continue
		}
		pkgPath := fields[0]
		for _, imp := range fields[1:] {
			if imp == factoryRuntimeImportRoot {
				continue
			}
			if strings.HasPrefix(imp, factoryRuntimeImportRoot+"/") {
				t.Fatalf(
					"%s must import Factory Runtime only through %s; found direct import %s",
					pkgPath,
					factoryRuntimeImportRoot,
					imp,
				)
			}
		}
	}
}
