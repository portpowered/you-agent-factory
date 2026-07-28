package http_test

import (
	"os/exec"
	"slices"
	"strings"
	"testing"
)

func TestPackageBoundary_DoesNotImportRecordingsInternals(t *testing.T) {
	t.Helper()

	cmd := exec.Command(
		"go",
		"list",
		"-deps",
		"-f",
		"{{if not .Standard}}{{.ImportPath}}{{end}}",
		"github.com/portpowered/infinite-you/pkg/services/recordings/transports/http",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list deps: %v\n%s", err, output)
	}

	forbidden := "github.com/portpowered/infinite-you/pkg/services/recordings/internal"
	allowedTransitional := []string{
		// Legacy portable-recording aliases in recordings/contracts.go still reach
		// the folded implementation through the transitional artifacts/ shim until
		// CLN-REC-CONTRACT-ROOTS removes that root alias cluster.
		"github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/artifacts_export/artifacts",
		// Workstation projection contracts are folded into internal projection_query
		// while DEL-REC retargets imports off the transitional projections/ shim.
		"github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/projection_query/projections/workstation",
	}
	for _, dep := range strings.Fields(string(output)) {
		if dep == forbidden || strings.HasPrefix(dep, forbidden+"/") {
			if slices.Contains(allowedTransitional, dep) {
				continue
			}
			t.Fatalf("Recordings HTTP adapter must not import %s; found dependency %s", forbidden, dep)
		}
	}
}
