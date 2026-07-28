package wire

import (
	"os/exec"
	"strings"
	"testing"
)

const (
	modulePrefix              = "github.com/portpowered/infinite-you/"
	workersWirePackage        = modulePrefix + "pkg/services/workers/wire"
	transitionalWorkersService = modulePrefix + "pkg/services/workers/service"
)

// TestWireConstructionDoesNotDependOnTransitionalServiceShim seals the folded
// Workers construction bridge: wire must compose from owner-internal paths only.
func TestWireConstructionDoesNotDependOnTransitionalServiceShim(t *testing.T) {
	t.Parallel()

	for _, dep := range listNonStandardDeps(t, workersWirePackage) {
		if dep == transitionalWorkersService || strings.HasPrefix(dep, transitionalWorkersService+"/") {
			t.Fatalf(
				"%s must not depend on transitional Workers service shim %s; found dependency %s",
				workersWirePackage,
				transitionalWorkersService,
				dep,
			)
		}
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
