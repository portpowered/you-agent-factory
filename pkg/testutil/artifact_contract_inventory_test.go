package testutil

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testpath"
)

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

func TestArtifactContractInventory_CanonicalMaintainerBacklogSurfaceIsSingular(t *testing.T) {
	t.Parallel()

	repoRoot := MustRepoRoot(t)
	inventory := testpath.ArtifactContractInventory()
	got := checkedInMaintainerPaths(inventory)
	want := []string{
		"factory/internal/asks.md",
		"factory/internal/meta.md",
		"factory/internal/progress.md",
		"factory/internal/view.md",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("checked-in maintainer paths = %#v, want %#v", got, want)
	}

	asksPath := filepath.Join(repoRoot, filepath.FromSlash("factory/internal/asks.md"))
	asksContents, err := os.ReadFile(asksPath)
	if err != nil {
		t.Fatalf("read canonical maintainer backlog: %v", err)
	}
	asksText := string(asksContents)
	for _, required := range []string{
		"canonical checked-in customer-ask backlog",
		"factory/internal/view.md",
		"factory/internal/progress.md",
		"factory/internal/meta.md",
	} {
		if !strings.Contains(asksText, required) {
			t.Fatalf("factory/internal/asks.md missing canonical maintainer reference %q", required)
		}
	}
	if strings.Contains(asksText, "factory/logs/meta/asks.md") {
		t.Fatal("factory/internal/asks.md must not point back at the deleted legacy maintainer backlog")
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
