package systeminitializationwire_test

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestWireDoesNotDependOnInitializerOrProcessLifecycle(t *testing.T) {
	t.Parallel()

	source, err := os.ReadFile("wire.go")
	if err != nil {
		t.Fatalf("read wire.go: %v", err)
	}
	for _, forbidden := range []string{
		"pkg/initializer",
		"pkg/wire",
		"pkg/transports/cli",
		"factory_definitions/packagedinstallation",
	} {
		if strings.Contains(string(source), forbidden) {
			t.Fatalf("wire.go imports forbidden lifecycle or store-ownership package %q", forbidden)
		}
	}
}

func TestWirePackageBoundary_DoesNotImportInitializerLifecyclePackages(t *testing.T) {
	t.Parallel()

	cmd := exec.Command(
		"go",
		"list",
		"-deps",
		"-f",
		"{{if not .Standard}}{{.ImportPath}}{{end}}",
		"github.com/portpowered/infinite-you/pkg/services/system_initialization/wire",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list deps: %v\n%s", err, output)
	}

	forbiddenRoots := []string{
		"github.com/portpowered/infinite-you/pkg/initializer",
		"github.com/portpowered/infinite-you/pkg/wire",
		"github.com/portpowered/infinite-you/pkg/transports",
		"github.com/portpowered/infinite-you/pkg/services/factory_definitions/packagedinstallation",
		"github.com/portpowered/infinite-you/pkg/services/operator_settings/servicewire",
		"github.com/portpowered/infinite-you/pkg/services/operator_settings/identityinventory",
		"github.com/portpowered/infinite-you/pkg/services/operator_settings/testlink",
		"github.com/portpowered/infinite-you/pkg/services/operator_settings/testproviders",
		"github.com/portpowered/infinite-you/pkg/services/operator_settings/internal",
	}
	for _, dep := range strings.Fields(string(output)) {
		for _, forbidden := range forbiddenRoots {
			if dep == forbidden || strings.HasPrefix(dep, forbidden+"/") {
				t.Fatalf("system initialization wire must not import %s; found dependency %s", forbidden, dep)
			}
		}
	}
}
