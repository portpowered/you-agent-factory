package service_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinition "github.com/portpowered/infinite-you/pkg/services/factory_definitions/definition"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog/wire"
	factorynamedpaths "github.com/portpowered/infinite-you/pkg/services/factory_definitions/namedpaths"
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

func TestPrivateCatalog_RequiresInjectedPorts(t *testing.T) {
	t.Parallel()

	fileSystem := platformfilesystem.Local{}
	paths, err := factorynamedpaths.New(fileSystem)
	if err != nil {
		t.Fatalf("namedpaths.New: %v", err)
	}

	if _, err := catalogwire.NewService(nil, fileSystem); err == nil {
		t.Fatal("NewService(nil, filesystem): expected path resolver required error")
	}
	if _, err := catalogwire.NewService(paths, nil); err == nil {
		t.Fatal("NewService(paths, nil): expected catalog filesystem required error")
	}
}

func newRootCatalog(t *testing.T) factorydefinitions.Service {
	t.Helper()

	fileSystem := platformfilesystem.Local{}
	paths, err := factorynamedpaths.New(fileSystem)
	if err != nil {
		t.Fatalf("namedpaths.New: %v", err)
	}
	catalog, err := catalogwire.NewService(paths, fileSystem)
	if err != nil {
		t.Fatalf("catalogwire.NewService: %v", err)
	}
	return factorydefinition.NewWithCatalog(nil, catalog)
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
