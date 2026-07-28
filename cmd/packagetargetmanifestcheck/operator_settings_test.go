package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

const (
	operatorSettingsIdentityInventoryPackagePath = "pkg/services/operator_settings/identityinventory"
	operatorSettingsIdentityInventoryManifestDest = "operator_settings/internal"
	operatorSettingsServicewirePackagePath        = "pkg/services/operator_settings/servicewire"
	operatorSettingsTestlinkPackagePath           = "pkg/services/operator_settings/testlink"
	operatorSettingsTestprovidersPackagePath      = "pkg/services/operator_settings/testproviders"
)

func TestMapCommittedOwnerPackageOperatorSettingsUnexpectedSiblingsMoveDestinations(t *testing.T) {
	t.Parallel()

	cases := []struct {
		packagePath string
		destination string
	}{
		{packagePath: operatorSettingsIdentityInventoryPackagePath, destination: operatorSettingsIdentityInventoryManifestDest},
		{packagePath: operatorSettingsServicewirePackagePath, destination: operatorSettingsIdentityInventoryManifestDest},
		{packagePath: operatorSettingsTestlinkPackagePath, destination: operatorSettingsIdentityInventoryManifestDest},
		{packagePath: operatorSettingsTestprovidersPackagePath, destination: operatorSettingsIdentityInventoryManifestDest},
	}
	for _, tc := range cases {
		got, ok := mapCommittedOwnerPackage(tc.packagePath)
		if !ok {
			t.Fatalf("mapCommittedOwnerPackage(%q) ok = false", tc.packagePath)
		}
		want := PackageMapping{
			PackagePath: tc.packagePath,
			Disposition: DispositionMove,
			Destination: tc.destination,
		}
		if got != want {
			t.Fatalf("mapCommittedOwnerPackage(%q) = %#v, want %#v", tc.packagePath, got, want)
		}
	}
}

func TestOperatorSettingsCommittedManifestLocksUnexpectedSiblingMoveDestinations(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	manifest, err := loadManifest(filepath.Join(repoRoot, manifestRelativePath))
	if err != nil {
		t.Fatalf("loadManifest() error = %v", err)
	}

	wantDestinations := map[string]string{
		operatorSettingsIdentityInventoryPackagePath: operatorSettingsIdentityInventoryManifestDest,
		operatorSettingsServicewirePackagePath:       operatorSettingsIdentityInventoryManifestDest,
		operatorSettingsTestlinkPackagePath:          operatorSettingsIdentityInventoryManifestDest,
		operatorSettingsTestprovidersPackagePath:       operatorSettingsIdentityInventoryManifestDest,
	}
	for packagePath, wantDestination := range wantDestinations {
		var row *PackageMapping
		for index := range manifest.Packages {
			if manifest.Packages[index].PackagePath == packagePath {
				row = &manifest.Packages[index]
				break
			}
		}
		if row == nil {
			t.Fatalf("committed package-target manifest missing %q", packagePath)
		}
		if row.Disposition != DispositionMove {
			t.Fatalf("committed manifest row %q disposition = %q, want move", packagePath, row.Disposition)
		}
		if row.Destination != wantDestination {
			t.Fatalf("committed manifest row %q destination = %q, want %s", packagePath, row.Destination, wantDestination)
		}
	}
}

func TestCommittedManifestOperatorSettingsRejectsRetainToOwnerRootForUnexpectedSiblings(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	topLevel, err := ownershipinventory.LoadOperatorSettingsTopLevelInventory(repoRoot)
	if err != nil {
		t.Fatalf("LoadOperatorSettingsTopLevelInventory() error = %v", err)
	}
	manifest, err := loadManifest(filepath.Join(repoRoot, manifestRelativePath))
	if err != nil {
		t.Fatalf("loadManifest() error = %v", err)
	}

	manifestByPath := make(map[string]PackageMapping, len(manifest.Packages))
	for _, row := range manifest.Packages {
		manifestByPath[row.PackagePath] = row
	}

	for _, packagePath := range ownershipinventory.OperatorSettingsUnexpectedPublicSiblingPackagePaths(topLevel) {
		row, ok := manifestByPath[packagePath]
		if !ok {
			t.Fatalf("committed manifest missing unexpected public sibling %q", packagePath)
		}
		if row.Disposition == DispositionRetain && row.Destination == "operator_settings" {
			t.Fatalf("committed manifest row retain→operator_settings for %q", packagePath)
		}
		if row.Disposition != DispositionMove {
			t.Fatalf("committed manifest row %q disposition = %q, want move", packagePath, row.Disposition)
		}
		if row.Destination == "operator_settings" {
			t.Fatalf("committed manifest row %q move destination = owner root, want nested plan path", packagePath)
		}
	}
}

func TestVerifyOperatorSettingsDualLedgerAlignmentPassesOnRepository(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	if err := ownershipinventory.VerifyOperatorSettingsDualLedgerAlignment(repoRoot); err != nil {
		t.Fatalf("VerifyOperatorSettingsDualLedgerAlignment() error = %v", err)
	}
}

func TestOperatorSettingsCommittedBaselinesAlignMoveDestinations(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	manifest, err := loadManifest(filepath.Join(repoRoot, manifestRelativePath))
	if err != nil {
		t.Fatalf("loadManifest() error = %v", err)
	}
	ownership, err := ownershipinventory.Load(repoRoot)
	if err != nil {
		t.Fatalf("ownershipinventory.Load() error = %v", err)
	}

	ownershipByPath := make(map[string]ownershipinventory.PackageRow, len(ownership.Packages))
	for _, row := range ownership.Packages {
		ownershipByPath[row.PackagePath] = row
	}

	const ownerPrefix = "pkg/services/operator_settings/"
	for _, row := range manifest.Packages {
		if row.PackagePath != "pkg/services/operator_settings" && !strings.HasPrefix(row.PackagePath, ownerPrefix) {
			continue
		}
		if row.Disposition != DispositionMove {
			continue
		}
		ownershipRow, ok := ownershipByPath[row.PackagePath]
		if !ok {
			t.Fatalf("ownership inventory missing committed manifest move row %q", row.PackagePath)
		}
		wantSuccessor := "pkg/services/" + row.Destination
		if ownershipRow.Successor != wantSuccessor {
			t.Fatalf("dual-ledger drift for %q: manifest destination %q => successor %q, ownership has %q",
				row.PackagePath, row.Destination, wantSuccessor, ownershipRow.Successor)
		}
	}
}

func TestOperatorSettingsInventoryRejectsRetainToOwnerRootForUnexpectedPublicSibling(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	topLevel, err := ownershipinventory.LoadOperatorSettingsTopLevelInventory(repoRoot)
	if err != nil {
		t.Fatalf("LoadOperatorSettingsTopLevelInventory() error = %v", err)
	}

	for _, packagePath := range ownershipinventory.OperatorSettingsUnexpectedPublicSiblingPackagePaths(topLevel) {
		got, ok := mapCommittedOwnerPackage(packagePath)
		if !ok {
			t.Fatalf("mapCommittedOwnerPackage(%q) ok = false", packagePath)
		}
		if got.Disposition == DispositionRetain && got.Destination == "operator_settings" {
			t.Fatalf("unexpected retain→operator_settings for inventory path %q", packagePath)
		}
		if got.Disposition != DispositionMove {
			t.Fatalf("inventory path %q disposition = %q, want move", packagePath, got.Disposition)
		}
		if got.Destination == "operator_settings" {
			t.Fatalf("inventory path %q move destination = owner root, want nested plan path", packagePath)
		}
	}
}
