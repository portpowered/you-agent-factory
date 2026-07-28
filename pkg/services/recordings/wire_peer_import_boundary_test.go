package recordings_test

import (
	"os/exec"
	"strings"
	"testing"
)

const applicationWireRoot = modulePrefix + "pkg/wire"

var foldedRecordingsSiblingShims = []string{
	recordingsOwnerRoot + "/artifacts",
	recordingsOwnerRoot + "/events",
	recordingsOwnerRoot + "/projections",
	recordingsOwnerRoot + "/replay",
}

// TestApplicationWireDoesNotImportFoldedRecordingsSiblingShims seals
// pss-cln-rec-legacy-packages-005: pkg/wire must compose Recordings through
// recordings/wire and the published root contract, not transitional sibling
// shims.
func TestApplicationWireDoesNotImportFoldedRecordingsSiblingShims(t *testing.T) {
	t.Parallel()

	for _, shimPath := range foldedRecordingsSiblingShims {
		shimPath := shimPath
		t.Run(shortPeerPackageName(shimPath), func(t *testing.T) {
			t.Parallel()
			assertPackageDoesNotImportRecordingsShim(t, applicationWireRoot, shimPath)
		})
	}
}

func assertPackageDoesNotImportRecordingsShim(t *testing.T, packagePath, shimPath string) {
	t.Helper()

	cmd := exec.Command("go", "list", "-f", "{{join .Imports \"\\n\"}}", packagePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list imports for %s: %v\n%s", packagePath, err, output)
	}
	for _, importPath := range strings.Fields(string(output)) {
		if importPath == shimPath || strings.HasPrefix(importPath, shimPath+"/") {
			t.Fatalf(
				"%s must not import %s; construct through recordings/wire or the published Recordings root",
				packagePath,
				importPath,
			)
		}
	}
}
