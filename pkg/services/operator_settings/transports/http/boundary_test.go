package http_test

import (
	"os/exec"
	"strings"
	"testing"
)

func TestPackageBoundary_DoesNotImportOperatorSettingsInternal(t *testing.T) {
	t.Helper()

	cmd := exec.Command(
		"go",
		"list",
		"-deps",
		"-f",
		"{{if not .Standard}}{{.ImportPath}}{{end}}",
		"github.com/portpowered/infinite-you/pkg/services/operator_settings/transports/http",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list deps: %v\n%s", err, output)
	}

	forbiddenPrefixes := []string{
		"github.com/portpowered/infinite-you/pkg/services/operator_settings/internal",
		"github.com/portpowered/infinite-you/pkg/services/operator_settings/servicewire",
		"github.com/portpowered/infinite-you/pkg/services/operator_settings/identityinventory",
		"github.com/portpowered/infinite-you/pkg/services/operator_settings/testlink",
		"github.com/portpowered/infinite-you/pkg/services/operator_settings/testproviders",
	}
	for _, dep := range strings.Fields(string(output)) {
		for _, forbidden := range forbiddenPrefixes {
			if dep == forbidden || strings.HasPrefix(dep, forbidden+"/") {
				t.Fatalf(
					"Operator Settings HTTP adapter must not import %s; found dependency %s",
					forbidden,
					dep,
				)
			}
		}
	}
}
