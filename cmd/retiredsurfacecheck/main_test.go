package main

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunPassesOnRepositoryRoot(t *testing.T) {
	root := repositoryRoot(t)
	var stdout, stderr bytes.Buffer
	if err := run(config{root: root}, &stdout, &stderr); err != nil {
		t.Fatalf("run() error = %v, stderr:\n%s", err, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "[agent-factory:retired-surface] retired command/docs surfaces, encoded-path production resolution, and handwritten run/server registration remain absent") {
		t.Fatalf("stdout = %q, want retired-surface success summary", got)
	}
}

func TestMakeRetiredSurfaceCheckTargetPassesOnRepositoryRoot(t *testing.T) {
	repoRoot := repositoryRoot(t)
	cmd := exec.Command("make", "retired-surface-check", "RETIRED_SURFACE_CHECK_ROOT="+repoRoot)
	cmd.Dir = repoRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("make retired-surface-check failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "[agent-factory:retired-surface] retired command/docs surfaces, encoded-path production resolution, and handwritten run/server registration remain absent") {
		t.Fatalf("make output = %q, want retired-surface success summary", output)
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
