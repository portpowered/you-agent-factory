package logicaltarget_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/logicaltarget"
)

const testBackendScopeID = "backend-scope-001"

func TestNormalizeFolderPathRequiresAndUsesInjectedSymlinkResolver(t *testing.T) {
	if _, err := factorysessions.NormalizeLogicalTargetFolderPath(nil, os.UserHomeDir, t.TempDir()); err == nil {
		t.Fatal("NormalizeLogicalTargetFolderPath(nil) error = nil, want required resolver")
	}

	var received string
	want := filepath.Clean(filepath.Join(t.TempDir(), "canonical"))
	got, err := factorysessions.NormalizeLogicalTargetFolderPath(
		func(path string) (string, error) {
			received = path
			return want, nil
		},
		os.UserHomeDir,
		t.TempDir(),
	)
	if err != nil {
		t.Fatalf("NormalizeLogicalTargetFolderPath(injected resolver): %v", err)
	}
	if received == "" {
		t.Fatal("injected resolver did not receive the absolute folder")
	}
	if got != want {
		t.Fatalf("normalized folder = %q, want injected canonical path %q", got, want)
	}
}

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
	if scoped.NamedTarget != scopedName {
		t.Fatalf("NamedTarget = %q, want canonical display name %q", scoped.NamedTarget, scopedName)
	}
	if strings.Contains(scoped.NamedTarget, "%2F") {
		t.Fatalf("NamedTarget = %q, must not use percent-encoded scoped layout segments", scoped.NamedTarget)
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

	t.Run("missing-backend-scope", func(t *testing.T) {
		_, err := logicaltarget.NormalizeTargetRef("", absoluteFolder, factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault})
		assertLogicalTargetValidationError(t, err, logicaltarget.ReasonRequired, "backendScopeId")
	})
	t.Run("missing-folder-path", func(t *testing.T) {
		_, err := logicaltarget.NormalizeTargetRef(testBackendScopeID, "", factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault})
		assertLogicalTargetValidationError(t, err, logicaltarget.ReasonRequired, "folderPath")
	})
	t.Run("default-with-name", func(t *testing.T) {
		_, err := logicaltarget.NormalizeTargetRef(
			testBackendScopeID,
			absoluteFolder,
			factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault, Name: "beta"},
		)
		assertLogicalTargetValidationError(t, err, logicaltarget.ReasonAmbiguousTarget, "target")
	})
	t.Run("named-without-name", func(t *testing.T) {
		_, err := logicaltarget.NormalizeTargetRef(
			testBackendScopeID,
			absoluteFolder,
			factorysessions.TargetRef{Kind: factorysessions.TargetKindNamed},
		)
		assertLogicalTargetValidationError(t, err, logicaltarget.ReasonRequired, "target.name")
	})
	t.Run("unsupported-kind", func(t *testing.T) {
		_, err := logicaltarget.NormalizeTargetRef(
			testBackendScopeID,
			absoluteFolder,
			factorysessions.TargetRef{Kind: "unsupported"},
		)
		assertLogicalTargetValidationError(t, err, logicaltarget.ReasonInvalidTarget, "target.kind")
	})
	t.Run("secret-provider-boundary", func(t *testing.T) {
		_, err := logicaltarget.NormalizeProviderTarget(
			testBackendScopeID,
			absoluteFolder,
			logicaltarget.ProviderBoundary{
				Provider: "cursor",
				Kind:     "workspace",
				Boundary: "sk-live-secret-token",
			},
		)
		assertLogicalTargetValidationError(t, err, logicaltarget.ReasonInvalidTarget, "provider.boundary")
	})
}

func assertLogicalTargetValidationError(
	t *testing.T,
	err error,
	wantReason string,
	wantField string,
) {
	t.Helper()
	if err == nil {
		t.Fatalf("validation error = nil, want %q on %q", wantReason, wantField)
	}
	reason, field, ok := logicaltarget.ValidationReasonFromError(err)
	if !ok || reason != wantReason || field != wantField {
		t.Fatalf("validation = (%q, %q, %v), want (%q, %q, true)", reason, field, ok, wantReason, wantField)
	}
}
