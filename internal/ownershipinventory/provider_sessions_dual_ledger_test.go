package ownershipinventory_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

func TestVerifyProviderSessionsDualLedgerAlignmentPassesOnRepository(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	if err := ownershipinventory.VerifyProviderSessionsDualLedgerAlignment(root); err != nil {
		t.Fatalf("VerifyProviderSessionsDualLedgerAlignment() error = %v", err)
	}
}

func TestVerifyProviderSessionsDualLedgerAlignmentFailsWhenMoveSuccessorDrifts(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	copyProviderSessionsDualLedgerFixtures(t, root)

	inventory, err := ownershipinventory.Load(root)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	const hypotheticalPackagePath = ownershipinventory.ProviderSessionsOwnerPackagePath + "/hypothetical"
	inventory.Packages = append(inventory.Packages, ownershipinventory.PackageRow{
		PackagePath:       hypotheticalPackagePath,
		Disposition:       ownershipinventory.DispositionMove,
		Destination:       "provider_sessions",
		DestinationKind:   ownershipinventory.DestinationKindOwner,
		Successor:         "pkg/services/provider_sessions/wire",
		DeletionCondition: "test fixture drift",
	})
	manifest, err := ownershipinventory.LoadPackageTargetLedger(root)
	if err != nil {
		t.Fatalf("LoadPackageTargetLedger() error = %v", err)
	}
	manifest.Packages = append(manifest.Packages, ownershipinventory.PackageTargetLedgerRow{
		PackagePath: hypotheticalPackagePath,
		Disposition: ownershipinventory.DispositionMove,
		Destination: "provider_sessions/internal",
	})
	writeOwnershipInventoryFixture(t, root, inventory)
	writePackageTargetManifestFixture(t, root, manifest)

	err = ownershipinventory.VerifyProviderSessionsDualLedgerAlignment(root)
	if err == nil {
		t.Fatal("VerifyProviderSessionsDualLedgerAlignment() error = nil, want successor drift failure")
	}
	if !strings.Contains(err.Error(), "dual-ledger move drift") || !strings.Contains(err.Error(), "hypothetical") {
		t.Fatalf("VerifyProviderSessionsDualLedgerAlignment() error = %v, want successor drift failure", err)
	}
}

func TestVerifyProviderSessionsDualLedgerAlignmentFailsWhenManifestMissingOwnershipRow(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	copyProviderSessionsDualLedgerFixtures(t, root)

	manifest, err := ownershipinventory.LoadPackageTargetLedger(root)
	if err != nil {
		t.Fatalf("LoadPackageTargetLedger() error = %v", err)
	}
	manifest.Packages = append(manifest.Packages, ownershipinventory.PackageTargetLedgerRow{
		PackagePath: ownershipinventory.ProviderSessionsOwnerPackagePath + "/hypothetical",
		Disposition: ownershipinventory.DispositionMove,
		Destination: "provider_sessions/internal",
	})
	writePackageTargetManifestFixture(t, root, manifest)

	err = ownershipinventory.VerifyProviderSessionsDualLedgerAlignment(root)
	if err == nil {
		t.Fatal("VerifyProviderSessionsDualLedgerAlignment() error = nil, want missing ownership row failure")
	}
	if !strings.Contains(err.Error(), "ownership inventory missing committed manifest row") ||
		!strings.Contains(err.Error(), "hypothetical") {
		t.Fatalf("VerifyProviderSessionsDualLedgerAlignment() error = %v, want missing ownership row failure", err)
	}
}

func copyProviderSessionsDualLedgerFixtures(t *testing.T, root string) {
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
	topLevel, err := ownershipinventory.LoadProviderSessionsTopLevelInventory(repoRoot)
	if err != nil {
		t.Fatalf("LoadProviderSessionsTopLevelInventory() error = %v", err)
	}
	writeOwnershipInventoryFixture(t, root, inventory)
	writePackageTargetManifestFixture(t, root, manifest)
	writeJSON(t, filepath.Join(root, filepath.FromSlash(ownershipinventory.ProviderSessionsTopLevelInventoryRelativePath)), topLevel)
	for _, child := range topLevel.Children {
		mkdirAll(t, filepath.Join(root, "pkg/services/provider_sessions", child.Directory))
	}
}

func writeOwnershipInventoryFixture(t *testing.T, root string, inventory ownershipinventory.Inventory) {
	t.Helper()
	writeJSON(t, filepath.Join(root, filepath.FromSlash(ownershipinventory.InventoryRelativePath)), inventory)
}

func writePackageTargetManifestFixture(t *testing.T, root string, manifest ownershipinventory.PackageTargetLedger) {
	t.Helper()
	writeJSON(t, filepath.Join(root, filepath.FromSlash(ownershipinventory.PackageTargetManifestRelativePath)), manifest)
}
