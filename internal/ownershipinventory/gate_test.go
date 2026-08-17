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
	// A live package with no row is now valid, so the surviving package-row
	// failure mode is the reverse: a row naming a package absent from disk.
	inventory.Packages = append(append([]ownershipinventory.PackageRow(nil), inventory.Packages...), ownershipinventory.PackageRow{
		PackagePath:       "pkg/services/workers/dcp2deletedpackage",
		Disposition:       ownershipinventory.DispositionMove,
		Destination:       "workers",
		DestinationKind:   ownershipinventory.DestinationKindOwner,
		Successor:         "pkg/services/workers/internal",
		DeletionCondition: "delete after cutover proof",
	})
	freeze, err := ownershipinventory.LoadPathLeaseFreeze(root)
	if err != nil {
		t.Fatalf("LoadPathLeaseFreeze() error = %v", err)
	}

	report := ownershipinventory.VerifyFreezeArtifacts(inventory, freeze, mustListPackages(t, root))
	if report.OK() {
		t.Fatal("VerifyFreezeArtifacts() unexpectedly passed with a row naming an absent package")
	}
	if report.Completeness {
		t.Fatal("completeness unexpectedly proved with a row naming an absent package")
	}
	if report.InventoryOK {
		t.Fatal("inventoryOK unexpectedly true with a row naming an absent package")
	}
}

func TestVerificationGatePassesWhenLivePackageHasNoRow(t *testing.T) {
	root := repositoryRoot(t)
	inventory, err := ownershipinventory.Load(root)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	freeze, err := ownershipinventory.LoadPathLeaseFreeze(root)
	if err != nil {
		t.Fatalf("LoadPathLeaseFreeze() error = %v", err)
	}
	packages := append(mustListPackages(t, root), "pkg/services/workers/dcp2scratchpackage")

	report := ownershipinventory.VerifyFreezeArtifacts(inventory, freeze, packages)
	if !report.OK() {
		t.Fatalf("VerifyFreezeArtifacts() rejected a new package that has no registry row; report=%#v", report)
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
