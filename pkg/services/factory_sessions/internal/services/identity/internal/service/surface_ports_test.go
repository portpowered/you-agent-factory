package service_test

import (
	"context"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
	"time"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	identity "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/identity"
	identitywire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/identity/wire"
)

// Compile-time seal: identity construction accepts only exact host-effect ports.
// Adding peer implementations, Runtime/Petri types, or Wire/root ownership fields
// to Dependencies would break this assignment without an intentional edit here.
var _ = identity.Dependencies{
	ResolveSymlinks: func(string) (string, error) { return "", nil },
	ResolveHome:     func() (string, error) { return "", nil },
	Directories:     surfaceDirectories{},
}

func TestWireConstructionRejectsMissingEffectPorts(t *testing.T) {
	t.Parallel()

	symlink := factorysessions.LogicalTargetResolveSymlinks(func(string) (string, error) {
		return "canonical", nil
	})
	home := factorysessions.HomeDirectoryResolver(func() (string, error) { return "home", nil })
	directories := factorysessions.DirectoryInspection(surfaceDirectories{})

	tests := []struct {
		name string
		deps identity.Dependencies
		want string
	}{
		{
			name: "missing symlink resolver",
			deps: identity.Dependencies{ResolveHome: home, Directories: directories},
			want: "symlink resolver is required",
		},
		{
			name: "missing home resolver",
			deps: identity.Dependencies{ResolveSymlinks: symlink, Directories: directories},
			want: "home resolver is required",
		},
		{
			name: "missing directory inspection",
			deps: identity.Dependencies{ResolveSymlinks: symlink, ResolveHome: home},
			want: "directory inspection is required",
		},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			svc, err := identitywire.NewService(tc.deps)
			if svc != nil || err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("NewService(%s) = %#v, %v; want error containing %q", tc.name, svc, err, tc.want)
			}
		})
	}
}

func TestNormalizeAndResolveFolderUseExactInjectedEffectPorts(t *testing.T) {
	t.Parallel()

	canonicalFolder := filepath.Clean(filepath.Join(t.TempDir(), "canonical"))
	var symlinkCalls, homeCalls, statCalls int
	svc, err := identitywire.NewService(identity.Dependencies{
		ResolveSymlinks: func(path string) (string, error) {
			symlinkCalls++
			if path == "" {
				t.Fatal("ResolveSymlinks received empty path")
			}
			return canonicalFolder, nil
		},
		ResolveHome: func() (string, error) {
			homeCalls++
			return "injected-home", nil
		},
		Directories: &recordingDirectories{stat: func(string) (fs.FileInfo, error) {
			statCalls++
			return surfaceFileInfo{}, nil
		}},
	})
	if err != nil {
		t.Fatalf("identitywire.NewService: %v", err)
	}

	resolved, err := svc.Normalize(context.Background(), identity.NormalizeRequest{
		BackendScopeID: "backend-scope",
		FolderPath:     "submitted-folder",
		Target:         factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
	})
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if resolved.Reference.FolderPath != canonicalFolder {
		t.Fatalf("Normalize folder = %q, want injected canonical %q", resolved.Reference.FolderPath, canonicalFolder)
	}
	if symlinkCalls == 0 {
		t.Fatal("Normalize did not invoke injected symlink resolver")
	}

	folder, err := svc.ResolveFolder("~/sessions")
	if err != nil {
		t.Fatalf("ResolveFolder: %v", err)
	}
	if !strings.Contains(folder, "injected-home") && !strings.Contains(folder, "sessions") {
		t.Fatalf("ResolveFolder = %q, want path derived from injected home", folder)
	}
	if homeCalls == 0 {
		t.Fatal("ResolveFolder did not invoke injected home resolver")
	}
	if statCalls == 0 {
		t.Fatal("ResolveFolder did not invoke injected directory inspection")
	}
}

type surfaceDirectories struct{}

func (surfaceDirectories) Stat(string) (fs.FileInfo, error)      { return surfaceFileInfo{}, nil }
func (surfaceDirectories) ReadDir(string) ([]fs.DirEntry, error) { return nil, nil }

type recordingDirectories struct {
	stat func(string) (fs.FileInfo, error)
}

func (d *recordingDirectories) Stat(path string) (fs.FileInfo, error) {
	if d.stat != nil {
		return d.stat(path)
	}
	return surfaceFileInfo{}, nil
}

func (d *recordingDirectories) ReadDir(string) ([]fs.DirEntry, error) { return nil, nil }

type surfaceFileInfo struct{}

func (surfaceFileInfo) Name() string       { return "folder" }
func (surfaceFileInfo) Size() int64        { return 0 }
func (surfaceFileInfo) Mode() fs.FileMode  { return fs.ModeDir }
func (surfaceFileInfo) ModTime() time.Time { return time.Time{} }
func (surfaceFileInfo) IsDir() bool        { return true }
func (surfaceFileInfo) Sys() any           { return nil }
