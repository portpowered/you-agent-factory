package ownershipinventory_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
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

func TestVerifyProviderSessionsTopLevelInventoryFailsWhenLiveDirectoryMissingFromInventory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeProviderSessionsTopLevelInventoryFixture(t, root, providerSessionsTopLevelInventoryFixture{
		children: []ownershipinventory.ProviderSessionsTopLevelChild{
			{Directory: "internal", Classification: ownershipinventory.ProviderSessionsTopLevelCanonicalRetain},
			{Directory: "transports", Classification: ownershipinventory.ProviderSessionsTopLevelCanonicalRetain},
			{Directory: "wire", Classification: ownershipinventory.ProviderSessionsTopLevelCanonicalRetain},
		},
		hasUnexpectedBeyondService: false,
	})
	for _, directory := range []string{"internal", "transports", "wire", "surprise"} {
		mkdirAll(t, filepath.Join(root, "pkg/services/provider_sessions", directory))
	}

	err := ownershipinventory.VerifyProviderSessionsTopLevelInventory(root)
	if err == nil {
		t.Fatal("VerifyProviderSessionsTopLevelInventory() error = nil, want drift failure")
	}
	if !strings.Contains(err.Error(), "drift") {
		t.Fatalf("VerifyProviderSessionsTopLevelInventory() error = %v, want drift failure", err)
	}
}

func TestVerifyProviderSessionsTopLevelInventoryFailsWhenUnexpectedSiblingBeyondServiceUnrecorded(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeProviderSessionsTopLevelInventoryFixture(t, root, providerSessionsTopLevelInventoryFixture{
		children: []ownershipinventory.ProviderSessionsTopLevelChild{
			{Directory: "internal", Classification: ownershipinventory.ProviderSessionsTopLevelCanonicalRetain},
			{Directory: "surprise", Classification: ownershipinventory.ProviderSessionsTopLevelINVUnexpectedPublicSibling},
			{Directory: "transports", Classification: ownershipinventory.ProviderSessionsTopLevelCanonicalRetain},
			{Directory: "wire", Classification: ownershipinventory.ProviderSessionsTopLevelCanonicalRetain},
		},
		hasUnexpectedBeyondService: false,
	})
	for _, directory := range []string{"internal", "surprise", "transports", "wire"} {
		mkdirAll(t, filepath.Join(root, "pkg/services/provider_sessions", directory))
	}

	err := ownershipinventory.VerifyProviderSessionsTopLevelInventory(root)
	if err == nil {
		t.Fatal("VerifyProviderSessionsTopLevelInventory() error = nil, want unexpected sibling flag failure")
	}
	if !strings.Contains(err.Error(), "hasUnexpectedPublicSiblingsBeyondService") {
		t.Fatalf("VerifyProviderSessionsTopLevelInventory() error = %v, want unexpected sibling flag failure", err)
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

func TestProviderSessionsTopLevelInventoryHasNoUnexpectedPublicSiblings(t *testing.T) {
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
	if len(unexpected) != 0 {
		t.Fatalf("unexpected public siblings = %v, want none after DEL-PSES story 002", unexpected)
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

type providerSessionsTopLevelInventoryFixture struct {
	children                   []ownershipinventory.ProviderSessionsTopLevelChild
	hasUnexpectedBeyondService bool
}

func writeProviderSessionsTopLevelInventoryFixture(t *testing.T, root string, fixture providerSessionsTopLevelInventoryFixture) {
	t.Helper()

	unexpectedBeyondService := make([]string, 0)
	for _, child := range fixture.children {
		if child.Directory == "service" {
			continue
		}
		if child.Classification == ownershipinventory.ProviderSessionsTopLevelUnexpectedPublicSibling ||
			child.Classification == ownershipinventory.ProviderSessionsTopLevelINVUnexpectedPublicSibling {
			unexpectedBeyondService = append(unexpectedBeyondService, child.Directory)
		}
	}

	payload, err := json.MarshalIndent(ownershipinventory.ProviderSessionsTopLevelInventory{
		FormatVersion:                            "pss-provider-sessions-top-level-inventory/v1",
		OwnerPackage:                             ownershipinventory.ProviderSessionsOwnerPackagePath,
		SortKey:                                  "directory name ascending byte order",
		HasUnexpectedPublicSiblingsBeyondService: fixture.hasUnexpectedBeyondService,
		UnexpectedPublicSiblingsBeyondService:    unexpectedBeyondService,
		Children:                                 fixture.children,
	}, "", "  ")
	if err != nil {
		t.Fatalf("marshal provider sessions top-level inventory fixture: %v", err)
	}
	path := filepath.Join(root, filepath.FromSlash(ownershipinventory.ProviderSessionsTopLevelInventoryRelativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir provider sessions top-level inventory fixture dir: %v", err)
	}
	if err := os.WriteFile(path, append(payload, '\n'), 0o644); err != nil {
		t.Fatalf("write provider sessions top-level inventory fixture: %v", err)
	}
}

func mkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}
