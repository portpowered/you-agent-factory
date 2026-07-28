package ownershipinventory_test

import (
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

const (
	operatorSettingsInternalSuccessor = "pkg/services/operator_settings/internal"
)

func TestMapPackageOperatorSettingsCanonicalChildrenRetain(t *testing.T) {
	t.Parallel()

	canonical := []string{
		"pkg/services/operator_settings",
		"pkg/services/operator_settings/wire",
		"pkg/services/operator_settings/transports",
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

func TestMapPackageOperatorSettingsUnmatchedPathsDefaultToInternalMove(t *testing.T) {
	t.Parallel()

	got, err := ownershipinventory.MapPackage("pkg/services/operator_settings/hypothetical")
	if err != nil {
		t.Fatalf("MapPackage() error = %v", err)
	}
	if got.Disposition != ownershipinventory.DispositionMove {
		t.Fatalf("MapPackage() disposition = %q, want move", got.Disposition)
	}
	if got.Successor != operatorSettingsInternalSuccessor {
		t.Fatalf("MapPackage() successor = %q, want %s", got.Successor, operatorSettingsInternalSuccessor)
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
