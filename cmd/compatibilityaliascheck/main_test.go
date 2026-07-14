package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunRejectsDeliberateCompatibilityAliasAdoption(t *testing.T) {
	root := filepath.Join(repositoryRoot(t), "cmd", "compatibilityaliascheck", "testdata", "violation-repo")

	var stdout, stderr bytes.Buffer
	err := run(config{root: root}, &stdout, &stderr)
	if err == nil {
		t.Fatal("run() error = nil, want deliberate adoption failure")
	}
	output := err.Error() + stderr.String()
	for _, want := range []string{
		"[agent-factory:compatibility-alias] found 1 prohibited compatibility alias adoption(s)",
		"pkg/work/adopter.go",
		"you.workflow.validate",
		"mcp.alias.you.workflow.validate",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("stderr = %q, want substring %q", output, want)
		}
	}
}

func TestRunPassesOnCleanCompatibilityBoundaryFixture(t *testing.T) {
	root := filepath.Join(repositoryRoot(t), "cmd", "compatibilityaliascheck", "testdata", "clean-repo")
	var stdout, stderr bytes.Buffer
	if err := run(config{root: root}, &stdout, &stderr); err != nil {
		t.Fatalf("run() error = %v, stderr:\n%s", err, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "[agent-factory:compatibility-alias] inventoried compatibility aliases are not adopted outside approved boundaries") {
		t.Fatalf("stdout = %q, want clean compatibility alias scan summary", got)
	}
}

func TestRunPassesOnRepositoryRoot(t *testing.T) {
	root := repositoryRoot(t)
	var stdout, stderr bytes.Buffer
	if err := run(config{root: root}, &stdout, &stderr); err != nil {
		t.Fatalf("run() error = %v, stderr:\n%s", err, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "[agent-factory:compatibility-alias] inventoried compatibility aliases are not adopted outside approved boundaries") {
		t.Fatalf("stdout = %q, want clean compatibility alias scan summary", got)
	}
}

func TestMakeCompatibilityAliasCheckTargetPassesOnRepositoryRoot(t *testing.T) {
	repoRoot := repositoryRoot(t)
	cmd := exec.Command("make", "compatibility-alias-check", "COMPATIBILITY_ALIAS_CHECK_ROOT="+repoRoot)
	cmd.Dir = repoRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("make compatibility-alias-check failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "[agent-factory:compatibility-alias] inventoried compatibility aliases are not adopted outside approved boundaries") {
		t.Fatalf("make output = %q, want clean compatibility alias scan summary", output)
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}
	return root
}

func fixtureRepository(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for relative, contents := range files {
		path := filepath.Join(root, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create fixture directory: %v", err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatalf("write fixture %s: %v", relative, err)
		}
	}
	return root
}
