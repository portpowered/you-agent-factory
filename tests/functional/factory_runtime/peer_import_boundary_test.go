package factory_runtime_test

import (
	"os/exec"
	"strings"
	"testing"
)

const (
	modulePrefix                 = "github.com/portpowered/infinite-you/"
	factoryRuntimeRoot           = modulePrefix + "pkg/services/factory_runtime"
	functionalFactoryRuntimeRoot = modulePrefix + "tests/functional/factory_runtime"
)

// factoryRuntimeProductionPeerPackages are composition peers that must reach
// Factory Runtime only through published root contracts and factory_runtime/wire
// assembly seams, not owner-private internal or deleted transitional packages.
var factoryRuntimeProductionPeerPackages = []string{
	"github.com/portpowered/infinite-you/pkg/root",
	"github.com/portpowered/infinite-you/pkg/wire",
	"github.com/portpowered/infinite-you/pkg/services/workers",
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions",
	"github.com/portpowered/infinite-you/pkg/services/factory_visualization",
	"github.com/portpowered/infinite-you/pkg/services/edges",
	"github.com/portpowered/infinite-you/pkg/transports/cli",
	"github.com/portpowered/infinite-you/pkg/transports/http",
	"github.com/portpowered/infinite-you/pkg/transports/mcp/server",
	"github.com/portpowered/infinite-you/pkg/initializer/application",
}

var forbiddenTransitionalFactoryRuntimeTopLevel = []string{
	"service",
	"engine",
	"runtime",
	"scheduler",
	"state",
	"subsystems",
	"testkit",
	"exhaustiontests",
	"build",
	"checkpointstore",
	"checkpointsummary",
	"context",
	"definitionmapping",
	"javascript",
	"metrics",
	"orchestrationowner",
	"orchestratorcontract",
	"replayhooks",
	"runtimecontract",
	"throttle",
	"token",
	"token_transformer",
	"tooling",
}

// TestFunctionalFactoryRuntimePackageUsesPublicProcessImportsOnly seals
// pss-fun-runtime-004: Factory Runtime functional proofs construct the process
// only through root.BuildProcess / shared functional support and must not import
// factory_runtime/internal, factory_runtime/wire, or deleted transitional public
// Runtime packages.
func TestFunctionalFactoryRuntimePackageUsesPublicProcessImportsOnly(t *testing.T) {
	t.Parallel()

	forbidden := forbiddenFunctionalFactoryRuntimeImports()
	for _, pkg := range listFunctionalFactoryRuntimePackages(t) {
		pkg := pkg
		t.Run(shortFunctionalFactoryRuntimePackageName(pkg), func(t *testing.T) {
			t.Parallel()
			assertFactoryRuntimeImportsForbidden(t, pkg, forbidden)
		})
	}
}

// TestProductionPeersReachFactoryRuntimeThroughPublishedSurfacesOnly seals
// pss-fun-runtime-004: named production peers compose Factory Runtime only through
// published root contracts and factory_runtime/wire, not owner-private internal
// or deleted transitional public Runtime packages.
func TestProductionPeersReachFactoryRuntimeThroughPublishedSurfacesOnly(t *testing.T) {
	t.Parallel()

	forbidden := forbiddenProductionPeerFactoryRuntimeImports()
	for _, packagePath := range factoryRuntimeProductionPeerPackages {
		packagePath := packagePath
		t.Run(shortFunctionalFactoryRuntimePackageName(packagePath), func(t *testing.T) {
			t.Parallel()
			assertFactoryRuntimeImportsForbidden(t, packagePath, forbidden)
			assertProductionPeerFactoryRuntimeImportsArePublished(t, packagePath)
		})
	}
}

func forbiddenFunctionalFactoryRuntimeImports() []string {
	forbidden := []string{
		factoryRuntimeRoot + "/internal",
		factoryRuntimeRoot + "/wire",
	}
	return append(forbidden, transitionalFactoryRuntimeImportPrefixes()...)
}

func forbiddenProductionPeerFactoryRuntimeImports() []string {
	forbidden := []string{factoryRuntimeRoot + "/internal"}
	return append(forbidden, transitionalFactoryRuntimeImportPrefixes()...)
}

func transitionalFactoryRuntimeImportPrefixes() []string {
	prefixes := make([]string, 0, len(forbiddenTransitionalFactoryRuntimeTopLevel))
	for _, name := range forbiddenTransitionalFactoryRuntimeTopLevel {
		prefixes = append(prefixes, factoryRuntimeRoot+"/"+name)
	}
	return prefixes
}

func listFunctionalFactoryRuntimePackages(t *testing.T) []string {
	t.Helper()

	cmd := exec.Command("go", "list", functionalFactoryRuntimeRoot+"/...")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list functional factory runtime packages: %v\n%s", err, output)
	}
	return strings.Fields(string(output))
}

func assertFactoryRuntimeImportsForbidden(t *testing.T, packagePath string, forbidden []string) {
	t.Helper()

	for _, importPath := range listDirectImports(t, packagePath) {
		for _, blocked := range forbidden {
			if importPath == blocked || strings.HasPrefix(importPath, blocked+"/") {
				t.Fatalf(
					"%s must not import %s; use root.BuildProcess and published Factory Runtime contracts",
					packagePath,
					importPath,
				)
			}
		}
	}
}

func assertProductionPeerFactoryRuntimeImportsArePublished(t *testing.T, packagePath string) {
	t.Helper()

	const factoryRuntimeWireImport = factoryRuntimeRoot + "/wire"

	for _, importPath := range listDirectImports(t, packagePath) {
		if !strings.HasPrefix(importPath, factoryRuntimeRoot) {
			continue
		}
		if importPath == factoryRuntimeRoot ||
			importPath == factoryRuntimeWireImport ||
			strings.HasPrefix(importPath, factoryRuntimeWireImport+"/") {
			continue
		}
		t.Fatalf(
			"%s must reach Factory Runtime only through %s or %s; found %s",
			packagePath,
			factoryRuntimeRoot,
			factoryRuntimeWireImport,
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

func shortFunctionalFactoryRuntimePackageName(packagePath string) string {
	if strings.HasPrefix(packagePath, functionalFactoryRuntimeRoot) {
		rest := strings.TrimPrefix(packagePath, functionalFactoryRuntimeRoot)
		if rest == "" {
			return "factory_runtime"
		}
		return strings.TrimPrefix(rest, "/")
	}
	if strings.HasPrefix(packagePath, modulePrefix) {
		return strings.TrimPrefix(packagePath, modulePrefix)
	}
	return packagePath
}
