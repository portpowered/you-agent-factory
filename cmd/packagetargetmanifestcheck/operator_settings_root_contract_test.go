package main

import (
	"slices"
	"strings"
	"testing"
)

func TestOperatorSettingsRootGoFilesMatchCommittedInventory(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	live, err := listOperatorSettingsRootGoFiles(repoRoot)
	if err != nil {
		t.Fatalf("listOperatorSettingsRootGoFiles() error = %v", err)
	}

	want := operatorSettingsRootContractInventory()
	if !slices.Equal(live, want) {
		t.Fatalf("live root .go files = %v, want committed inventory %v", live, want)
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

	for _, target := range operatorSettingsExcessRootContractFolds {
		wantDestination, ok := want[target.cluster]
		if !ok {
			t.Fatalf("unexpected fold cluster %q", target.cluster)
		}
		if target.destination != wantDestination {
			t.Fatalf("cluster %q destination = %q, want %q", target.cluster, target.destination, wantDestination)
		}
	}
}

func TestOperatorSettingsRootContractClassificationPartitionsLiveTree(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	live, err := listOperatorSettingsRootGoFiles(repoRoot)
	if err != nil {
		t.Fatalf("listOperatorSettingsRootGoFiles() error = %v", err)
	}

	for _, fileName := range live {
		kind, destination, ok := classifyOperatorSettingsRootContractFile(fileName)
		if !ok {
			t.Fatalf("live root .go file %q is not classified", fileName)
		}
		switch kind {
		case "thin_root_retain":
			if destination != operatorSettingsRootRelative {
				t.Fatalf("thin root file %q destination = %q, want %s", fileName, destination, operatorSettingsRootRelative)
			}
		case "excess_fold":
			if destination == "" || destination == operatorSettingsRootRelative {
				t.Fatalf("excess fold file %q destination = %q, want private subservice path", fileName, destination)
			}
			if !isOperatorSettingsPrivateRootContractFoldDestination(destination) {
				t.Fatalf("excess fold file %q destination = %q, want private fold path under %s/internal", fileName, destination, operatorSettingsRootRelative)
			}
		default:
			t.Fatalf("live root .go file %q has unknown kind %q", fileName, kind)
		}
	}
}

func TestOperatorSettingsExcessRootContractFoldDestinationsRejectOwnerRootRetain(t *testing.T) {
	t.Parallel()

	const ownerRoot = "pkg/services/operator_settings"
	for _, target := range operatorSettingsExcessRootContractFolds {
		if target.destination == ownerRoot {
			t.Fatalf("cluster %q folds to owner root retain destination", target.cluster)
		}
		if !isOperatorSettingsPrivateRootContractFoldDestination(target.destination) {
			t.Fatalf("cluster %q destination = %q, want private fold path under %s/internal", target.cluster, target.destination, ownerRoot)
		}
		for _, fileName := range target.files {
			kind, destination, ok := classifyOperatorSettingsRootContractFile(fileName)
			if !ok {
				t.Fatalf("classifyOperatorSettingsRootContractFile(%q) ok = false", fileName)
			}
			if kind != "excess_fold" {
				t.Fatalf("classifyOperatorSettingsRootContractFile(%q) = %q, want excess_fold", fileName, kind)
			}
			if destination == ownerRoot {
				t.Fatalf("excess fold file %q regressed to owner root retain destination", fileName)
			}
			if !isOperatorSettingsPrivateRootContractFoldDestination(destination) {
				t.Fatalf("excess fold file %q destination = %q, want private fold path under %s/internal", fileName, destination, ownerRoot)
			}
		}
	}
}

func TestOperatorSettingsThinRootContractFilesClassifyAsRetain(t *testing.T) {
	t.Parallel()

	for _, fileName := range operatorSettingsThinRootContractFiles {
		kind, destination, ok := classifyOperatorSettingsRootContractFile(fileName)
		if !ok {
			t.Fatalf("classifyOperatorSettingsRootContractFile(%q) ok = false", fileName)
		}
		if kind != "thin_root_retain" {
			t.Fatalf("classifyOperatorSettingsRootContractFile(%q) = %q, want thin_root_retain", fileName, kind)
		}
		if destination != operatorSettingsRootRelative {
			t.Fatalf("classifyOperatorSettingsRootContractFile(%q) destination = %q, want %s", fileName, destination, operatorSettingsRootRelative)
		}
	}
}

func TestOperatorSettingsExcessRootContractFoldClustersMatchInventoryNote(t *testing.T) {
	t.Parallel()

	gotClusters := make([]string, 0, len(operatorSettingsExcessRootContractFolds))
	for _, target := range operatorSettingsExcessRootContractFolds {
		gotClusters = append(gotClusters, target.cluster)
		if len(target.files) == 0 {
			t.Fatalf("cluster %q has no inventoried files", target.cluster)
		}
		if !strings.HasPrefix(target.destination, operatorSettingsRootRelative+"/internal") {
			t.Fatalf("cluster %q destination = %q, want path under %s/internal", target.cluster, target.destination, operatorSettingsRootRelative)
		}
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
