package ownershipinventory_test

import (
	"slices"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

func TestVerifyProviderSessionsTopLevelInventoryPassesOnRepository(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	if err := ownershipinventory.VerifyProviderSessionsTopLevelInventory(root); err != nil {
		t.Fatalf("VerifyProviderSessionsTopLevelInventory() error = %v", err)
	}
}

func TestListProviderSessionsTopLevelDirectoriesMatchesCommittedInventory(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	live, err := ownershipinventory.ListProviderSessionsTopLevelDirectories(root)
	if err != nil {
		t.Fatalf("ListProviderSessionsTopLevelDirectories() error = %v", err)
	}
	inventory, err := ownershipinventory.LoadProviderSessionsTopLevelInventory(root)
	if err != nil {
		t.Fatalf("LoadProviderSessionsTopLevelInventory() error = %v", err)
	}

	committed := make([]string, 0, len(inventory.Children))
	for _, child := range inventory.Children {
		committed = append(committed, child.Directory)
	}
	if !slices.Equal(live, committed) {
		t.Fatalf("live top-level directories = %v, committed inventory = %v", live, committed)
	}
}

func TestProviderSessionsTopLevelInventoryClassifiesCanonicalRetainChildren(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	inventory, err := ownershipinventory.LoadProviderSessionsTopLevelInventory(root)
	if err != nil {
		t.Fatalf("LoadProviderSessionsTopLevelInventory() error = %v", err)
	}

	wantCanonical := []string{"internal", "transports", "wire"}
	var gotCanonical []string
	for _, child := range inventory.Children {
		if child.Classification == ownershipinventory.ProviderSessionsTopLevelCanonicalRetain {
			gotCanonical = append(gotCanonical, child.Directory)
		}
	}
	slices.Sort(gotCanonical)
	if !slices.Equal(gotCanonical, wantCanonical) {
		t.Fatalf("canonical retain directories = %v, want %v", gotCanonical, wantCanonical)
	}
}

func TestProviderSessionsTopLevelInventoryClassifiesUnexpectedService(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	inventory, err := ownershipinventory.LoadProviderSessionsTopLevelInventory(root)
	if err != nil {
		t.Fatalf("LoadProviderSessionsTopLevelInventory() error = %v", err)
	}

	var unexpected []string
	for _, child := range inventory.Children {
		if child.Classification == ownershipinventory.ProviderSessionsTopLevelUnexpectedPublicSibling ||
			child.Classification == ownershipinventory.ProviderSessionsTopLevelINVUnexpectedPublicSibling {
			unexpected = append(unexpected, child.Directory)
		}
	}
	if !slices.Equal(unexpected, []string{"service"}) {
		t.Fatalf("unexpected public siblings = %v, want [service]", unexpected)
	}
}

func TestProviderSessionsTopLevelInventoryRecordsNoUnexpectedSiblingsBeyondService(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	inventory, err := ownershipinventory.LoadProviderSessionsTopLevelInventory(root)
	if err != nil {
		t.Fatalf("LoadProviderSessionsTopLevelInventory() error = %v", err)
	}
	if inventory.HasUnexpectedPublicSiblingsBeyondService {
		t.Fatalf("hasUnexpectedPublicSiblingsBeyondService = true, want false")
	}
	if len(inventory.UnexpectedPublicSiblingsBeyondService) != 0 {
		t.Fatalf("unexpectedPublicSiblingsBeyondService = %v, want none", inventory.UnexpectedPublicSiblingsBeyondService)
	}
}
