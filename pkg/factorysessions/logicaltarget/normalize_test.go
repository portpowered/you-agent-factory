package logicaltarget_test

import (
	"os"
	"path/filepath"
	"testing"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factorysessions"
	"github.com/portpowered/infinite-you/pkg/factorysessions/logicaltarget"
)

const testBackendScopeID = "backend-scope-001"

func TestNormalizeDefaultTarget_EquivalentAliasesMatch(t *testing.T) {
	root := t.TempDir()
	absoluteFolder, err := filepath.Abs(root)
	if err != nil {
		t.Fatalf("Abs: %v", err)
	}
	if absoluteFolder, err = filepath.EvalSymlinks(absoluteFolder); err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}

	want, err := logicaltarget.NormalizeDefaultTarget(testBackendScopeID, absoluteFolder)
	if err != nil {
		t.Fatalf("NormalizeDefaultTarget: %v", err)
	}

	t.Run("absolute", func(t *testing.T) {
		got, err := logicaltarget.NormalizeDefaultTarget(testBackendScopeID, absoluteFolder)
		if err != nil {
			t.Fatalf("NormalizeDefaultTarget: %v", err)
		}
		if !logicaltarget.Equivalent(want, got) {
			t.Fatalf("canonical = %#v, want %#v", got, want)
		}
	})

	t.Run("trimmed-scope", func(t *testing.T) {
		got, err := logicaltarget.NormalizeDefaultTarget("  "+testBackendScopeID+"  ", absoluteFolder)
		if err != nil {
			t.Fatalf("NormalizeDefaultTarget: %v", err)
		}
		if !logicaltarget.Equivalent(want, got) {
			t.Fatalf("canonical = %#v, want %#v", got, want)
		}
	})

	t.Run("trailing-separator", func(t *testing.T) {
		got, err := logicaltarget.NormalizeDefaultTarget(testBackendScopeID, absoluteFolder+string(os.PathSeparator))
		if err != nil {
			t.Fatalf("NormalizeDefaultTarget: %v", err)
		}
		if !logicaltarget.Equivalent(want, got) {
			t.Fatalf("canonical = %#v, want %#v", got, want)
		}
	})

	t.Run("relative", func(t *testing.T) {
		cwd, err := os.Getwd()
		if err != nil {
			t.Fatalf("Getwd: %v", err)
		}
		t.Cleanup(func() {
			_ = os.Chdir(cwd)
		})
		if err := os.Chdir(root); err != nil {
			t.Fatalf("Chdir: %v", err)
		}
		got, err := logicaltarget.NormalizeDefaultTarget(testBackendScopeID, ".")
		if err != nil {
			t.Fatalf("NormalizeDefaultTarget: %v", err)
		}
		if !logicaltarget.Equivalent(want, got) {
			t.Fatalf("canonical = %#v, want %#v", got, want)
		}
	})
}

func TestIsDefaultSessionSelector_RecognizesRouteAliases(t *testing.T) {
	t.Parallel()

	for _, selector := range []string{"", factorysessions.DefaultSessionID, "default", "  default  "} {
		if !logicaltarget.IsDefaultSessionSelector(selector) {
			t.Fatalf("IsDefaultSessionSelector(%q) = false, want true", selector)
		}
	}
	if logicaltarget.IsDefaultSessionSelector("beta") {
		t.Fatal("IsDefaultSessionSelector(beta) = true, want false")
	}
}

func TestNormalizeFolderPath_EquivalentSpellingsMatch(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "workspace")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	absoluteNested, err := filepath.Abs(nested)
	if err != nil {
		t.Fatalf("Abs: %v", err)
	}

	want, err := logicaltarget.NormalizeFolderPath(absoluteNested)
	if err != nil {
		t.Fatalf("NormalizeFolderPath: %v", err)
	}

	t.Run("absolute", func(t *testing.T) {
		got, err := logicaltarget.NormalizeFolderPath(absoluteNested)
		if err != nil {
			t.Fatalf("NormalizeFolderPath: %v", err)
		}
		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("trailing-separator", func(t *testing.T) {
		got, err := logicaltarget.NormalizeFolderPath(absoluteNested + string(os.PathSeparator))
		if err != nil {
			t.Fatalf("NormalizeFolderPath: %v", err)
		}
		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("relative", func(t *testing.T) {
		cwd, err := os.Getwd()
		if err != nil {
			t.Fatalf("Getwd: %v", err)
		}
		t.Cleanup(func() {
			_ = os.Chdir(cwd)
		})
		if err := os.Chdir(root); err != nil {
			t.Fatalf("Chdir: %v", err)
		}
		got, err := logicaltarget.NormalizeFolderPath("workspace")
		if err != nil {
			t.Fatalf("NormalizeFolderPath: %v", err)
		}
		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})
}

func TestNormalizeNamedTarget_EquivalentFormattingMatchesDistinctNamesDoNot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	absoluteFolder, err := filepath.Abs(root)
	if err != nil {
		t.Fatalf("Abs: %v", err)
	}

	whitespaceName := "  beta  "
	want, err := logicaltarget.NormalizeNamedTarget(testBackendScopeID, absoluteFolder, whitespaceName)
	if err != nil {
		t.Fatalf("NormalizeNamedTarget: %v", err)
	}
	got, err := logicaltarget.NormalizeNamedTarget(testBackendScopeID, absoluteFolder, "beta")
	if err != nil {
		t.Fatalf("NormalizeNamedTarget(beta): %v", err)
	}
	if !logicaltarget.Equivalent(want, got) {
		t.Fatalf("whitespace canonical = %#v, want %#v", got, want)
	}

	otherScope, err := logicaltarget.NormalizeNamedTarget("other-scope", absoluteFolder, "beta")
	if err != nil {
		t.Fatalf("NormalizeNamedTarget(other scope): %v", err)
	}
	if logicaltarget.Equivalent(want, otherScope) {
		t.Fatal("distinct backend scopes must not collide")
	}

	alpha, err := logicaltarget.NormalizeNamedTarget(testBackendScopeID, absoluteFolder, "alpha")
	if err != nil {
		t.Fatalf("NormalizeNamedTarget(alpha): %v", err)
	}
	if logicaltarget.Equivalent(want, alpha) {
		t.Fatal("distinct named targets must not collide")
	}

	scopedName := "@you/goal"
	scoped, err := logicaltarget.NormalizeNamedTarget(testBackendScopeID, absoluteFolder, scopedName)
	if err != nil {
		t.Fatalf("NormalizeNamedTarget(@you/goal): %v", err)
	}
	segment, err := factoryconfig.NamedFactoryNameToLayoutSegment(scopedName)
	if err != nil {
		t.Fatalf("NamedFactoryNameToLayoutSegment: %v", err)
	}
	if scoped.NamedTarget != segment {
		t.Fatalf("NamedTarget = %q, want layout segment %q", scoped.NamedTarget, segment)
	}
}

func TestNormalizeProviderTarget_IncludesStableBoundaryWithoutSecrets(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	absoluteFolder, err := filepath.Abs(root)
	if err != nil {
		t.Fatalf("Abs: %v", err)
	}

	want, err := logicaltarget.NormalizeProviderTarget(
		testBackendScopeID,
		absoluteFolder,
		logicaltarget.ProviderBoundary{
			Provider: "Cursor",
			Kind:     "Workspace",
			Boundary: "team-alpha",
		},
	)
	if err != nil {
		t.Fatalf("NormalizeProviderTarget: %v", err)
	}
	if want.Kind != logicaltarget.KindProvider || want.Provider == nil {
		t.Fatalf("provider canonical = %#v", want)
	}
	if want.Provider.Provider != "cursor" || want.Provider.Kind != "workspace" || want.Provider.Boundary != "team-alpha" {
		t.Fatalf("provider boundary = %#v, want normalized provider scope", want.Provider)
	}

	equivalent, err := logicaltarget.NormalizeProviderTarget(
		" "+testBackendScopeID+" ",
		absoluteFolder+string(os.PathSeparator),
		logicaltarget.ProviderBoundary{
			Provider: " cursor ",
			Kind:     " workspace ",
			Boundary: " team-alpha ",
		},
	)
	if err != nil {
		t.Fatalf("NormalizeProviderTarget(equivalent): %v", err)
	}
	if !logicaltarget.Equivalent(want, equivalent) {
		t.Fatalf("equivalent provider targets = %#v vs %#v", want, equivalent)
	}

	otherProvider, err := logicaltarget.NormalizeProviderTarget(
		testBackendScopeID,
		absoluteFolder,
		logicaltarget.ProviderBoundary{
			Provider: "codex",
			Kind:     "workspace",
			Boundary: "team-alpha",
		},
	)
	if err != nil {
		t.Fatalf("NormalizeProviderTarget(codex): %v", err)
	}
	if logicaltarget.Equivalent(want, otherProvider) {
		t.Fatal("distinct provider kinds must not collide")
	}
}

func TestNormalizeTargetRef_RejectsInvalidAndAmbiguousReferences(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	absoluteFolder, err := filepath.Abs(root)
	if err != nil {
		t.Fatalf("Abs: %v", err)
	}

	_, err = logicaltarget.NormalizeTargetRef("", absoluteFolder, factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault})
	if err == nil {
		t.Fatal("missing backend scope error = nil, want required")
	}
	reason, field, ok := logicaltarget.ValidationReasonFromError(err)
	if !ok || reason != logicaltarget.ReasonRequired || field != "backendScopeId" {
		t.Fatalf("validation = (%q, %q, %v), want required backendScopeId", reason, field, ok)
	}

	_, err = logicaltarget.NormalizeTargetRef(testBackendScopeID, "", factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault})
	if err == nil {
		t.Fatal("missing folder error = nil, want required")
	}
	reason, field, ok = logicaltarget.ValidationReasonFromError(err)
	if !ok || reason != logicaltarget.ReasonRequired || field != "folderPath" {
		t.Fatalf("validation = (%q, %q, %v), want required folderPath", reason, field, ok)
	}

	_, err = logicaltarget.NormalizeTargetRef(
		testBackendScopeID,
		absoluteFolder,
		factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault, Name: "beta"},
	)
	if err == nil {
		t.Fatal("default with name error = nil, want ambiguous")
	}
	reason, field, ok = logicaltarget.ValidationReasonFromError(err)
	if !ok || reason != logicaltarget.ReasonAmbiguousTarget || field != "target" {
		t.Fatalf("validation = (%q, %q, %v), want ambiguous target", reason, field, ok)
	}

	_, err = logicaltarget.NormalizeTargetRef(
		testBackendScopeID,
		absoluteFolder,
		factorysessions.TargetRef{Kind: factorysessions.TargetKindNamed},
	)
	if err == nil {
		t.Fatal("named without name error = nil, want required")
	}
	reason, field, ok = logicaltarget.ValidationReasonFromError(err)
	if !ok || reason != logicaltarget.ReasonRequired || field != "target.name" {
		t.Fatalf("validation = (%q, %q, %v), want required target.name", reason, field, ok)
	}

	_, err = logicaltarget.NormalizeTargetRef(
		testBackendScopeID,
		absoluteFolder,
		factorysessions.TargetRef{Kind: "unsupported"},
	)
	if err == nil {
		t.Fatal("unsupported kind error = nil, want invalid")
	}
	reason, field, ok = logicaltarget.ValidationReasonFromError(err)
	if !ok || reason != logicaltarget.ReasonInvalidTarget || field != "target.kind" {
		t.Fatalf("validation = (%q, %q, %v), want invalid target.kind", reason, field, ok)
	}

	_, err = logicaltarget.NormalizeProviderTarget(
		testBackendScopeID,
		absoluteFolder,
		logicaltarget.ProviderBoundary{
			Provider: "cursor",
			Kind:     "workspace",
			Boundary: "sk-live-secret-token",
		},
	)
	if err == nil {
		t.Fatal("secret boundary error = nil, want invalid")
	}
	reason, field, ok = logicaltarget.ValidationReasonFromError(err)
	if !ok || reason != logicaltarget.ReasonInvalidTarget || field != "provider.boundary" {
		t.Fatalf("validation = (%q, %q, %v), want invalid provider.boundary", reason, field, ok)
	}
}
