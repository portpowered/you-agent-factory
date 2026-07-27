package activationlifecycle_test

import (
	"os/exec"
	"strings"
	"testing"
)

// Compile-time seal: activation_lifecycle remains a parent-private owner. Peers
// consume Factory Visualization Root lifecycle vocabulary instead of a second
// peer-facing nested activation authority.
func TestActivationLifecyclePackagesDoNotImportSiblingVisualizationOwners(t *testing.T) {
	t.Parallel()

	packages := []string{
		"github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/activation_lifecycle",
		"github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/activation_lifecycle/internal/service",
		"github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/activation_lifecycle/wire",
	}
	forbidden := []string{
		"github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/live_view_projection",
		"github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/response_event_presentation",
	}

	for _, pkg := range packages {
		pkg := pkg
		t.Run(pkg, func(t *testing.T) {
			t.Parallel()

			cmd := exec.Command(
				"go",
				"list",
				"-deps",
				"-f",
				"{{if not .Standard}}{{.ImportPath}}{{end}}",
				pkg,
			)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("go list deps for %s: %v\n%s", pkg, err, output)
			}

			for _, dep := range strings.Fields(string(output)) {
				for _, blocked := range forbidden {
					if dep == blocked || strings.HasPrefix(dep, blocked+"/") {
						t.Fatalf(
							"%s must not absorb sibling Visualization owner %s; found dependency %s",
							pkg,
							blocked,
							dep,
						)
					}
				}
			}
		})
	}
}
