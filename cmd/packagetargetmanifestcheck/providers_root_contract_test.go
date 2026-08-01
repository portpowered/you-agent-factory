package main

import (
	"slices"
	"testing"
)

func TestProvidersRootGoFilesMatchCommittedInventory(t *testing.T) {
	t.Parallel()

	live, err := listProvidersRootGoFiles(findRepoRoot(t))
	if err != nil {
		t.Fatalf("listProvidersRootGoFiles() error = %v", err)
	}
	if !slices.Equal(live, providersRootContractInventory()) {
		t.Fatalf("live Providers root .go files = %v, want committed inventory %v", live, providersRootContractInventory())
	}
}

func TestProvidersRootContractClassificationPartitionsLiveTree(t *testing.T) {
	t.Parallel()

	live, err := listProvidersRootGoFiles(findRepoRoot(t))
	if err != nil {
		t.Fatalf("listProvidersRootGoFiles() error = %v", err)
	}
	for _, fileName := range live {
		kind, destination, ok := classifyProvidersRootContractFile(fileName)
		if !ok {
			t.Fatalf("live Providers root .go file %q is not classified", fileName)
		}
		switch kind {
		case "thin_root_retain":
			if destination != providersRootRelative {
				t.Fatalf("thin root file %q destination = %q, want %s", fileName, destination, providersRootRelative)
			}
		case "excess_fold":
			if destination == "" || !isProvidersPrivateRootContractFoldDestination(destination) {
				t.Fatalf("excess fold file %q destination = %q, want private path under %s/internal", fileName, destination, providersRootRelative)
			}
		default:
			t.Fatalf("live Providers root .go file %q has unknown kind %q", fileName, kind)
		}
	}
}

func TestProvidersThinRootContractFilesClassifyAsRetain(t *testing.T) {
	t.Parallel()

	for _, fileName := range providersThinRootContractFiles {
		kind, destination, ok := classifyProvidersRootContractFile(fileName)
		if !ok || kind != "thin_root_retain" || destination != providersRootRelative {
			t.Fatalf("classifyProvidersRootContractFile(%q) = (%q, %q, %v), want (thin_root_retain, %s, true)", fileName, kind, destination, ok, providersRootRelative)
		}
	}
}

func TestProvidersExcessRootContractFoldDestinationsStayPrivate(t *testing.T) {
	t.Parallel()

	for _, target := range providersExcessRootContractFolds {
		if target.destination == providersRootRelative {
			t.Fatalf("cluster %q folds to owner root retain destination", target.cluster)
		}
		if target.destination == "" || !isProvidersPrivateRootContractFoldDestination(target.destination) {
			t.Fatalf("cluster %q destination = %q, want private destination", target.cluster, target.destination)
		}
	}
}
