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
	}
	for _, dep := range strings.Fields(string(output)) {
		for _, forbidden := range forbiddenRoots {
			if dep == forbidden || strings.HasPrefix(dep, forbidden+"/") {
				t.Fatalf("workflow must not import %s; found dependency %s", forbidden, dep)
			}
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
