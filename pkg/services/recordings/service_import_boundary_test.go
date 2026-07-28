package recordings_test

import (
	"os/exec"
	"strings"
	"testing"
)

const (
	recordingsOwnerRoot   = modulePrefix + "pkg/services/recordings"
	recordingsServiceShim = recordingsOwnerRoot + "/service"
)

// TestPeerProductionPackagesDoNotImportRecordingsService seals
// pss-cln-rec-fold-service-004: production packages outside the Recordings owner
// must not import the transitional recordings/service shim.
func TestPeerProductionPackagesDoNotImportRecordingsService(t *testing.T) {
	t.Parallel()

	for _, packagePath := range listPeerProductionPackages(t) {
		packagePath := packagePath
		t.Run(shortPeerPackageName(packagePath), func(t *testing.T) {
			t.Parallel()
			assertPackageDoesNotImportRecordingsServiceShim(t, packagePath)
		})
	}
}

// TestRecordingsOwnerConstructsThroughInternalNotServiceShim seals
// pss-cln-rec-fold-service-004: owner-owned production packages must construct
// through internal or wire, not the transitional service/ shim.
func TestRecordingsOwnerConstructsThroughInternalNotServiceShim(t *testing.T) {
	t.Parallel()

	for _, packagePath := range listRecordingsOwnerProductionPackages(t) {
		packagePath := packagePath
		t.Run(shortRecordingsOwnerPackageName(packagePath), func(t *testing.T) {
			t.Parallel()
			assertPackageDoesNotImportRecordingsServiceShim(t, packagePath)
		})
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
		if packagePath == recordingsServiceShim {
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

func assertPackageDoesNotImportRecordingsServiceShim(t *testing.T, packagePath string) {
	t.Helper()

	cmd := exec.Command("go", "list", "-f", "{{join .Imports \"\\n\"}}", packagePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list imports for %s: %v\n%s", packagePath, err, output)
	}
	for _, importPath := range strings.Fields(string(output)) {
		if importPath == recordingsServiceShim {
			t.Fatalf(
				"%s must not import %s; construct through recordings/wire or owner-internal constructors",
				packagePath,
				recordingsServiceShim,
			)
		}
	}
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
