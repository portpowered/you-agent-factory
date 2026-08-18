package main

import (
	"slices"
	"strings"
	"testing"
)

func TestRecordingsTopLevelChildrenMatchCommittedInventory(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	live, err := listRecordingsTopLevelChildren(repoRoot)
	if err != nil {
		t.Fatalf("listRecordingsTopLevelChildren() error = %v", err)
	}

	want := recordingsTopLevelInventory()
	if !slices.Equal(live, want) {
		t.Fatalf("live top-level children = %v, want committed inventory %v", live, want)
	}
}

func TestRecordingsCanonicalRetainRestMatchesTopLevelInventory(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	live, err := listRecordingsTopLevelChildren(repoRoot)
	if err != nil {
		t.Fatalf("listRecordingsTopLevelChildren() error = %v", err)
	}

	for _, name := range live {
		if slices.Contains(recordingsTopLevelExpectedRetain, name) {
			if !recordingsCanonicalRetainRest(name) {
				t.Fatalf("recordingsCanonicalRetainRest(%q) = false, want true", name)
			}
			continue
		}
		if slices.Contains(recordingsTopLevelUnexpected, name) {
			if recordingsUnexpectedTopLevelRest(name) != true {
				t.Fatalf("recordingsUnexpectedTopLevelRest(%q) = false, want true", name)
			}
			continue
		}
		t.Fatalf("live top-level child %q is missing from committed inventory", name)
	}
}

func TestCommittedLedgerRecordingsMovesOffTheOwnerRoot(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	moveByPath := committedMoveLedgerRows(t, repoRoot)

	const ownerPrefix = "pkg/services/recordings/"
	for packagePath, row := range moveByPath {
		if packagePath == "pkg/services/recordings" {
			continue
		}
		if !strings.HasPrefix(packagePath, ownerPrefix) {
			continue
		}
		rest := strings.TrimPrefix(packagePath, ownerPrefix)
		if recordingsCanonicalRetainRest(rest) {
			continue
		}
		if row.Destination == "recordings" {
			t.Fatalf("move ledger row %q destination = owner root, want nested plan path", packagePath)
		}
	}
}
