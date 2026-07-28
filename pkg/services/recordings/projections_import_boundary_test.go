package recordings_test

import (
	"os/exec"
	"strings"
	"testing"
)

const recordingsProjectionsShim = recordingsOwnerRoot + "/projections"

// TestPeerProductionPackagesDoNotImportRecordingsProjections seals
// pss-cln-rec-legacy-packages-002: production packages outside the Recordings
// owner must not import the transitional recordings/projections shim.
func TestPeerProductionPackagesDoNotImportRecordingsProjections(t *testing.T) {
	t.Parallel()

	for _, packagePath := range listPeerProductionPackages(t) {
		packagePath := packagePath
		t.Run(shortPeerPackageName(packagePath), func(t *testing.T) {
			t.Parallel()
			assertPackageDoesNotImportRecordingsProjectionsShim(t, packagePath)
		})
	}
}

// TestRecordingsOwnerConstructsThroughProjectionQueryProjections seals
// pss-cln-rec-legacy-packages-002: owner-owned production packages must reach
// projection implementation through projection_query/projections, not the
// transitional projections/ shim.
func TestRecordingsOwnerConstructsThroughProjectionQueryProjections(t *testing.T) {
	t.Parallel()

	for _, packagePath := range listRecordingsOwnerProductionPackages(t) {
		packagePath := packagePath
		t.Run(shortRecordingsOwnerPackageName(packagePath), func(t *testing.T) {
			t.Parallel()
			assertPackageDoesNotImportRecordingsProjectionsShim(t, packagePath)
		})
	}
}

func assertPackageDoesNotImportRecordingsProjectionsShim(t *testing.T, packagePath string) {
	t.Helper()

	cmd := exec.Command("go", "list", "-f", "{{join .Imports \"\\n\"}}", packagePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list imports for %s: %v\n%s", packagePath, err, output)
	}
	for _, importPath := range strings.Fields(string(output)) {
		if importPath == recordingsProjectionsShim ||
			strings.HasPrefix(importPath, recordingsProjectionsShim+"/") {
			t.Fatalf(
				"%s must not import %s; construct through projection_query/projections or recordings/wire",
				packagePath,
				importPath,
			)
		}
	}
}
