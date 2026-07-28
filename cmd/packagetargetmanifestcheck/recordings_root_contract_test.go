package main

import (
	"slices"
	"strings"
	"testing"
)

func TestRecordingsRootGoFilesMatchCommittedInventory(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	live, err := listRecordingsRootGoFiles(repoRoot)
	if err != nil {
		t.Fatalf("listRecordingsRootGoFiles() error = %v", err)
	}

	want := recordingsRootContractInventory()
	if !slices.Equal(live, want) {
		t.Fatalf("live root .go files = %v, want committed inventory %v", live, want)
	}
}

func TestRecordingsExcessRootContractFoldDestinations(t *testing.T) {
	t.Parallel()

	want := map[string]string{
		"event":                 "pkg/services/recordings/internal/services/canonical_ledger",
		"world_state":           "pkg/services/recordings/internal/services/projection_query",
		"replay":                "pkg/services/recordings/internal/services/replay",
		"dispatch":              "pkg/services/recordings/internal/services/projection_query",
		"workstation_request":   "pkg/services/recordings/internal/services/projection_query",
		"live_recording_target": "pkg/services/recordings/internal/services/recording_lifecycle",
	}

	for _, target := range recordingsExcessRootContractFolds {
		wantDestination, ok := want[target.cluster]
		if !ok {
			t.Fatalf("unexpected fold cluster %q", target.cluster)
		}
		if target.destination != wantDestination {
			t.Fatalf("cluster %q destination = %q, want %q", target.cluster, target.destination, wantDestination)
		}
	}
}

func TestRecordingsRootContractClassificationPartitionsLiveTree(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	live, err := listRecordingsRootGoFiles(repoRoot)
	if err != nil {
		t.Fatalf("listRecordingsRootGoFiles() error = %v", err)
	}

	for _, fileName := range live {
		kind, destination, ok := classifyRecordingsRootContractFile(fileName)
		if !ok {
			t.Fatalf("live root .go file %q is not classified", fileName)
		}
		switch kind {
		case "thin_root_retain":
			if destination != "pkg/services/recordings" {
				t.Fatalf("thin root file %q destination = %q, want pkg/services/recordings", fileName, destination)
			}
		case "excess_fold":
			if destination == "" || destination == "pkg/services/recordings" {
				t.Fatalf("excess fold file %q destination = %q, want private subservice path", fileName, destination)
			}
		default:
			t.Fatalf("live root .go file %q has unknown kind %q", fileName, kind)
		}
	}
}

func TestRecordingsExcessRootContractFoldDestinationsRejectOwnerRootRetain(t *testing.T) {
	t.Parallel()

	const ownerRoot = "pkg/services/recordings"
	for _, target := range recordingsExcessRootContractFolds {
		if target.destination == ownerRoot {
			t.Fatalf("cluster %q folds to owner root retain destination", target.cluster)
		}
		if !strings.HasPrefix(target.destination, ownerRoot+"/internal/") {
			t.Fatalf("cluster %q destination = %q, want private subservice path under %s/internal/", target.cluster, target.destination, ownerRoot)
		}
		for _, fileName := range target.files {
			kind, destination, ok := classifyRecordingsRootContractFile(fileName)
			if !ok {
				t.Fatalf("classifyRecordingsRootContractFile(%q) ok = false", fileName)
			}
			if kind != "excess_fold" {
				t.Fatalf("classifyRecordingsRootContractFile(%q) = %q, want excess_fold", fileName, kind)
			}
			if destination == ownerRoot {
				t.Fatalf("excess fold file %q regressed to owner root retain destination", fileName)
			}
		}
	}
}
