package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/javascriptcontractsmoke"
)

func moduleRoot(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(1)
	if !ok {
		t.Fatal("cannot determine test file path")
	}
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find module root from test file")
		}
		dir = parent
	}
}

func TestRunPassesCleanRepositoryWithoutWriting(t *testing.T) {
	root := moduleRoot(t)
	before := repositoryTree(t, root)
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}

	if status := run(root, stdout, stderr); status != 0 {
		t.Fatalf("run() status = %d, want 0; stderr = %q", status, stderr)
	}
	if got := stdout.String(); got != successMessage+"\n" || stderr.Len() != 0 {
		t.Fatalf("run() stdout = %q, stderr = %q", got, stderr.String())
	}
	if after := repositoryTree(t, root); !mapsEqual(after, before) {
		t.Fatal("run() changed repository bytes on success")
	}
}

func TestRunRepeatPassesAreDeterministicWithoutWriting(t *testing.T) {
	root := moduleRoot(t)
	before := repositoryTree(t, root)

	for runIndex := 0; runIndex < 2; runIndex++ {
		stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
		if status := run(root, stdout, stderr); status != 0 {
			t.Fatalf("run %d status = %d, want 0; stderr = %q", runIndex, status, stderr)
		}
		if got := stdout.String(); got != successMessage+"\n" || stderr.Len() != 0 {
			t.Fatalf("run %d stdout = %q, stderr = %q", runIndex, got, stderr.String())
		}
	}
	if after := repositoryTree(t, root); !mapsEqual(after, before) {
		t.Fatal("run() changed repository bytes across repeat passes")
	}
}

func TestRunReportsStaleProjectionDeterministicallyWithoutWriting(t *testing.T) {
	root := mutationFixtureRoot(t)
	stagedPath := filepath.Join(root, filepath.FromSlash(javascriptcontractsmoke.StagedProjectionRelativePath))
	original, err := os.ReadFile(stagedPath)
	if err != nil {
		t.Fatalf("read staged projection: %v", err)
	}
	if err := os.WriteFile(stagedPath, append(original, '\n'), 0o644); err != nil {
		t.Fatalf("mutate staged projection: %v", err)
	}

	want := strings.Join([]string{
		"[agent-factory:javascript-contract-smoke] packages/api/generated/javascript/runtime-api.json (javascript.projection.stale): staged JavaScript runtime projection diverges from the authored catalog; run `make contracts-generate` and `make contracts-check` to restore staging parity",
		"[agent-factory:javascript-contract-smoke] JavaScript contract parity failed; restore catalog, staging, binding descriptor, and call-behavior baseline alignment",
		"",
	}, "\n")

	for runIndex := 0; runIndex < 2; runIndex++ {
		stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
		if status := run(root, stdout, stderr); status != 1 {
			t.Fatalf("run %d status = %d, want 1", runIndex, status)
		}
		if stdout.Len() != 0 || stderr.String() != want {
			t.Fatalf("run %d stdout = %q, stderr = %q, want %q", runIndex, stdout, stderr, want)
		}
	}
}

func mutationFixtureRoot(t *testing.T) string {
	t.Helper()

	repoRoot := moduleRoot(t)
	root := t.TempDir()
	for _, rel := range []string{
		javascriptcontractsmoke.AuthoredCatalogRelativePath,
		javascriptcontractsmoke.StagedProjectionRelativePath,
	} {
		source := filepath.Join(repoRoot, filepath.FromSlash(rel))
		target := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatalf("create fixture directory for %s: %v", rel, err)
		}
		data, err := os.ReadFile(source)
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		if err := os.WriteFile(target, data, 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	return root
}

func repositoryTree(t *testing.T, root string) map[string][]byte {
	t.Helper()

	paths := []string{
		javascriptcontractsmoke.AuthoredCatalogRelativePath,
		javascriptcontractsmoke.StagedProjectionRelativePath,
	}
	tree := make(map[string][]byte, len(paths))
	for _, rel := range paths {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		tree[rel] = append([]byte(nil), data...)
	}
	return tree
}

func mapsEqual(left, right map[string][]byte) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		other, ok := right[key]
		if !ok || !bytes.Equal(value, other) {
			return false
		}
	}
	return true
}
