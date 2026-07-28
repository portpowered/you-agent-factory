package artifactsexport_test

import (
	"os/exec"
	"strings"
	"testing"
)

const (
	modulePrefix            = "github.com/portpowered/infinite-you/"
	recordingsOwnerRoot     = modulePrefix + "pkg/services/recordings"
	recordingsArtifactsShim = recordingsOwnerRoot + "/artifacts"
)

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

func listPeerProductionPackages(t *testing.T) []string {
	t.Helper()

	var packages []string
	for _, packagePath := range listRepositoryProductionPackages(t) {
		if strings.HasPrefix(packagePath, recordingsOwnerRoot) {
			continue
		}
		packages = append(packages, packagePath)
	}
	return packages
}

func listRecordingsOwnerProductionPackages(t *testing.T) []string {
	t.Helper()

	var packages []string
	for _, packagePath := range listRepositoryProductionPackages(t) {
		if !strings.HasPrefix(packagePath, recordingsOwnerRoot) {
			continue
		}
		packages = append(packages, packagePath)
	}
	return packages
}

func listRepositoryProductionPackages(t *testing.T) []string {
	t.Helper()

	cmd := exec.Command("go", "list", "./...")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list repository packages: %v\n%s", err, output)
	}

	var packages []string
	for _, packagePath := range strings.Fields(string(output)) {
		if strings.HasSuffix(packagePath, "_test") {
			continue
		}
		packages = append(packages, packagePath)
	}
	return packages
}

func shortPeerPackageName(packagePath string) string {
	return strings.TrimPrefix(packagePath, modulePrefix)
}

func shortRecordingsOwnerPackageName(packagePath string) string {
	if strings.HasPrefix(packagePath, recordingsOwnerRoot) {
		rest := strings.TrimPrefix(packagePath, recordingsOwnerRoot)
		if rest == "" {
			return "recordings"
		}
		return strings.TrimPrefix(rest, "/")
	}
	return packagePath
}
