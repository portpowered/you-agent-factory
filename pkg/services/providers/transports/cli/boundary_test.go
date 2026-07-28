package cli_test

import (
	"os/exec"
	"strings"
	"testing"
)

const providersCLIImportPath = "github.com/portpowered/infinite-you/pkg/services/providers/transports/cli"

func TestPackageBoundary_DoesNotImportProvidersInternal(t *testing.T) {
	t.Parallel()

	const forbiddenPrefix = "github.com/portpowered/infinite-you/pkg/services/providers/internal"

	cmd := exec.Command(
		"go",
		"list",
		"-deps",
		"-f",
		"{{if not .Standard}}{{.ImportPath}}{{end}}",
		providersCLIImportPath,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list deps: %v\n%s", err, output)
	}

	for _, dep := range strings.Fields(string(output)) {
		if dep == forbiddenPrefix || strings.HasPrefix(dep, forbiddenPrefix+"/") {
			t.Fatalf(
				"Providers CLI adapter must not import %s; found dependency %s",
				forbiddenPrefix,
				dep,
			)
		}
	}
}
