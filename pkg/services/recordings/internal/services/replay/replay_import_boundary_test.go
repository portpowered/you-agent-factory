package recordingsreplay_test

import (
	"os/exec"
	"strings"
	"testing"
)

const (
	modulePrefix        = "github.com/portpowered/infinite-you/"
	recordingsOwnerRoot   = modulePrefix + "pkg/services/recordings"
	recordingsReplayShim = recordingsOwnerRoot + "/replay"
)

// TestPeerProductionPackagesDoNotImportRecordingsReplay seals
// pss-cln-rec-legacy-packages-003: production packages outside the Recordings
// owner must not import the transitional recordings/replay shim.
func TestPeerProductionPackagesDoNotImportRecordingsReplay(t *testing.T) {
	t.Parallel()

	for _, packagePath := range listPeerProductionPackages(t) {
		packagePath := packagePath
		t.Run(shortPeerPackageName(packagePath), func(t *testing.T) {
			t.Parallel()
			assertPackageDoesNotImportRecordingsReplayShim(t, packagePath)
		})
	}
}

// TestRecordingsOwnerConstructsThroughPrivateReplay seals
// pss-cln-rec-legacy-packages-003: owner-owned production packages must reach
// replay implementation through internal/services/replay/replay, not the
// transitional replay/ shim.
func TestRecordingsOwnerConstructsThroughPrivateReplay(t *testing.T) {
	t.Parallel()

	for _, packagePath := range listRecordingsOwnerProductionPackages(t) {
		packagePath := packagePath
		t.Run(shortRecordingsOwnerPackageName(packagePath), func(t *testing.T) {
			t.Parallel()
			assertPackageDoesNotImportRecordingsReplayShim(t, packagePath)
		})
	}
}

func assertPackageDoesNotImportRecordingsReplayShim(t *testing.T, packagePath string) {
	t.Helper()

	cmd := exec.Command("go", "list", "-f", "{{join .Imports \"\\n\"}}", packagePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list imports for %s: %v\n%s", packagePath, err, output)
	}
	for _, importPath := range strings.Fields(string(output)) {
		if importPath == recordingsReplayShim ||
			strings.HasPrefix(importPath, recordingsReplayShim+"/") {
			t.Fatalf(
				"%s must not import %s; construct through internal/services/replay/replay or recordings/wire",
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
