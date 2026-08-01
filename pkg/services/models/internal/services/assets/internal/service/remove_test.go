package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	models "github.com/portpowered/infinite-you/pkg/services/models"
)

func TestRemoveModelAssetsRemovesExistingCacheAndIsIdempotent(t *testing.T) {
	t.Parallel()

	cacheDirectory := t.TempDir()
	writeCacheFixture(t, cacheDirectory, true)
	scopes := newScopes(t, "remove-existing")
	ref := openScope(t, scopes, cacheDirectory, runtimeConfig(""))
	service := newTestService(scopes, nil)

	result, err := service.RemoveModelAssets(context.Background(), modelsRemoveRequest(ref))
	if err != nil {
		t.Fatalf("RemoveModelAssets: %v", err)
	}
	assertRemovedResult(t, result, AssetRemovalExpectation{
		modelName: "OMNIVOICE_Q4_K_M",
		outcome:   models.AssetRemovalRemoved,
	})
	assertPathAbsent(t, filepath.Join(cacheDirectory, "OMNIVOICE_Q4_K_M"))

	repeated, err := service.RemoveModelAssets(context.Background(), modelsRemoveRequest(ref))
	if err != nil {
		t.Fatalf("RemoveModelAssets repeated: %v", err)
	}
	assertRemovedResult(t, repeated, AssetRemovalExpectation{
		modelName: "OMNIVOICE_Q4_K_M",
		outcome:   models.AssetRemovalAlreadyAbsent,
	})
}

func TestRemoveModelAssetsReportsConfiguredModelWithoutCacheAsAlreadyAbsent(t *testing.T) {
	t.Parallel()

	cacheDirectory := t.TempDir()
	scopes := newScopes(t, "remove-absent")
	ref := openScope(t, scopes, cacheDirectory, runtimeConfig(""))
	service := newTestService(scopes, nil)

	result, err := service.RemoveModelAssets(context.Background(), modelsRemoveRequest(ref))
	if err != nil {
		t.Fatalf("RemoveModelAssets absent: %v", err)
	}
	assertRemovedResult(t, result, AssetRemovalExpectation{
		modelName: "OMNIVOICE_Q4_K_M",
		outcome:   models.AssetRemovalAlreadyAbsent,
	})
}

func TestRemoveModelAssetsRejectsInvalidScopesAndIdentitiesBeforeFilesystemEffects(t *testing.T) {
	t.Parallel()

	cacheDirectory := t.TempDir()
	outsidePath := filepath.Join(cacheDirectory, "outside-marker")
	if err := os.WriteFile(outsidePath, []byte("preserve"), 0o644); err != nil {
		t.Fatalf("write outside marker: %v", err)
	}
	scopes := newScopes(t, "remove-validation")
	ref := openScope(t, scopes, cacheDirectory, runtimeConfig(""))
	foreignScopes := newScopes(t, "remove-foreign")
	foreign := openScope(t, foreignScopes, cacheDirectory, runtimeConfig(""))
	service := newTestService(scopes, nil)

	tests := []struct {
		name  string
		ref   models.RuntimeScopeRef
		model string
		want  error
	}{
		{name: "empty identity", ref: ref, want: models.ErrNotFound},
		{
			name:  "identity path traversal",
			ref:   ref,
			model: filepath.Join("..", "outside-marker"),
			want:  models.ErrAssetSourceUnsupported,
		},
		{name: "unsupported identity", ref: ref, model: "OTHER_MODEL", want: models.ErrAssetSourceUnsupported},
		{name: "foreign scope", ref: foreign, model: "OMNIVOICE_Q4_K_M", want: models.ErrRuntimeScopeForeign},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := modelsRemoveRequest(test.ref)
			request.Name = test.model
			_, err := service.RemoveModelAssets(context.Background(), request)
			if !errors.Is(err, test.want) {
				t.Fatalf("RemoveModelAssets error = %v, want %v", err, test.want)
			}
		})
	}
	assertPathPresent(t, outsidePath)
}

func TestRemoveModelAssetsRejectsMissingSourceAndMalformedCacheRoot(t *testing.T) {
	t.Parallel()

	cacheDirectory := t.TempDir()
	scopes := newScopes(t, "remove-shape")
	missingSource := openScope(t, scopes, cacheDirectory, models.RuntimeConfig{})
	service := newTestService(scopes, nil)
	if _, err := service.RemoveModelAssets(context.Background(), modelsRemoveRequest(missingSource)); !errors.Is(err, models.ErrAssetSourceMissing) {
		t.Fatalf("missing source error = %v, want ErrAssetSourceMissing", err)
	}

	cachePath := filepath.Join(cacheDirectory, "OMNIVOICE_Q4_K_M")
	if err := os.WriteFile(cachePath, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("write malformed cache root: %v", err)
	}
	ref := openScope(t, scopes, cacheDirectory, runtimeConfig(""))
	if _, err := service.RemoveModelAssets(context.Background(), modelsRemoveRequest(ref)); !errors.Is(err, models.ErrAssetUnavailable) {
		t.Fatalf("malformed cache root error = %v, want ErrAssetUnavailable", err)
	}
	assertPathPresent(t, cachePath)
}

func TestRemoveModelAssetsRejectsDirectoryEntriesOutsideModelRoot(t *testing.T) {
	t.Parallel()

	cacheDirectory := t.TempDir()
	modelRoot := filepath.Join(cacheDirectory, "OMNIVOICE_Q4_K_M")
	if err := os.MkdirAll(modelRoot, 0o755); err != nil {
		t.Fatalf("create model root: %v", err)
	}
	outsidePath := filepath.Join(cacheDirectory, "outside-marker")
	if err := os.WriteFile(outsidePath, []byte("preserve"), 0o644); err != nil {
		t.Fatalf("write outside marker: %v", err)
	}
	scopes := newScopes(t, "remove-path-boundary")
	ref := openScope(t, scopes, cacheDirectory, runtimeConfig(""))
	service := newTestService(scopes, nil)
	originalReadDirectory := service.readDirectory
	service.readDirectory = func(path string) ([]os.DirEntry, error) {
		if filepath.Clean(path) == filepath.Clean(modelRoot) {
			return []os.DirEntry{unsafeAssetDirEntry{name: filepath.Join("..", "outside-marker")}}, nil
		}
		return originalReadDirectory(path)
	}

	_, err := service.RemoveModelAssets(context.Background(), modelsRemoveRequest(ref))
	if !errors.Is(err, errAssetCachePathOutsideRoot) || !errors.Is(err, models.ErrAssetUnavailable) {
		t.Fatalf("path-boundary error = %v, want boundary and unavailable classifications", err)
	}
	assertPathPresent(t, outsidePath)
}

func TestRemoveModelAssetsPropagatesFilesystemFailures(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name  string
		setup func(*service, string, error)
	}{
		{
			name: "inspect",
			setup: func(service *service, modelRoot string, failure error) {
				original := service.inspectPath
				service.inspectPath = func(path string) (os.FileInfo, error) {
					if filepath.Clean(path) == filepath.Clean(modelRoot) {
						return nil, failure
					}
					return original(path)
				}
			},
		},
		{
			name: "read directory",
			setup: func(service *service, modelRoot string, failure error) {
				original := service.readDirectory
				service.readDirectory = func(path string) ([]os.DirEntry, error) {
					if filepath.Clean(path) == filepath.Clean(modelRoot) {
						return nil, failure
					}
					return original(path)
				}
			},
		},
		{
			name: "remove path",
			setup: func(service *service, _ string, failure error) {
				original := service.removePath
				service.removePath = func(path string) error {
					if filepath.Base(path) == "omnivoice-base-Q4_K_M.gguf" {
						return failure
					}
					return original(path)
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			cacheDirectory := t.TempDir()
			writeCacheFixture(t, cacheDirectory, true)
			scopes := newScopes(t, "remove-failure-"+test.name)
			ref := openScope(t, scopes, cacheDirectory, runtimeConfig(""))
			service := newTestService(scopes, nil)
			failure := errors.New(test.name + " denied")
			test.setup(service, filepath.Join(cacheDirectory, "OMNIVOICE_Q4_K_M"), failure)

			_, err := service.RemoveModelAssets(context.Background(), modelsRemoveRequest(ref))
			if !errors.Is(err, failure) {
				t.Fatalf("filesystem error = %v, want injected failure %v", err, failure)
			}
		})
	}
}

func TestRemoveModelAssetsHonorsPreCancelledAndInFlightCancellation(t *testing.T) {
	t.Parallel()

	t.Run("pre-cancelled", func(t *testing.T) {
		cacheDirectory := t.TempDir()
		writeCacheFixture(t, cacheDirectory, true)
		scopes := newScopes(t, "remove-pre-cancelled")
		ref := openScope(t, scopes, cacheDirectory, runtimeConfig(""))
		service := newTestService(scopes, nil)
		cause := errors.New("operator stopped removal")
		ctx, cancel := context.WithCancelCause(context.Background())
		cancel(cause)

		_, err := service.RemoveModelAssets(ctx, modelsRemoveRequest(ref))
		assertAssetCancellation(t, err, cause)
		assertPathPresent(t, filepath.Join(cacheDirectory, "OMNIVOICE_Q4_K_M"))
	})

	t.Run("in-flight", func(t *testing.T) {
		cacheDirectory := t.TempDir()
		writeCacheFixture(t, cacheDirectory, true)
		scopes := newScopes(t, "remove-in-flight")
		ref := openScope(t, scopes, cacheDirectory, runtimeConfig(""))
		service := newTestService(scopes, nil)
		cause := errors.New("scope closed during removal")
		ctx, cancel := context.WithCancelCause(context.Background())
		var removals int
		originalRemovePath := service.removePath
		service.removePath = func(path string) error {
			removals++
			err := originalRemovePath(path)
			if removals == 1 {
				cancel(cause)
			}
			return err
		}

		_, err := service.RemoveModelAssets(ctx, modelsRemoveRequest(ref))
		assertAssetCancellation(t, err, cause)
		if removals != 1 {
			t.Fatalf("removal effects = %d, want cancellation to stop after first effect", removals)
		}
	})
}

type AssetRemovalExpectation struct {
	modelName string
	outcome   models.AssetRemovalOutcome
}

func assertRemovedResult(t *testing.T, result models.RemoveModelAssetsResult, want AssetRemovalExpectation) {
	t.Helper()
	if result.ModelName != want.modelName || result.Readiness != models.AssetReadinessMissing || result.Outcome != want.outcome {
		t.Fatalf("RemoveModelAssets = %#v, want model=%q readiness=%q outcome=%q", result, want.modelName, models.AssetReadinessMissing, want.outcome)
	}
}

func assertAssetCancellation(t *testing.T, err error, cause error) {
	t.Helper()
	if !errors.Is(err, models.ErrAssetCancelled) || !errors.Is(err, context.Canceled) || !errors.Is(err, cause) {
		t.Fatalf("cancellation error = %v, want ErrAssetCancelled, context.Canceled, and %v", err, cause)
	}
}

func assertPathPresent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("path %q is not present: %v", path, err)
	}
}

func assertPathAbsent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("path %q = %v, want absent", path, err)
	}
}

func modelsRemoveRequest(scope models.RuntimeScopeRef) models.RemoveModelAssetsRequest {
	return models.RemoveModelAssetsRequest{Scope: scope, Name: "OMNIVOICE_Q4_K_M"}
}

type unsafeAssetDirEntry struct{ name string }

func (entry unsafeAssetDirEntry) Name() string         { return entry.name }
func (unsafeAssetDirEntry) IsDir() bool                { return false }
func (unsafeAssetDirEntry) Type() os.FileMode          { return 0 }
func (unsafeAssetDirEntry) Info() (os.FileInfo, error) { return nil, errors.New("unsafe test entry") }
