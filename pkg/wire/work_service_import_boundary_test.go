package wire

import (
	"strings"
	"testing"
)

const (
	transitionalWorkService = modulePrefix + "pkg/services/work/service"
	workWirePackage         = modulePrefix + "pkg/services/work/wire"
)

// TestRootWireDoesNotImportTransitionalWorkServiceShim seals root application
// composition on the Work wire bridge instead of the transitional service shim.
func TestRootWireDoesNotImportTransitionalWorkServiceShim(t *testing.T) {
	t.Parallel()

	for _, dep := range listNonStandardDeps(t, rootWirePackage) {
		if dep == transitionalWorkService || strings.HasPrefix(dep, transitionalWorkService+"/") {
			t.Fatalf(
				"%s must not depend on transitional Work service shim %s; found dependency %s",
				rootWirePackage,
				transitionalWorkService,
				dep,
			)
		}
	}
}

// TestRootWireDependsOnWorkWireBridge proves application composition reaches
// Work runtime construction through the public wire bridge.
func TestRootWireDependsOnWorkWireBridge(t *testing.T) {
	t.Parallel()

	found := false
	for _, dep := range listNonStandardDeps(t, rootWirePackage) {
		if dep == workWirePackage || strings.HasPrefix(dep, workWirePackage+"/") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("%s must depend on %s for Work construction", rootWirePackage, workWirePackage)
	}
}
