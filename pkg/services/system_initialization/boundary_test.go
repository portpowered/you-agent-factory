package systeminitialization_test

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

var operatorSettingsForbiddenImportRoots = []string{
	"github.com/portpowered/infinite-you/pkg/services/operator_settings/servicewire",
	"github.com/portpowered/infinite-you/pkg/services/operator_settings/identityinventory",
	"github.com/portpowered/infinite-you/pkg/services/operator_settings/testlink",
	"github.com/portpowered/infinite-you/pkg/services/operator_settings/testproviders",
	"github.com/portpowered/infinite-you/pkg/services/operator_settings/internal",
}

var operatorSettingsForbiddenImportPathFragments = []string{
	"pkg/services/operator_settings/servicewire",
	"pkg/services/operator_settings/identityinventory",
	"pkg/services/operator_settings/testlink",
	"pkg/services/operator_settings/testproviders",
	"pkg/services/operator_settings/internal/",
}

var factoryDefinitionsForbiddenImportRoots = []string{
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/packagedinstallation",
}

var factoryDefinitionsForbiddenImportPathFragments = []string{
	"factory_definitions/packagedinstallation",
}

var bootstrapProductionPackages = []string{
	"github.com/portpowered/infinite-you/pkg/services/system_initialization",
	"github.com/portpowered/infinite-you/pkg/services/system_initialization/wire",
	"github.com/portpowered/infinite-you/pkg/services/system_initialization/internal/workflow",
}

func TestPackageBoundary_ProductionSourceImportsOperatorSettingsRootOnly(t *testing.T) {
	t.Parallel()

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read system initialization root package: %v", err)
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
		for _, forbidden := range operatorSettingsForbiddenImportPathFragments {
			if strings.Contains(string(source), forbidden) {
				t.Fatalf(
					"%s imports forbidden Operator Settings package %q; depend on pkg/services/operator_settings root only",
					entry.Name(),
					forbidden,
				)
			}
		}
	}
}

func TestPackageBoundary_DoesNotImportOperatorSettingsTransitionalPackages(t *testing.T) {
	t.Parallel()

	assertPackageDepsForbidden(
		t,
		"github.com/portpowered/infinite-you/pkg/services/system_initialization",
		operatorSettingsForbiddenImportRoots,
	)
}

func TestPackageBoundary_ProductionSourceImportsFactoryDefinitionsRootOnly(t *testing.T) {
	t.Parallel()

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read system initialization root package: %v", err)
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
		for _, forbidden := range factoryDefinitionsForbiddenImportPathFragments {
			if strings.Contains(string(source), forbidden) {
				t.Fatalf(
					"%s imports forbidden Factory Definitions package %q; depend on pkg/services/factory_definitions root only",
					entry.Name(),
					forbidden,
				)
			}
		}
	}
}

func TestPackageBoundary_DoesNotImportFactoryDefinitionsTransitionalPackages(t *testing.T) {
	t.Parallel()

	for _, packagePath := range bootstrapProductionPackages {
		assertPackageDepsForbidden(t, packagePath, factoryDefinitionsForbiddenImportRoots)
	}
}

func TestPackageBoundary_PublishedCollaboratorsUseFactoryDefinitionsRootContracts(t *testing.T) {
	t.Parallel()

	source, err := os.ReadFile("service.go")
	if err != nil {
		t.Fatalf("read service.go: %v", err)
	}
	text := string(source)
	for _, required := range []string{
		"type PackagedFactoryInstaller = factorydefinitions.PackagedFactoryInstaller",
		"type PackagedFactoryCatalogOperations = factorydefinitions.PackagedFactoryCatalogOperations",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("service.go must publish Definitions root collaborator aliases via %q", required)
		}
	}
}

func assertPackageDepsForbidden(t *testing.T, packagePath string, forbiddenRoots []string) {
	t.Helper()

	cmd := exec.Command(
		"go",
		"list",
		"-deps",
		"-f",
		"{{if not .Standard}}{{.ImportPath}}{{end}}",
		packagePath,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list deps for %s: %v\n%s", packagePath, err, output)
	}
	for _, dep := range strings.Fields(string(output)) {
		for _, forbidden := range forbiddenRoots {
			if dep == forbidden || strings.HasPrefix(dep, forbidden+"/") {
				t.Fatalf(
					"%s must not import %s; found dependency %s",
					packagePath,
					forbidden,
					dep,
				)
			}
		}
	}
}
