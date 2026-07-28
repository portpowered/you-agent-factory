package ownershipinventory_test

import (
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

const (
	providerSessionsServicePackagePath = "pkg/services/provider_sessions/service"
	providerSessionsServiceSuccessor   = "pkg/services/provider_sessions/internal"
	providerSessionsServiceDeletion    = "delete transitional service/ package after owner wire retargets to internal implementation and DEL cutover proof completes"
)

func TestMapPackageProviderSessionsServiceMoveDestination(t *testing.T) {
	t.Parallel()

	got, err := ownershipinventory.MapPackage(providerSessionsServicePackagePath)
	if err != nil {
		t.Fatalf("MapPackage(%q) error = %v", providerSessionsServicePackagePath, err)
	}
	want := ownershipinventory.PackageRow{
		PackagePath:       providerSessionsServicePackagePath,
		Disposition:       ownershipinventory.DispositionMove,
		Destination:       "provider_sessions",
		DestinationKind:   ownershipinventory.DestinationKindOwner,
		Successor:         providerSessionsServiceSuccessor,
		DeletionCondition: providerSessionsServiceDeletion,
	}
	if got != want {
		t.Fatalf("MapPackage(%q) = %#v, want %#v", providerSessionsServicePackagePath, got, want)
	}
}

func TestMapPackageProviderSessionsPrivateReadersDoNotReplaceServiceMove(t *testing.T) {
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

	serviceRow, err := ownershipinventory.MapPackage(providerSessionsServicePackagePath)
	if err != nil {
		t.Fatalf("MapPackage(%q) error = %v", providerSessionsServicePackagePath, err)
	}
	if serviceRow.Disposition != ownershipinventory.DispositionMove {
		t.Fatalf("public service/ still mapped move after private readers exist; got %#v", serviceRow)
	}
}

func TestProviderSessionsCommittedOwnershipLocksServiceMoveDestination(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	inventory, err := ownershipinventory.Load(root)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	var serviceRow *ownershipinventory.PackageRow
	for index := range inventory.Packages {
		if inventory.Packages[index].PackagePath == providerSessionsServicePackagePath {
			serviceRow = &inventory.Packages[index]
			break
		}
	}
	if serviceRow == nil {
		t.Fatalf("committed ownership inventory missing %q", providerSessionsServicePackagePath)
	}
	if serviceRow.Disposition != ownershipinventory.DispositionMove {
		t.Fatalf("committed ownership row disposition = %q, want move", serviceRow.Disposition)
	}
	if serviceRow.Destination != "provider_sessions" {
		t.Fatalf("committed ownership row destination = %q, want provider_sessions", serviceRow.Destination)
	}
	if serviceRow.Successor != providerSessionsServiceSuccessor {
		t.Fatalf("committed ownership row successor = %q, want %s", serviceRow.Successor, providerSessionsServiceSuccessor)
	}
	if serviceRow.DeletionCondition == "" {
		t.Fatal("committed ownership row missing deletionCondition")
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
