package workflow

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestWorkflowDoesNotDependOnInitializerOrTransports(t *testing.T) {
	t.Parallel()

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read workflow package: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") ||
			strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		source, err := os.ReadFile(entry.Name())
		if err != nil {
			t.Fatalf("read %s: %v", entry.Name(), err)
		}
		for _, forbidden := range []string{
			"pkg/initializer",
			"pkg/wire",
			"pkg/transports/cli",
			"pkg/transports/http",
			"pkg/transports/mcp",
			"pkg/services/edges",
			"pkg/services/operator_settings/servicewire",
			"pkg/services/operator_settings/identityinventory",
			"pkg/services/operator_settings/internal/",
		} {
			if strings.Contains(string(source), forbidden) {
				t.Errorf("%s imports forbidden lifecycle, transport, or composition package %q", entry.Name(), forbidden)
			}
		}
	}
}

func TestWorkflowDoesNotOwnSettingsOrDefinitionsStoreSurfaces(t *testing.T) {
	t.Parallel()

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read workflow package: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") ||
			strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		source, err := os.ReadFile(entry.Name())
		if err != nil {
			t.Fatalf("read %s: %v", entry.Name(), err)
		}
		for _, forbidden := range []string{
			"factory_definitions/packagedinstallation",
			"operatorsettings.EnsureLocalBackendScope(",
			"operatorsettings.FileSystem",
			"operatorsettings.ConfigEncoder",
			"operatorsettings.ConfigDecoder",
			"factorydefinitions.Persistence",
		} {
			if strings.Contains(string(source), forbidden) {
				t.Errorf("%s owns Settings or Definitions store surface %q; command peers through injected roots instead", entry.Name(), forbidden)
			}
		}
	}
}

func TestWorkflowPackageBoundary_DoesNotImportInitializerOrStoreOwnershipPackages(t *testing.T) {
	t.Parallel()

	cmd := exec.Command(
		"go",
		"list",
		"-deps",
		"-f",
		"{{if not .Standard}}{{.ImportPath}}{{end}}",
		"github.com/portpowered/infinite-you/pkg/services/system_initialization/internal/workflow",
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
		"github.com/portpowered/infinite-you/pkg/services/operator_settings/internal",
	}
	for _, dep := range strings.Fields(string(output)) {
		for _, forbidden := range forbiddenRoots {
			if dep == forbidden || strings.HasPrefix(dep, forbidden+"/") {
				t.Fatalf("workflow must not import %s; found dependency %s", forbidden, dep)
			}
		}
	}
}

func TestInitializeProductionUsesSettingsRootConfigPathAndCollaboratorPorts(t *testing.T) {
	t.Parallel()

	source, err := os.ReadFile("workflow.go")
	if err != nil {
		t.Fatalf("read workflow.go: %v", err)
	}
	text := string(source)

	for _, required := range []string{
		"operatorsettings.DefaultConfigPath(",
		"settings.LoadFileConfig(",
		"settings.EnsureLocalBackendScope(",
	} {
		if !strings.Contains(text, required) {
			t.Errorf(
				"workflow.go must route Initialize Settings commands through %q via the injected collaborator",
				required,
			)
		}
	}

	for _, forbidden := range []string{
		".you-agent-factory",
		"operatorsettings.LoadFileConfig(",
		"operatorsettings.EnsureLocalBackendScope(",
	} {
		if strings.Contains(text, forbidden) {
			t.Errorf(
				"workflow.go must not own Settings path formulas or package-level store APIs via %q",
				forbidden,
			)
		}
	}
}

func TestInitializeRollbackProofDoesNotDependOnInitializer(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"workflow_test.go", "workflow_context_test.go"} {
		source, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if strings.Contains(string(source), "pkg/initializer") {
			t.Fatalf("%s imports pkg/initializer; initialize/rollback proof must use injected collaborators only", name)
		}
	}
}
