package testutil

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
)

type ArtifactClassification string

const (
	ArtifactCheckedIn ArtifactClassification = "checked_in"
	ArtifactGenerated ArtifactClassification = "generated"
	ArtifactObsolete  ArtifactClassification = "obsolete"
)

type ArtifactContractEntry struct {
	Path           string
	Classification ArtifactClassification
	Reason         string
}

var artifactContractEntries = []ArtifactContractEntry{
	{
		Path:           "factory",
		Classification: ArtifactCheckedIn,
		Reason:         "Canonical checked-in repository starter root.",
	},
	{
		Path:           "factory/README.md",
		Classification: ArtifactCheckedIn,
		Reason:         "Canonical checked-in repository starter documentation.",
	},
	{
		Path:           "factory/factory.json",
		Classification: ArtifactCheckedIn,
		Reason:         "Canonical checked-in repository starter config.",
	},
	{
		Path:           "factory/scripts/setup-workspace.py",
		Classification: ArtifactCheckedIn,
		Reason:         "Checked-in workspace setup helper used by the canonical repository workflow.",
	},
	{
		Path:           "factory/inputs",
		Classification: ArtifactCheckedIn,
		Reason:         "Checked-in starter input directories used by the repository-local workflow.",
	},
	{
		Path:           "factory/inputs/idea/default",
		Classification: ArtifactCheckedIn,
		Reason:         "Checked-in repository workflow idea inbox.",
	},
	{
		Path:           "factory/inputs/plan/default",
		Classification: ArtifactCheckedIn,
		Reason:         "Checked-in repository workflow plan inbox.",
	},
	{
		Path:           "factory/inputs/task/default",
		Classification: ArtifactCheckedIn,
		Reason:         "Checked-in repository workflow task inbox.",
	},
	{
		Path:           "factory/inputs/thoughts/default",
		Classification: ArtifactCheckedIn,
		Reason:         "Checked-in repository workflow thought inbox.",
	},
	{
		Path:           "factory/logs/agent-fails.json",
		Classification: ArtifactCheckedIn,
		Reason:         "Checked-in event-stream sample used for replay conversion coverage.",
	},
	{
		Path:           "factory/logs/agent-fails.replay.json",
		Classification: ArtifactCheckedIn,
		Reason:         "Checked-in replay artifact sample paired with the event-stream conversion smoke.",
	},
	{
		Path:           "tests/adhoc/factory-recording-04-11-02.json",
		Classification: ArtifactCheckedIn,
		Reason:         "Canonical replay fixture used by targeted adhoc and replay tests.",
	},
	{
		Path:           "tests/adhoc/factory/README.md",
		Classification: ArtifactCheckedIn,
		Reason:         "Checked-in adhoc fixture documentation.",
	},
	{
		Path:           "tests/adhoc/factory/factory.json",
		Classification: ArtifactCheckedIn,
		Reason:         "Checked-in adhoc fixture config.",
	},
	{
		Path:           "ui/src/api/generated/openapi.ts",
		Classification: ArtifactGenerated,
		Reason:         "Generated TypeScript client types checked in from the authored OpenAPI contract.",
	},
	{
		Path:           "ui/src/components/dashboard/fixtures/failure-analysis-events.ts",
		Classification: ArtifactCheckedIn,
		Reason:         "Checked-in dashboard replay fixture.",
	},
	{
		Path:           "ui/src/components/dashboard/fixtures/graph-state-smoke-events.ts",
		Classification: ArtifactCheckedIn,
		Reason:         "Checked-in dashboard replay fixture.",
	},
	{
		Path:           "ui/src/components/dashboard/fixtures/resource-count-events.ts",
		Classification: ArtifactCheckedIn,
		Reason:         "Checked-in dashboard replay fixture.",
	},
	{
		Path:           "ui/src/components/dashboard/fixtures/runtime-details-events.ts",
		Classification: ArtifactCheckedIn,
		Reason:         "Checked-in dashboard replay fixture.",
	},
	{
		Path:           "factory/workers/processor/AGENTS.md",
		Classification: ArtifactCheckedIn,
		Reason:         "Checked-in canonical processor worker prompt.",
	},
	{
		Path:           "factory/workers/workspace-setup/AGENTS.md",
		Classification: ArtifactCheckedIn,
		Reason:         "Checked-in canonical workspace setup worker prompt.",
	},
	{
		Path:           "factory/workstations/cleaner/AGENTS.md",
		Classification: ArtifactCheckedIn,
		Reason:         "Checked-in canonical repository cleanup workstation prompt.",
	},
	{
		Path:           "factory/workstations/ideafy/AGENTS.md",
		Classification: ArtifactCheckedIn,
		Reason:         "Checked-in canonical ideation workstation prompt.",
	},
	{
		Path:           "factory/workstations/plan/AGENTS.md",
		Classification: ArtifactCheckedIn,
		Reason:         "Checked-in canonical planning workstation prompt.",
	},
	{
		Path:           "factory/workstations/process/AGENTS.md",
		Classification: ArtifactCheckedIn,
		Reason:         "Checked-in canonical execution workstation prompt.",
	},
	{
		Path:           "factory/workstations/review/AGENTS.md",
		Classification: ArtifactCheckedIn,
		Reason:         "Checked-in canonical review workstation prompt.",
	},
	{
		Path:           "factory/workers/executor/AGENTS.md",
		Classification: ArtifactObsolete,
		Reason:         "Legacy story-starter worker path no longer belongs to the canonical root factory.",
	},
	{
		Path:           "factory/workers/reviewer/AGENTS.md",
		Classification: ArtifactObsolete,
		Reason:         "Legacy story-starter worker path no longer belongs to the canonical root factory.",
	},
	{
		Path:           "factory/workstations/execute-story/AGENTS.md",
		Classification: ArtifactObsolete,
		Reason:         "Legacy story-starter workstation path no longer belongs to the canonical root factory.",
	},
	{
		Path:           "factory/workstations/review-story/AGENTS.md",
		Classification: ArtifactObsolete,
		Reason:         "Legacy story-starter workstation path no longer belongs to the canonical root factory.",
	},
	{
		Path:           "factory/inputs/story/default/example-story.md",
		Classification: ArtifactObsolete,
		Reason:         "Legacy story-starter seed file is not part of the canonical root factory surface.",
	},
	{
		Path:           "factory/old/README.md",
		Classification: ArtifactObsolete,
		Reason:         "Legacy historical starter tree is not part of the canonical root factory surface.",
	},
}

func ArtifactContract() []ArtifactContractEntry {
	return append([]ArtifactContractEntry(nil), artifactContractEntries...)
}

func ArtifactContractEntryByPath(path string) (ArtifactContractEntry, bool) {
	normalized := filepath.ToSlash(filepath.Clean(path))
	for _, entry := range artifactContractEntries {
		if entry.Path == normalized {
			return entry, true
		}
	}
	return ArtifactContractEntry{}, false
}

func MustArtifactContractEntry(t testing.TB, path string) ArtifactContractEntry {
	t.Helper()

	entry, ok := ArtifactContractEntryByPath(path)
	if !ok {
		t.Fatalf("artifact path %q is not classified in the checked-in artifact contract inventory", filepath.ToSlash(filepath.Clean(path)))
	}
	return entry
}

func MustRepoRoot(t testing.TB) string {
	t.Helper()

	_, callerFile, _, ok := runtime.Caller(1)
	if !ok {
		t.Fatal("cannot determine caller file path")
	}

	root, err := findRepoRoot(filepath.Dir(callerFile))
	if err != nil {
		t.Fatalf("find repo root from %s: %v", callerFile, err)
	}
	return root
}

func MustRepoPath(t testing.TB, rel string) string {
	t.Helper()
	return filepath.Join(MustRepoRoot(t), filepath.FromSlash(rel))
}

func MustClassifiedArtifactPath(t testing.TB, rel string, allowed ...ArtifactClassification) string {
	t.Helper()

	entry := MustArtifactContractEntry(t, rel)
	if len(allowed) > 0 && !slices.Contains(allowed, entry.Classification) {
		t.Fatalf(
			"artifact path %q classified as %s, want one of %v",
			entry.Path,
			entry.Classification,
			allowed,
		)
	}
	return MustRepoPath(t, entry.Path)
}

func findRepoRoot(startDir string) (string, error) {
	current := filepath.Clean(startDir)
	for {
		goModPath := filepath.Join(current, "go.mod")
		if info, err := os.Stat(goModPath); err == nil && !info.IsDir() {
			return current, nil
		}

		parent := filepath.Dir(current)
		if parent == current {
			return "", os.ErrNotExist
		}
		current = parent
	}
}
