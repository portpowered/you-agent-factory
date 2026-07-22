package responseevents_test

import (
	"os/exec"
	"strings"
	"testing"
)

func TestPackageBoundary_DoesNotImportForbiddenTransportOrProviderPackages(t *testing.T) {
	t.Parallel()

	cmd := exec.Command(
		"go",
		"list",
		"-deps",
		"-f",
		"{{if not .Standard}}{{.ImportPath}}{{end}}",
		"github.com/portpowered/infinite-you/pkg/services/factory_sessions/responseevents",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list deps: %v\n%s", err, output)
	}

	forbiddenRoots := []string{
		"github.com/portpowered/infinite-you/pkg/transports/cli",
		"net/http",
		"os/exec",
	}
	for _, dep := range strings.Fields(string(output)) {
		if strings.HasPrefix(dep, "github.com/portpowered/infinite-you/pkg/services/workers/") {
			t.Fatalf("Factory Session response events must not import a Workers implementation package; found dependency %s", dep)
		}
		for _, forbidden := range forbiddenRoots {
			if dep == forbidden || strings.HasPrefix(dep, forbidden+"/") {
				t.Fatalf("Factory Session response events must not import %s; found dependency %s", forbidden, dep)
			}
		}
	}
}
