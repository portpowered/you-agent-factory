package wire_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/directoryreplace"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	catalog "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog/wire"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/impl"
)

type recordingPathResolver struct {
	requireDefinitionDirCalls int
	readCurrentPointerCalls   int
	writeCurrentPointerCalls  int
	resolveExistingDirCalls   int
	currentName               string
	existing                  map[string]string
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

	if svc, err := catalogwire.NewService(nil, fileSystem); err == nil || svc != nil || !strings.Contains(err.Error(), "path resolver is required") {
		t.Fatalf("NewService(nil paths) = %#v, %v; want path resolver required error", svc, err)
	}
	if svc, err := catalogwire.NewService(paths, nil); err == nil || svc != nil || !strings.Contains(err.Error(), "catalog filesystem is required") {
		t.Fatalf("NewService(nil filesystem) = %#v, %v; want catalog filesystem required error", svc, err)
	}

	svc, err := catalogwire.NewService(paths, fileSystem)
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

	svc, err := catalogwire.NewService(paths, fileSystem)
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

func TestNewPathResolverAndCatalogWireResolveNamedFactoryAndCurrentPointer(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	factoryDir := filepath.Join(rootDir, "alpha")
	if err := os.MkdirAll(factoryDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(factoryDir, "factory.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatalf("WriteFile(factory.json): %v", err)
	}

	fileSystem := platformfilesystem.Local{}
	paths, err := catalogwire.NewPathResolver(fileSystem)
	if err != nil {
		t.Fatalf("NewPathResolver: %v", err)
	}
	svc, err := catalogwire.NewService(paths, fileSystem)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	ctx := context.Background()
	got, err := svc.GetNamedFactory(ctx, factorydefinitions.GetNamedFactoryRequest{
		RootDir: rootDir,
		Name:    "alpha",
	})
	if err != nil {
		t.Fatalf("GetNamedFactory: %v", err)
	}
	if got.Entry.FactoryDir != factoryDir {
		t.Fatalf("factoryDir = %q, want %q", got.Entry.FactoryDir, factoryDir)
	}

	if _, err := svc.SetCurrentFactoryPointer(ctx, factorydefinitions.SetCurrentFactoryPointerRequest{
		RootDir: rootDir,
		Name:    "alpha",
	}); err != nil {
		t.Fatalf("SetCurrentFactoryPointer: %v", err)
	}
	pointer, err := svc.GetCurrentFactoryPointer(ctx, factorydefinitions.GetCurrentFactoryPointerRequest{
		RootDir: rootDir,
	})
	if err != nil {
		t.Fatalf("GetCurrentFactoryPointer: %v", err)
	}
	if pointer.Name != "alpha" || pointer.FactoryDir != factoryDir {
		t.Fatalf("pointer = %#v, want alpha at %q", pointer, factoryDir)
	}
}

func TestNewService_ListGetResolveDeleteNamedFactory(t *testing.T) {
	t.Parallel()

	projectRoot := t.TempDir()
	globalRoot := t.TempDir()
	alphaDir := filepath.Join(projectRoot, "alpha")
	betaDir := filepath.Join(projectRoot, "beta")
	for _, spec := range []struct {
		dir string
	}{
		{dir: alphaDir},
		{dir: betaDir},
	} {
		if err := os.MkdirAll(spec.dir, 0o755); err != nil {
			t.Fatalf("MkdirAll(%s): %v", spec.dir, err)
		}
		if err := os.WriteFile(filepath.Join(spec.dir, "factory.json"), []byte(`{}`), 0o644); err != nil {
			t.Fatalf("WriteFile(%s/factory.json): %v", spec.dir, err)
		}
	}
	if err := os.MkdirAll(filepath.Join(globalRoot, "alpha"), 0o755); err != nil {
		t.Fatalf("MkdirAll(global alpha): %v", err)
	}
	if err := os.WriteFile(filepath.Join(globalRoot, "alpha", "factory.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatalf("WriteFile(global alpha/factory.json): %v", err)
	}

	fileSystem := platformfilesystem.Local{}
	paths, err := catalogwire.NewPathResolver(fileSystem)
	if err != nil {
		t.Fatalf("NewPathResolver: %v", err)
	}
	svc, err := catalogwire.NewService(paths, fileSystem)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	ctx := context.Background()
	listed, err := svc.ListNamedFactories(ctx, factorydefinitions.ListNamedFactoriesRequest{RootDir: projectRoot})
	if err != nil {
		t.Fatalf("ListNamedFactories: %v", err)
	}
	if len(listed.Entries) != 2 {
		t.Fatalf("listed entries = %#v, want alpha and beta", listed.Entries)
	}

	got, err := svc.GetNamedFactory(ctx, factorydefinitions.GetNamedFactoryRequest{
		RootDir: projectRoot,
		Name:    "alpha",
	})
	if err != nil {
		t.Fatalf("GetNamedFactory: %v", err)
	}
	if got.Entry.FactoryDir != alphaDir {
		t.Fatalf("factoryDir = %q, want %q", got.Entry.FactoryDir, alphaDir)
	}

	resolved, err := svc.ResolveNamedFactory(ctx, factorydefinitions.ResolveNamedFactoryRequest{
		ProjectRoot: projectRoot,
		GlobalRoot:  globalRoot,
		Name:        "alpha",
	})
	if err != nil {
		t.Fatalf("ResolveNamedFactory: %v", err)
	}
	if resolved.Resolution.FactoryDir != alphaDir ||
		resolved.Resolution.Source != factorydefinitions.NamedFactoryResolutionSourceProjectLocal {
		t.Fatalf("resolution = %#v, want project-local alpha at %q", resolved.Resolution, alphaDir)
	}

	if _, err := svc.SetCurrentFactoryPointer(ctx, factorydefinitions.SetCurrentFactoryPointerRequest{
		RootDir: projectRoot,
		Name:    "beta",
	}); err != nil {
		t.Fatalf("SetCurrentFactoryPointer(beta): %v", err)
	}
	deleted, err := svc.DeleteNamedFactory(ctx, factorydefinitions.DeleteNamedFactoryRequest{
		RootDir: projectRoot,
		Name:    "alpha",
	})
	if err != nil {
		t.Fatalf("DeleteNamedFactory: %v", err)
	}
	if deleted.FactoryDir != alphaDir {
		t.Fatalf("deleted.FactoryDir = %q, want %q", deleted.FactoryDir, alphaDir)
	}

	listed, err = svc.ListNamedFactories(ctx, factorydefinitions.ListNamedFactoriesRequest{RootDir: projectRoot})
	if err != nil {
		t.Fatalf("ListNamedFactories after delete: %v", err)
	}
	if len(listed.Entries) != 1 || listed.Entries[0].Name != "beta" {
		t.Fatalf("listed after delete = %#v, want only beta", listed.Entries)
	}
	_, err = svc.GetNamedFactory(ctx, factorydefinitions.GetNamedFactoryRequest{
		RootDir: projectRoot,
		Name:    "alpha",
	})
	if err == nil || !errors.Is(err, factorydefinitions.ErrNamedFactoryNotFound) {
		t.Fatalf("GetNamedFactory(alpha) after delete = %v, want %v", err, factorydefinitions.ErrNamedFactoryNotFound)
	}
}

func TestNewPersistencePrepareAndCreateAndReplaceNamedFactoryLayout(t *testing.T) {
	t.Parallel()

	fileSystem := platformfilesystem.Local{}
	paths, err := catalogwire.NewPathResolver(fileSystem)
	if err != nil {
		t.Fatalf("NewPathResolver: %v", err)
	}
	validator := factoryvalidation.New(nil)
	prepared := &factorydefinitions.PreparedFactoryLayoutPayload{}
	persistence, err := catalogwire.NewPersistence(
		validator,
		func([]byte) (factorydefinitions.DefinitionValidationRequest, error) {
			return factorydefinitions.DefinitionValidationRequest{
				Config:           &factorydefinitions.FactoryConfig{},
				CanonicalPayload: []byte(`{}`),
				CanonicalFactoryLoader: func([]byte, factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
					return nil, nil
				},
			}, nil
		},
		func(
			_ context.Context,
			segment string,
			payload []byte,
			_ factorydefinitions.Validator,
		) (*factorydefinitions.PreparedFactoryLayoutPayload, error) {
			if segment != "alpha" || string(payload) != "payload" {
				t.Fatalf("prepare values = %q, %q", segment, payload)
			}
			return prepared, nil
		},
		func(stagingDir string, _ *factorydefinitions.PreparedFactoryLayoutPayload, _ string) error {
			return os.WriteFile(
				filepath.Join(stagingDir, factorydefinitions.FactoryConfigFile),
				[]byte(`{}`),
				0o644,
			)
		},
		func(string) error { return nil },
		nil,
		nil,
		nil,
		fileSystem,
		paths.RequireDefinitionDir,
		directoryreplace.Local{},
	)
	if err != nil {
		t.Fatalf("NewPersistence: %v", err)
	}

	ctx := context.Background()
	gotPrepared, err := persistence.PrepareFactoryLayout(ctx, "alpha", []byte("payload"))
	if err != nil || gotPrepared != prepared {
		t.Fatalf("PrepareFactoryLayout() = %p, %v", gotPrepared, err)
	}

	rootDir := t.TempDir()
	targetDir := filepath.Join(rootDir, "alpha")
	createdDir, err := persistence.CreateNamedFactory(rootDir, "alpha", prepared)
	if err != nil || createdDir != targetDir {
		t.Fatalf("CreateNamedFactory() = %q, %v", createdDir, err)
	}
	if _, err := os.Stat(filepath.Join(targetDir, factorydefinitions.FactoryConfigFile)); err != nil {
		t.Fatalf("created factory.json: %v", err)
	}

	replacedDir, err := persistence.ReplaceNamedFactory(rootDir, "alpha", prepared)
	if err != nil || replacedDir != targetDir {
		t.Fatalf("ReplaceNamedFactory() = %q, %v", replacedDir, err)
	}
}
