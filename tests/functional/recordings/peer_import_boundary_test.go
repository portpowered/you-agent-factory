package recordings_test

import (
	"os/exec"
	"strings"
	"testing"
)

const (
	modulePrefix                  = "github.com/portpowered/infinite-you/"
	recordingsRoot                = modulePrefix + "pkg/services/recordings"
	recordingsWireImport          = recordingsRoot + "/wire"
	recordingsTransportsImport    = recordingsRoot + "/transports"
	functionalRecordingsRoot      = modulePrefix + "tests/functional/recordings"
	functionalReplayContractsRoot = modulePrefix + "tests/functional/replay_contracts"
)

var forbiddenTransitionalRecordingsTopLevel = []string{
	"service",
	"artifacts",
	"events",
	"projections",
	"replay",
}

// recordingsProductionPeerPackages are composition peers that must reach
// Recordings only through published root contracts and terminal adapters, not
// owner-private implementation or deleted transitional public packages.
var recordingsProductionPeerPackages = []string{
	modulePrefix + "pkg/root",
	modulePrefix + "pkg/wire",
	modulePrefix + "pkg/services/workers",
	modulePrefix + "pkg/services/work",
	modulePrefix + "pkg/services/factory_runtime",
	modulePrefix + "pkg/services/factory_sessions",
	modulePrefix + "pkg/services/factory_visualization",
	modulePrefix + "pkg/services/edges",
	modulePrefix + "pkg/services/factory_definitions",
	modulePrefix + "pkg/transports/cli",
	modulePrefix + "pkg/transports/http",
	modulePrefix + "pkg/transports/mapping/factoryeventprojection",
	modulePrefix + "pkg/transports/http/workstationprojection",
	modulePrefix + "pkg/initializer/application",
}

// TestFunctionalRecordingsPackageUsesPublicProcessImportsOnly seals
// pss-fun-recordings-005: Recordings functional proofs construct the process
// only through root.BuildProcess / shared functional support and must not import
// recordings/internal or deleted transitional Recordings public roots.
func TestFunctionalRecordingsPackageUsesPublicProcessImportsOnly(t *testing.T) {
	t.Parallel()

	forbidden := forbiddenFunctionalRecordingsImports()
	for _, pkg := range listFunctionalRecordingsPackages(t) {
		pkg := pkg
		t.Run(shortFunctionalRecordingsPackageName(pkg), func(t *testing.T) {
			t.Parallel()
			assertRecordingsImportsForbidden(t, pkg, forbidden)
		})
	}
}

// TestFunctionalReplayContractsPackageUsesPublicProcessImportsOnly seals
// pss-fun-recordings-005: replay_contracts proofs retargeted under the FUN
// lease must not import recordings/internal or deleted transitional Recordings
// public roots.
func TestFunctionalReplayContractsPackageUsesPublicProcessImportsOnly(t *testing.T) {
	t.Parallel()

	forbidden := forbiddenFunctionalRecordingsImports()
	for _, pkg := range listFunctionalReplayContractsPackages(t) {
		pkg := pkg
		t.Run(shortFunctionalReplayContractsPackageName(pkg), func(t *testing.T) {
			t.Parallel()
			assertRecordingsImportsForbidden(t, pkg, forbidden)
		})
	}
}

// TestProductionPeersReachRecordingsThroughPublishedSurfacesOnly seals
// pss-fun-recordings-005: named production peers compose Recordings only
// through published root contracts and terminal adapters, not recordings/internal
// or deleted transitional Recordings public roots.
func TestProductionPeersReachRecordingsThroughPublishedSurfacesOnly(t *testing.T) {
	t.Parallel()

	forbidden := forbiddenProductionPeerRecordingsImports()
	for _, packagePath := range recordingsProductionPeerPackages {
		packagePath := packagePath
		t.Run(shortProductionPeerPackageName(packagePath), func(t *testing.T) {
			t.Parallel()
			assertRecordingsImportsForbidden(t, packagePath, forbidden)
			assertProductionPeerRecordingsImportsArePublished(t, packagePath)
		})
	}
}

func forbiddenFunctionalRecordingsImports() []string {
	forbidden := []string{recordingsRoot + "/internal"}
	return append(forbidden, transitionalRecordingsImportPrefixes()...)
}

func forbiddenProductionPeerRecordingsImports() []string {
	return forbiddenFunctionalRecordingsImports()
}

func transitionalRecordingsImportPrefixes() []string {
	prefixes := make([]string, 0, len(forbiddenTransitionalRecordingsTopLevel))
	for _, name := range forbiddenTransitionalRecordingsTopLevel {
		prefixes = append(prefixes, recordingsRoot+"/"+name)
	}
	return prefixes
}

func listFunctionalRecordingsPackages(t *testing.T) []string {
	t.Helper()
	return listGoPackages(t, functionalRecordingsRoot+"/...")
}

func listFunctionalReplayContractsPackages(t *testing.T) []string {
	t.Helper()
	return listGoPackages(t, functionalReplayContractsRoot+"/...")
}

func listGoPackages(t *testing.T, pattern string) []string {
	t.Helper()

	cmd := exec.Command("go", "list", pattern)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list %s: %v\n%s", pattern, err, output)
	}
	return strings.Fields(string(output))
}

func assertRecordingsImportsForbidden(t *testing.T, packagePath string, forbidden []string) {
	t.Helper()

	for _, importPath := range listDirectImports(t, packagePath) {
		for _, blocked := range forbidden {
			if importPath == blocked || strings.HasPrefix(importPath, blocked+"/") {
				t.Fatalf(
					"%s must not import %s; use root.BuildProcess and published Recordings contracts",
					packagePath,
					importPath,
				)
			}
		}
	}
}

func assertProductionPeerRecordingsImportsArePublished(t *testing.T, packagePath string) {
	t.Helper()

	for _, importPath := range listDirectImports(t, packagePath) {
		if !strings.HasPrefix(importPath, recordingsRoot) {
			continue
		}
		if isPublishedRecordingsImport(importPath) {
			continue
		}
		t.Fatalf(
			"%s must reach Recordings only through %s, %s, or %s; found %s",
			packagePath,
			recordingsRoot,
			recordingsWireImport,
			recordingsTransportsImport,
			importPath,
		)
	}
}

func isPublishedRecordingsImport(importPath string) bool {
	if importPath == recordingsRoot {
		return true
	}
	if importPath == recordingsWireImport || strings.HasPrefix(importPath, recordingsWireImport+"/") {
		return true
	}
	if importPath == recordingsTransportsImport || strings.HasPrefix(importPath, recordingsTransportsImport+"/") {
		return true
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

func shortFunctionalRecordingsPackageName(packagePath string) string {
	if strings.HasPrefix(packagePath, functionalRecordingsRoot) {
		rest := strings.TrimPrefix(packagePath, functionalRecordingsRoot)
		if rest == "" {
			return "recordings"
		}
		return strings.TrimPrefix(rest, "/")
	}
	return packagePath
}

func shortFunctionalReplayContractsPackageName(packagePath string) string {
	if strings.HasPrefix(packagePath, functionalReplayContractsRoot) {
		rest := strings.TrimPrefix(packagePath, functionalReplayContractsRoot)
		if rest == "" {
			return "replay_contracts"
		}
		return strings.TrimPrefix(rest, "/")
	}
	return packagePath
}

func shortProductionPeerPackageName(packagePath string) string {
	return strings.TrimPrefix(packagePath, modulePrefix)
}
