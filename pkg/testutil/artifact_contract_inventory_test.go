package testutil

import (
	"bufio"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testpath"
)

func TestArtifactContractInventory_DocumentMatchesEnforcedEntries(t *testing.T) {
	t.Parallel()

	if diff := compareArtifactContractEntries(
		parseArtifactContractInventoryDoc(t),
		testpath.ArtifactContractInventory(),
	); diff != "" {
		t.Fatal(diff)
	}
}

func TestArtifactContractInventory_CheckedInExistsAndObsoleteStaysAbsent(t *testing.T) {
	t.Parallel()

	repoRoot := MustRepoRoot(t)
	for _, entry := range testpath.ArtifactContractInventory() {
		path := filepath.Join(repoRoot, filepath.FromSlash(entry.Path))
		_, err := os.Stat(path)
		switch entry.Classification {
		case "checked_in":
			if err != nil {
				t.Fatalf("%s classified %s but missing: %v", entry.Path, entry.Classification, err)
			}
		case "obsolete":
			if err == nil {
				t.Fatalf("%s classified obsolete but exists in checkout", entry.Path)
			}
			if !os.IsNotExist(err) {
				t.Fatalf("%s classified obsolete but stat failed with non-absence error: %v", entry.Path, err)
			}
		}
	}
}

func TestArtifactContractInventory_CanonicalMaintainerSurfaceUsesFactoryInternalFiles(t *testing.T) {
	t.Parallel()

	inventory := testpath.ArtifactContractInventory()
	got := checkedInMaintainerPaths(inventory)
	want := []string{
		"factory/internal/asks.md",
		"factory/internal/meta.md",
		"factory/internal/progress.md",
		"factory/internal/view.md",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("checked-in maintainer paths = %#v, want %#v", got, want)
	}

	for _, legacy := range []string{
		"factory/logs/meta/asks.md",
		"factory/logs/meta/view.md",
		"factory/logs/meta/progress.txt",
		"factory/meta/asks.md",
	} {
		if classification := classificationForPath(inventory, legacy); classification != "obsolete" {
			t.Fatalf("%s classification = %q, want %q", legacy, classification, "obsolete")
		}
	}
}

func parseArtifactContractInventoryDoc(t *testing.T) []testpath.ArtifactContractEntry {
	t.Helper()

	path := MustRepoPath(t, "docs/internal/development/root-factory-artifact-contract-inventory.md")
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open artifact inventory doc: %v", err)
	}
	defer file.Close()

	var (
		inInventory bool
		inTable     bool
		entries     []testpath.ArtifactContractEntry
	)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case line == "## Inventory":
			inInventory = true
		case inInventory && strings.HasPrefix(line, "## "):
			inInventory = false
			inTable = false
		case inInventory && strings.HasPrefix(line, "| Path | Classification | Notes |"):
			inTable = true
		case inTable && strings.HasPrefix(line, "| --- | --- | --- |"):
			continue
		case inTable && strings.HasPrefix(line, "| `"):
			entry := parseArtifactContractInventoryRow(t, line)
			entries = append(entries, entry)
		case inTable && strings.TrimSpace(line) == "":
			inTable = false
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan artifact inventory doc: %v", err)
	}
	return entries
}

func parseArtifactContractInventoryRow(t *testing.T, line string) testpath.ArtifactContractEntry {
	t.Helper()

	parts := strings.Split(line, "|")
	if len(parts) != 5 {
		t.Fatalf("unexpected artifact inventory row format: %q", line)
	}
	return testpath.ArtifactContractEntry{
		Path:           strings.Trim(strings.TrimSpace(parts[1]), "`"),
		Classification: strings.Trim(strings.TrimSpace(parts[2]), "`"),
	}
}

func compareArtifactContractEntries(got, want []testpath.ArtifactContractEntry) string {
	gotPairs := contractEntryPairs(got)
	wantPairs := contractEntryPairs(want)
	if reflect.DeepEqual(gotPairs, wantPairs) {
		return ""
	}
	return "artifact inventory doc entries differ from internal/testpath.ArtifactContractInventory()"
}

func contractEntryPairs(entries []testpath.ArtifactContractEntry) []testpath.ArtifactContractEntry {
	pairs := make([]testpath.ArtifactContractEntry, 0, len(entries))
	for _, entry := range entries {
		pairs = append(pairs, testpath.ArtifactContractEntry{
			Path:           normalizeArtifactContractPath(entry.Path),
			Classification: entry.Classification,
		})
	}
	return pairs
}

func normalizeArtifactContractPath(path string) string {
	if strings.HasSuffix(path, "/") {
		return strings.TrimSuffix(path, "/")
	}
	return path
}

func checkedInMaintainerPaths(entries []testpath.ArtifactContractEntry) []string {
	paths := make([]string, 0, 4)
	for _, entry := range entries {
		if entry.Classification != "checked_in" {
			continue
		}
		if !strings.HasPrefix(entry.Path, "factory/internal/") {
			continue
		}
		if !strings.HasSuffix(entry.Path, ".md") {
			continue
		}
		paths = append(paths, entry.Path)
	}
	sort.Strings(paths)
	return paths
}

func classificationForPath(entries []testpath.ArtifactContractEntry, path string) string {
	for _, entry := range entries {
		if entry.Path == path {
			return entry.Classification
		}
	}
	return ""
}
