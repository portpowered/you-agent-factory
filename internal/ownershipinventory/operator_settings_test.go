package ownershipinventory_test

import (
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

const (
	operatorSettingsIdentityInventoryPackagePath = "pkg/services/operator_settings/identityinventory"
	operatorSettingsIdentityInventorySuccessor = "pkg/services/operator_settings/internal"
	operatorSettingsServicewirePackagePath       = "pkg/services/operator_settings/servicewire"
	operatorSettingsTestlinkPackagePath          = "pkg/services/operator_settings/testlink"
	operatorSettingsTestprovidersPackagePath     = "pkg/services/operator_settings/testproviders"
)

func TestMapPackageOperatorSettingsUnexpectedSiblingsMoveDestinations(t *testing.T) {
	t.Parallel()

	cases := []struct {
		packagePath string
		successor   string
	}{
		{packagePath: operatorSettingsIdentityInventoryPackagePath, successor: operatorSettingsIdentityInventorySuccessor},
		{packagePath: operatorSettingsServicewirePackagePath, successor: operatorSettingsIdentityInventorySuccessor},
		{packagePath: operatorSettingsTestlinkPackagePath, successor: operatorSettingsIdentityInventorySuccessor},
		{packagePath: operatorSettingsTestprovidersPackagePath, successor: operatorSettingsIdentityInventorySuccessor},
	}
	for _, tc := range cases {
		got, err := ownershipinventory.MapPackage(tc.packagePath)
		if err != nil {
			t.Fatalf("MapPackage(%q) error = %v", tc.packagePath, err)
		}
		if got.Disposition != ownershipinventory.DispositionMove {
			t.Fatalf("MapPackage(%q) disposition = %q, want move", tc.packagePath, got.Disposition)
		}
		if got.Destination != "operator_settings" {
			t.Fatalf("MapPackage(%q) destination = %q, want operator_settings", tc.packagePath, got.Destination)
		}
		if got.Successor != tc.successor {
			t.Fatalf("MapPackage(%q) successor = %q, want %s", tc.packagePath, got.Successor, tc.successor)
		}
		if got.DeletionCondition == "" {
			t.Fatalf("MapPackage(%q) missing deletionCondition", tc.packagePath)
		}
	}
}

func TestMapPackageOperatorSettingsCanonicalChildrenRetain(t *testing.T) {
	t.Parallel()

	canonical := []string{
		"pkg/services/operator_settings",
		"pkg/services/operator_settings/wire",
		"pkg/services/operator_settings/transports",
		"pkg/services/operator_settings/transports/cli",
		"pkg/services/operator_settings/internal",
		"pkg/services/operator_settings/internal/service",
		"pkg/services/operator_settings/internal/services/document",
		"pkg/services/operator_settings/internal/services/resolution",
	}
	for _, packagePath := range canonical {
		got, err := ownershipinventory.MapPackage(packagePath)
		if err != nil {
			t.Fatalf("MapPackage(%q) error = %v", packagePath, err)
		}
		if got.Disposition != ownershipinventory.DispositionRetain || got.Destination != "operator_settings" {
			t.Fatalf("MapPackage(%q) = %#v, want retain under operator_settings", packagePath, got)
		}
	}
}

func TestOperatorSettingsCommittedOwnershipLocksUnexpectedSiblingMoveDestinations(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	inventory, err := ownershipinventory.Load(root)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	wantSuccessors := map[string]string{
		operatorSettingsIdentityInventoryPackagePath: operatorSettingsIdentityInventorySuccessor,
		operatorSettingsServicewirePackagePath:       operatorSettingsIdentityInventorySuccessor,
		operatorSettingsTestlinkPackagePath:          operatorSettingsIdentityInventorySuccessor,
		operatorSettingsTestprovidersPackagePath:       operatorSettingsIdentityInventorySuccessor,
	}
	for packagePath, wantSuccessor := range wantSuccessors {
		var row *ownershipinventory.PackageRow
		for index := range inventory.Packages {
			if inventory.Packages[index].PackagePath == packagePath {
				row = &inventory.Packages[index]
				break
			}
		}
		if row == nil {
			t.Fatalf("committed ownership inventory missing %q", packagePath)
		}
		if row.Disposition != ownershipinventory.DispositionMove {
			t.Fatalf("committed ownership row %q disposition = %q, want move", packagePath, row.Disposition)
		}
		if row.Successor != wantSuccessor {
			t.Fatalf("committed ownership row %q successor = %q, want %s", packagePath, row.Successor, wantSuccessor)
		}
		if row.DeletionCondition == "" {
			t.Fatalf("committed ownership row %q missing deletionCondition", packagePath)
		}
	}
}

func TestMapPackageOperatorSettingsHypotheticalUnexpectedSiblingDefaultsToNoMapping(t *testing.T) {
	t.Parallel()

	_, err := ownershipinventory.MapPackage("pkg/services/operator_settings/hypothetical")
	if err == nil {
		t.Fatal("MapPackage() error = nil, want no committed destination")
	}
}

func TestOperatorSettingsInventoryRejectsRetainToOwnerRootForUnexpectedPublicSibling(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	topLevel, err := ownershipinventory.LoadOperatorSettingsTopLevelInventory(root)
	if err != nil {
		t.Fatalf("LoadOperatorSettingsTopLevelInventory() error = %v", err)
	}

	for _, packagePath := range ownershipinventory.OperatorSettingsUnexpectedPublicSiblingPackagePaths(topLevel) {
		got, err := ownershipinventory.MapPackage(packagePath)
		if err != nil {
			t.Fatalf("MapPackage(%q) error = %v", packagePath, err)
		}
		if got.Disposition == ownershipinventory.DispositionRetain && got.Destination == "operator_settings" {
			t.Fatalf("unexpected retain→operator_settings for inventory path %q", packagePath)
		}
		if got.Disposition != ownershipinventory.DispositionMove {
			t.Fatalf("inventory path %q disposition = %q, want move", packagePath, got.Disposition)
		}
		if got.Successor == "" || got.DeletionCondition == "" {
			t.Fatalf("inventory path %q missing successor/deletionCondition: %#v", packagePath, got)
		}
	}
}
