package factorydefinition_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil/factoryfixtures"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	factoryroot "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinition "github.com/portpowered/infinite-you/pkg/services/factory_definitions/definition"
	catalog "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog"
	factorynamedpaths "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog/namedpaths"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog/wire"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/impl"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// crossPathPrePersistFixtures are shared JSON fixtures used to prove validate and
// editable-save pre-checks agree when both use ProfilePrePersist.
var crossPathPrePersistFixtures = []struct {
	name     string
	decode   func() (factoryapi.Factory, error)
	wantFail bool
}{
	{
		name:     "cross_path_invalid",
		decode:   factoryfixtures.DecodeCrossPathInvalidFactory,
		wantFail: true,
	},
	{
		name:     "cross_path_valid_alpha",
		decode:   factoryfixtures.DecodeCrossPathValidAlphaFactory,
		wantFail: false,
	},
}

func TestCrossPathFixtures_ValidateFactoryAPIPrePersistMatchesEditableSavePreCheck(t *testing.T) {
	t.Parallel()

	for _, tc := range crossPathPrePersistFixtures {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			factory, err := tc.decode()
			if err != nil {
				t.Fatalf("decode fixture: %v", err)
			}

			apiResult, err := validateFactoryAPIPrePersistForTest(context.Background(), factory)
			if err != nil {
				t.Fatalf("ValidateFactoryAPI: %v", err)
			}

			saveErr := validateEditableFactoryTopology(factory, nil)
			apiFailed := apiResult.HasBlockingTargets()
			var topologyErr *factoryroot.ValidationTopologyError
			saveFailed := errors.As(saveErr, &topologyErr)

			if apiFailed != tc.wantFail {
				t.Fatalf("ValidateFactoryAPI failed = %v, want %v (targets = %#v)", apiFailed, tc.wantFail, apiResult.Targets)
			}
			if saveFailed != tc.wantFail {
				t.Fatalf("validateEditableFactoryTopology failed = %v, want %v (err = %v)", saveFailed, tc.wantFail, saveErr)
			}
			if apiFailed != saveFailed {
				t.Fatalf("ValidateFactoryAPI failed = %v, save pre-check failed = %v, want identical failure vs success",
					apiFailed, saveFailed)
			}
			if !tc.wantFail {
				return
			}

			apiSignatures := factoryvalidation.CanonicalTargetSignatures(apiResult.BlockingTargets())
			saveSignatures := factoryvalidation.CanonicalTargetSignatures(topologyErr.Targets)
			if !factoryvalidation.EquivalentCanonicalTargetSignatures(apiSignatures, saveSignatures) {
				t.Fatalf("ValidateFactoryAPI signatures = %#v, save signatures = %#v",
					apiSignatures, saveSignatures)
			}
		})
	}
}

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
	return factorydefinition.NewWithCatalog(nil, factorydefinition.StubActivationGateway(), catalogService)
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

// peerExerciseRootCatalogDeleteResolveAndCurrent proves a peer-shaped consumer can
// drive delete, resolve, and current-factory lookup through the attached private
// implementation while depending only on the root Service vocabulary.
func peerExerciseRootCatalogDeleteResolveAndCurrent(
	t *testing.T,
	service factoryroot.Service,
	projectRoot,
	globalRoot,
	factoryDir string,
) {
	t.Helper()
	ctx := context.Background()

	resolved, err := service.ResolveNamedFactory(
		ctx,
		factoryroot.ResolveNamedFactoryRequest{
			ProjectRoot: projectRoot,
			GlobalRoot:  globalRoot,
			Name:        "alpha",
		},
	)
	if err != nil {
		t.Fatalf("ResolveNamedFactory: %v", err)
	}
	if resolved.Resolution.Name != "alpha" || resolved.Resolution.FactoryDir != factoryDir {
		t.Fatalf("ResolveNamedFactory result = %#v, want alpha at %q", resolved, factoryDir)
	}

	if _, err := service.SetCurrentFactoryPointer(
		ctx,
		factoryroot.SetCurrentFactoryPointerRequest{RootDir: projectRoot, Name: "alpha"},
	); err != nil {
		t.Fatalf("SetCurrentFactoryPointer: %v", err)
	}

	currentDir, err := factoryroot.ResolveCurrentFactoryDirectory(ctx, service, projectRoot)
	if err != nil {
		t.Fatalf("ResolveCurrentFactoryDirectory: %v", err)
	}
	if currentDir != factoryDir {
		t.Fatalf("ResolveCurrentFactoryDirectory = %q, want %q", currentDir, factoryDir)
	}

	if _, err := service.SetCurrentFactoryPointer(
		ctx,
		factoryroot.SetCurrentFactoryPointerRequest{RootDir: projectRoot, Name: "beta"},
	); err != nil {
		t.Fatalf("SetCurrentFactoryPointer(beta): %v", err)
	}

	deleted, err := service.DeleteNamedFactory(
		ctx,
		factoryroot.DeleteNamedFactoryRequest{RootDir: projectRoot, Name: "alpha"},
	)
	if err != nil {
		t.Fatalf("DeleteNamedFactory: %v", err)
	}
	if deleted.Name != "alpha" || deleted.FactoryDir != factoryDir {
		t.Fatalf("DeleteNamedFactory result = %#v, want alpha at %q", deleted, factoryDir)
	}

	listed, err := service.ListNamedFactories(
		ctx,
		factoryroot.ListNamedFactoriesRequest{RootDir: projectRoot},
	)
	if err != nil {
		t.Fatalf("ListNamedFactories after delete: %v", err)
	}
	if len(listed.Entries) != 1 || listed.Entries[0].Name != "beta" {
		t.Fatalf("ListNamedFactories after delete = %#v, want only beta", listed.Entries)
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

func TestRootCatalogEquivalence_DeleteResolveAndCurrentThroughPrivateImplementation(t *testing.T) {
	t.Parallel()

	projectRoot := t.TempDir()
	globalRoot := t.TempDir()
	factoryDir := writeEquivalenceNamedFactory(t, projectRoot, "alpha")
	_ = writeEquivalenceNamedFactory(t, projectRoot, "beta")
	_ = writeEquivalenceNamedFactory(t, globalRoot, "alpha")
	service := newRootCatalogServiceForPeer(t)

	peerExerciseRootCatalogDeleteResolveAndCurrent(t, service, projectRoot, globalRoot, factoryDir)
}
