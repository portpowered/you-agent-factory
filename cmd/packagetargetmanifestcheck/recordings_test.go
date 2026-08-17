package main

import (
	"path/filepath"
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

func TestCommittedManifestRecordingsRejectsRetainToOwnerRoot(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	manifest, err := loadManifest(filepath.Join(repoRoot, manifestRelativePath))
	if err != nil {
		t.Fatalf("loadManifest() error = %v", err)
	}

	const ownerPrefix = "pkg/services/recordings/"
	for _, row := range manifest.Packages {
		if row.PackagePath == "pkg/services/recordings" {
			continue
		}
		if !strings.HasPrefix(row.PackagePath, ownerPrefix) {
			continue
		}
		rest := strings.TrimPrefix(row.PackagePath, ownerPrefix)
		if recordingsCanonicalRetainRest(rest) {
			continue
		}
		if row.Disposition == DispositionRetain && row.Destination == "recordings" {
			t.Fatalf("committed manifest row retain→recordings for %q", row.PackagePath)
		}
		if row.Disposition != DispositionMove {
			t.Fatalf("committed manifest row %q disposition = %q, want move", row.PackagePath, row.Disposition)
		}
		if row.Destination == "recordings" {
			t.Fatalf("committed manifest row %q move destination = owner root, want nested plan path", row.PackagePath)
		}
	}
}
