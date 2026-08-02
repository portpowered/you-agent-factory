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
		"acp_integrations.go",
		"backend_scope.go",
		"config_document.go",
		"construction_ports_contract.go",
		"defaults_contract.go",
		"defaults_resolution.go",
		"doc.go",
		"document_contract.go",
		"input_inventory_contract.go",
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

	if len(inventory.Clusters) != 0 {
		t.Fatalf("fold clusters = %#v, want empty after story-005 fold", inventory.Clusters)
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
		"acp_integrations_test.go",
		"del_set_proof_gate_test.go",
		"packaged_root_shape_test.go",
		"root_contract_legacy_preservation_test.go",
		"root_wire_behavioral_boundary_test.go",
		"service_root_contract_invariants_test.go",
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
	if len(gotFoldTests) != 0 {
		t.Fatalf("implementation fold test targets = %v, want empty after story-005 fold", gotFoldTests)
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
