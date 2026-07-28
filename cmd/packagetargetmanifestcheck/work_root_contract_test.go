package main

import (
	"slices"
	"strings"
	"testing"
)

func TestWorkRootGoFilesMatchCommittedInventory(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	live, err := listWorkRootGoFiles(repoRoot)
	if err != nil {
		t.Fatalf("listWorkRootGoFiles() error = %v", err)
	}

	want := workRootContractInventory()
	if !slices.Equal(live, want) {
		t.Fatalf("live root .go files = %v, want committed inventory %v", live, want)
	}
}

func TestWorkExcessRootContractFoldDestinations(t *testing.T) {
	t.Parallel()

	want := map[string]string{
		"request_admission":            "pkg/services/work/internal",
		"invocation_return_policy":     "pkg/services/work/internal",
		"lineage_graph_modules":        "pkg/services/work/internal/services/state_access",
		"state_access_query":           "pkg/services/work/internal/services/state_access",
		"content_staging_impl":         "pkg/services/work/internal/services/content_staging",
		"content_materialization_impl": "pkg/services/work/internal/services/content_materialization",
	}

	for _, target := range workExcessRootContractFolds {
		wantDestination, ok := want[target.cluster]
		if !ok {
			t.Fatalf("unexpected fold cluster %q", target.cluster)
		}
		if target.destination != wantDestination {
			t.Fatalf("cluster %q destination = %q, want %q", target.cluster, target.destination, wantDestination)
		}
		if !isWorkPrivateRootContractFoldDestination(target.destination) {
			t.Fatalf("cluster %q destination = %q, want private fold path under pkg/services/work/internal", target.cluster, target.destination)
		}
	}
}

func TestWorkExcessRootContractFoldDestinationsRejectOwnerRootRetain(t *testing.T) {
	t.Parallel()

	const ownerRoot = "pkg/services/work"
	for _, target := range workExcessRootContractFolds {
		if target.destination == ownerRoot {
			t.Fatalf("cluster %q folds to owner root retain destination", target.cluster)
		}
		if !isWorkPrivateRootContractFoldDestination(target.destination) {
			t.Fatalf("cluster %q destination = %q, want private fold path under %s/internal", target.cluster, target.destination, ownerRoot)
		}
		for _, fileName := range target.files {
			kind, destination, ok := classifyWorkRootContractFile(fileName)
			if !ok {
				t.Fatalf("classifyWorkRootContractFile(%q) ok = false", fileName)
			}
			if kind != "excess_fold" {
				t.Fatalf("classifyWorkRootContractFile(%q) = %q, want excess_fold", fileName, kind)
			}
			if destination == ownerRoot {
				t.Fatalf("excess fold file %q regressed to owner root retain destination", fileName)
			}
			if !isWorkPrivateRootContractFoldDestination(destination) {
				t.Fatalf("excess fold file %q destination = %q, want private fold path under %s/internal", fileName, destination, ownerRoot)
			}
		}
	}
}

func TestWorkRootContractClassificationPartitionsLiveTree(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	live, err := listWorkRootGoFiles(repoRoot)
	if err != nil {
		t.Fatalf("listWorkRootGoFiles() error = %v", err)
	}

	for _, fileName := range live {
		kind, destination, ok := classifyWorkRootContractFile(fileName)
		if !ok {
			t.Fatalf("live root .go file %q is not classified", fileName)
		}
		switch kind {
		case "thin_root_retain":
			if destination != "pkg/services/work" {
				t.Fatalf("thin root file %q destination = %q, want pkg/services/work", fileName, destination)
			}
		case "excess_fold":
			if destination == "" || !isWorkPrivateRootContractFoldDestination(destination) {
				t.Fatalf("excess fold file %q destination = %q, want private fold path under pkg/services/work/internal", fileName, destination)
			}
		default:
			t.Fatalf("live root .go file %q has unknown kind %q", fileName, kind)
		}
	}
}

func TestWorkThinRootContractFilesClassifyAsRetain(t *testing.T) {
	t.Parallel()

	for _, fileName := range workThinRootContractFiles {
		kind, destination, ok := classifyWorkRootContractFile(fileName)
		if !ok {
			t.Fatalf("classifyWorkRootContractFile(%q) ok = false", fileName)
		}
		if kind != "thin_root_retain" {
			t.Fatalf("classifyWorkRootContractFile(%q) = %q, want thin_root_retain", fileName, kind)
		}
		if destination != "pkg/services/work" {
			t.Fatalf("classifyWorkRootContractFile(%q) destination = %q, want pkg/services/work", fileName, destination)
		}
	}
}

func TestWorkExcessRootContractFoldClustersMatchInventoryNote(t *testing.T) {
	t.Parallel()

	gotClusters := make([]string, 0, len(workExcessRootContractFolds))
	for _, target := range workExcessRootContractFolds {
		gotClusters = append(gotClusters, target.cluster)
		if len(target.files) == 0 {
			t.Fatalf("cluster %q has no inventoried files", target.cluster)
		}
		if !strings.HasPrefix(target.destination, "pkg/services/work/internal") {
			t.Fatalf("cluster %q destination = %q, want path under pkg/services/work/internal", target.cluster, target.destination)
		}
	}
	slices.Sort(gotClusters)
	wantClusters := []string{
		"content_materialization_impl",
		"content_staging_impl",
		"invocation_return_policy",
		"lineage_graph_modules",
		"request_admission",
		"state_access_query",
	}
	if !slices.Equal(gotClusters, wantClusters) {
		t.Fatalf("fold clusters = %v, want %v", gotClusters, wantClusters)
	}
}
