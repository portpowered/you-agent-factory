package ownershipinventory_test

import (
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

const deletedProviderSessionsServicePackagePath = "pkg/services/provider_sessions/service"

func TestMapPackageProviderSessionsPrivateReadersRetainUnderOwner(t *testing.T) {
	t.Parallel()

	privateReaders := []string{
		"pkg/services/provider_sessions/internal/services/codex_reader",
		"pkg/services/provider_sessions/internal/services/cursor_reader",
	}
	for _, packagePath := range privateReaders {
		got, err := ownershipinventory.MapPackage(packagePath)
		if err != nil {
			t.Fatalf("MapPackage(%q) error = %v", packagePath, err)
		}
		if got.Disposition != ownershipinventory.DispositionRetain || got.Destination != "provider_sessions" {
			t.Fatalf("MapPackage(%q) = %#v, want retain under provider_sessions owner", packagePath, got)
		}
	}
}

func TestProviderSessionsCommittedOwnershipOmitsDeletedTransitionalServicePackage(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	inventory, err := ownershipinventory.Load(root)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	for _, row := range inventory.Packages {
		if row.PackagePath == deletedProviderSessionsServicePackagePath {
			t.Fatalf("committed ownership inventory still lists deleted transitional package %q", deletedProviderSessionsServicePackagePath)
		}
	}
}

func TestMapPackageProviderSessionsHypotheticalUnexpectedSiblingDefaultsToRetain(t *testing.T) {
	t.Parallel()

	got, err := ownershipinventory.MapPackage("pkg/services/provider_sessions/hypothetical")
	if err != nil {
		t.Fatalf("MapPackage() error = %v", err)
	}
	if got.Disposition != ownershipinventory.DispositionRetain || got.Destination != "provider_sessions" {
		t.Fatalf("MapPackage() = %#v, want default retain→provider_sessions for unmapped owner child", got)
	}
}

func TestProviderSessionsInventoryRejectsRetainToOwnerRootForUnexpectedPublicSibling(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	topLevel, err := ownershipinventory.LoadProviderSessionsTopLevelInventory(root)
	if err != nil {
		t.Fatalf("LoadProviderSessionsTopLevelInventory() error = %v", err)
	}

	for _, child := range topLevel.Children {
		if child.Classification != ownershipinventory.ProviderSessionsTopLevelUnexpectedPublicSibling &&
			child.Classification != ownershipinventory.ProviderSessionsTopLevelINVUnexpectedPublicSibling {
			continue
		}
		packagePath := ownershipinventory.ProviderSessionsOwnerPackagePath + "/" + child.Directory
		got, err := ownershipinventory.MapPackage(packagePath)
		if err != nil {
			t.Fatalf("MapPackage(%q) error = %v", packagePath, err)
		}
		if got.Disposition == ownershipinventory.DispositionRetain && got.Destination == "provider_sessions" {
			t.Fatalf("unexpected retain→provider_sessions for inventory path %q", packagePath)
		}
		if got.Disposition != ownershipinventory.DispositionMove {
			t.Fatalf("inventory path %q disposition = %q, want move", packagePath, got.Disposition)
		}
		if got.Successor == "" || got.DeletionCondition == "" {
			t.Fatalf("inventory path %q missing successor/deletionCondition: %#v", packagePath, got)
		}
	}
}
