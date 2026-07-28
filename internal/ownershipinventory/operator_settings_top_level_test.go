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

func TestVerifyOperatorSettingsTopLevelInventoryPassesOnRepository(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	if err := ownershipinventory.VerifyOperatorSettingsTopLevelInventory(root); err != nil {
		t.Fatalf("VerifyOperatorSettingsTopLevelInventory() error = %v", err)
	}
}

func TestVerifyOperatorSettingsTopLevelInventoryFailsWhenLiveDirectoryMissingFromInventory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeOperatorSettingsTopLevelInventoryFixture(t, root, operatorSettingsTopLevelInventoryFixture{
		children: []ownershipinventory.OperatorSettingsTopLevelChild{
			{Directory: "internal", Classification: ownershipinventory.OperatorSettingsTopLevelCanonicalRetain},
			{Directory: "transports", Classification: ownershipinventory.OperatorSettingsTopLevelCanonicalRetain},
			{Directory: "wire", Classification: ownershipinventory.OperatorSettingsTopLevelCanonicalRetain},
		},
	})
	for _, directory := range []string{"internal", "surprise", "transports", "wire"} {
		mkdirAll(t, filepath.Join(root, "pkg/services/operator_settings", directory))
	}

	err := ownershipinventory.VerifyOperatorSettingsTopLevelInventory(root)
	if err == nil {
		t.Fatal("VerifyOperatorSettingsTopLevelInventory() error = nil, want drift failure")
	}
	if !strings.Contains(err.Error(), "drift") {
		t.Fatalf("VerifyOperatorSettingsTopLevelInventory() error = %v, want drift failure", err)
	}
}

func TestVerifyOperatorSettingsTopLevelInventoryFailsWhenUnexpectedSiblingsUnrecorded(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeOperatorSettingsTopLevelInventoryFixture(t, root, operatorSettingsTopLevelInventoryFixture{
		children: []ownershipinventory.OperatorSettingsTopLevelChild{
			{Directory: "identityinventory", Classification: ownershipinventory.OperatorSettingsTopLevelUnexpectedPublicSibling},
			{Directory: "internal", Classification: ownershipinventory.OperatorSettingsTopLevelCanonicalRetain},
			{Directory: "servicewire", Classification: ownershipinventory.OperatorSettingsTopLevelUnexpectedPublicSibling},
			{Directory: "transports", Classification: ownershipinventory.OperatorSettingsTopLevelCanonicalRetain},
			{Directory: "wire", Classification: ownershipinventory.OperatorSettingsTopLevelCanonicalRetain},
		},
		unexpectedPublicSiblings: []string{"identityinventory"},
	})
	for _, directory := range []string{"identityinventory", "internal", "servicewire", "transports", "wire"} {
		mkdirAll(t, filepath.Join(root, "pkg/services/operator_settings", directory))
	}

	err := ownershipinventory.VerifyOperatorSettingsTopLevelInventory(root)
	if err == nil {
		t.Fatal("VerifyOperatorSettingsTopLevelInventory() error = nil, want unexpected sibling list failure")
	}
	if !strings.Contains(err.Error(), "unexpectedPublicSiblings") {
		t.Fatalf("VerifyOperatorSettingsTopLevelInventory() error = %v, want unexpected sibling list failure", err)
	}
}

func TestListOperatorSettingsTopLevelDirectoriesMatchesCommittedInventory(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	live, err := ownershipinventory.ListOperatorSettingsTopLevelDirectories(root)
	if err != nil {
		t.Fatalf("ListOperatorSettingsTopLevelDirectories() error = %v", err)
	}
	inventory, err := ownershipinventory.LoadOperatorSettingsTopLevelInventory(root)
	if err != nil {
		t.Fatalf("LoadOperatorSettingsTopLevelInventory() error = %v", err)
	}

	committed := make([]string, 0, len(inventory.Children))
	for _, child := range inventory.Children {
		committed = append(committed, child.Directory)
	}
	if !slices.Equal(live, committed) {
		t.Fatalf("live top-level directories = %v, committed inventory = %v", live, committed)
	}
}

func TestOperatorSettingsTopLevelInventoryClassifiesCanonicalRetainChildren(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	inventory, err := ownershipinventory.LoadOperatorSettingsTopLevelInventory(root)
	if err != nil {
		t.Fatalf("LoadOperatorSettingsTopLevelInventory() error = %v", err)
	}

	wantCanonical := []string{"internal", "transports", "wire"}
	var gotCanonical []string
	for _, child := range inventory.Children {
		if child.Classification == ownershipinventory.OperatorSettingsTopLevelCanonicalRetain {
			gotCanonical = append(gotCanonical, child.Directory)
		}
	}
	slices.Sort(gotCanonical)
	if !slices.Equal(gotCanonical, wantCanonical) {
		t.Fatalf("canonical retain directories = %v, want %v", gotCanonical, wantCanonical)
	}
}

func TestOperatorSettingsTopLevelInventoryClassifiesUnexpectedPublicSiblings(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	inventory, err := ownershipinventory.LoadOperatorSettingsTopLevelInventory(root)
	if err != nil {
		t.Fatalf("LoadOperatorSettingsTopLevelInventory() error = %v", err)
	}

	wantUnexpected := []string{"identityinventory", "servicewire", "testlink", "testproviders"}
	if !slices.Equal(inventory.UnexpectedPublicSiblings, wantUnexpected) {
		t.Fatalf("unexpected public siblings = %v, want %v", inventory.UnexpectedPublicSiblings, wantUnexpected)
	}
}

func TestOperatorSettingsTopLevelInventoryClassifiesTestdataAsTestOnlyRetain(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	inventory, err := ownershipinventory.LoadOperatorSettingsTopLevelInventory(root)
	if err != nil {
		t.Fatalf("LoadOperatorSettingsTopLevelInventory() error = %v", err)
	}

	var testdata *ownershipinventory.OperatorSettingsTopLevelChild
	for index := range inventory.Children {
		child := inventory.Children[index]
		if child.Directory == "testdata" {
			testdata = &inventory.Children[index]
			break
		}
	}
	if testdata == nil {
		t.Fatal("testdata child missing from committed inventory")
	}
	if testdata.Classification != ownershipinventory.OperatorSettingsTopLevelTestOnlyRetain {
		t.Fatalf("testdata classification = %q, want %q", testdata.Classification, ownershipinventory.OperatorSettingsTopLevelTestOnlyRetain)
	}
	if strings.TrimSpace(testdata.Note) == "" {
		t.Fatal("testdata test_only_retain classification requires an INV note")
	}
}

type operatorSettingsTopLevelInventoryFixture struct {
	children                 []ownershipinventory.OperatorSettingsTopLevelChild
	unexpectedPublicSiblings []string
}

func writeOperatorSettingsTopLevelInventoryFixture(t *testing.T, root string, fixture operatorSettingsTopLevelInventoryFixture) {
	t.Helper()

	unexpectedPublicSiblings := fixture.unexpectedPublicSiblings
	if unexpectedPublicSiblings == nil {
		unexpectedPublicSiblings = make([]string, 0)
		for _, child := range fixture.children {
			if child.Classification == ownershipinventory.OperatorSettingsTopLevelUnexpectedPublicSibling {
				unexpectedPublicSiblings = append(unexpectedPublicSiblings, child.Directory)
			}
		}
	}

	payload, err := json.MarshalIndent(ownershipinventory.OperatorSettingsTopLevelInventory{
		FormatVersion:            "pss-operator-settings-top-level-inventory/v1",
		OwnerPackage:             ownershipinventory.OperatorSettingsOwnerPackagePath,
		SortKey:                  "directory name ascending byte order",
		UnexpectedPublicSiblings: unexpectedPublicSiblings,
		Children:                 fixture.children,
	}, "", "  ")
	if err != nil {
		t.Fatalf("marshal operator settings top-level inventory fixture: %v", err)
	}
	path := filepath.Join(root, filepath.FromSlash(ownershipinventory.OperatorSettingsTopLevelInventoryRelativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir operator settings top-level inventory fixture dir: %v", err)
	}
	if err := os.WriteFile(path, append(payload, '\n'), 0o644); err != nil {
		t.Fatalf("write operator settings top-level inventory fixture: %v", err)
	}
}
