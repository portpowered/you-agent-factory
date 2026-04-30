package testutil

import (
	"os"
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
