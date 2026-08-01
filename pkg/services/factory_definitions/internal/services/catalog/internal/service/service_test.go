package service_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/lifecycle"
	catalog "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog"
	catalognamedpaths "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog/internal/namedpaths"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog/wire"
)

func TestPrivateCatalog_RootGetSucceedsThroughOwnership(t *testing.T) {
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

func TestPrivateCatalog_RootListMarksCurrentPointer(t *testing.T) {
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

func TestPrivateCatalog_RootResolveReturnsDetachedFacts(t *testing.T) {
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

func TestPrivateCatalog_RootDeleteRemovesFromSubsequentListGet(t *testing.T) {
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

func TestPrivateCatalog_RootCurrentPointerUpdatePersistsAtomically(t *testing.T) {
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

func TestPrivateCatalog_RootFailedCurrentPointerUpdatePreservesPrior(t *testing.T) {
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

func TestPrivateCatalog_RootTypedInvalidNameFailures(t *testing.T) {
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

func TestPrivateCatalog_RootTypedMissingAndCurrentNotFound(t *testing.T) {
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

func TestPrivateCatalog_RequiresInjectedPorts(t *testing.T) {
	t.Parallel()

	fileSystem := platformfilesystem.Local{}
	paths, err := catalognamedpaths.New(fileSystem)
	if err != nil {
		t.Fatalf("catalognamedpaths.New: %v", err)
	}

	if _, err := catalogwire.NewService(catalog.Dependencies{
		Paths:      nil,
		FileSystem: fileSystem,
	}); err == nil {
		t.Fatal("NewService(nil paths): expected path resolver required error")
	}
	if _, err := catalogwire.NewService(catalog.Dependencies{
		Paths:      paths,
		FileSystem: nil,
	}); err == nil {
		t.Fatal("NewService(nil filesystem): expected catalog filesystem required error")
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

func newRootCatalog(t *testing.T) factorydefinitions.Service {
	t.Helper()

	fileSystem := platformfilesystem.Local{}
	paths, err := catalognamedpaths.New(fileSystem)
	if err != nil {
		t.Fatalf("catalognamedpaths.New: %v", err)
	}
	catalogService, err := catalogwire.NewService(catalog.Dependencies{
		Paths:      paths,
		FileSystem: fileSystem,
	})
	if err != nil {
		t.Fatalf("catalogwire.NewService: %v", err)
	}
	return lifecycle.NewWithCatalog(catalogService)
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
