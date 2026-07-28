package recordings_test

import (
	"os/exec"
	"strings"
	"testing"
)

const recordingsEventsShim = recordingsOwnerRoot + "/events"

// TestPeerProductionPackagesDoNotImportRecordingsEvents seals
// pss-cln-rec-legacy-packages-001: production packages outside the Recordings
// owner must not import the transitional recordings/events shim.
func TestPeerProductionPackagesDoNotImportRecordingsEvents(t *testing.T) {
	t.Parallel()

	for _, packagePath := range listPeerProductionPackages(t) {
		packagePath := packagePath
		t.Run(shortPeerPackageName(packagePath), func(t *testing.T) {
			t.Parallel()
			assertPackageDoesNotImportRecordingsEventsShim(t, packagePath)
		})
	}
}

// TestRecordingsOwnerConstructsThroughCanonicalLedgerEvents seals
// pss-cln-rec-legacy-packages-001: owner-owned production packages must reach
// event history through canonical_ledger/events, not the transitional events/
// shim.
func TestRecordingsOwnerConstructsThroughCanonicalLedgerEvents(t *testing.T) {
	t.Parallel()

	for _, packagePath := range listRecordingsOwnerProductionPackages(t) {
		packagePath := packagePath
		t.Run(shortRecordingsOwnerPackageName(packagePath), func(t *testing.T) {
			t.Parallel()
			assertPackageDoesNotImportRecordingsEventsShim(t, packagePath)
		})
	}
}

func assertPackageDoesNotImportRecordingsEventsShim(t *testing.T, packagePath string) {
	t.Helper()

	cmd := exec.Command("go", "list", "-f", "{{join .Imports \"\\n\"}}", packagePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list imports for %s: %v\n%s", packagePath, err, output)
	}
	for _, importPath := range strings.Fields(string(output)) {
		if importPath == recordingsEventsShim ||
			strings.HasPrefix(importPath, recordingsEventsShim+"/") {
			t.Fatalf(
				"%s must not import %s; construct through canonical_ledger/events or recordings/wire",
				packagePath,
				importPath,
			)
		}
	}
}
