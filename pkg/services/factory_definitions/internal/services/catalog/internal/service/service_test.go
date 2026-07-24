package service_test

import (
	"context"
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
	factoryDir := filepath.Join(rootDir, "alpha")
	if err := os.MkdirAll(factoryDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(factoryDir, "factory.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatalf("WriteFile(factory.json): %v", err)
	}

	fileSystem := platformfilesystem.Local{}
	paths, err := factorynamedpaths.New(fileSystem)
	if err != nil {
		t.Fatalf("namedpaths.New: %v", err)
	}
	catalog, err := catalogwire.NewService(paths, fileSystem)
	if err != nil {
		t.Fatalf("catalogwire.NewService: %v", err)
	}

	var root factorydefinitions.Service = factorydefinition.NewWithCatalog(nil, catalog)
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
