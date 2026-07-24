package factorydefinition

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	factoryroot "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	catalog "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog/wire"
	factorynamedpaths "github.com/portpowered/infinite-you/pkg/services/factory_definitions/namedpaths"
)

// newRootCatalogServiceForPeer attaches private catalog behind the public root
// Service. Construction may import owner-local wire; peer exercise below must
// not depend on catalog or other Definitions internals beyond the root Service.
func newRootCatalogServiceForPeer(t *testing.T) factoryroot.Service {
	t.Helper()

	fileSystem := platformfilesystem.Local{}
	paths, err := factorynamedpaths.New(fileSystem)
	if err != nil {
		t.Fatalf("namedpaths.New: %v", err)
	}
	catalogService, err := catalogwire.NewService(catalog.Dependencies{
		Paths:      paths,
		FileSystem: fileSystem,
	})
	if err != nil {
		t.Fatalf("catalogwire.NewService: %v", err)
	}
	return NewWithCatalog(nil, catalogService)
}

func writeEquivalenceNamedFactory(t *testing.T, rootDir, name string) string {
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

// peerExerciseRootCatalogSuccess proves a peer-shaped consumer can drive
// CTR-DEF catalog success cases through the attached private implementation
// while depending only on the root Service vocabulary.
func peerExerciseRootCatalogSuccess(t *testing.T, service factoryroot.Service, rootDir, factoryDir string) {
	t.Helper()
	ctx := context.Background()

	if _, err := service.SetCurrentFactoryPointer(
		ctx,
		factoryroot.SetCurrentFactoryPointerRequest{RootDir: rootDir, Name: "alpha"},
	); err != nil {
		t.Fatalf("SetCurrentFactoryPointer: %v", err)
	}

	listed, err := service.ListNamedFactories(
		ctx,
		factoryroot.ListNamedFactoriesRequest{RootDir: rootDir},
	)
	if err != nil {
		t.Fatalf("ListNamedFactories: %v", err)
	}
	if len(listed.Entries) != 1 || listed.Entries[0].Name != "alpha" {
		t.Fatalf("ListNamedFactories result = %#v, want alpha entry", listed)
	}
	if !listed.Entries[0].Current || listed.Entries[0].FactoryDir != factoryDir {
		t.Fatalf("ListNamedFactories entry = %#v, want current alpha at %q", listed.Entries[0], factoryDir)
	}

	got, err := service.GetNamedFactory(
		ctx,
		factoryroot.GetNamedFactoryRequest{RootDir: rootDir, Name: "alpha"},
	)
	if err != nil {
		t.Fatalf("GetNamedFactory: %v", err)
	}
	if got.Entry.Name != "alpha" || got.Entry.FactoryDir != factoryDir || !got.Entry.Current {
		t.Fatalf("GetNamedFactory result = %#v, want current alpha identity facts", got)
	}

	pointer, err := service.GetCurrentFactoryPointer(
		ctx,
		factoryroot.GetCurrentFactoryPointerRequest{RootDir: rootDir},
	)
	if err != nil {
		t.Fatalf("GetCurrentFactoryPointer: %v", err)
	}
	if pointer.Name != "alpha" || pointer.FactoryDir != factoryDir {
		t.Fatalf("GetCurrentFactoryPointer result = %#v, want alpha current pointer", pointer)
	}
}

// peerExerciseRootCatalogTypedFailures proves a peer-shaped consumer can
// distinguish CTR-DEF typed catalog failures through the attached private
// implementation using only root vocabulary.
func peerExerciseRootCatalogTypedFailures(t *testing.T, service factoryroot.Service, rootDir string) {
	t.Helper()
	ctx := context.Background()

	_, invalidErr := service.GetNamedFactory(
		ctx,
		factoryroot.GetNamedFactoryRequest{RootDir: rootDir, Name: "../evil"},
	)
	if !errors.Is(invalidErr, factoryroot.ErrInvalidNamedFactoryName) {
		t.Fatalf(
			"GetNamedFactory invalid-name error = %v, want %v",
			invalidErr,
			factoryroot.ErrInvalidNamedFactoryName,
		)
	}

	_, missingErr := service.GetNamedFactory(
		ctx,
		factoryroot.GetNamedFactoryRequest{RootDir: rootDir, Name: "missing"},
	)
	if !errors.Is(missingErr, factoryroot.ErrNamedFactoryNotFound) {
		t.Fatalf(
			"GetNamedFactory missing error = %v, want %v",
			missingErr,
			factoryroot.ErrNamedFactoryNotFound,
		)
	}
	if errors.Is(missingErr, factoryroot.ErrInvalidNamedFactoryName) {
		t.Fatal("missing named Factory must not also match ErrInvalidNamedFactoryName")
	}

	_, missingPointerErr := service.GetCurrentFactoryPointer(
		ctx,
		factoryroot.GetCurrentFactoryPointerRequest{RootDir: rootDir},
	)
	if !errors.Is(missingPointerErr, factoryroot.ErrCurrentFactoryNotFound) {
		t.Fatalf(
			"GetCurrentFactoryPointer missing error = %v, want %v",
			missingPointerErr,
			factoryroot.ErrCurrentFactoryNotFound,
		)
	}
}

func TestRootCatalogEquivalence_CTRDEFSuccessThroughPrivateImplementation(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	factoryDir := writeEquivalenceNamedFactory(t, rootDir, "alpha")
	service := newRootCatalogServiceForPeer(t)

	peerExerciseRootCatalogSuccess(t, service, rootDir, factoryDir)
}

func TestRootCatalogEquivalence_CTRDEFTypedFailuresThroughPrivateImplementation(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	service := newRootCatalogServiceForPeer(t)

	peerExerciseRootCatalogTypedFailures(t, service, rootDir)
}

func TestRootCatalogEquivalence_PeerExercisesRootWithoutCatalogImport(t *testing.T) {
	t.Parallel()

	// Owner-local construction attaches private catalog. The peer exercise
	// helpers accept only factoryroot.Service and root request/result/error
	// types, proving a peer can drive the slice end-to-end without importing
	// catalog or other Definitions internals.
	rootDir := t.TempDir()
	factoryDir := writeEquivalenceNamedFactory(t, rootDir, "alpha")
	successService := newRootCatalogServiceForPeer(t)
	peerExerciseRootCatalogSuccess(t, successService, rootDir, factoryDir)

	failureRoot := t.TempDir()
	failureService := newRootCatalogServiceForPeer(t)
	peerExerciseRootCatalogTypedFailures(t, failureService, failureRoot)
}
