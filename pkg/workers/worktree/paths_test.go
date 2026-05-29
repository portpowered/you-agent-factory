package worktree

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestResolveFactoryWorktreeParent_DefaultsToDotWorktrees(t *testing.T) {
	factoryRoot := t.TempDir()

	parent, err := ResolveFactoryWorktreeParent(factoryRoot)
	if err != nil {
		t.Fatalf("ResolveFactoryWorktreeParent() error = %v", err)
	}

	want := filepath.Join(factoryRoot, ".worktrees")
	if parent != want {
		t.Fatalf("parent = %q, want %q", parent, want)
	}
}

func TestResolveFactoryWorktreeParent_PrefersExistingDirectoriesInOrder(t *testing.T) {
	tests := []struct {
		name      string
		seedDirs  []string
		wantRel   string
	}{
		{
			name:     "dot worktrees",
			seedDirs: []string{".worktrees"},
			wantRel:  ".worktrees",
		},
		{
			name:     "worktrees",
			seedDirs: []string{"worktrees"},
			wantRel:  "worktrees",
		},
		{
			name:     "claude worktrees",
			seedDirs: []string{filepath.Join(".claude", "worktrees")},
			wantRel:  filepath.Join(".claude", "worktrees"),
		},
		{
			name:     "precedence dot worktrees over worktrees",
			seedDirs: []string{"worktrees", ".worktrees"},
			wantRel:  ".worktrees",
		},
		{
			name:     "precedence dot worktrees over claude worktrees",
			seedDirs: []string{filepath.Join(".claude", "worktrees"), ".worktrees"},
			wantRel:  ".worktrees",
		},
		{
			name:     "precedence worktrees over claude worktrees",
			seedDirs: []string{filepath.Join(".claude", "worktrees"), "worktrees"},
			wantRel:  "worktrees",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			factoryRoot := t.TempDir()
			for _, rel := range tc.seedDirs {
				if err := os.MkdirAll(filepath.Join(factoryRoot, rel), 0o755); err != nil {
					t.Fatalf("MkdirAll(%q): %v", rel, err)
				}
			}

			parent, err := ResolveFactoryWorktreeParent(factoryRoot)
			if err != nil {
				t.Fatalf("ResolveFactoryWorktreeParent() error = %v", err)
			}

			want := filepath.Join(factoryRoot, tc.wantRel)
			if parent != want {
				t.Fatalf("parent = %q, want %q", parent, want)
			}
		})
	}
}

func TestResolveFactoryWorktreeParent_IgnoresFilesAndMissingPaths(t *testing.T) {
	factoryRoot := t.TempDir()
	for _, rel := range []string{".worktrees", "worktrees", filepath.Join(".claude", "worktrees")} {
		path := filepath.Join(factoryRoot, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("MkdirAll(%q): %v", rel, err)
		}
		if err := os.WriteFile(path, []byte("not-a-directory"), 0o644); err != nil {
			t.Fatalf("WriteFile(%q): %v", rel, err)
		}
	}

	parent, err := ResolveFactoryWorktreeParent(factoryRoot)
	if err != nil {
		t.Fatalf("ResolveFactoryWorktreeParent() error = %v", err)
	}

	want := filepath.Join(factoryRoot, ".worktrees")
	if parent != want {
		t.Fatalf("parent = %q, want %q", parent, want)
	}
}

func TestResolveFactoryWorktreeCheckoutPath_UsesParentAndPlatformCleanedName(t *testing.T) {
	factoryRoot := t.TempDir()

	checkout, err := ResolveFactoryWorktreeCheckoutPath(factoryRoot, "feature-a")
	if err != nil {
		t.Fatalf("ResolveFactoryWorktreeCheckoutPath() error = %v", err)
	}

	want := filepath.Join(factoryRoot, ".worktrees", "feature-a")
	if checkout != want {
		t.Fatalf("checkout = %q, want %q", checkout, want)
	}
}

func TestResolveFactoryWorktreeCheckoutPath_UsesExistingParent(t *testing.T) {
	factoryRoot := t.TempDir()
	existingParent := filepath.Join(factoryRoot, "worktrees")
	if err := os.MkdirAll(existingParent, 0o755); err != nil {
		t.Fatalf("MkdirAll(): %v", err)
	}

	checkout, err := ResolveFactoryWorktreeCheckoutPath(factoryRoot, "feature-a")
	if err != nil {
		t.Fatalf("ResolveFactoryWorktreeCheckoutPath() error = %v", err)
	}

	want := filepath.Join(existingParent, "feature-a")
	if checkout != want {
		t.Fatalf("checkout = %q, want %q", checkout, want)
	}
}

func TestResolveFactoryWorktreeCheckoutPath_NormalizesSlashSeparatedNames(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("slash normalization covered on unix paths")
	}

	factoryRoot := t.TempDir()

	checkout, err := ResolveFactoryWorktreeCheckoutPath(factoryRoot, "feature-a/nested")
	if err != nil {
		t.Fatalf("ResolveFactoryWorktreeCheckoutPath() error = %v", err)
	}

	want := filepath.Join(factoryRoot, ".worktrees", "feature-a", "nested")
	if checkout != want {
		t.Fatalf("checkout = %q, want %q", checkout, want)
	}
}

func TestResolveFactoryWorktreeCheckoutPath_RejectsInvalidNames(t *testing.T) {
	factoryRoot := t.TempDir()

	for _, name := range []string{"", " ", "..", "../escape"} {
		if _, err := ResolveFactoryWorktreeCheckoutPath(factoryRoot, name); err == nil {
			t.Fatalf("ResolveFactoryWorktreeCheckoutPath(%q) error = nil, want error", name)
		}
	}
}

func TestResolveFactoryWorktreeParent_RejectsEmptyFactoryRoot(t *testing.T) {
	if _, err := ResolveFactoryWorktreeParent(""); err == nil {
		t.Fatal("ResolveFactoryWorktreeParent() error = nil, want error")
	}
}
