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

func TestVerifyOperatorSettingsRootGoInventoryPassesOnRepository(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	if err := ownershipinventory.VerifyOperatorSettingsRootGoInventory(root); err != nil {
		t.Fatalf("VerifyOperatorSettingsRootGoInventory() error = %v", err)
	}
}

func TestVerifyOperatorSettingsRootGoInventoryFailsWhenLiveFileMissingFromInventory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeOperatorSettingsRootGoInventoryFixture(t, root, operatorSettingsRootGoInventoryFixture{
		clusters: []ownershipinventory.OperatorSettingsRootGoCluster{
			{
				Cluster:     "construction_ports",
				Destination: "pkg/services/operator_settings/internal",
				Files:       []string{"dependencies.go"},
			},
		},
		files: []ownershipinventory.OperatorSettingsRootGoFile{
			{File: "dependencies.go", Classification: ownershipinventory.OperatorSettingsRootGoFoldTargetConstruction, FoldDestination: "pkg/services/operator_settings/internal", Cluster: "construction_ports"},
			{File: "doc.go", Classification: ownershipinventory.OperatorSettingsRootGoThinContract},
			{File: "service_contract.go", Classification: ownershipinventory.OperatorSettingsRootGoThinContract},
		},
	})
	serviceRoot := filepath.Join(root, "pkg/services/operator_settings")
	mkdirAll(t, serviceRoot)
	for _, name := range []string{"dependencies.go", "doc.go", "service_contract.go", "surprise.go"} {
		writeFile(t, filepath.Join(serviceRoot, name), "package operatorsettings\n")
	}

	err := ownershipinventory.VerifyOperatorSettingsRootGoInventory(root)
	if err == nil {
		t.Fatal("VerifyOperatorSettingsRootGoInventory() error = nil, want drift failure")
	}
	if !strings.Contains(err.Error(), "drift") {
		t.Fatalf("VerifyOperatorSettingsRootGoInventory() error = %v, want drift failure", err)
	}
}

func TestVerifyOperatorSettingsRootGoInventoryFailsWhenFoldTargetMissingDestination(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeOperatorSettingsRootGoInventoryFixture(t, root, operatorSettingsRootGoInventoryFixture{
		clusters: []ownershipinventory.OperatorSettingsRootGoCluster{
			{
				Cluster:     "construction_ports",
				Destination: "pkg/services/operator_settings/internal",
				Files:       []string{"dependencies.go"},
			},
		},
		files: []ownershipinventory.OperatorSettingsRootGoFile{
			{
				File:           "dependencies.go",
				Classification: ownershipinventory.OperatorSettingsRootGoFoldTargetConstruction,
				Cluster:        "construction_ports",
			},
		},
	})
	writeFile(t, filepath.Join(root, "pkg/services/operator_settings/dependencies.go"), "package operatorsettings\n")

	err := ownershipinventory.VerifyOperatorSettingsRootGoInventory(root)
	if err == nil {
		t.Fatal("VerifyOperatorSettingsRootGoInventory() error = nil, want foldDestination failure")
	}
	if !strings.Contains(err.Error(), "foldDestination") {
		t.Fatalf("VerifyOperatorSettingsRootGoInventory() error = %v, want foldDestination failure", err)
	}
}

func TestListOperatorSettingsRootGoFilesMatchesCommittedInventory(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	live, err := ownershipinventory.ListOperatorSettingsRootGoFiles(root)
	if err != nil {
		t.Fatalf("ListOperatorSettingsRootGoFiles() error = %v", err)
	}
	inventory, err := ownershipinventory.LoadOperatorSettingsRootGoInventory(root)
	if err != nil {
		t.Fatalf("LoadOperatorSettingsRootGoInventory() error = %v", err)
	}

	committed := make([]string, 0, len(inventory.Files))
	for _, file := range inventory.Files {
		committed = append(committed, file.File)
	}
	if !slices.Equal(live, committed) {
		t.Fatalf("live root .go files = %v, committed inventory = %v", live, committed)
	}
}

func TestOperatorSettingsRootGoInventoryClassifiesThinRootContractSurfaces(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	inventory, err := ownershipinventory.LoadOperatorSettingsRootGoInventory(root)
	if err != nil {
		t.Fatalf("LoadOperatorSettingsRootGoInventory() error = %v", err)
	}

	wantThin := []string{
		"doc.go",
		"document_contract.go",
		"resolution_contract.go",
		"service_contract.go",
	}
	var gotThin []string
	for _, file := range inventory.Files {
		if file.Classification == ownershipinventory.OperatorSettingsRootGoThinContract {
			gotThin = append(gotThin, file.File)
		}
	}
	slices.Sort(gotThin)
	if !slices.Equal(gotThin, wantThin) {
		t.Fatalf("thin root contract files = %v, want %v", gotThin, wantThin)
	}
}

func TestOperatorSettingsRootGoInventoryNamesExcessFoldClusters(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	inventory, err := ownershipinventory.LoadOperatorSettingsRootGoInventory(root)
	if err != nil {
		t.Fatalf("LoadOperatorSettingsRootGoInventory() error = %v", err)
	}

	want := map[string]string{
		"document_construction_bridge":        "pkg/services/operator_settings/internal/services/document",
		"identity_input_index_inventory":        "pkg/services/operator_settings/internal",
		"resolution_composition":                "pkg/services/operator_settings/internal/services/resolution",
		"providers_root_construction":           "pkg/services/operator_settings/internal",
		"construction_ports":                    "pkg/services/operator_settings/internal",
		"defaults_resolution_implementation":    "pkg/services/operator_settings/internal/services/resolution",
	}

	for _, cluster := range inventory.Clusters {
		wantDestination, ok := want[cluster.Cluster]
		if !ok {
			t.Fatalf("unexpected fold cluster %q", cluster.Cluster)
		}
		if cluster.Destination != wantDestination {
			t.Fatalf("cluster %q destination = %q, want %q", cluster.Cluster, cluster.Destination, wantDestination)
		}
		if len(cluster.Files) == 0 {
			t.Fatalf("cluster %q has no inventoried files", cluster.Cluster)
		}
		if !strings.HasPrefix(ownershipinventory.OperatorSettingsRootContractFoldCondition(cluster.Cluster), "CLN-SET-CONTRACT-ROOTS") {
			t.Fatalf("fold condition for %q missing CLN-SET-CONTRACT-ROOTS prefix", cluster.Cluster)
		}
	}

	gotClusters := make([]string, 0, len(inventory.Clusters))
	for _, cluster := range inventory.Clusters {
		gotClusters = append(gotClusters, cluster.Cluster)
	}
	slices.Sort(gotClusters)
	wantClusters := []string{
		"construction_ports",
		"defaults_resolution_implementation",
		"document_construction_bridge",
		"identity_input_index_inventory",
		"providers_root_construction",
		"resolution_composition",
	}
	if !slices.Equal(gotClusters, wantClusters) {
		t.Fatalf("fold clusters = %v, want %v", gotClusters, wantClusters)
	}
}

func TestOperatorSettingsRootGoInventoryDistinguishesThinContractTestsFromImplementationFoldTargets(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	inventory, err := ownershipinventory.LoadOperatorSettingsRootGoInventory(root)
	if err != nil {
		t.Fatalf("LoadOperatorSettingsRootGoInventory() error = %v", err)
	}

	wantThinTests := []string{
		"root_contract_legacy_preservation_test.go",
		"service_root_contract_invariants_test.go",
	}
	wantFoldTests := []string{
		"atomic_config_test.go",
		"dependencies_test.go",
		"document_characterization_test.go",
		"document_routing_test.go",
		"identity_persist_test.go",
		"identity_test.go",
		"input_inventory_test.go",
		"operator_config_test.go",
		"resolution_characterization_test.go",
		"service_characterization_test.go",
		"testmain_test.go",
	}

	var gotThinTests []string
	var gotFoldTests []string
	for _, file := range inventory.Files {
		switch file.Classification {
		case ownershipinventory.OperatorSettingsRootGoThinContractTest:
			gotThinTests = append(gotThinTests, file.File)
		case ownershipinventory.OperatorSettingsRootGoFoldTargetImplTest:
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

func TestOperatorSettingsRootGoFoldTargetsUsePrivateDestinations(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	inventory, err := ownershipinventory.LoadOperatorSettingsRootGoInventory(root)
	if err != nil {
		t.Fatalf("LoadOperatorSettingsRootGoInventory() error = %v", err)
	}

	const ownerRoot = "pkg/services/operator_settings"
	for _, target := range ownershipinventory.OperatorSettingsRootGoFoldTargets(inventory) {
		if target.FoldDestination == "" {
			t.Fatalf("fold target %q missing foldDestination", target.File)
		}
		if target.FoldDestination == ownerRoot {
			t.Fatalf("fold target %q regressed to owner root retain destination", target.File)
		}
		if !strings.HasPrefix(target.FoldDestination, ownerRoot+"/internal") {
			t.Fatalf("fold target %q destination %q outside operator_settings private homes", target.File, target.FoldDestination)
		}
	}
}

type operatorSettingsRootGoInventoryFixture struct {
	clusters []ownershipinventory.OperatorSettingsRootGoCluster
	files    []ownershipinventory.OperatorSettingsRootGoFile
}

func writeOperatorSettingsRootGoInventoryFixture(t *testing.T, root string, fixture operatorSettingsRootGoInventoryFixture) {
	t.Helper()

	payload, err := json.MarshalIndent(ownershipinventory.OperatorSettingsRootGoInventory{
		FormatVersion: "pss-operator-settings-root-go-inventory/v1",
		OwnerPackage:  ownershipinventory.OperatorSettingsOwnerPackagePath,
		SortKey:       "file name ascending byte order",
		Clusters:      fixture.clusters,
		Files:         fixture.files,
	}, "", "  ")
	if err != nil {
		t.Fatalf("marshal operator settings root go inventory fixture: %v", err)
	}
	path := filepath.Join(root, filepath.FromSlash(ownershipinventory.OperatorSettingsRootGoInventoryRelativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir operator settings root go inventory fixture dir: %v", err)
	}
	if err := os.WriteFile(path, append(payload, '\n'), 0o644); err != nil {
		t.Fatalf("write operator settings root go inventory fixture: %v", err)
	}
}
