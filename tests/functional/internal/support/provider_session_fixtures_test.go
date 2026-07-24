package support

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
)

func TestProviderSessionFixtureRootIsTrackedDespiteDocsTempIgnore(t *testing.T) {
	repoRoot := testutil.MustRepoRoot(t)

	fixtureRel := ProviderSessionFixturePath("README.md")
	fixtureAbs := filepath.Join(repoRoot, filepath.FromSlash(fixtureRel))
	if _, err := os.Stat(fixtureAbs); err != nil {
		t.Fatalf("tracked fixture placeholder %s: %v", fixtureRel, err)
	}

	probeRel := ProviderSessionFixturePath("_ignore_probe.txt")
	probeAbs := filepath.Join(repoRoot, filepath.FromSlash(probeRel))
	if err := os.WriteFile(probeAbs, []byte("fixture root probe\n"), 0o644); err != nil {
		t.Fatalf("write fixture probe %s: %v", probeRel, err)
	}
	t.Cleanup(func() {
		_ = os.Remove(probeAbs)
	})

	siblingRel := "docs/temp/functional/provider-sessions-sibling-probe.txt"
	siblingAbs := filepath.Join(repoRoot, filepath.FromSlash(siblingRel))
	if err := os.WriteFile(siblingAbs, []byte("ignored sibling probe\n"), 0o644); err != nil {
		t.Fatalf("write sibling probe %s: %v", siblingRel, err)
	}
	t.Cleanup(func() {
		_ = os.Remove(siblingAbs)
	})

	if ignored := gitCheckIgnoreQuiet(t, repoRoot, fixtureRel); ignored {
		t.Fatalf("git check-ignore reports %s as ignored; fixture root must be tracked", fixtureRel)
	}
	if ignored := gitCheckIgnoreQuiet(t, repoRoot, probeRel); ignored {
		t.Fatalf("git check-ignore reports %s as ignored; new files under fixture root must be trackable", probeRel)
	}
	if ignored := gitCheckIgnoreQuiet(t, repoRoot, siblingRel); !ignored {
		t.Fatalf("git check-ignore reports %s as not ignored; sibling docs/temp paths must stay ignored", siblingRel)
	}

	trackedProbe := gitLsFilesOthersExcludeStandard(t, repoRoot, probeRel)
	if len(trackedProbe) != 1 || filepath.ToSlash(trackedProbe[0]) != probeRel {
		t.Fatalf("git ls-files --others --exclude-standard for %s = %#v, want [%q]", probeRel, trackedProbe, probeRel)
	}

	siblingTracked := gitLsFilesOthersExcludeStandard(t, repoRoot, siblingRel)
	if len(siblingTracked) != 0 {
		t.Fatalf("git ls-files --others --exclude-standard for ignored sibling %s = %#v, want empty", siblingRel, siblingTracked)
	}

	if !gitIsTrackedOrTrackable(t, repoRoot, fixtureRel) {
		t.Fatalf("%s must be tracked or trackable under the fixture-root exception", fixtureRel)
	}
}

func gitCheckIgnoreQuiet(t *testing.T, repoRoot, relPath string) bool {
	t.Helper()

	cmd := exec.Command("git", "check-ignore", "-q", "--", relPath)
	cmd.Dir = repoRoot
	err := cmd.Run()
	if err == nil {
		return true
	}
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
		return false
	}
	t.Fatalf("git check-ignore -q %s: %v", relPath, err)
	return false
}

func gitLsFilesOthersExcludeStandard(t *testing.T, repoRoot, relPath string) []string {
	t.Helper()

	cmd := exec.Command("git", "ls-files", "--others", "--exclude-standard", "--", relPath)
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git ls-files --others --exclude-standard -- %s: %v", relPath, err)
	}
	return splitNonEmptyLines(string(out))
}

func gitIsTrackedOrTrackable(t *testing.T, repoRoot, relPath string) bool {
	t.Helper()

	cmd := exec.Command("git", "ls-files", "--error-unmatch", "--", relPath)
	cmd.Dir = repoRoot
	if err := cmd.Run(); err == nil {
		return true
	}

	others := gitLsFilesOthersExcludeStandard(t, repoRoot, relPath)
	return len(others) == 1 && filepath.ToSlash(others[0]) == relPath
}

func splitNonEmptyLines(value string) []string {
	var lines []string
	for _, line := range strings.Split(strings.ReplaceAll(value, "\r\n", "\n"), "\n") {
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}
