package http_test

import (
	"os/exec"
	"strings"
	"testing"
)

func TestPackageBoundary_DoesNotImportProvidersInternal(t *testing.T) {
	t.Helper()

	cmd := exec.Command(
		"go",
		"list",
		"-deps",
		"-f",
		"{{if not .Standard}}{{.ImportPath}}{{end}}",
		"github.com/portpowered/infinite-you/pkg/services/providers/transports/http",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list deps: %v\n%s", err, output)
	}

	const forbiddenPrefix = "github.com/portpowered/infinite-you/pkg/services/providers/internal"
	for _, dep := range strings.Fields(string(output)) {
		if dep == forbiddenPrefix || strings.HasPrefix(dep, forbiddenPrefix+"/") {
			t.Fatalf("Providers HTTP adapter must not import %s; found dependency %s", forbiddenPrefix, dep)
		}
	}
}
