package http_test

import (
	"os/exec"
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
	for _, dep := range strings.Fields(string(output)) {
		if dep == forbidden || strings.HasPrefix(dep, forbidden+"/") {
			t.Fatalf("Recordings HTTP adapter must not import %s; found dependency %s", forbidden, dep)
		}
	}
}
