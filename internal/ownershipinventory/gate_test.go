package ownershipinventory_test

import (
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
	"github.com/portpowered/infinite-you/internal/psslease"
)

func TestVerificationGatePassesOnFrozenArtifacts(t *testing.T) {
	root := repositoryRoot(t)
	report, err := ownershipinventory.VerifyFreeze(root)
	if err != nil {
		t.Fatalf("VerifyFreeze() error = %v", err)
	}
	if !report.OK() {
		t.Fatalf("VerifyFreeze() failed: %#v", report)
	}
	assertProved(t, "completeness", report.Completeness)
	assertProved(t, "stableSortOrder", report.StableSortOrder)
	assertProved(t, "requiredRationaleFields", report.RequiredRationaleFields)
	assertProved(t, "edgeClassifications", report.EdgeClassifications)
	assertProved(t, "namedOwnerCoverage", report.NamedOwnerCoverage)
	assertProved(t, "processEdgesException", report.ProcessEdgesException)
	assertProved(t, "nonOverlappingActiveLeases", report.NonOverlappingActiveLeases)
	assertProved(t, "inventoryOK", report.InventoryOK)
	assertProved(t, "pathLeaseOK", report.PathLeaseOK)
}

func TestVerificationGateFailsWhenInventoryBroken(t *testing.T) {
	root := repositoryRoot(t)
	inventory, err := ownershipinventory.Load(root)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	inventory.Packages = inventory.Packages[1:]
	freeze, err := ownershipinventory.LoadPathLeaseFreeze(root)
	if err != nil {
		t.Fatalf("LoadPathLeaseFreeze() error = %v", err)
	}

	report := ownershipinventory.VerifyFreezeArtifacts(inventory, freeze, mustListPackages(t, root))
	if report.OK() {
		t.Fatal("VerifyFreezeArtifacts() unexpectedly passed with incomplete inventory")
	}
	if report.Completeness {
		t.Fatal("completeness unexpectedly proved with missing package")
	}
	if report.InventoryOK {
		t.Fatal("inventoryOK unexpectedly true with missing package")
	}
}

func TestVerificationGateFailsWhenActiveLeasesOverlap(t *testing.T) {
	root := repositoryRoot(t)
	inventory, err := ownershipinventory.Load(root)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	freeze := ownershipinventory.BuildPathLeaseFreeze()
	for i := range freeze.Packets {
		if freeze.Packets[i].PacketID == "PSS-F02" {
			freeze.Packets[i].State = psslease.StateActive
			freeze.Packets[i].ExclusivePaths = []string{ownershipinventory.InventoryRelativePath}
		}
	}

	report := ownershipinventory.VerifyFreezeArtifacts(inventory, freeze, mustListPackages(t, root))
	if report.OK() {
		t.Fatal("VerifyFreezeArtifacts() unexpectedly passed with overlapping active leases")
	}
	if report.NonOverlappingActiveLeases {
		t.Fatal("nonOverlappingActiveLeases unexpectedly proved with overlap")
	}
	if report.PathLeaseOK {
		t.Fatal("pathLeaseOK unexpectedly true with overlapping active leases")
	}
}

func assertProved(t *testing.T, name string, proved bool) {
	t.Helper()
	if !proved {
		t.Fatalf("%s was not proved by verification gate", name)
	}
}

func mustListPackages(t *testing.T, root string) []string {
	t.Helper()
	packages, err := ownershipinventory.ListProductionPackages(root)
	if err != nil {
		t.Fatalf("ListProductionPackages() error = %v", err)
	}
	return packages
}
