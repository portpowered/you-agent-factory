package catalog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestPackagedFactoryDefinitionsValidateThroughPublicCLI proves the two new
// authored definitions pass the same customer-facing validation command used
// for ordinary Factory configuration files.
func TestPackagedFactoryDefinitionsValidateThroughPublicCLI(t *testing.T) {
	for _, name := range []string{"@you/fix", "@you/ralph"} {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			factoryDir := support.InstallPackagedFactory(t, home, name)
			inputs := support.FakeInputs(t.Context(), []string{"you", "factory", "config", "validate", factoryDir})
			inputs.Input.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
			inputs.Input.WorkingDirectory = filepath.Dir(factoryDir)
			if err := support.BuildProcess(t, serviceedges.Edges{}).Execute(inputs.Input); err != nil {
				t.Fatalf("Process.Execute(factory config validate %s) error = %v\nstdout=%s\nstderr=%s", name, err, inputs.Stdout(), inputs.Stderr())
			}
			if !strings.Contains(inputs.Stdout(), "Factory validation passed") {
				t.Fatalf("factory config validate %s output = %q", name, inputs.Stdout())
			}
		})
	}
}
