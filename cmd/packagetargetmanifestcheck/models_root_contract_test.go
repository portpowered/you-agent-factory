package main

import (
	"slices"
	"testing"
)

func TestModelsRootGoFilesMatchCommittedInventory(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	live, err := listModelsRootGoFiles(repoRoot)
	if err != nil {
		t.Fatalf("listModelsRootGoFiles() error = %v", err)
	}
	want := modelsRootContractInventory()
	if !slices.Equal(live, want) {
		t.Fatalf("live Models root .go files = %v, want committed inventory %v", live, want)
	}
}

func TestModelsRootContractClassificationPartitionsLiveTree(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	live, err := listModelsRootGoFiles(repoRoot)
	if err != nil {
		t.Fatalf("listModelsRootGoFiles() error = %v", err)
	}
	for _, fileName := range live {
		kind, destination, ok := classifyModelsRootContractFile(fileName)
		if !ok {
			t.Fatalf("live Models root .go file %q is not classified", fileName)
		}
		switch kind {
		case "thin_root_retain":
			if destination != modelsRootRelative {
				t.Fatalf("thin root file %q destination = %q, want %s", fileName, destination, modelsRootRelative)
			}
		case "excess_fold":
			if destination == "" || !isModelsPrivateRootContractFoldDestination(destination) {
				t.Fatalf("excess fold file %q destination = %q, want private path under %s/internal", fileName, destination, modelsRootRelative)
			}
		default:
			t.Fatalf("live Models root .go file %q has unknown kind %q", fileName, kind)
		}
	}
}

func TestModelsThinRootContractFilesClassifyAsRetain(t *testing.T) {
	t.Parallel()

	for _, fileName := range modelsThinRootContractFiles {
		kind, destination, ok := classifyModelsRootContractFile(fileName)
		if !ok {
			t.Fatalf("classifyModelsRootContractFile(%q) ok = false", fileName)
		}
		if kind != "thin_root_retain" || destination != modelsRootRelative {
			t.Fatalf("classifyModelsRootContractFile(%q) = (%q, %q), want (thin_root_retain, %s)", fileName, kind, destination, modelsRootRelative)
		}
	}
}

func TestModelsExcessRootContractFoldDestinationsStayPrivate(t *testing.T) {
	t.Parallel()

	if len(modelsExcessRootContractFolds) != 0 {
		t.Fatalf("modelsExcessRootContractFolds = %#v, want empty for the root-contract freeze", modelsExcessRootContractFolds)
	}
	for _, target := range modelsExcessRootContractFolds {
		if target.destination == modelsRootRelative {
			t.Fatalf("cluster %q folds to owner root retain destination", target.cluster)
		}
		if !isModelsPrivateRootContractFoldDestination(target.destination) {
			t.Fatalf("cluster %q destination = %q, want private destination", target.cluster, target.destination)
		}
	}
}
