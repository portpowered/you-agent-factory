package main

import (
	"slices"
	"strings"
	"testing"
)

func TestWorkersRootGoFilesMatchCommittedInventory(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	live, err := listWorkersRootGoFiles(repoRoot)
	if err != nil {
		t.Fatalf("listWorkersRootGoFiles() error = %v", err)
	}

	want := workersRootContractInventory()
	if !slices.Equal(live, want) {
		t.Fatalf("live root .go files = %v, want committed inventory %v", live, want)
	}

	if len(live) != 36 {
		t.Fatalf("live root .go file count = %d, want baseline 36", len(live))
	}
}

func TestWorkersRootContractMoveDestinations(t *testing.T) {
	t.Parallel()

	want := map[string]string{
		"workers_internal": "pkg/services/workers/internal",
	}

	for _, target := range workersRootContractMoveTargets {
		wantDestination, ok := want[target.cluster]
		if !ok {
			t.Fatalf("unexpected move cluster %q", target.cluster)
		}
		if target.destination != wantDestination {
			t.Fatalf("cluster %q destination = %q, want %q", target.cluster, target.destination, wantDestination)
		}
		if !isWorkersPrivateRootContractMoveDestination(target.destination) {
			t.Fatalf("cluster %q destination = %q, want private move path under pkg/services/workers/internal", target.cluster, target.destination)
		}
	}
}

func TestWorkersRootContractMoveDestinationsRejectOwnerRootRetain(t *testing.T) {
	t.Parallel()

	const ownerRoot = "pkg/services/workers"
	for _, target := range workersRootContractMoveTargets {
		if target.destination == ownerRoot {
			t.Fatalf("cluster %q moves to owner root retain destination", target.cluster)
		}
		if !isWorkersPrivateRootContractMoveDestination(target.destination) {
			t.Fatalf("cluster %q destination = %q, want private move path under %s/internal", target.cluster, target.destination, ownerRoot)
		}
		for _, fileName := range target.files {
			kind, destination, ok := classifyWorkersRootContractFile(fileName)
			if !ok {
				t.Fatalf("classifyWorkersRootContractFile(%q) ok = false", fileName)
			}
			if kind != "root_move" {
				t.Fatalf("classifyWorkersRootContractFile(%q) = %q, want root_move", fileName, kind)
			}
			if destination == ownerRoot {
				t.Fatalf("root move file %q regressed to owner root retain destination", fileName)
			}
			if !isWorkersPrivateRootContractMoveDestination(destination) {
				t.Fatalf("root move file %q destination = %q, want private move path under %s/internal", fileName, destination, ownerRoot)
			}
		}
	}
}

func TestWorkersRootContractClassificationPartitionsLiveTree(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	live, err := listWorkersRootGoFiles(repoRoot)
	if err != nil {
		t.Fatalf("listWorkersRootGoFiles() error = %v", err)
	}

	for _, fileName := range live {
		kind, destination, ok := classifyWorkersRootContractFile(fileName)
		if !ok {
			t.Fatalf("live root .go file %q is not classified", fileName)
		}
		switch kind {
		case "thin_root_retain":
			if destination != "pkg/services/workers" {
				t.Fatalf("thin root file %q destination = %q, want pkg/services/workers", fileName, destination)
			}
		case "root_move":
			if destination == "" || !isWorkersPrivateRootContractMoveDestination(destination) {
				t.Fatalf("root move file %q destination = %q, want private move path under pkg/services/workers/internal", fileName, destination)
			}
		default:
			t.Fatalf("live root .go file %q has unknown kind %q", fileName, kind)
		}
	}
}

func TestWorkersThinRootContractFilesClassifyAsRetain(t *testing.T) {
	t.Parallel()

	for _, fileName := range workersThinRootContractFiles {
		kind, destination, ok := classifyWorkersRootContractFile(fileName)
		if !ok {
			t.Fatalf("classifyWorkersRootContractFile(%q) ok = false", fileName)
		}
		if kind != "thin_root_retain" {
			t.Fatalf("classifyWorkersRootContractFile(%q) = %q, want thin_root_retain", fileName, kind)
		}
		if destination != "pkg/services/workers" {
			t.Fatalf("classifyWorkersRootContractFile(%q) destination = %q, want pkg/services/workers", fileName, destination)
		}
	}
}

func TestWorkersRootContractMoveClustersMatchInventoryNote(t *testing.T) {
	t.Parallel()

	gotClusters := make([]string, 0, len(workersRootContractMoveTargets))
	for _, target := range workersRootContractMoveTargets {
		gotClusters = append(gotClusters, target.cluster)
		if len(target.files) == 0 {
			t.Fatalf("cluster %q has no inventoried files", target.cluster)
		}
		if !strings.HasPrefix(target.destination, "pkg/services/workers/internal") {
			t.Fatalf("cluster %q destination = %q, want path under pkg/services/workers/internal", target.cluster, target.destination)
		}
	}
	slices.Sort(gotClusters)
	wantClusters := []string{}
	slices.Sort(wantClusters)
	if !slices.Equal(gotClusters, wantClusters) {
		t.Fatalf("move clusters = %v, want %v", gotClusters, wantClusters)
	}
}
