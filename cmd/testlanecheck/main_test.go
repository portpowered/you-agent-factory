package main

import (
	"testing"

	"github.com/portpowered/infinite-you/internal/testlanes"
)

func TestOptInPackagesAreExcludedFromRequiredLaneAudit(t *testing.T) {
	if !isOptInPackage(testlanes.ModulePath + "/tests/adhoc/provider") {
		t.Fatal("adhoc package should be opt-in")
	}
	if isOptInPackage(testlanes.ModulePath + "/pkg/services/work") {
		t.Fatal("ordinary package must be lane-owned")
	}
}
