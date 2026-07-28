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
	"github.com/portpowered/infinite-you/pkg/services/operator_settings/internal",
}

var operatorSettingsForbiddenImportPathFragments = []string{
	"pkg/services/operator_settings/servicewire",
	"pkg/services/operator_settings/identityinventory",
	"pkg/services/operator_settings/internal/",
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
