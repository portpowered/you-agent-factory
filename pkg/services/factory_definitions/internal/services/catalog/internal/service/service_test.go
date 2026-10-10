package service_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	catalogservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog/internal/service"
)

func TestCatalog_GetSucceedsThroughOwnership(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	factoryDir := writeNamedFactory(t, rootDir, "alpha")
	root := newRootCatalog(t)

	got, err := root.GetNamedFactory(
		context.Background(),
		factorydefinitions.GetNamedFactoryRequest{RootDir: rootDir, Name: "alpha"},
	)
	if err != nil {
		t.Fatalf("GetNamedFactory through root: %v", err)
	}
	if got.Entry.Name != "alpha" || got.Entry.FactoryDir != factoryDir {
		t.Fatalf("GetNamedFactory result = %#v, want alpha at %q", got, factoryDir)
	}

	listed, err := root.ListNamedFactories(
		context.Background(),
		factorydefinitions.ListNamedFactoriesRequest{RootDir: rootDir},
	)
	if err != nil {
		t.Fatalf("ListNamedFactories through root: %v", err)
	}
	if len(listed.Entries) != 1 || listed.Entries[0].Name != "alpha" {
		t.Fatalf("ListNamedFactories result = %#v, want alpha entry", listed)
	}
}

func TestCatalog_ListMarksCurrentPointer(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	alphaDir := writeNamedFactory(t, rootDir, "alpha")
	betaDir := writeNamedFactory(t, rootDir, "beta")
	root := newRootCatalog(t)

	if _, err := root.SetCurrentFactoryPointer(
		context.Background(),
		factorydefinitions.SetCurrentFactoryPointerRequest{RootDir: rootDir, Name: "beta"},
	); err != nil {
		t.Fatalf("SetCurrentFactoryPointer: %v", err)
	}

	listed, err := root.ListNamedFactories(
		context.Background(),
		factorydefinitions.ListNamedFactoriesRequest{RootDir: rootDir},
	)
	if err != nil {
		t.Fatalf("ListNamedFactories through root: %v", err)
	}
	if len(listed.Entries) != 2 {
		t.Fatalf("ListNamedFactories entry count = %d, want 2", len(listed.Entries))
	}

	byName := map[string]factorydefinitions.NamedFactoryListEntry{}
	for _, entry := range listed.Entries {
		byName[entry.Name] = entry
	}
	alpha, ok := byName["alpha"]
	if !ok || alpha.FactoryDir != alphaDir || alpha.Current {
		t.Fatalf("alpha list entry = %#v, want non-current at %q", alpha, alphaDir)
	}
	beta, ok := byName["beta"]
	if !ok || beta.FactoryDir != betaDir || !beta.Current {
		t.Fatalf("beta list entry = %#v, want current at %q", beta, betaDir)
	}

	got, err := root.GetNamedFactory(
		context.Background(),
		factorydefinitions.GetNamedFactoryRequest{RootDir: rootDir, Name: "beta"},
	)
	if err != nil {
		t.Fatalf("GetNamedFactory(beta): %v", err)
	}
	if !got.Entry.Current || got.Entry.Name != "beta" || got.Entry.FactoryDir != betaDir {
		t.Fatalf("GetNamedFactory(beta) = %#v, want current beta at %q", got, betaDir)
	}
}

func TestCatalog_ResolveReturnsDetachedFacts(t *testing.T) {
	t.Parallel()

	projectRoot := t.TempDir()
	globalRoot := t.TempDir()
	projectDir := writeNamedFactory(t, projectRoot, "alpha")
	_ = writeNamedFactory(t, globalRoot, "alpha")
	root := newRootCatalog(t)

	resolved, err := root.ResolveNamedFactory(
		context.Background(),
		factorydefinitions.ResolveNamedFactoryRequest{
			ProjectRoot: projectRoot,
			GlobalRoot:  globalRoot,
			Name:        "alpha",
		},
	)
	if err != nil {
		t.Fatalf("ResolveNamedFactory through root: %v", err)
	}
	if resolved.Resolution.Name != "alpha" ||
		resolved.Resolution.FactoryDir != projectDir ||
		resolved.Resolution.Source != factorydefinitions.NamedFactoryResolutionSourceProjectLocal ||
		resolved.Resolution.ProjectRoot != projectRoot ||
		resolved.Resolution.GlobalRoot != globalRoot ||
		resolved.Resolution.PrecedenceDecision != factorydefinitions.NamedFactoryPrecedenceDecisionProjectOverGlobal {
		t.Fatalf("ResolveNamedFactory result = %#v, want project-local alpha", resolved)
	}
}

func TestCatalog_DeleteRemovesFromSubsequentListGet(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	alphaDir := writeNamedFactory(t, rootDir, "alpha")
	_ = writeNamedFactory(t, rootDir, "beta")
	root := newRootCatalog(t)

	deleted, err := root.DeleteNamedFactory(
		context.Background(),
		factorydefinitions.DeleteNamedFactoryRequest{RootDir: rootDir, Name: "alpha"},
	)
	if err != nil {
		t.Fatalf("DeleteNamedFactory through root: %v", err)
	}
	if deleted.Name != "alpha" || deleted.FactoryDir != alphaDir {
		t.Fatalf("DeleteNamedFactory result = %#v, want alpha at %q", deleted, alphaDir)
	}

	listed, err := root.ListNamedFactories(
		context.Background(),
		factorydefinitions.ListNamedFactoriesRequest{RootDir: rootDir},
	)
	if err != nil {
		t.Fatalf("ListNamedFactories after delete: %v", err)
	}
	if len(listed.Entries) != 1 || listed.Entries[0].Name != "beta" {
		t.Fatalf("ListNamedFactories after delete = %#v, want only beta", listed)
	}

	_, getErr := root.GetNamedFactory(
		context.Background(),
		factorydefinitions.GetNamedFactoryRequest{RootDir: rootDir, Name: "alpha"},
	)
	if !errors.Is(getErr, factorydefinitions.ErrNamedFactoryNotFound) {
		t.Fatalf(
			"GetNamedFactory(alpha) after delete error = %v, want %v",
			getErr,
			factorydefinitions.ErrNamedFactoryNotFound,
		)
	}
}

func TestCatalog_CurrentPointerUpdateSelectsOnlyNewCurrent(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	alphaDir := writeNamedFactory(t, rootDir, "alpha")
	betaDir := writeNamedFactory(t, rootDir, "beta")
	root := newRootCatalog(t)

	set, err := root.SetCurrentFactoryPointer(
		context.Background(),
		factorydefinitions.SetCurrentFactoryPointerRequest{RootDir: rootDir, Name: "alpha"},
	)
	if err != nil {
		t.Fatalf("SetCurrentFactoryPointer(alpha): %v", err)
	}
	if set.Name != "alpha" {
		t.Fatalf("SetCurrentFactoryPointer(alpha) result = %#v, want alpha", set)
	}

	pointer, err := root.GetCurrentFactoryPointer(
		context.Background(),
		factorydefinitions.GetCurrentFactoryPointerRequest{RootDir: rootDir},
	)
	if err != nil {
		t.Fatalf("GetCurrentFactoryPointer after set alpha: %v", err)
	}
	if pointer.Name != "alpha" || pointer.FactoryDir != alphaDir {
		t.Fatalf("GetCurrentFactoryPointer after set alpha = %#v, want alpha at %q", pointer, alphaDir)
	}

	if _, err := root.SetCurrentFactoryPointer(
		context.Background(),
		factorydefinitions.SetCurrentFactoryPointerRequest{RootDir: rootDir, Name: "beta"},
	); err != nil {
		t.Fatalf("SetCurrentFactoryPointer(beta): %v", err)
	}

	pointer, err = root.GetCurrentFactoryPointer(
		context.Background(),
		factorydefinitions.GetCurrentFactoryPointerRequest{RootDir: rootDir},
	)
	if err != nil {
		t.Fatalf("GetCurrentFactoryPointer after set beta: %v", err)
	}
	if pointer.Name != "beta" || pointer.FactoryDir != betaDir {
		t.Fatalf("GetCurrentFactoryPointer after set beta = %#v, want beta at %q", pointer, betaDir)
	}

	listed, err := root.ListNamedFactories(
		context.Background(),
		factorydefinitions.ListNamedFactoriesRequest{RootDir: rootDir},
	)
	if err != nil {
		t.Fatalf("ListNamedFactories after pointer update: %v", err)
	}
	byName := map[string]factorydefinitions.NamedFactoryListEntry{}
	for _, entry := range listed.Entries {
		byName[entry.Name] = entry
	}
	if alpha := byName["alpha"]; alpha.Current {
		t.Fatalf("alpha list entry still marked current after switch to beta: %#v", alpha)
	}
	if beta := byName["beta"]; !beta.Current || beta.FactoryDir != betaDir {
		t.Fatalf("beta list entry = %#v, want current at %q", beta, betaDir)
	}
}

func TestCatalog_FailedCurrentPointerUpdatePreservesPrior(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	alphaDir := writeNamedFactory(t, rootDir, "alpha")
	root := newRootCatalog(t)

	if _, err := root.SetCurrentFactoryPointer(
		context.Background(),
		factorydefinitions.SetCurrentFactoryPointerRequest{RootDir: rootDir, Name: "alpha"},
	); err != nil {
		t.Fatalf("SetCurrentFactoryPointer(alpha): %v", err)
	}

	_, missingErr := root.SetCurrentFactoryPointer(
		context.Background(),
		factorydefinitions.SetCurrentFactoryPointerRequest{RootDir: rootDir, Name: "missing"},
	)
	if !errors.Is(missingErr, factorydefinitions.ErrNamedFactoryNotFound) {
		t.Fatalf(
			"SetCurrentFactoryPointer(missing) error = %v, want %v",
			missingErr,
			factorydefinitions.ErrNamedFactoryNotFound,
		)
	}

	pointer, err := root.GetCurrentFactoryPointer(
		context.Background(),
		factorydefinitions.GetCurrentFactoryPointerRequest{RootDir: rootDir},
	)
	if err != nil {
		t.Fatalf("GetCurrentFactoryPointer after missing set: %v", err)
	}
	if pointer.Name != "alpha" || pointer.FactoryDir != alphaDir {
		t.Fatalf(
			"GetCurrentFactoryPointer after missing set = %#v, want preserved alpha at %q",
			pointer,
			alphaDir,
		)
	}

	_, invalidErr := root.SetCurrentFactoryPointer(
		context.Background(),
		factorydefinitions.SetCurrentFactoryPointerRequest{RootDir: rootDir, Name: "../evil"},
	)
	assertTypedInvalidName(t, "SetCurrentFactoryPointer", invalidErr)

	pointer, err = root.GetCurrentFactoryPointer(
		context.Background(),
		factorydefinitions.GetCurrentFactoryPointerRequest{RootDir: rootDir},
	)
	if err != nil {
		t.Fatalf("GetCurrentFactoryPointer after invalid-name set: %v", err)
	}
	if pointer.Name != "alpha" || pointer.FactoryDir != alphaDir {
		t.Fatalf(
			"GetCurrentFactoryPointer after invalid-name set = %#v, want preserved alpha at %q",
			pointer,
			alphaDir,
		)
	}
}

func TestCatalog_TypedInvalidNameFailures(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	_ = writeNamedFactory(t, rootDir, "alpha")
	root := newRootCatalog(t)
	invalidName := "../evil"

	_, getErr := root.GetNamedFactory(
		context.Background(),
		factorydefinitions.GetNamedFactoryRequest{RootDir: rootDir, Name: invalidName},
	)
	assertTypedInvalidName(t, "GetNamedFactory", getErr)

	_, resolveErr := root.ResolveNamedFactory(
		context.Background(),
		factorydefinitions.ResolveNamedFactoryRequest{
			ProjectRoot: rootDir,
			GlobalRoot:  t.TempDir(),
			Name:        invalidName,
		},
	)
	assertTypedInvalidName(t, "ResolveNamedFactory", resolveErr)

	_, deleteErr := root.DeleteNamedFactory(
		context.Background(),
		factorydefinitions.DeleteNamedFactoryRequest{RootDir: rootDir, Name: invalidName},
	)
	assertTypedInvalidName(t, "DeleteNamedFactory", deleteErr)

	_, setErr := root.SetCurrentFactoryPointer(
		context.Background(),
		factorydefinitions.SetCurrentFactoryPointerRequest{RootDir: rootDir, Name: invalidName},
	)
	assertTypedInvalidName(t, "SetCurrentFactoryPointer", setErr)
}

func TestCatalog_TypedMissingAndCurrentNotFound(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	_ = writeNamedFactory(t, rootDir, "alpha")
	root := newRootCatalog(t)

	_, missingErr := root.GetNamedFactory(
		context.Background(),
		factorydefinitions.GetNamedFactoryRequest{RootDir: rootDir, Name: "missing"},
	)
	if !errors.Is(missingErr, factorydefinitions.ErrNamedFactoryNotFound) {
		t.Fatalf(
			"GetNamedFactory missing error = %v, want %v",
			missingErr,
			factorydefinitions.ErrNamedFactoryNotFound,
		)
	}
	if errors.Is(missingErr, factorydefinitions.ErrInvalidNamedFactoryName) {
		t.Fatal("missing named Factory must not also match ErrInvalidNamedFactoryName")
	}

	_, resolveErr := root.ResolveNamedFactory(
		context.Background(),
		factorydefinitions.ResolveNamedFactoryRequest{
			ProjectRoot: rootDir,
			GlobalRoot:  t.TempDir(),
			Name:        "missing",
		},
	)
	if !errors.Is(resolveErr, factorydefinitions.ErrNamedFactoryNotFound) {
		t.Fatalf(
			"ResolveNamedFactory missing error = %v, want %v",
			resolveErr,
			factorydefinitions.ErrNamedFactoryNotFound,
		)
	}

	_, deleteErr := root.DeleteNamedFactory(
		context.Background(),
		factorydefinitions.DeleteNamedFactoryRequest{RootDir: rootDir, Name: "missing"},
	)
	if !errors.Is(deleteErr, factorydefinitions.ErrNamedFactoryNotFound) {
		t.Fatalf(
			"DeleteNamedFactory missing error = %v, want %v",
			deleteErr,
			factorydefinitions.ErrNamedFactoryNotFound,
		)
	}

	_, pointerErr := root.GetCurrentFactoryPointer(
		context.Background(),
		factorydefinitions.GetCurrentFactoryPointerRequest{RootDir: rootDir},
	)
	if !errors.Is(pointerErr, factorydefinitions.ErrCurrentFactoryNotFound) {
		t.Fatalf(
			"GetCurrentFactoryPointer missing error = %v, want %v",
			pointerErr,
			factorydefinitions.ErrCurrentFactoryNotFound,
		)
	}
	if errors.Is(pointerErr, factorydefinitions.ErrNamedFactoryNotFound) {
		t.Fatal("missing current pointer must not also match ErrNamedFactoryNotFound")
	}
}

func assertTypedInvalidName(t *testing.T, op string, err error) {
	t.Helper()
	if !errors.Is(err, factorydefinitions.ErrInvalidNamedFactoryName) {
		t.Fatalf("%s invalid-name error = %v, want %v", op, err, factorydefinitions.ErrInvalidNamedFactoryName)
	}
	if errors.Is(err, factorydefinitions.ErrNamedFactoryNotFound) {
		t.Fatalf("%s invalid-name error also matched ErrNamedFactoryNotFound: %v", op, err)
	}
}

// Catalog owns file inspection/deletion. Path selection and pointer storage are
// controlled peers, rather than a second real resolver and Lifecycle graph.
func newRootCatalog(t *testing.T) *catalogservice.Service {
	t.Helper()
	return catalogservice.New(&catalogPaths{current: map[string]string{}}, platformfilesystem.Local{})
}

type catalogPaths struct {
	factorydefinitions.NamedPathResolver
	current map[string]string
}

func (*catalogPaths) RequireDefinitionDir(dir string) error {
	_, err := os.Stat(filepath.Join(dir, "factory.json"))
	return err
}
func (p *catalogPaths) ResolveExistingDir(root, name string) (string, error) {
	dir := filepath.Join(root, name)
	if err := p.RequireDefinitionDir(dir); err != nil {
		return "", factorydefinitions.ErrNamedFactoryNotFound
	}
	return dir, nil
}
func (p *catalogPaths) ReadCurrentPointer(root string) (string, error) {
	if name, ok := p.current[root]; ok {
		return name, nil
	}
	return "", os.ErrNotExist
}
func (p *catalogPaths) WriteCurrentPointer(root, name string) error {
	p.current[root] = name
	return nil
}

func writeNamedFactory(t *testing.T, rootDir, name string) string {
	t.Helper()

	factoryDir := filepath.Join(rootDir, filepath.FromSlash(name))
	if err := os.MkdirAll(factoryDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", name, err)
	}
	if err := os.WriteFile(filepath.Join(factoryDir, "factory.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatalf("WriteFile(%s/factory.json): %v", name, err)
	}
	return factoryDir
}

type recordingPathResolver struct {
	resolveCandidatePathsCalls int
	resolveCurrentDirCalls     int
	requireDefinitionDirCalls  int
	readCurrentPointerCalls    int
	writeCurrentPointerCalls   int
	resolveExistingDirCalls    int
	currentName                string
	existing                   map[string]string
}

func (r *recordingPathResolver) ResolveCandidatePaths(_, _, _ string) (factorydefinitions.NamedFactoryCandidatePaths, error) {
	r.resolveCandidatePathsCalls++
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
	r.resolveCurrentDirCalls++
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

func TestCatalogConstructionIsInert(t *testing.T) {
	t.Parallel()
	paths := &recordingPathResolver{}
	fileSystem := &recordingCatalogFileSystem{}
	svc := catalogservice.New(paths, fileSystem)
	if svc == nil {
		t.Fatal("NewService returned nil service")
	}
	if paths.resolveCandidatePathsCalls != 0 || paths.resolveCurrentDirCalls != 0 ||
		paths.requireDefinitionDirCalls != 0 || paths.readCurrentPointerCalls != 0 ||
		paths.writeCurrentPointerCalls != 0 || paths.resolveExistingDirCalls != 0 ||
		fileSystem.statCalls != 0 || fileSystem.readDirCalls != 0 || fileSystem.removeAllCalls != 0 {
		t.Fatal("catalog construction performed a host effect")
	}
}

func TestCatalogEffectsUseSelectedPorts(t *testing.T) {
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

	svc := catalogservice.New(paths, fileSystem)

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
