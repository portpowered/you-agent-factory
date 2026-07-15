package javascriptcontractsmoke_test

import (
	"encoding/json"
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

func TestCheck_ReportsStaleProjectionSymbolPathDeterministicallyWithoutWriting(t *testing.T) {
	root := mutationFixtureRoot(t)
	stagedPath := filepath.Join(root, filepath.FromSlash(javascriptcontractsmoke.StagedProjectionRelativePath))
	original, err := os.ReadFile(stagedPath)
	if err != nil {
		t.Fatalf("read staged projection: %v", err)
	}

	var staged map[string]any
	if err := json.Unmarshal(original, &staged); err != nil {
		t.Fatalf("decode staged projection: %v", err)
	}
	symbols := staged["symbols"].(map[string]any)
	phase := symbols["javascript.phase"].(map[string]any)
	phase["kind"] = "method"
	mutated, err := json.MarshalIndent(staged, "", "  ")
	if err != nil {
		t.Fatalf("encode staged projection: %v", err)
	}
	mutated = append(mutated, '\n')
	if err := os.WriteFile(stagedPath, mutated, 0o644); err != nil {
		t.Fatalf("write staged projection: %v", err)
	}

	first := runCheckDiagnostics(t, root)
	second := runCheckDiagnostics(t, root)
	if len(first) != 1 || len(second) != 1 || first[0] != second[0] {
		t.Fatalf("stale diagnostics are not deterministic: first=%+v second=%+v", first, second)
	}
	diagnostic := first[0]
	if diagnostic.Code != "javascript.projection.stale" {
		t.Fatalf("diagnostic code = %q, want javascript.projection.stale", diagnostic.Code)
	}
	if diagnostic.Path != javascriptcontractsmoke.StagedProjectionRelativePath {
		t.Fatalf("diagnostic path = %q, want %q", diagnostic.Path, javascriptcontractsmoke.StagedProjectionRelativePath)
	}
	if !strings.Contains(diagnostic.Message, "divergent symbol paths: phase") {
		t.Fatalf("diagnostic message = %q, want divergent symbol path", diagnostic.Message)
	}
	if !strings.Contains(diagnostic.Message, "make contracts-generate") || !strings.Contains(diagnostic.Message, "make contracts-check") {
		t.Fatalf("diagnostic message = %q, want generation/check remediation", diagnostic.Message)
	}
	after, err := os.ReadFile(stagedPath)
	if err != nil {
		t.Fatalf("read staged projection after checks: %v", err)
	}
	if string(after) != string(mutated) {
		t.Fatal("Check() rewrote the stale staged projection")
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

func TestCheck_FailsWhenCatalogContainsForbiddenRootGlobal(t *testing.T) {
	root := mutationFixtureRoot(t)
	mutateCatalogFixture(t, root, func(catalog map[string]any) {
		symbols := catalog["symbols"].(map[string]any)
		symbols["javascript.context"] = map[string]any{
			"path": "context",
			"kind": "value",
		}
	})

	assertForbiddenSymbolFailure(t, root, forbiddenSymbolExpectation{
		code: "javascript.path.forbidden",
		path: "context",
	})
}

func TestCheck_FailsWhenCatalogContainsOrchestratorGlobal(t *testing.T) {
	root := mutationFixtureRoot(t)
	mutateCatalogFixture(t, root, func(catalog map[string]any) {
		symbols := catalog["symbols"].(map[string]any)
		symbols["javascript.orchestrator"] = map[string]any{
			"path": "orchestrator",
			"kind": "namespace",
		}
	})

	assertForbiddenSymbolFailure(t, root, forbiddenSymbolExpectation{
		code: "javascript.path.forbidden",
		path: "orchestrator",
	})
}

func TestCheck_FailsWhenCatalogContainsComparisonProjectHelper(t *testing.T) {
	root := mutationFixtureRoot(t)
	mutateCatalogFixture(t, root, func(catalog map[string]any) {
		symbols := catalog["symbols"].(map[string]any)
		symbols["javascript.workflow.sleep"] = map[string]any{
			"path": "workflow.sleep",
			"kind": "method",
		}
	})

	assertForbiddenSymbolFailure(t, root, forbiddenSymbolExpectation{
		code: "javascript.path.unsupported_helper",
		path: "workflow.sleep",
	})
}

func TestCheck_ForbiddenSymbolFailuresAreDeterministic(t *testing.T) {
	root := mutationFixtureRoot(t)
	mutateCatalogFixture(t, root, func(catalog map[string]any) {
		symbols := catalog["symbols"].(map[string]any)
		symbols["javascript.context"] = map[string]any{
			"path": "context",
			"kind": "value",
		}
	})

	first := runCheckDiagnostics(t, root)
	second := runCheckDiagnostics(t, root)
	if len(first) != len(second) {
		t.Fatalf("diagnostic count drift: first=%d second=%d", len(first), len(second))
	}
	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("diagnostic[%d] drift: first=%+v second=%+v", i, first[i], second[i])
		}
	}
}

type forbiddenSymbolExpectation struct {
	code string
	path string
}

func assertForbiddenSymbolFailure(t *testing.T, root string, want forbiddenSymbolExpectation) {
	t.Helper()

	diagnostics := runCheckDiagnostics(t, root)
	if len(diagnostics) != 1 {
		t.Fatalf("Check() diagnostics = %+v, want one forbidden-surface issue", diagnostics)
	}
	diagnostic := diagnostics[0]
	if diagnostic.Code != want.code {
		t.Fatalf("diagnostic code = %q, want %q (full=%+v)", diagnostic.Code, want.code, diagnostic)
	}
	if diagnostic.Path != want.path {
		t.Fatalf("diagnostic path = %q, want repository-relative symbol path %q (full=%+v)", diagnostic.Path, want.path, diagnostic)
	}
	if !strings.Contains(diagnostic.Message, "remove the path from the contracted supported surface") {
		t.Fatalf("diagnostic message = %q, want actionable remediation", diagnostic.Message)
	}
}

func TestCheck_FailsWhenInstalledPathMissingFromCatalog(t *testing.T) {
	root := mutationFixtureRoot(t)
	mutateCatalogFixture(t, root, func(catalog map[string]any) {
		symbols := catalog["symbols"].(map[string]any)
		delete(symbols, "javascript.phase")
	})

	assertPathCompletenessFailure(t, root, pathCompletenessExpectation{
		wantCount: 1,
		code:      "javascript.path.missing",
		path:      "phase",
	})
}

func TestCheck_FailsWhenCatalogContainsExtraPath(t *testing.T) {
	root := mutationFixtureRoot(t)
	mutateCatalogFixture(t, root, func(catalog map[string]any) {
		symbols := catalog["symbols"].(map[string]any)
		symbols["javascript.workflow.extra"] = map[string]any{
			"path": "workflow.extra",
		}
	})

	assertPathCompletenessFailure(t, root, pathCompletenessExpectation{
		wantCount: 1,
		code:      "javascript.path.extra",
		path:      "workflow.extra",
	})
}

func TestCheck_FailsWhenCatalogPathDuplicated(t *testing.T) {
	root := mutationFixtureRoot(t)
	mutateCatalogFixture(t, root, func(catalog map[string]any) {
		symbols := catalog["symbols"].(map[string]any)
		symbols["javascript.duplicate-phase"] = map[string]any{
			"path": "phase",
		}
	})

	assertPathCompletenessFailure(t, root, pathCompletenessExpectation{
		wantCount: 2,
		code:      "javascript.path.duplicate",
		path:      "phase",
	})
}

func TestCheck_PathCompletenessFailuresAreDeterministic(t *testing.T) {
	root := mutationFixtureRoot(t)
	mutateCatalogFixture(t, root, func(catalog map[string]any) {
		symbols := catalog["symbols"].(map[string]any)
		delete(symbols, "javascript.phase")
	})

	first := runCheckDiagnostics(t, root)
	second := runCheckDiagnostics(t, root)
	if len(first) != len(second) {
		t.Fatalf("diagnostic count drift: first=%d second=%d", len(first), len(second))
	}
	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("diagnostic[%d] drift: first=%+v second=%+v", i, first[i], second[i])
		}
	}
}

type pathCompletenessExpectation struct {
	wantCount int
	code      string
	path      string
}

func assertPathCompletenessFailure(t *testing.T, root string, want pathCompletenessExpectation) {
	t.Helper()

	diagnostics := runCheckDiagnostics(t, root)
	if len(diagnostics) != want.wantCount {
		t.Fatalf("Check() diagnostics = %+v, want %d issue(s)", diagnostics, want.wantCount)
	}
	for _, diagnostic := range diagnostics {
		if diagnostic.Code != want.code {
			t.Fatalf("diagnostic code = %q, want %q (full=%+v)", diagnostic.Code, want.code, diagnostic)
		}
		if diagnostic.Path != want.path {
			t.Fatalf("diagnostic path = %q, want repository-relative symbol path %q (full=%+v)", diagnostic.Path, want.path, diagnostic)
		}
		if !strings.Contains(diagnostic.Message, "restore authored catalog and staging parity") {
			t.Fatalf("diagnostic message = %q, want actionable remediation", diagnostic.Message)
		}
	}
}

func runCheckDiagnostics(t *testing.T, root string) []javascriptcontractsmoke.Diagnostic {
	t.Helper()

	diagnostics, err := javascriptcontractsmoke.Check(root)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if len(diagnostics) == 0 {
		t.Fatal("Check() diagnostics = none, want path completeness failure")
	}
	return diagnostics
}

func mutateCatalogFixture(t *testing.T, root string, mutate func(map[string]any)) {
	t.Helper()

	authoredPath := filepath.Join(root, filepath.FromSlash(javascriptcontractsmoke.AuthoredCatalogRelativePath))
	data, err := os.ReadFile(authoredPath)
	if err != nil {
		t.Fatalf("read authored catalog: %v", err)
	}

	var catalog map[string]any
	if err := json.Unmarshal(data, &catalog); err != nil {
		t.Fatalf("decode authored catalog: %v", err)
	}
	mutate(catalog)

	updated, err := json.MarshalIndent(catalog, "", "  ")
	if err != nil {
		t.Fatalf("encode catalog fixture: %v", err)
	}
	updated = append(updated, '\n')

	for _, rel := range []string{
		javascriptcontractsmoke.AuthoredCatalogRelativePath,
		javascriptcontractsmoke.StagedProjectionRelativePath,
	} {
		target := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.WriteFile(target, updated, 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
}
