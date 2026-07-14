package agypty_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/workers/agypty"
)

func TestWorkspaceFixtures(t *testing.T) {
	t.Parallel()

	factoryRoot := t.TempDir()
	fixtures, err := agypty.LoadWorkspaceFixtures()
	if err != nil {
		t.Fatalf("LoadWorkspaceFixtures() error = %v", err)
	}

	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.Name, func(t *testing.T) {
			t.Parallel()

			root := factoryRoot
			if fixture.FactoryRoot != "FACTORY_ROOT" {
				root = fixture.FactoryRoot
			}

			got, err := agypty.ResolveWorkspaceDir(root, fixture.RawPath)
			if fixture.WantError != "" {
				if err == nil {
					t.Fatal("ResolveWorkspaceDir() error = nil, want error")
				}
				if !strings.Contains(err.Error(), fixture.WantError) {
					t.Fatalf("ResolveWorkspaceDir() error = %v, want substring %q", err, fixture.WantError)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveWorkspaceDir() error = %v", err)
			}

			want := filepath.Join(append([]string{root}, fixture.WantSuffix...)...)
			if got != want {
				t.Fatalf("ResolveWorkspaceDir() = %q, want %q", got, want)
			}
			if !strings.HasPrefix(got, root+string(filepath.Separator)) && got != root {
				t.Fatalf("resolved path %q is not under factory root %q", got, root)
			}
		})
	}
}

func TestResolveWorkspaceDir_RejectsEmptyFactoryRoot(t *testing.T) {
	t.Parallel()

	if _, err := agypty.ResolveWorkspaceDir("", "workspaces/a"); err == nil {
		t.Fatal("ResolveWorkspaceDir() error = nil, want error")
	}
}

func TestResolveWorkspaceDir_RejectsAbsoluteOutsideRoot(t *testing.T) {
	t.Parallel()

	factoryRoot := t.TempDir()
	outside := filepath.Join(filepath.Dir(factoryRoot), "outside-agypty-workspace")

	if _, err := agypty.ResolveWorkspaceDir(factoryRoot, outside); err == nil {
		t.Fatal("ResolveWorkspaceDir() error = nil, want error for path outside factory root")
	}
}
