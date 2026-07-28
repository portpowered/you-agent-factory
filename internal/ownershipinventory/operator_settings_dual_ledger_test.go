package ownershipinventory_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

func TestVerifyOperatorSettingsDualLedgerAlignmentPassesOnRepository(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	if err := ownershipinventory.VerifyOperatorSettingsDualLedgerAlignment(root); err != nil {
		t.Fatalf("VerifyOperatorSettingsDualLedgerAlignment() error = %v", err)
	}
}

func TestVerifyOperatorSettingsDualLedgerAlignmentFailsWhenMoveSuccessorDrifts(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	copyOperatorSettingsDualLedgerFixtures(t, root)

	inventory, err := ownershipinventory.Load(root)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	for index := range inventory.Packages {
		if inventory.Packages[index].PackagePath == ownershipinventory.OperatorSettingsOwnerPackagePath+"/internal/construct" {
			inventory.Packages[index].Successor = "pkg/services/operator_settings/wire"
			break
		}
	}
	writeOwnershipInventoryFixture(t, root, inventory)

	err = ownershipinventory.VerifyOperatorSettingsDualLedgerAlignment(root)
	if err == nil {
		t.Fatal("VerifyOperatorSettingsDualLedgerAlignment() error = nil, want successor drift failure")
	}
	if !strings.Contains(err.Error(), "dual-ledger move drift") || !strings.Contains(err.Error(), "internal/construct") {
		t.Fatalf("VerifyOperatorSettingsDualLedgerAlignment() error = %v, want successor drift failure", err)
	}
}

func TestVerifyOperatorSettingsDualLedgerAlignmentFailsWhenManifestMissingOwnershipRow(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	copyOperatorSettingsDualLedgerFixtures(t, root)

	manifest, err := ownershipinventory.LoadPackageTargetLedger(root)
	if err != nil {
		t.Fatalf("LoadPackageTargetLedger() error = %v", err)
	}
	manifest.Packages = append(manifest.Packages, ownershipinventory.PackageTargetLedgerRow{
		PackagePath: ownershipinventory.OperatorSettingsOwnerPackagePath + "/hypothetical",
		Disposition: ownershipinventory.DispositionMove,
		Destination: "operator_settings/internal",
	})
	writePackageTargetManifestFixture(t, root, manifest)

	err = ownershipinventory.VerifyOperatorSettingsDualLedgerAlignment(root)
	if err == nil {
		t.Fatal("VerifyOperatorSettingsDualLedgerAlignment() error = nil, want missing ownership row failure")
	}
	if !strings.Contains(err.Error(), "ownership inventory missing committed manifest row") ||
		!strings.Contains(err.Error(), "hypothetical") {
		t.Fatalf("VerifyOperatorSettingsDualLedgerAlignment() error = %v, want missing ownership row failure", err)
	}
}

func TestVerifyOperatorSettingsDualLedgerAlignmentFailsWhenUnexpectedSiblingRegressesToRetain(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	copyOperatorSettingsDualLedgerFixtures(t, root)

	inventory, err := ownershipinventory.Load(root)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	for index := range inventory.Packages {
		if inventory.Packages[index].PackagePath == ownershipinventory.OperatorSettingsOwnerPackagePath+"/internal/construct" {
			inventory.Packages[index].Disposition = ownershipinventory.DispositionRetain
			inventory.Packages[index].Destination = "operator_settings"
			inventory.Packages[index].Successor = ""
			inventory.Packages[index].DeletionCondition = ""
			break
		}
	}
	writeOwnershipInventoryFixture(t, root, inventory)

	err = ownershipinventory.VerifyOperatorSettingsDualLedgerAlignment(root)
	if err == nil {
		t.Fatal("VerifyOperatorSettingsDualLedgerAlignment() error = nil, want retain regression failure")
	}
	if !strings.Contains(err.Error(), "dual-ledger disposition drift") || !strings.Contains(err.Error(), "internal/construct") {
		t.Fatalf("VerifyOperatorSettingsDualLedgerAlignment() error = %v, want retain regression failure", err)
	}
}

func copyOperatorSettingsDualLedgerFixtures(t *testing.T, root string) {
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
	topLevel, err := ownershipinventory.LoadOperatorSettingsTopLevelInventory(repoRoot)
	if err != nil {
		t.Fatalf("LoadOperatorSettingsTopLevelInventory() error = %v", err)
	}
	rootGo, err := ownershipinventory.LoadOperatorSettingsRootGoInventory(repoRoot)
	if err != nil {
		t.Fatalf("LoadOperatorSettingsRootGoInventory() error = %v", err)
	}
	writeOwnershipInventoryFixture(t, root, inventory)
	writePackageTargetManifestFixture(t, root, manifest)
	writeJSON(t, filepath.Join(root, filepath.FromSlash(ownershipinventory.OperatorSettingsTopLevelInventoryRelativePath)), topLevel)
	writeJSON(t, filepath.Join(root, filepath.FromSlash(ownershipinventory.OperatorSettingsRootGoInventoryRelativePath)), rootGo)
	for _, child := range topLevel.Children {
		mkdirAll(t, filepath.Join(root, "pkg/services/operator_settings", child.Directory))
	}
	for _, file := range rootGo.Files {
		writeFile(t, filepath.Join(root, "pkg/services/operator_settings", file.File), "package operatorsettings\n")
	}
}
