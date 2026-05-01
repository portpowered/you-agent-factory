package handwrittensourceguard

import (
	"path/filepath"
	"testing"
)

func TestShouldSkipDir_BroadHandwrittenSourceGuardsShareCloseoutContract(t *testing.T) {
	t.Parallel()

	type expectation struct {
		relativePath string
		wantSkip     bool
	}

	rootExpectations := map[string][]expectation{
		"repo-root": {
			{relativePath: ".claude/worktrees/stale", wantSkip: true},
			{relativePath: "pkg/api/generated", wantSkip: true},
			{relativePath: "pkg/petri", wantSkip: false},
		},
		"pkg": {
			{relativePath: ".cache", wantSkip: true},
			{relativePath: "api/generated", wantSkip: true},
			{relativePath: "interfaces", wantSkip: false},
		},
	}

	repoRoot := filepath.Join("repo", "root")
	pkgRoot := filepath.Join(repoRoot, "pkg")

	for _, entry := range Inventory() {
		expectations, ok := rootExpectations[entry.WalkRoot]
		if !ok {
			continue
		}

		walkRoot := repoRoot
		if entry.WalkRoot == "pkg" {
			walkRoot = pkgRoot
		}

		for _, tc := range expectations {
			path := filepath.Join(walkRoot, filepath.FromSlash(tc.relativePath))
			if got := ShouldSkipDir(entry.GuardFile, walkRoot, path); got != tc.wantSkip {
				t.Fatalf("%s skip(%s) = %t, want %t", entry.GuardFile, tc.relativePath, got, tc.wantSkip)
			}
		}
	}
}
