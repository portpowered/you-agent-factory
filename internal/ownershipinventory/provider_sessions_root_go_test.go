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

func TestVerifyProviderSessionsRootGoInventoryPassesOnRepository(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	if err := ownershipinventory.VerifyProviderSessionsRootGoInventory(root); err != nil {
		t.Fatalf("VerifyProviderSessionsRootGoInventory() error = %v", err)
	}
}

func TestVerifyProviderSessionsRootGoInventoryFailsWhenLiveFileMissingFromInventory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeProviderSessionsRootGoInventoryFixture(t, root, providerSessionsRootGoInventoryFixture{
		files: []ownershipinventory.ProviderSessionsRootGoFile{
			{File: "contracts.go", Classification: ownershipinventory.ProviderSessionsRootGoThinContract},
			{File: "doc.go", Classification: ownershipinventory.ProviderSessionsRootGoThinContract},
		},
	})
	serviceRoot := filepath.Join(root, "pkg/services/provider_sessions")
	mkdirAll(t, serviceRoot)
	for _, name := range []string{"contracts.go", "doc.go", "surprise.go"} {
		writeFile(t, filepath.Join(serviceRoot, name), "package providersessions\n")
	}

	err := ownershipinventory.VerifyProviderSessionsRootGoInventory(root)
	if err == nil {
		t.Fatal("VerifyProviderSessionsRootGoInventory() error = nil, want drift failure")
	}
	if !strings.Contains(err.Error(), "drift") {
		t.Fatalf("VerifyProviderSessionsRootGoInventory() error = %v, want drift failure", err)
	}
}

func TestVerifyProviderSessionsRootGoInventoryFailsWhenFoldTargetMissingDestination(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeProviderSessionsRootGoInventoryFixture(t, root, providerSessionsRootGoInventoryFixture{
		files: []ownershipinventory.ProviderSessionsRootGoFile{
			{
				File:           "construction_ports.go",
				Classification: ownershipinventory.ProviderSessionsRootGoFoldTargetConstruction,
			},
		},
	})
	writeFile(t, filepath.Join(root, "pkg/services/provider_sessions/construction_ports.go"), "package providersessions\n")

	err := ownershipinventory.VerifyProviderSessionsRootGoInventory(root)
	if err == nil {
		t.Fatal("VerifyProviderSessionsRootGoInventory() error = nil, want foldDestination failure")
	}
	if !strings.Contains(err.Error(), "foldDestination") {
		t.Fatalf("VerifyProviderSessionsRootGoInventory() error = %v, want foldDestination failure", err)
	}
}

func TestListProviderSessionsRootGoFilesMatchesCommittedInventory(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	live, err := ownershipinventory.ListProviderSessionsRootGoFiles(root)
	if err != nil {
		t.Fatalf("ListProviderSessionsRootGoFiles() error = %v", err)
	}
	inventory, err := ownershipinventory.LoadProviderSessionsRootGoInventory(root)
	if err != nil {
		t.Fatalf("LoadProviderSessionsRootGoInventory() error = %v", err)
	}

	committed := make([]string, 0, len(inventory.Files))
	for _, file := range inventory.Files {
		committed = append(committed, file.File)
	}
	if !slices.Equal(live, committed) {
		t.Fatalf("live root .go files = %v, committed inventory = %v", live, committed)
	}
}

func TestProviderSessionsRootGoInventoryClassifiesThinRootContractSurfaces(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	inventory, err := ownershipinventory.LoadProviderSessionsRootGoInventory(root)
	if err != nil {
		t.Fatalf("LoadProviderSessionsRootGoInventory() error = %v", err)
	}

	wantThin := []string{"contracts.go", "doc.go"}
	var gotThin []string
	for _, file := range inventory.Files {
		if file.Classification == ownershipinventory.ProviderSessionsRootGoThinContract {
			gotThin = append(gotThin, file.File)
		}
	}
	slices.Sort(gotThin)
	if !slices.Equal(gotThin, wantThin) {
		t.Fatalf("thin root contract files = %v, want %v", gotThin, wantThin)
	}
}

func TestProviderSessionsRootGoInventoryNamesConstructionPortsFoldTarget(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	inventory, err := ownershipinventory.LoadProviderSessionsRootGoInventory(root)
	if err != nil {
		t.Fatalf("LoadProviderSessionsRootGoInventory() error = %v", err)
	}

	targets := ownershipinventory.ProviderSessionsRootGoFoldTargets(inventory)
	var construction *ownershipinventory.ProviderSessionsRootGoFile
	for index := range targets {
		if targets[index].File == "construction_ports.go" {
			construction = &targets[index]
			break
		}
	}
	if construction == nil {
		t.Fatal("fold targets missing construction_ports.go")
	}
	if construction.Classification != ownershipinventory.ProviderSessionsRootGoFoldTargetConstruction {
		t.Fatalf("construction_ports.go classification = %q, want %q", construction.Classification, ownershipinventory.ProviderSessionsRootGoFoldTargetConstruction)
	}
	if construction.FoldDestination != "pkg/services/provider_sessions/internal" {
		t.Fatalf("construction_ports.go foldDestination = %q, want pkg/services/provider_sessions/internal", construction.FoldDestination)
	}
}

func TestProviderSessionsRootGoInventoryDistinguishesThinContractTestsFromImplementationFoldTargets(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	inventory, err := ownershipinventory.LoadProviderSessionsRootGoInventory(root)
	if err != nil {
		t.Fatalf("LoadProviderSessionsRootGoInventory() error = %v", err)
	}

	wantThinTests := []string{
		"providers_import_boundary_test.go",
		"root_contracts_providers_boundary_test.go",
		"service_root_contract_test.go",
	}
	wantFoldTests := []string{
		"details_providers_boundary_test.go",
		"inspect_providers_boundary_test.go",
		"project_providers_boundary_test.go",
		"readers_providers_boundary_test.go",
		"service_test.go",
	}

	var gotThinTests []string
	var gotFoldTests []string
	for _, file := range inventory.Files {
		switch file.Classification {
		case ownershipinventory.ProviderSessionsRootGoThinContractTest:
			gotThinTests = append(gotThinTests, file.File)
		case ownershipinventory.ProviderSessionsRootGoFoldTargetImplTest:
			gotFoldTests = append(gotFoldTests, file.File)
		}
	}
	slices.Sort(gotThinTests)
	slices.Sort(gotFoldTests)
	if !slices.Equal(gotThinTests, wantThinTests) {
		t.Fatalf("thin root contract tests = %v, want %v", gotThinTests, wantThinTests)
	}
	if !slices.Equal(gotFoldTests, wantFoldTests) {
		t.Fatalf("implementation fold test targets = %v, want %v", gotFoldTests, wantFoldTests)
	}
}

func TestProviderSessionsRootGoFoldTargetsUsePrivateDestinations(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	inventory, err := ownershipinventory.LoadProviderSessionsRootGoInventory(root)
	if err != nil {
		t.Fatalf("LoadProviderSessionsRootGoInventory() error = %v", err)
	}

	for _, target := range ownershipinventory.ProviderSessionsRootGoFoldTargets(inventory) {
		if target.FoldDestination == "" {
			t.Fatalf("fold target %q missing foldDestination", target.File)
		}
		if !strings.HasPrefix(target.FoldDestination, "pkg/services/provider_sessions/internal") {
			t.Fatalf("fold target %q destination %q outside provider_sessions private homes", target.File, target.FoldDestination)
		}
	}
}

type providerSessionsRootGoInventoryFixture struct {
	files []ownershipinventory.ProviderSessionsRootGoFile
}

func writeProviderSessionsRootGoInventoryFixture(t *testing.T, root string, fixture providerSessionsRootGoInventoryFixture) {
	t.Helper()

	payload, err := json.MarshalIndent(ownershipinventory.ProviderSessionsRootGoInventory{
		FormatVersion: "pss-provider-sessions-root-go-inventory/v1",
		OwnerPackage:  ownershipinventory.ProviderSessionsOwnerPackagePath,
		SortKey:       "file name ascending byte order",
		Files:         fixture.files,
	}, "", "  ")
	if err != nil {
		t.Fatalf("marshal provider sessions root go inventory fixture: %v", err)
	}
	path := filepath.Join(root, filepath.FromSlash(ownershipinventory.ProviderSessionsRootGoInventoryRelativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir provider sessions root go inventory fixture dir: %v", err)
	}
	if err := os.WriteFile(path, append(payload, '\n'), 0o644); err != nil {
		t.Fatalf("write provider sessions root go inventory fixture: %v", err)
	}
}

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
