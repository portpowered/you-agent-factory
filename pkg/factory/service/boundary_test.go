package service_test

import (
	"os/exec"
	"strings"
	"testing"
)

func TestPackageBoundary_DoesNotImportRootServiceOrStatefulFactorySessions(t *testing.T) {
	t.Helper()

	cmd := exec.Command(
		"go",
		"list",
		"-deps",
		"-f",
		"{{if not .Standard}}{{.ImportPath}}{{end}}",
		"github.com/portpowered/infinite-you/pkg/factory/service",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list deps: %v\n%s", err, output)
	}

	forbiddenRoots := []string{
		"github.com/portpowered/infinite-you/pkg/service",
		"github.com/portpowered/infinite-you/pkg/factory/sessions",
	}
	// Provider adapters share the pure canonical response-event vocabulary, and
	// transport mapping shares the pure reconnect-cursor contract. Session
	// registries, concrete stores, projections, and runtime state remain forbidden.
	allowedFactorySessionLeaves := map[string]struct{}{
		"github.com/portpowered/infinite-you/pkg/factory/sessions/cursors":        {},
		"github.com/portpowered/infinite-you/pkg/factory/sessions/responseevents": {},
	}
	for _, dep := range strings.Fields(string(output)) {
		if _, allowed := allowedFactorySessionLeaves[dep]; allowed {
			continue
		}
		for _, forbidden := range forbiddenRoots {
			if dep == forbidden || strings.HasPrefix(dep, forbidden+"/") {
				t.Fatalf("pkg/factory/service must not import %s; found dependency %s", forbidden, dep)
			}
		}
	}
}
