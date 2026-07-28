package work_test

import (
	"os/exec"
	"strings"
	"testing"
)

const (
	modulePrefix           = "github.com/portpowered/infinite-you/"
	workServiceRoot        = modulePrefix + "pkg/services/work"
	functionalWorkRoot     = modulePrefix + "tests/functional/work"
)

var forbiddenFunctionalWorkImports = []string{
	workServiceRoot + "/internal",
	workServiceRoot + "/service",
	workServiceRoot + "/stateaccessrecordings",
	modulePrefix + "pkg/work",
	modulePrefix + "pkg/workcontent",
	modulePrefix + "pkg/workgraph",
	modulePrefix + "pkg/workquery",
}

var forbiddenWorkOwnerPrivateImportPrefixes = []string{
	workServiceRoot + "/internal",
	workServiceRoot + "/wire",
	workServiceRoot + "/service",
	workServiceRoot + "/stateaccessrecordings",
	workServiceRoot + "/testdata",
}

var retiredWorkConsumerImportRoots = []string{
	modulePrefix + "pkg/work",
	modulePrefix + "pkg/workcontent",
	modulePrefix + "pkg/workgraph",
	modulePrefix + "pkg/workquery",
}

// workProductionPeerPackages are composition peers that must reach Work only
// through published root contracts and terminal adapters, not owner-private
// implementation, wire, or deleted transitional packages.
var workProductionPeerPackages = []string{
	modulePrefix + "pkg/root",
	modulePrefix + "pkg/services/workers",
	modulePrefix + "pkg/services/factory_runtime",
	modulePrefix + "pkg/services/factory_sessions",
	modulePrefix + "pkg/services/models",
	modulePrefix + "pkg/services/automations",
	modulePrefix + "pkg/services/provider_sessions",
	modulePrefix + "pkg/services/recordings",
	modulePrefix + "pkg/services/edges",
	modulePrefix + "pkg/services/factory_definitions",
	modulePrefix + "pkg/platform/contentstaging",
	modulePrefix + "pkg/transports/cli",
	modulePrefix + "pkg/transports/http",
	modulePrefix + "pkg/initializer/application",
}

// TestFunctionalWorkPackageUsesPublicProcessImportsOnly seals pss-fun-work-005:
// Work functional proofs construct the process only through root.BuildProcess /
// shared functional support and must not import work/internal, deleted
// transitional Work packages, or legacy pkg/work* consumer edges.
func TestFunctionalWorkPackageUsesPublicProcessImportsOnly(t *testing.T) {
	t.Parallel()

	for _, pkg := range listFunctionalWorkPackages(t) {
		pkg := pkg
		t.Run(shortFunctionalWorkPackageName(pkg), func(t *testing.T) {
			t.Parallel()
			assertFunctionalWorkImportsArePublic(t, pkg)
		})
	}
}

// TestWorkProductionPeersReachWorkThroughPublishedSurfacesOnly proves named
// production peers compose Work only through the published service root or
// terminal transports adapters, not owner-private implementation seams or
// deleted transitional packages.
func TestWorkProductionPeersReachWorkThroughPublishedSurfacesOnly(t *testing.T) {
	t.Parallel()

	for _, packagePath := range workProductionPeerPackages {
		packagePath := packagePath
		t.Run(shortProductionPeerPackageName(packagePath), func(t *testing.T) {
			t.Parallel()
			assertProductionPeerWorkImportsArePublishedOnly(t, packagePath)
		})
	}
}

func listFunctionalWorkPackages(t *testing.T) []string {
	t.Helper()

	cmd := exec.Command("go", "list", functionalWorkRoot+"/...")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list functional work packages: %v\n%s", err, output)
	}
	return strings.Fields(string(output))
}

func assertFunctionalWorkImportsArePublic(t *testing.T, packagePath string) {
	t.Helper()

	for _, importPath := range listDirectImports(t, packagePath) {
		for _, forbidden := range forbiddenFunctionalWorkImports {
			if importPath == forbidden || strings.HasPrefix(importPath, forbidden+"/") {
				t.Fatalf(
					"%s must not import %s; use root.BuildProcess and published Work contracts",
					packagePath,
					importPath,
				)
			}
		}
	}
}

func isWorkServiceImport(importPath string) bool {
	return importPath == workServiceRoot || strings.HasPrefix(importPath, workServiceRoot+"/")
}

func assertProductionPeerWorkImportsArePublishedOnly(t *testing.T, packagePath string) {
	t.Helper()

	for _, importPath := range listDirectImports(t, packagePath) {
		if isForbiddenRetiredWorkConsumerImport(importPath) {
			t.Fatalf(
				"%s must not import retired Work consumer edge %s; use %s",
				packagePath,
				importPath,
				workServiceRoot,
			)
		}
		if !isWorkServiceImport(importPath) {
			continue
		}
		for _, forbidden := range forbiddenWorkOwnerPrivateImportPrefixes {
			if importPath == forbidden || strings.HasPrefix(importPath, forbidden+"/") {
				t.Fatalf(
					"%s must not import owner-private Work package %s",
					packagePath,
					importPath,
				)
			}
		}
		if importPath != workServiceRoot &&
			!strings.HasPrefix(importPath, workServiceRoot+"/transports/") {
			t.Fatalf(
				"%s must reach Work only through %s or terminal transports; found %s",
				packagePath,
				workServiceRoot,
				importPath,
			)
		}
	}
}

func isForbiddenRetiredWorkConsumerImport(importPath string) bool {
	for _, legacyRoot := range retiredWorkConsumerImportRoots {
		if importPath == legacyRoot || strings.HasPrefix(importPath, legacyRoot+"/") {
			return true
		}
	}
	return false
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

func shortFunctionalWorkPackageName(packagePath string) string {
	if strings.HasPrefix(packagePath, functionalWorkRoot) {
		rest := strings.TrimPrefix(packagePath, functionalWorkRoot)
		if rest == "" {
			return "work"
		}
		return strings.TrimPrefix(rest, "/")
	}
	return packagePath
}

func shortProductionPeerPackageName(packagePath string) string {
	return strings.TrimPrefix(packagePath, modulePrefix)
}
