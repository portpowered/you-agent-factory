package wire

import (
	"os/exec"
	"strings"
	"testing"
)

const (
	modulePrefix               = "github.com/portpowered/infinite-you/"
	rootWirePackage            = modulePrefix + "pkg/wire"
	transitionalWorkersService = modulePrefix + "pkg/services/workers/service"
	workersWirePackage         = modulePrefix + "pkg/services/workers/wire"
)

// TestRootWireDoesNotImportTransitionalWorkersServiceShim seals root application
// composition on the Workers wire bridge instead of the transitional service shim.
func TestRootWireDoesNotImportTransitionalWorkersServiceShim(t *testing.T) {
	t.Parallel()

	for _, dep := range listNonStandardDeps(t, rootWirePackage) {
		if dep == transitionalWorkersService || strings.HasPrefix(dep, transitionalWorkersService+"/") {
			t.Fatalf(
				"%s must not depend on transitional Workers service shim %s; found dependency %s",
				rootWirePackage,
				transitionalWorkersService,
				dep,
			)
		}
	}
}

// TestRootWireDependsOnWorkersWireBridge proves application composition reaches
// Workers runtime construction through the public wire bridge.
func TestRootWireDependsOnWorkersWireBridge(t *testing.T) {
	t.Parallel()

	found := false
	for _, dep := range listNonStandardDeps(t, rootWirePackage) {
		if dep == workersWirePackage || strings.HasPrefix(dep, workersWirePackage+"/") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("%s must depend on %s for Workers construction", rootWirePackage, workersWirePackage)
	}
}

func listNonStandardDeps(t *testing.T, packagePath string) []string {
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
