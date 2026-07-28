package ownershipinventory_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

func TestVerifyWorkDualLedgerAlignmentPassesOnRepository(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	if err := ownershipinventory.VerifyWorkDualLedgerAlignment(root); err != nil {
		t.Fatalf("VerifyWorkDualLedgerAlignment() error = %v", err)
	}
}

func TestVerifyWorkDualLedgerAlignmentFailsWhenMoveSuccessorDrifts(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	copyWorkDualLedgerFixtures(t, root)

	inventory, err := ownershipinventory.Load(root)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	const hypotheticalPackagePath = ownershipinventory.WorkOwnerPackagePath + "/hypothetical"
	inventory.Packages = append(inventory.Packages, ownershipinventory.PackageRow{
		PackagePath:       hypotheticalPackagePath,
		Disposition:       ownershipinventory.DispositionMove,
		Destination:       "work",
		DestinationKind:   ownershipinventory.DestinationKindOwner,
		Successor:         "pkg/services/work/wire",
		DeletionCondition: "test fixture drift",
	})
	manifest, err := ownershipinventory.LoadPackageTargetLedger(root)
	if err != nil {
		t.Fatalf("LoadPackageTargetLedger() error = %v", err)
	}
	manifest.Packages = append(manifest.Packages, ownershipinventory.PackageTargetLedgerRow{
		PackagePath: hypotheticalPackagePath,
		Disposition: ownershipinventory.DispositionMove,
		Destination: "work/internal",
	})
	writeOwnershipInventoryFixture(t, root, inventory)
	writePackageTargetManifestFixture(t, root, manifest)

	err = ownershipinventory.VerifyWorkDualLedgerAlignment(root)
	if err == nil {
		t.Fatal("VerifyWorkDualLedgerAlignment() error = nil, want successor drift failure")
	}
	if !strings.Contains(err.Error(), "dual-ledger move drift") || !strings.Contains(err.Error(), "hypothetical") {
		t.Fatalf("VerifyWorkDualLedgerAlignment() error = %v, want successor drift failure", err)
	}
}

func copyWorkDualLedgerFixtures(t *testing.T, root string) {
	t.Helper()

	repoRoot := repositoryRoot(t)
	inventory, err := ownershipinventory.Load(repoRoot)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	manifest, err := ownershipinventory.LoadPackageTargetLedger(repoRoot)
	if err != nil {
		t.Fatalf("LoadPackageTargetLedger() error = %v", err)
	}
	writeOwnershipInventoryFixture(t, root, inventory)
	writePackageTargetManifestFixture(t, root, manifest)
	spec, ok := ownershipinventory.OwnerTopLevelSpecFor("work")
	if !ok {
		t.Fatal("OwnerTopLevelSpecFor(work) ok = false")
	}
	serviceRoot := filepath.Join("pkg", "services", "work")
	for _, child := range spec.ExpectedRetain {
		mkdirAll(t, filepath.Join(root, serviceRoot, child))
	}
	for _, child := range spec.Unexpected {
		mkdirAll(t, filepath.Join(root, serviceRoot, child))
	}
}
