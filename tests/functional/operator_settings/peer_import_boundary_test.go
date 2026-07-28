package operator_settings_test

import (
	"os/exec"
	"strings"
	"testing"
)

const (
	modulePrefix                       = "github.com/portpowered/infinite-you/"
	operatorSettingsRoot               = modulePrefix + "pkg/services/operator_settings"
	operatorSettingsWireImport         = operatorSettingsRoot + "/wire"
	functionalOperatorSettingsRoot     = modulePrefix + "tests/functional/operator_settings"
)

var forbiddenFunctionalOperatorSettingsImports = []string{
	operatorSettingsRoot + "/internal",
	operatorSettingsRoot + "/servicewire",
	operatorSettingsRoot + "/identityinventory",
	operatorSettingsRoot + "/testlink",
	operatorSettingsRoot + "/testproviders",
}

// operatorSettingsProductionPeerPackages are composition peers that must reach
// Operator Settings only through published root contracts, wire assembly seams,
// and terminal transport adapters — not owner-private internal or deleted
// transitional Settings packages.
var operatorSettingsProductionPeerPackages = []string{
	"github.com/portpowered/infinite-you/pkg/root",
	"github.com/portpowered/infinite-you/pkg/wire",
	"github.com/portpowered/infinite-you/pkg/services/system_initialization",
	"github.com/portpowered/infinite-you/pkg/services/system_initialization/wire",
	"github.com/portpowered/infinite-you/pkg/services/system_initialization/internal/workflow",
	"github.com/portpowered/infinite-you/pkg/transports/cli",
	"github.com/portpowered/infinite-you/pkg/transports/cli/initsetup",
	"github.com/portpowered/infinite-you/pkg/transports/mapping/globalconfig",
	"github.com/portpowered/infinite-you/pkg/services/workers/cliprovider",
}

// TestFunctionalOperatorSettingsPackageUsesPublicProcessImportsOnly seals
// pss-fun-settings-005: Operator Settings functional proofs construct the process
// only through root.BuildProcess / shared functional support and must not import
// operator_settings/internal or deleted transitional Settings packages.
func TestFunctionalOperatorSettingsPackageUsesPublicProcessImportsOnly(t *testing.T) {
	t.Parallel()

	for _, pkg := range listFunctionalOperatorSettingsPackages(t) {
		pkg := pkg
		t.Run(shortFunctionalOperatorSettingsPackageName(pkg), func(t *testing.T) {
			t.Parallel()
			assertOperatorSettingsImportsForbidden(t, pkg, forbiddenFunctionalOperatorSettingsImports)
		})
	}
}

// TestOperatorSettingsProductionPeersReachSettingsThroughPublishedSurfacesOnly
// proves named production peers compose Operator Settings only through published
// root contracts, wire assembly seams, and terminal transport adapters.
func TestOperatorSettingsProductionPeersReachSettingsThroughPublishedSurfacesOnly(t *testing.T) {
	t.Parallel()

	forbidden := forbiddenFunctionalOperatorSettingsImports
	for _, packagePath := range operatorSettingsProductionPeerPackages {
		packagePath := packagePath
		t.Run(shortFunctionalOperatorSettingsPackageName(packagePath), func(t *testing.T) {
			t.Parallel()
			assertOperatorSettingsImportsForbidden(t, packagePath, forbidden)
			assertProductionPeerOperatorSettingsImportsArePublished(t, packagePath)
		})
	}
}

func listFunctionalOperatorSettingsPackages(t *testing.T) []string {
	t.Helper()

	cmd := exec.Command("go", "list", functionalOperatorSettingsRoot+"/...")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list functional operator settings packages: %v\n%s", err, output)
	}
	return strings.Fields(string(output))
}

func assertOperatorSettingsImportsForbidden(t *testing.T, packagePath string, forbidden []string) {
	t.Helper()

	for _, importPath := range listDirectImports(t, packagePath) {
		for _, blocked := range forbidden {
			if importPath == blocked || strings.HasPrefix(importPath, blocked+"/") {
				t.Fatalf(
					"%s must not import %s; use root.BuildProcess and published Operator Settings contracts",
					packagePath,
					importPath,
				)
			}
		}
	}
}

func assertProductionPeerOperatorSettingsImportsArePublished(t *testing.T, packagePath string) {
	t.Helper()

	for _, importPath := range listDirectImports(t, packagePath) {
		if !strings.HasPrefix(importPath, operatorSettingsRoot) {
			continue
		}
		if importPath == operatorSettingsRoot ||
			importPath == operatorSettingsWireImport ||
			strings.HasPrefix(importPath, operatorSettingsWireImport+"/") ||
			strings.HasPrefix(importPath, operatorSettingsRoot+"/transports/") {
			continue
		}
		t.Fatalf(
			"%s must reach Operator Settings only through %s, %s, or %s/transports/*; found %s",
			packagePath,
			operatorSettingsRoot,
			operatorSettingsWireImport,
			operatorSettingsRoot,
			importPath,
		)
	}
}

func listDirectImports(t *testing.T, packagePath string) []string {
	t.Helper()

	cmd := exec.Command("go", "list", "-f", "{{join .Imports \"\\n\"}}", packagePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list imports for %s: %v\n%s", packagePath, err, output)
	}
	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "\n")
}

func shortFunctionalOperatorSettingsPackageName(packagePath string) string {
	if strings.HasPrefix(packagePath, functionalOperatorSettingsRoot) {
		rest := strings.TrimPrefix(packagePath, functionalOperatorSettingsRoot)
		if rest == "" {
			return "operator_settings"
		}
		return strings.TrimPrefix(rest, "/")
	}
	if strings.HasPrefix(packagePath, modulePrefix) {
		return strings.TrimPrefix(packagePath, modulePrefix)
	}
	return packagePath
}
