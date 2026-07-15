package javascriptcontractsmoke_test

import (
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

func TestCheck_PassesForCleanRepository(t *testing.T) {
	root := moduleRoot(t)

	diagnostics, err := javascriptcontractsmoke.Check(root)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if len(diagnostics) != 0 {
		t.Fatalf("Check() diagnostics = %+v, want none on clean repository", diagnostics)
	}
}

func TestCheck_RepeatRunsAreDeterministic(t *testing.T) {
	root := moduleRoot(t)

	first, err := javascriptcontractsmoke.Check(root)
	if err != nil {
		t.Fatalf("first Check() error = %v", err)
	}
	second, err := javascriptcontractsmoke.Check(root)
	if err != nil {
		t.Fatalf("second Check() error = %v", err)
	}
	if len(first) != len(second) {
		t.Fatalf("diagnostic count drift: first=%d second=%d", len(first), len(second))
	}
	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("diagnostic[%d] drift: first=%+v second=%+v", i, first[i], second[i])
		}
	}
}

func TestCheck_ReportsStaleProjectionWithoutWriting(t *testing.T) {
	root := mutationFixtureRoot(t)

	diagnostics, err := javascriptcontractsmoke.Check(root)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if len(diagnostics) != 0 {
		t.Fatalf("Check() diagnostics = %+v, want none before staging mutation", diagnostics)
	}

	stagedPath := filepath.Join(root, filepath.FromSlash(javascriptcontractsmoke.StagedProjectionRelativePath))
	original, err := os.ReadFile(stagedPath)
	if err != nil {
		t.Fatalf("read staged projection: %v", err)
	}
	if err := os.WriteFile(stagedPath, append(original, '\n'), 0o644); err != nil {
		t.Fatalf("mutate staged projection: %v", err)
	}

	diagnostics, err = javascriptcontractsmoke.Check(root)
	if err != nil {
		t.Fatalf("Check() after mutation error = %v", err)
	}
	if len(diagnostics) != 1 {
		t.Fatalf("Check() diagnostics = %+v, want one stale projection failure", diagnostics)
	}
	diagnostic := diagnostics[0]
	if diagnostic.Path != javascriptcontractsmoke.StagedProjectionRelativePath {
		t.Fatalf("diagnostic path = %q, want %q", diagnostic.Path, javascriptcontractsmoke.StagedProjectionRelativePath)
	}
	if !strings.Contains(diagnostic.Message, "contracts-generate") {
		t.Fatalf("diagnostic message = %q, want contracts-generate remediation", diagnostic.Message)
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
