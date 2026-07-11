package dashboard_test

import (
	"os/exec"
	"strings"
	"testing"
)

func TestDashboardPackageDoesNotDependOnBroadRuntimeCarriers(t *testing.T) {
	t.Parallel()

	cmd := exec.Command(
		"go",
		"list",
		"-deps",
		"-f",
		"{{if not .Standard}}{{.ImportPath}}{{end}}",
		"github.com/portpowered/infinite-you/pkg/initializer/dashboard",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list dashboard dependencies: %v\n%s", err, output)
	}

	for _, dependency := range strings.Fields(string(output)) {
		for _, forbidden := range []string{
			"github.com/portpowered/infinite-you/pkg/runtimehost",
			"github.com/portpowered/infinite-you/pkg/service",
		} {
			if dependency == forbidden || strings.HasPrefix(dependency, forbidden+"/") {
				t.Fatalf("bounded dashboard package must not depend on broad carrier %s; found %s", forbidden, dependency)
			}
		}
	}
}

func TestRuntimeHostDoesNotDependOnProcessDashboard(t *testing.T) {
	t.Parallel()

	cmd := exec.Command(
		"go",
		"list",
		"-f",
		"{{join .Imports \"\\n\"}}",
		"github.com/portpowered/infinite-you/pkg/runtimehost",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list runtimehost imports: %v\n%s", err, output)
	}
	const processDashboard = "github.com/portpowered/infinite-you/pkg/initializer/dashboard"
	for _, dependency := range strings.Fields(string(output)) {
		if dependency == processDashboard {
			t.Fatalf("runtimehost must not construct or start the process dashboard; found dependency %s", dependency)
		}
	}
}
