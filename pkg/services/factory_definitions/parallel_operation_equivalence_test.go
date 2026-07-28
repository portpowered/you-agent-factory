package factorydefinitions_test

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/lifecycle"
	catalog "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog/wire"
	factorynamedfactories "github.com/portpowered/infinite-you/pkg/services/factory_definitions/namedfactories"
	factorynamedpaths "github.com/portpowered/infinite-you/pkg/services/factory_definitions/namedpaths"
)

func TestParallelOperationEquivalence_ServiceMatchesLegacyCatalogOperations(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	projectRoot := t.TempDir()
	globalRoot := t.TempDir()
	alphaDir := writeParallelOperationNamedFactory(t, projectRoot, "alpha")
	betaDir := writeParallelOperationNamedFactory(t, projectRoot, "beta")
	_ = writeParallelOperationNamedFactory(t, globalRoot, "alpha")

	legacyCatalog, service, paths := newParallelOperationCatalogPair(t)

	legacyListed, err := legacyCatalog.ListNamedFactories(projectRoot)
	if err != nil {
		t.Fatalf("legacy ListNamedFactories: %v", err)
	}
	rootListed, err := service.ListNamedFactories(
		ctx,
		factorydefinitions.ListNamedFactoriesRequest{RootDir: projectRoot},
	)
	if err != nil {
		t.Fatalf("service ListNamedFactories: %v", err)
	}
	if !reflect.DeepEqual(legacyListed, rootListed.Entries) {
		t.Fatalf("list mismatch: legacy = %#v, service = %#v", legacyListed, rootListed.Entries)
	}

	if _, err := service.SetCurrentFactoryPointer(
		ctx,
		factorydefinitions.SetCurrentFactoryPointerRequest{RootDir: projectRoot, Name: "alpha"},
	); err != nil {
		t.Fatalf("SetCurrentFactoryPointer(alpha): %v", err)
	}

	legacyCurrent, err := factorynamedfactories.ResolveCurrent(paths, projectRoot)
	if err != nil {
		t.Fatalf("legacy ResolveCurrent: %v", err)
	}
	serviceCurrent, err := factorydefinitions.ResolveCurrentFactoryDirectory(
		ctx,
		service,
		projectRoot,
	)
	if err != nil {
		t.Fatalf("ResolveCurrentFactoryDirectory: %v", err)
	}
	if legacyCurrent != serviceCurrent || serviceCurrent != alphaDir {
		t.Fatalf(
			"current directory mismatch: legacy = %q, service = %q, want %q",
			legacyCurrent,
			serviceCurrent,
			alphaDir,
		)
	}

	legacyResolved, err := legacyCatalog.ResolveNamedFactoryAcrossRoots(projectRoot, globalRoot, "alpha")
	if err != nil {
		t.Fatalf("legacy ResolveNamedFactoryAcrossRoots: %v", err)
	}
	rootResolved, err := service.ResolveNamedFactory(
		ctx,
		factorydefinitions.ResolveNamedFactoryRequest{
			ProjectRoot: projectRoot,
			GlobalRoot:  globalRoot,
			Name:        "alpha",
		},
	)
	if err != nil {
		t.Fatalf("service ResolveNamedFactory: %v", err)
	}
	if legacyResolved.Name != rootResolved.Resolution.Name ||
		legacyResolved.FactoryDir != rootResolved.Resolution.FactoryDir ||
		legacyResolved.Source != rootResolved.Resolution.Source ||
		legacyResolved.PrecedenceDecision != rootResolved.Resolution.PrecedenceDecision {
		t.Fatalf(
			"resolve mismatch: legacy = %#v, service = %#v",
			legacyResolved,
			rootResolved.Resolution,
		)
	}

	if _, err := service.SetCurrentFactoryPointer(
		ctx,
		factorydefinitions.SetCurrentFactoryPointerRequest{RootDir: projectRoot, Name: "beta"},
	); err != nil {
		t.Fatalf("SetCurrentFactoryPointer(beta): %v", err)
	}

	deleted, err := service.DeleteNamedFactory(
		ctx,
		factorydefinitions.DeleteNamedFactoryRequest{RootDir: projectRoot, Name: "alpha"},
	)
	if err != nil {
		t.Fatalf("service DeleteNamedFactory: %v", err)
	}
	if deleted.Name != "alpha" || deleted.FactoryDir != alphaDir {
		t.Fatalf("DeleteNamedFactory result = %#v, want alpha at %q", deleted, alphaDir)
	}

	legacyListed, err = legacyCatalog.ListNamedFactories(projectRoot)
	if err != nil {
		t.Fatalf("legacy ListNamedFactories after delete: %v", err)
	}
	rootListed, err = service.ListNamedFactories(
		ctx,
		factorydefinitions.ListNamedFactoriesRequest{RootDir: projectRoot},
	)
	if err != nil {
		t.Fatalf("service ListNamedFactories after delete: %v", err)
	}
	if !reflect.DeepEqual(legacyListed, rootListed.Entries) {
		t.Fatalf("list after delete mismatch: legacy = %#v, service = %#v", legacyListed, rootListed.Entries)
	}
	if len(rootListed.Entries) != 1 || rootListed.Entries[0].Name != "beta" || rootListed.Entries[0].FactoryDir != betaDir {
		t.Fatalf("service list after delete = %#v, want only beta at %q", rootListed.Entries, betaDir)
	}
}

func newParallelOperationCatalogPair(
	t *testing.T,
) (factorydefinitions.NamedFactoryCatalog, factorydefinitions.Service, factorydefinitions.NamedPathResolver) {
	t.Helper()

	fileSystem := platformfilesystem.Local{}
	paths, err := factorynamedpaths.New(fileSystem)
	if err != nil {
		t.Fatalf("namedpaths.New: %v", err)
	}
	legacyCatalog, err := factorynamedfactories.New(paths, fileSystem)
	if err != nil {
		t.Fatalf("namedfactories.New: %v", err)
	}
	catalogService, err := catalogwire.NewService(catalog.Dependencies{
		Paths:      paths,
		FileSystem: fileSystem,
	})
	if err != nil {
		t.Fatalf("catalogwire.NewService: %v", err)
	}
	return legacyCatalog, lifecycle.NewWithCatalog(nil, lifecycle.StubActivationGateway(), catalogService), paths
}

func writeParallelOperationNamedFactory(t *testing.T, rootDir, name string) string {
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
