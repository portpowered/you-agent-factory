package ownershipinventory_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

func TestOperatorSettingsRootGoFilesMatchCommittedInventory(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	live, err := ownershipinventory.ListOperatorSettingsRootGoFiles(root)
	if err != nil {
		t.Fatalf("ListOperatorSettingsRootGoFiles() error = %v", err)
	}

	want := ownershipinventory.OperatorSettingsRootContractInventory()
	if !slices.Equal(live, want) {
		t.Fatalf("live root .go files = %v, want committed inventory %v", live, want)
	}
}

func TestOperatorSettingsThinRootContractFiles(t *testing.T) {
	t.Parallel()

	want := []string{
		"config_document.go",
		"doc.go",
		"document_contract.go",
		"resolution_contract.go",
		"service_contract.go",
		"root_contract_legacy_preservation_test.go",
		"service_root_contract_invariants_test.go",
	}
	if !slices.Equal(ownershipinventory.OperatorSettingsThinRootContractFiles, want) {
		t.Fatalf("OperatorSettingsThinRootContractFiles = %v, want %v", ownershipinventory.OperatorSettingsThinRootContractFiles, want)
	}

	for _, fileName := range ownershipinventory.OperatorSettingsThinRootContractFiles {
		kind, _, ok := ownershipinventory.ClassifyOperatorSettingsRootContractFile(fileName)
		if !ok {
			t.Fatalf("ClassifyOperatorSettingsRootContractFile(%q) ok = false", fileName)
		}
		if kind != "thin_root_retain" {
			t.Fatalf("ClassifyOperatorSettingsRootContractFile(%q) = %q, want thin_root_retain", fileName, kind)
		}
	}
}

func TestOperatorSettingsExcessRootContractFoldDestinations(t *testing.T) {
	t.Parallel()

	want := map[string]string{
		"identity_input_index_inventory":     "pkg/services/operator_settings/internal",
		"resolution_composition":             "pkg/services/operator_settings/internal/services/resolution",
		"providers_root_construction":        "pkg/services/operator_settings/internal",
		"construction_ports":                 "pkg/services/operator_settings/internal",
		"defaults_resolution_implementation": "pkg/services/operator_settings/internal/services/resolution",
	}

	for _, target := range ownershipinventory.OperatorSettingsExcessRootContractFolds {
		wantDestination, ok := want[target.Cluster]
		if !ok {
			t.Fatalf("unexpected fold cluster %q", target.Cluster)
		}
		if target.Destination != wantDestination {
			t.Fatalf("cluster %q destination = %q, want %q", target.Cluster, target.Destination, wantDestination)
		}
		if len(target.Files) == 0 {
			t.Fatalf("cluster %q has no inventoried files", target.Cluster)
		}
		if !ownershipinventory.IsOperatorSettingsPrivateRootContractFoldDestination(target.Destination) {
			t.Fatalf("cluster %q destination = %q, want private fold path under operator_settings/internal", target.Cluster, target.Destination)
		}
		if !strings.HasPrefix(ownershipinventory.OperatorSettingsRootContractFoldCondition(target.Cluster), "CLN-SET-CONTRACT-ROOTS") {
			t.Fatalf("fold condition for %q missing CLN-SET-CONTRACT-ROOTS prefix", target.Cluster)
		}
	}

	gotClusters := make([]string, 0, len(ownershipinventory.OperatorSettingsExcessRootContractFolds))
	for _, target := range ownershipinventory.OperatorSettingsExcessRootContractFolds {
		gotClusters = append(gotClusters, target.Cluster)
	}
	slices.Sort(gotClusters)
	wantClusters := []string{
		"construction_ports",
		"defaults_resolution_implementation",
		"identity_input_index_inventory",
		"providers_root_construction",
		"resolution_composition",
	}
	if !slices.Equal(gotClusters, wantClusters) {
		t.Fatalf("fold clusters = %v, want %v", gotClusters, wantClusters)
	}
}

func TestOperatorSettingsExcessRootContractFoldDestinationsRejectOwnerRootRetain(t *testing.T) {
	t.Parallel()

	const ownerRoot = "pkg/services/operator_settings"
	for _, target := range ownershipinventory.OperatorSettingsExcessRootContractFolds {
		if target.Destination == ownerRoot {
			t.Fatalf("cluster %q folds to owner root retain destination", target.Cluster)
		}
		if !ownershipinventory.IsOperatorSettingsPrivateRootContractFoldDestination(target.Destination) {
			t.Fatalf("cluster %q destination = %q, want private fold path under %s/internal", target.Cluster, target.Destination, ownerRoot)
		}
		for _, fileName := range target.Files {
			kind, foldTarget, ok := ownershipinventory.ClassifyOperatorSettingsRootContractFile(fileName)
			if !ok {
				t.Fatalf("ClassifyOperatorSettingsRootContractFile(%q) ok = false", fileName)
			}
			if kind != "excess_fold" {
				t.Fatalf("ClassifyOperatorSettingsRootContractFile(%q) = %q, want excess_fold", fileName, kind)
			}
			if foldTarget.Destination == ownerRoot {
				t.Fatalf("excess fold file %q regressed to owner root retain destination", fileName)
			}
			if !ownershipinventory.IsOperatorSettingsPrivateRootContractFoldDestination(foldTarget.Destination) {
				t.Fatalf("excess fold file %q destination = %q, want private fold path under %s/internal", fileName, foldTarget.Destination, ownerRoot)
			}
		}
	}
}

func TestOperatorSettingsRootContractClassificationPartitionsLiveTree(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	live, err := ownershipinventory.ListOperatorSettingsRootGoFiles(root)
	if err != nil {
		t.Fatalf("ListOperatorSettingsRootGoFiles() error = %v", err)
	}

	for _, fileName := range live {
		kind, foldTarget, ok := ownershipinventory.ClassifyOperatorSettingsRootContractFile(fileName)
		if !ok {
			t.Fatalf("live root .go file %q is not classified", fileName)
		}
		switch kind {
		case "thin_root_retain":
			if foldTarget.Cluster != "" {
				t.Fatalf("thin root file %q carries fold cluster %q", fileName, foldTarget.Cluster)
			}
		case "excess_fold":
			if foldTarget.Cluster == "" || foldTarget.Destination == "" {
				t.Fatalf("excess fold file %q missing cluster/destination: %#v", fileName, foldTarget)
			}
		default:
			t.Fatalf("live root .go file %q has unknown kind %q", fileName, kind)
		}
	}
}

func TestVerifyOperatorSettingsCommittedRootContractInventoryAlignmentPassesOnRepository(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	if err := ownershipinventory.VerifyOperatorSettingsCommittedRootContractInventoryAlignment(root); err != nil {
		t.Fatalf("VerifyOperatorSettingsCommittedRootContractInventoryAlignment() error = %v", err)
	}
}

func TestVerifyOperatorSettingsRootReconciliationPassesOnRepository(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	if err := ownershipinventory.VerifyOperatorSettingsRootReconciliation(root); err != nil {
		t.Fatalf("VerifyOperatorSettingsRootReconciliation() error = %v", err)
	}
}
