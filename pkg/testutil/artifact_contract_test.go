package testutil

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestArtifactContractInventory_ClassifiesTargetedRootArtifacts(t *testing.T) {
	repoRoot := MustRepoRoot(t)
	if repoRoot == "" {
		t.Fatal("repo root must not be empty")
	}

	for _, entry := range ArtifactContract() {
		path := MustRepoPath(t, entry.Path)
		_, err := os.Stat(path)
		switch entry.Classification {
		case ArtifactCheckedIn:
			if err != nil {
				t.Fatalf("%s classified as checked_in but missing: %v", entry.Path, err)
			}
		case ArtifactGenerated:
			// Generated artifacts may or may not be present in a given worktree,
			// but they must stay explicitly classified when tests reference them.
		case ArtifactObsolete:
			if err == nil {
				t.Fatalf("%s classified as obsolete but still present on disk", entry.Path)
			}
		default:
			t.Fatalf("artifact %s has unknown classification %q", entry.Path, entry.Classification)
		}
	}
}

func TestArtifactContractInventory_DocumentationMatchesClassifications(t *testing.T) {
	docEntries := parseArtifactContractInventoryDoc(t)
	codeEntries := ArtifactContract()

	if len(docEntries) != len(codeEntries) {
		t.Fatalf("inventory doc entries = %d, want %d", len(docEntries), len(codeEntries))
	}

	for i, entry := range codeEntries {
		docEntry := docEntries[i]
		if docEntry.Path != normalizeArtifactContractDocPath(entry.Path) {
			t.Fatalf("inventory doc path[%d] = %q, want %q", i, docEntry.Path, normalizeArtifactContractDocPath(entry.Path))
		}
		if docEntry.Classification != string(entry.Classification) {
			t.Fatalf("inventory doc classification for %s = %q, want %q", entry.Path, docEntry.Classification, entry.Classification)
		}
	}
}

type artifactContractDocEntry struct {
	Path           string
	Classification string
}

func parseArtifactContractInventoryDoc(t *testing.T) []artifactContractDocEntry {
	t.Helper()

	path := MustRepoPath(t, "docs/development/root-factory-artifact-contract-inventory.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read inventory doc: %v", err)
	}

	var entries []artifactContractDocEntry
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "| `") {
			continue
		}

		parts := strings.Split(line, "|")
		if len(parts) < 4 {
			t.Fatalf("inventory doc row malformed: %q", line)
		}

		pathValue := normalizeArtifactContractDocPath(strings.Trim(parts[1], " `"))
		classification := strings.Trim(parts[2], " `")
		if pathValue == "" || classification == "" {
			t.Fatalf("inventory doc row missing required values: %q", line)
		}

		entries = append(entries, artifactContractDocEntry{
			Path:           pathValue,
			Classification: classification,
		})
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan inventory doc: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("inventory doc did not yield any artifact rows")
	}
	return entries
}

func normalizeArtifactContractDocPath(path string) string {
	normalized := filepath.ToSlash(filepath.Clean(path))
	if normalized == "." {
		return ""
	}
	return strings.TrimSuffix(normalized, "/")
}
