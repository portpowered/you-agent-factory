package recordings_test

import (
	"os/exec"
	"strings"
	"testing"
)

const recordingsArtifactsShim = recordingsOwnerRoot + "/artifacts"

// TestPeerProductionPackagesDoNotImportRecordingsArtifacts seals
// pss-cln-rec-legacy-packages-004: production packages outside the Recordings
// owner must not import the transitional recordings/artifacts shim.
func TestPeerProductionPackagesDoNotImportRecordingsArtifacts(t *testing.T) {
	t.Parallel()

	for _, packagePath := range listPeerProductionPackages(t) {
		packagePath := packagePath
		t.Run(shortPeerPackageName(packagePath), func(t *testing.T) {
			t.Parallel()
			assertPackageDoesNotImportRecordingsArtifactsShim(t, packagePath)
		})
	}
}

// TestRecordingsOwnerConstructsThroughPrivateArtifactsExport seals
// pss-cln-rec-legacy-packages-004: owner-owned production packages must reach
// artifact export implementation through internal/services/artifacts_export/artifacts,
// not the transitional artifacts/ shim.
func TestRecordingsOwnerConstructsThroughPrivateArtifactsExport(t *testing.T) {
	t.Parallel()

	for _, packagePath := range listRecordingsOwnerProductionPackages(t) {
		// contracts.go still aliases legacy portable-recording types through the
		// transitional artifacts/ shim until CLN-REC-CONTRACT-ROOTS folds that cluster.
		if packagePath == recordingsOwnerRoot {
			continue
		}
		packagePath := packagePath
		t.Run(shortRecordingsOwnerPackageName(packagePath), func(t *testing.T) {
			t.Parallel()
			assertPackageDoesNotImportRecordingsArtifactsShim(t, packagePath)
		})
	}
}

func assertPackageDoesNotImportRecordingsArtifactsShim(t *testing.T, packagePath string) {
	t.Helper()

	cmd := exec.Command("go", "list", "-f", "{{join .Imports \"\\n\"}}", packagePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list imports for %s: %v\n%s", packagePath, err, output)
	}
	for _, importPath := range strings.Fields(string(output)) {
		if importPath == recordingsArtifactsShim ||
			strings.HasPrefix(importPath, recordingsArtifactsShim+"/") {
			t.Fatalf(
				"%s must not import %s; construct through internal/services/artifacts_export/artifacts or recordings/wire",
				packagePath,
				importPath,
			)
		}
	}
}
