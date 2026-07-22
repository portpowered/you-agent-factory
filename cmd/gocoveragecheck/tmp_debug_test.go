package main

import (
	"strings"
	"testing"
)

func TestDebugResolvePackages(t *testing.T) {
	repoRoot, err := repoRootDir()
	if err != nil {
		t.Fatalf("repoRootDir: %v", err)
	}
	stdout, stderr, err := runCommand(commandInvocation{
		name: "go",
		args: []string{"list", "-f", "{{.ImportPath}}\t{{len .GoFiles}}", "./cmd/factory", "./pkg/..."},
		dir:  repoRoot,
	})
	t.Logf("err=%v", err)
	t.Logf("stderr=%q", stderr)
	t.Logf("stdout=%q len=%d", stdout, len(stdout))
	b := []byte(stdout)
	t.Logf("bytes0-12=%v", b[:min(len(b), 12)])
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	t.Logf("stdout lines=%d", len(lines))
	for i, line := range lines {
		if i >= 5 {
			break
		}
		t.Logf("line=%q", line)
	}
}

func min(a, b int) int {
	if a < b { return a }
	return b
}
