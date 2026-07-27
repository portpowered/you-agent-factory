package wire_test

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	catalog "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog/wire"
)

type recordingPathResolver struct {
	requireDefinitionDirCalls int
	readCurrentPointerCalls   int
	writeCurrentPointerCalls  int
	resolveExistingDirCalls   int
	currentName               string
	existing                  map[string]string
}

func (r *recordingPathResolver) ResolveCandidatePaths(_, _, _ string) (factorydefinitions.NamedFactoryCandidatePaths, error) {
	return factorydefinitions.NamedFactoryCandidatePaths{}, nil
}

func (r *recordingPathResolver) ResolveExistingDir(rootDir, name string) (string, error) {
	r.resolveExistingDirCalls++
	if dir, ok := r.existing[name]; ok {
		return dir, nil
	}
	return "", factorydefinitions.ErrNamedFactoryNotFound
}

func (r *recordingPathResolver) RequireDefinitionDir(factoryDir string) error {
	r.requireDefinitionDirCalls++
	for _, dir := range r.existing {
		if dir == factoryDir {
			return nil
		}
	}
	return os.ErrNotExist
}

func (r *recordingPathResolver) ResolveCurrentDir(rootDir string) (string, error) {
	if r.currentName == "" {
		return "", os.ErrNotExist
	}
	return r.ResolveExistingDir(rootDir, r.currentName)
}

func (r *recordingPathResolver) ReadCurrentPointer(string) (string, error) {
	r.readCurrentPointerCalls++
	if r.currentName == "" {
		return "", os.ErrNotExist
	}
	return r.currentName, nil
}

func (r *recordingPathResolver) WriteCurrentPointer(_ string, name string) error {
	r.writeCurrentPointerCalls++
	r.currentName = name
	return nil
}

type recordingCatalogFileSystem struct {
	statCalls      int
	readDirCalls   int
	removeAllCalls int
	entries        map[string][]fakeDirEntry
	nodes          map[string]fakeFileInfo
}

type fakeDirEntry struct {
	name  string
	isDir bool
}

func (e fakeDirEntry) Name() string { return e.name }
func (e fakeDirEntry) IsDir() bool  { return e.isDir }
func (e fakeDirEntry) Type() fs.FileMode {
	if e.isDir {
		return fs.ModeDir
	}
	return 0
}
func (e fakeDirEntry) Info() (fs.FileInfo, error) {
	return fakeFileInfo{name: e.name, isDir: e.isDir}, nil
}

type fakeFileInfo struct {
	name  string
	isDir bool
}

func (i fakeFileInfo) Name() string { return i.name }
func (i fakeFileInfo) Size() int64  { return 0 }
func (i fakeFileInfo) Mode() fs.FileMode {
	if i.isDir {
		return fs.ModeDir
	}
	return 0o644
}
func (i fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (i fakeFileInfo) IsDir() bool        { return i.isDir }
func (i fakeFileInfo) Sys() any           { return nil }

func (f *recordingCatalogFileSystem) Stat(path string) (fs.FileInfo, error) {
	f.statCalls++
	if info, ok := f.nodes[path]; ok {
		return info, nil
	}
	return nil, os.ErrNotExist
}

func (f *recordingCatalogFileSystem) ReadDir(path string) ([]fs.DirEntry, error) {
	f.readDirCalls++
	entries, ok := f.entries[path]
	if !ok {
		return nil, os.ErrNotExist
	}
	out := make([]fs.DirEntry, 0, len(entries))
	for _, entry := range entries {
		out = append(out, entry)
	}
	return out, nil
}

func (f *recordingCatalogFileSystem) RemoveAll(path string) error {
	f.removeAllCalls++
	delete(f.nodes, path)
	return nil
}

func TestNewService_RequiresExactInjectedPorts(t *testing.T) {
	t.Parallel()

	paths := &recordingPathResolver{}
	fileSystem := &recordingCatalogFileSystem{}

	if svc, err := catalogwire.NewService(catalog.Dependencies{
		Paths:      nil,
		FileSystem: fileSystem,
	}); err == nil || svc != nil || !strings.Contains(err.Error(), "path resolver is required") {
		t.Fatalf("NewService(nil paths) = %#v, %v; want path resolver required error", svc, err)
	}
	if svc, err := catalogwire.NewService(catalog.Dependencies{
		Paths:      paths,
		FileSystem: nil,
	}); err == nil || svc != nil || !strings.Contains(err.Error(), "catalog filesystem is required") {
		t.Fatalf("NewService(nil filesystem) = %#v, %v; want catalog filesystem required error", svc, err)
	}

	svc, err := catalogwire.NewService(catalog.Dependencies{
		Paths:      paths,
		FileSystem: fileSystem,
	})
	if err != nil {
		t.Fatalf("NewService with exact injected ports: %v", err)
	}
	if svc == nil {
		t.Fatal("NewService returned nil service")
	}
	var _ catalog.Service = svc
}

func TestNewService_HostEffectsComeOnlyFromInjectedPorts(t *testing.T) {
	t.Parallel()

	rootDir := filepath.Join(string(filepath.Separator), "factories")
	factoryDir := filepath.Join(rootDir, "alpha")
	paths := &recordingPathResolver{
		existing: map[string]string{"alpha": factoryDir},
	}
	factoryJSON := filepath.Join(factoryDir, "factory.json")
	fileSystem := &recordingCatalogFileSystem{
		nodes: map[string]fakeFileInfo{
			rootDir:     {name: filepath.Base(rootDir), isDir: true},
			factoryDir:  {name: "alpha", isDir: true},
			factoryJSON: {name: "factory.json", isDir: false},
		},
		entries: map[string][]fakeDirEntry{
			rootDir: {{name: "alpha", isDir: true}},
			factoryDir: {
				{name: "factory.json", isDir: false},
			},
		},
	}

	svc, err := catalogwire.NewService(catalog.Dependencies{
		Paths:      paths,
		FileSystem: fileSystem,
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	ctx := context.Background()
	listed, err := svc.ListNamedFactories(ctx, factorydefinitions.ListNamedFactoriesRequest{RootDir: rootDir})
	if err != nil {
		t.Fatalf("ListNamedFactories: %v", err)
	}
	if len(listed.Entries) != 1 || listed.Entries[0].Name != "alpha" {
		t.Fatalf("ListNamedFactories = %#v, want alpha through injected ports", listed)
	}
	if fileSystem.readDirCalls == 0 || fileSystem.statCalls == 0 {
		t.Fatal("list did not use the injected NamedFactoryCatalogFileSystem port")
	}
	if paths.readCurrentPointerCalls == 0 {
		t.Fatal("list did not use the injected NamedPathResolver port")
	}

	if _, err := svc.SetCurrentFactoryPointer(ctx, factorydefinitions.SetCurrentFactoryPointerRequest{
		RootDir: rootDir,
		Name:    "alpha",
	}); err != nil {
		t.Fatalf("SetCurrentFactoryPointer: %v", err)
	}
	if paths.requireDefinitionDirCalls == 0 || paths.writeCurrentPointerCalls == 0 {
		t.Fatal("set-current did not use the injected NamedPathResolver port")
	}
	if paths.currentName != "alpha" {
		t.Fatalf("current pointer = %q, want alpha written through injected port", paths.currentName)
	}
}
