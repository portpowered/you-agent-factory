package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
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
	assertRemovedResult(t, result, "OMNIVOICE_Q4_K_M", models.AssetRemovalRemoved)
	assertPathAbsent(t, filepath.Join(cacheDirectory, "OMNIVOICE_Q4_K_M"))

	repeated, err := service.RemoveModelAssets(context.Background(), modelsRemoveRequest(ref))
	if err != nil {
		t.Fatalf("RemoveModelAssets repeated: %v", err)
	}
	assertRemovedResult(t, repeated, "OMNIVOICE_Q4_K_M", models.AssetRemovalAlreadyAbsent)
}

func TestRemoveModelAssetsDoesNotRequireSourceOrPlatformReadiness(t *testing.T) {
	t.Parallel()

	cacheDirectory := t.TempDir()
	writeCacheFixture(t, cacheDirectory, true)
	scopes := newScopes(t, "remove-stale-cache")
	ref := openScope(t, scopes, cacheDirectory, models.RuntimeConfig{})
	var cacheReads int
	service := newTestService(scopes, &cacheReads)
	service.platform = models.AssetHostPlatform{}
	service.client = nil
	service.endpoints = models.RuntimeAssetEndpoints{}

	result, err := service.RemoveModelAssets(context.Background(), modelsRemoveRequest(ref))
	if err != nil {
		t.Fatalf("RemoveModelAssets stale cache: %v", err)
	}
	assertRemovedResult(t, result, "OMNIVOICE_Q4_K_M", models.AssetRemovalRemoved)
	assertPathAbsent(t, filepath.Join(cacheDirectory, "OMNIVOICE_Q4_K_M"))
	if cacheReads != 0 {
		t.Fatalf("removal consulted source/cache inspection effects %d times, want zero", cacheReads)
	}
}

func TestRemoveModelAssetsReportsAbsentCacheWithoutCreatingParent(t *testing.T) {
	t.Parallel()

	cacheDirectory := filepath.Join(t.TempDir(), "not-created")
	scopes := newScopes(t, "remove-absent")
	ref := openScope(t, scopes, cacheDirectory, models.RuntimeConfig{})
	service := newTestService(scopes, nil)

	result, err := service.RemoveModelAssets(context.Background(), modelsRemoveRequest(ref))
	if err != nil {
		t.Fatalf("RemoveModelAssets absent: %v", err)
	}
	assertRemovedResult(t, result, "OMNIVOICE_Q4_K_M", models.AssetRemovalAlreadyAbsent)
	if _, err := os.Stat(cacheDirectory); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cache parent = %v, want it to remain absent", err)
	}
}

func TestModelCacheParentNormalizesValidRelativeDirectory(t *testing.T) {
	t.Chdir(t.TempDir())

	service := &service{}
	got, err := service.modelCacheParent("managed-model-cache")
	if err != nil {
		t.Fatalf("modelCacheParent: %v", err)
	}
	want, err := filepath.Abs("managed-model-cache")
	if err != nil {
		t.Fatalf("filepath.Abs: %v", err)
	}
	if got != want {
		t.Fatalf("modelCacheParent = %q, want %q", got, want)
	}
}

func TestModelCacheParentNormalizesRelativeDotComponents(t *testing.T) {
	t.Chdir(t.TempDir())

	service := &service{}
	for _, input := range []string{
		filepath.Join(".", "managed-model-cache"),
		filepath.Join("nested", "..", "managed-model-cache"),
	} {
		got, err := service.modelCacheParent(input)
		if err != nil {
			t.Fatalf("modelCacheParent(%q): %v", input, err)
		}
		want, err := filepath.Abs(filepath.Clean(input))
		if err != nil {
			t.Fatalf("filepath.Abs(%q): %v", input, err)
		}
		if got != want {
			t.Fatalf("modelCacheParent(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestRemoveModelAssetsRejectsInvalidScopesAndIdentitiesBeforeFilesystemEffects(t *testing.T) {
	t.Parallel()

	cacheDirectory := t.TempDir()
	outsidePath := filepath.Join(cacheDirectory, "outside-marker")
	if err := os.WriteFile(outsidePath, []byte("preserve"), 0o644); err != nil {
		t.Fatalf("write outside marker: %v", err)
	}
	scopes := newScopes(t, "remove-validation")
	ref := openScope(t, scopes, cacheDirectory, models.RuntimeConfig{})
	foreignScopes := newScopes(t, "remove-foreign")
	foreign := openScope(t, foreignScopes, cacheDirectory, models.RuntimeConfig{})
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

func TestRemoveModelAssetsPreservesMalformedCacheAndTypedFilesystemFailure(t *testing.T) {
	t.Parallel()

	cacheDirectory := t.TempDir()
	modelRoot := filepath.Join(cacheDirectory, "OMNIVOICE_Q4_K_M")
	if err := os.WriteFile(modelRoot, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("write malformed cache root: %v", err)
	}
	scopes := newScopes(t, "remove-malformed")
	ref := openScope(t, scopes, cacheDirectory, models.RuntimeConfig{})
	service := newTestService(scopes, nil)

	_, err := service.RemoveModelAssets(context.Background(), modelsRemoveRequest(ref))
	if !errors.Is(err, models.ErrAssetUnavailable) {
		t.Fatalf("malformed cache error = %v, want ErrAssetUnavailable", err)
	}
	assertPathPresent(t, modelRoot)

	failure := errors.New("injected remove failure")
	var calls int
	service.removeTree = func(context.Context, string, string) (modelseffects.AssetRemoveTreeResult, error) {
		calls++
		return modelseffects.AssetRemoveTreeResult{State: modelseffects.AssetRemoveTreeRemaining}, failure
	}
	if _, err := service.RemoveModelAssets(context.Background(), modelsRemoveRequest(ref)); !errors.Is(err, models.ErrAssetUnavailable) || !errors.Is(err, failure) {
		t.Fatalf("typed removal failure = %v, want unavailable and injected failure", err)
	}
	if calls != 1 {
		t.Fatalf("removal effect calls = %d, want exactly once", calls)
	}
}

func TestRemoveModelAssetsHonorsPreCancelledAndInFlightCancellation(t *testing.T) {
	t.Parallel()

	t.Run("pre-cancelled", func(t *testing.T) {
		cacheDirectory := t.TempDir()
		writeCacheFixture(t, cacheDirectory, true)
		scopes := newScopes(t, "remove-pre-cancelled")
		ref := openScope(t, scopes, cacheDirectory, models.RuntimeConfig{})
		service := newTestService(scopes, nil)
		var calls int
		service.removeTree = func(context.Context, string, string) (modelseffects.AssetRemoveTreeResult, error) {
			calls++
			return modelseffects.AssetRemoveTreeResult{State: modelseffects.AssetRemoveTreeNotAttempted}, nil
		}
		cause := errors.New("operator stopped removal")
		ctx, cancel := context.WithCancelCause(context.Background())
		cancel(cause)

		_, err := service.RemoveModelAssets(ctx, modelsRemoveRequest(ref))
		assertAssetCancellation(t, err, cause)
		if calls != 0 {
			t.Fatalf("pre-cancelled removal effect calls = %d, want zero", calls)
		}
		assertPathPresent(t, filepath.Join(cacheDirectory, "OMNIVOICE_Q4_K_M"))
	})

	t.Run("in-flight", func(t *testing.T) {
		cacheDirectory := t.TempDir()
		writeCacheFixture(t, cacheDirectory, true)
		scopes := newScopes(t, "remove-in-flight")
		ref := openScope(t, scopes, cacheDirectory, models.RuntimeConfig{})
		service := newTestService(scopes, nil)
		cause := errors.New("scope closed during removal")
		ctx, cancel := context.WithCancelCause(context.Background())
		var calls int
		service.removeTree = func(context.Context, string, string) (modelseffects.AssetRemoveTreeResult, error) {
			calls++
			cancel(cause)
			return modelseffects.AssetRemoveTreeResult{State: modelseffects.AssetRemoveTreeRemaining}, context.Canceled
		}

		_, err := service.RemoveModelAssets(ctx, modelsRemoveRequest(ref))
		assertAssetCancellation(t, err, cause)
		if calls != 1 {
			t.Fatalf("in-flight removal effect calls = %d, want exactly once", calls)
		}
	})
}

func TestRemoveModelAssetsLogsSafeStartAndTerminalOutcome(t *testing.T) {
	t.Parallel()

	cacheDirectory := t.TempDir()
	scopes := newScopes(t, "remove-logging")
	ref := openScope(t, scopes, cacheDirectory, models.RuntimeConfig{})
	service := newTestService(scopes, nil)
	core, logs := observer.New(zap.WarnLevel)
	service.logger = zap.New(core)
	times := []time.Time{
		time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, time.January, 1, 0, 0, 0, int(125*time.Millisecond), time.UTC),
	}
	service.now = func() time.Time {
		value := times[0]
		times = times[1:]
		return value
	}

	result, err := service.RemoveModelAssets(context.Background(), modelsRemoveRequest(ref))
	if err != nil {
		t.Fatalf("RemoveModelAssets: %v", err)
	}
	assertRemovedResult(t, result, "OMNIVOICE_Q4_K_M", models.AssetRemovalAlreadyAbsent)
	observed := logs.All()
	if len(observed) != 2 {
		t.Fatalf("log count = %d, want start and terminal", len(observed))
	}
	startFields := observed[0].ContextMap()
	if startFields["operation"] != assetRemovalOperation ||
		startFields["phase"] != "start" || startFields["model"] != "OMNIVOICE_Q4_K_M" {
		t.Fatalf("start fields = %#v", startFields)
	}
	terminalFields := observed[1].ContextMap()
	if terminalFields["operation"] != assetRemovalOperation ||
		terminalFields["phase"] != "terminal" ||
		terminalFields["model"] != "OMNIVOICE_Q4_K_M" ||
		terminalFields["outcome"] != string(models.AssetRemovalAlreadyAbsent) ||
		terminalFields["error_classification"] != "none" ||
		terminalFields["cancelled"] != false ||
		terminalFields["removal_state"] != string(modelseffects.AssetRemoveTreeAbsent) ||
		terminalFields["duration_ms"] != int64(125) {
		t.Fatalf("terminal fields = %#v", terminalFields)
	}
	for _, log := range observed {
		if strings.Contains(log.Message, cacheDirectory) {
			t.Fatalf("log message leaked cache path: %q", log.Message)
		}
		fields := log.ContextMap()
		if _, ok := fields["path"]; ok {
			t.Fatalf("log fields leaked path: %#v", fields)
		}
	}
}

func TestRemoveModelAssetsLogsCancellationClassificationWithoutErrorText(t *testing.T) {
	t.Parallel()

	cacheDirectory := t.TempDir()
	scopes := newScopes(t, "remove-cancel-logging")
	ref := openScope(t, scopes, cacheDirectory, models.RuntimeConfig{})
	service := newTestService(scopes, nil)
	core, logs := observer.New(zap.WarnLevel)
	service.logger = zap.New(core)
	service.removeTree = func(ctx context.Context, _ string, _ string) (modelseffects.AssetRemoveTreeResult, error) {
		return modelseffects.AssetRemoveTreeResult{State: modelseffects.AssetRemoveTreeRemaining}, ctx.Err()
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := service.RemoveModelAssets(ctx, modelsRemoveRequest(ref))
	if !errors.Is(err, models.ErrAssetCancelled) {
		t.Fatalf("cancellation error = %v, want ErrAssetCancelled", err)
	}
	observed := logs.All()
	if len(observed) != 2 {
		t.Fatalf("log count = %d, want start and terminal", len(observed))
	}
	terminalFields := observed[1].ContextMap()
	if terminalFields["error_classification"] != "cancelled" ||
		terminalFields["outcome"] != "ERROR" ||
		terminalFields["cancelled"] != true ||
		terminalFields["removal_state"] != string(modelseffects.AssetRemoveTreeNotAttempted) {
		t.Fatalf("cancellation terminal fields = %#v", terminalFields)
	}
	if _, ok := terminalFields["error"]; ok {
		t.Fatalf("terminal log included raw error: %#v", terminalFields)
	}
}

func TestRemoveModelAssetsDoesNotMisclassifyCompletedDeletionAsPartialOnLateCancellation(t *testing.T) {
	t.Parallel()

	cacheDirectory := t.TempDir()
	scopes := newScopes(t, "remove-late-cancel-logging")
	ref := openScope(t, scopes, cacheDirectory, models.RuntimeConfig{})
	service := newTestService(scopes, nil)
	core, logs := observer.New(zap.WarnLevel)
	service.logger = zap.New(core)
	ctx, cancel := context.WithCancel(context.Background())
	service.removeTree = func(context.Context, string, string) (modelseffects.AssetRemoveTreeResult, error) {
		cancel()
		return modelseffects.AssetRemoveTreeResult{State: modelseffects.AssetRemoveTreeRemoved}, nil
	}

	_, err := service.RemoveModelAssets(ctx, modelsRemoveRequest(ref))
	if !errors.Is(err, models.ErrAssetCancelled) {
		t.Fatalf("late cancellation error = %v, want ErrAssetCancelled", err)
	}
	observed := logs.All()
	if len(observed) != 2 {
		t.Fatalf("log count = %d, want start and terminal", len(observed))
	}
	terminalFields := observed[1].ContextMap()
	if terminalFields["cancelled"] != true ||
		terminalFields["removal_state"] != string(modelseffects.AssetRemoveTreeRemoved) {
		t.Fatalf("late-cancellation terminal fields = %#v", terminalFields)
	}
}

func TestRemoveModelAssetsLogsFilesystemClassificationWithoutPath(t *testing.T) {
	t.Parallel()

	cacheDirectory := t.TempDir()
	scopes := newScopes(t, "remove-error-logging")
	ref := openScope(t, scopes, cacheDirectory, models.RuntimeConfig{})
	service := newTestService(scopes, nil)
	core, logs := observer.New(zap.WarnLevel)
	service.logger = zap.New(core)
	failure := errors.New("permission denied for " + cacheDirectory)
	service.removeTree = func(context.Context, string, string) (modelseffects.AssetRemoveTreeResult, error) {
		return modelseffects.AssetRemoveTreeResult{State: modelseffects.AssetRemoveTreeRemaining}, failure
	}

	_, err := service.RemoveModelAssets(context.Background(), modelsRemoveRequest(ref))
	if !errors.Is(err, models.ErrAssetUnavailable) || !errors.Is(err, failure) {
		t.Fatalf("filesystem error = %v, want unavailable and injected failure", err)
	}
	observed := logs.All()
	if len(observed) != 2 {
		t.Fatalf("log count = %d, want start and terminal", len(observed))
	}
	terminalFields := observed[1].ContextMap()
	if terminalFields["error_classification"] != "filesystem" ||
		terminalFields["outcome"] != "ERROR" ||
		terminalFields["cancelled"] != false ||
		terminalFields["removal_state"] != string(modelseffects.AssetRemoveTreeRemaining) {
		t.Fatalf("filesystem terminal fields = %#v", terminalFields)
	}
	for _, log := range observed {
		if strings.Contains(log.Message, cacheDirectory) {
			t.Fatalf("filesystem path leaked into log message: %q", log.Message)
		}
		if errorType, ok := log.ContextMap()["error_type"].(string); ok &&
			strings.Contains(errorType, cacheDirectory) {
			t.Fatalf("filesystem path leaked into log fields: %#v", log.ContextMap())
		}
	}
}

// testRemoveTree is only a policy-service test double. The handle security
// boundary is exercised by pkg/platform/filesystem tests, not by this fake.
func testRemoveTree(ctx context.Context, parent, target string) (modelseffects.AssetRemoveTreeResult, error) {
	if err := ctx.Err(); err != nil {
		return modelseffects.AssetRemoveTreeResult{State: modelseffects.AssetRemoveTreeNotAttempted}, err
	}
	path := filepath.Join(parent, target)
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return modelseffects.AssetRemoveTreeResult{State: modelseffects.AssetRemoveTreeAbsent}, nil
	}
	if err != nil {
		return modelseffects.AssetRemoveTreeResult{State: modelseffects.AssetRemoveTreeNotAttempted}, err
	}
	if !info.IsDir() {
		return modelseffects.AssetRemoveTreeResult{State: modelseffects.AssetRemoveTreeNotAttempted}, errors.New("test removal target is not a directory")
	}
	if err := ctx.Err(); err != nil {
		return modelseffects.AssetRemoveTreeResult{State: modelseffects.AssetRemoveTreeRemaining}, err
	}
	if err := os.RemoveAll(path); err != nil {
		return modelseffects.AssetRemoveTreeResult{State: modelseffects.AssetRemoveTreeRemaining}, err
	}
	return modelseffects.AssetRemoveTreeResult{State: modelseffects.AssetRemoveTreeRemoved}, nil
}

func assertRemovedResult(t *testing.T, result models.RemoveModelAssetsResult, modelName string, outcome models.AssetRemovalOutcome) {
	t.Helper()
	if result.ModelName != modelName ||
		result.Readiness != models.AssetReadinessMissing ||
		result.Outcome != outcome {
		t.Fatalf("RemoveModelAssets = %#v, want model=%q readiness=%q outcome=%q", result, modelName, models.AssetReadinessMissing, outcome)
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
	if _, err := os.Lstat(path); err != nil {
		t.Fatalf("path %q is not present: %v", path, err)
	}
}

func assertPathAbsent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("path %q = %v, want absent", path, err)
	}
}

func modelsRemoveRequest(scope models.RuntimeScopeRef) models.RemoveModelAssetsRequest {
	return models.RemoveModelAssetsRequest{Scope: scope, Name: "OMNIVOICE_Q4_K_M"}
}
