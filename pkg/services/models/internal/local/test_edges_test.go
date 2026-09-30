package local

import (
	"context"
	"io"
	"net/http"
	"os"
	"runtime"
	"testing"

	platformlocking "github.com/portpowered/infinite-you/pkg/platform/locking"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	assets "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets"
	assetswire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets/wire"
	runtimescopeswire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes/wire"
)

type recordingRuntimeCacheAssets struct {
	assets.Service
	requests []models.InspectModelAssetsRequest
}

func (a *recordingRuntimeCacheAssets) InspectRuntimeCache(
	_ context.Context, request models.InspectModelAssetsRequest,
) (assets.RuntimeCacheInspection, error) {
	a.requests = append(a.requests, request)
	return assets.RuntimeCacheInspection{
		Supported: true, Installed: true, CachePath: "cached/qwen3-tts-0.6b",
		InstalledFileCount: 2, ManifestPresent: true, ManifestValid: true,
	}, nil
}

func mustNewAssetPuller(t *testing.T, cacheDir string) AssetPuller {
	t.Helper()
	runtimeConfig := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	puller, err := newAssetPullerForTest(t, cacheDir, runtimeConfig)
	if err != nil {
		t.Fatalf("NewScopedAssetPuller: %v", err)
	}
	return puller
}

func newAssetPullerForTest(
	t *testing.T,
	cacheDir string,
	runtimeConfig *models.RuntimeConfig,
) (AssetPuller, error) {
	t.Helper()
	scopes, err := runtimescopeswire.NewService(func() string { return "local-assets-test" })
	if err != nil {
		return nil, err
	}
	privateScope, err := scopes.Open(models.RuntimeBinding{
		CacheDirectory: cacheDir,
		RuntimeConfig:  func() *models.RuntimeConfig { return runtimeConfig },
	})
	if err != nil {
		return nil, err
	}
	scope, err := (models.RuntimeScopeRef{}).Parse(string(privateScope))
	if err != nil {
		return nil, err
	}
	coordination, err := platformlocking.New(platformlocking.LocalFileSystem{})
	if err != nil {
		return nil, err
	}
	service, err := assetswire.NewService(
		scopes,
		models.AssetHostPlatform{OperatingSystem: runtime.GOOS, Architecture: runtime.GOARCH},
		http.DefaultClient,
		models.RuntimeAssetEndpoints{
			BaseURL: "https://huggingface.co", APIBaseURL: "https://huggingface.co/api",
		},
		os.MkdirAll,
		os.Stat,
		os.UserHomeDir,
		os.WriteFile,
		os.Rename,
		os.Remove,
		os.ReadFile,
		os.ReadDir,
		func(path string) (io.WriteCloser, error) { return os.Create(path) },
		func(path string) (io.ReadCloser, error) { return os.Open(path) },
		assets.ConstructionOptions{Coordination: coordination},
	)
	if err != nil {
		return nil, err
	}
	return NewScopedAssetPuller(service, scope)
}

func TestNewScopedAssetPullerRequiresServiceAndScope(t *testing.T) {
	t.Parallel()

	if _, err := NewScopedAssetPuller(nil, models.RuntimeScopeRef{}); err == nil {
		t.Fatal("NewScopedAssetPuller without service error = nil")
	}
	runtimeConfig := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	puller, err := newAssetPullerForTest(t, t.TempDir(), runtimeConfig)
	if err != nil {
		t.Fatalf("newAssetPullerForTest: %v", err)
	}
	if puller == nil {
		t.Fatal("NewScopedAssetPuller returned nil")
	}
}

func TestScopedAssetPullerSkipsCacheInspectionWithoutLocalModelResource(t *testing.T) {
	t.Parallel()

	runtimeConfig := mustLoadedCatalogConfig(t, catalogFactoryConfig(false))
	puller, err := newAssetPullerForTest(t, t.TempDir(), runtimeConfig)
	if err != nil {
		t.Fatalf("newAssetPullerForTest: %v", err)
	}
	inspection, err := puller.InspectRuntimeCache(
		t.Context(),
		runtimeConfig,
		"OMNIVOICE_Q4_K_M",
	)
	if err != nil {
		t.Fatalf("InspectRuntimeCache: %v", err)
	}
	if inspection.Supported || inspection.Installed || inspection.CachePath != "" {
		t.Fatalf("InspectRuntimeCache = %#v, want unsupported empty facts", inspection)
	}
}

func TestScopedAssetPullerInspectsOperatorModelWithoutFactoryWorker(t *testing.T) {
	t.Parallel()

	const modelName = "qwen3-tts-0.6b"
	scope, err := (models.RuntimeScopeRef{}).Parse("operator-model-test")
	if err != nil {
		t.Fatalf("parse scope: %v", err)
	}
	assetService := &recordingRuntimeCacheAssets{}
	puller, err := NewScopedAssetPuller(assetService, scope)
	if err != nil {
		t.Fatalf("NewScopedAssetPuller: %v", err)
	}
	runtimeConfig := &models.RuntimeConfig{Workers: []models.RuntimeWorker{{
		Name: "unrelated-cloud-worker", Model: "cloud-custom-model", ModelLocality: models.RuntimeModelLocalityCloud,
	}}}
	t.Run("operator model inspects scoped cache", func(t *testing.T) {
		assertOperatorModelScopedCacheInspection(t, puller, runtimeConfig, assetService, scope, modelName)
	})
	t.Run("operator model projects ready and installed", func(t *testing.T) {
		assertOperatorModelReadyAndInstalled(t, puller, runtimeConfig, modelName)
	})
	t.Run("unrelated cloud worker skips cache inspection", func(t *testing.T) {
		assertCloudWorkerSkipsCacheInspection(t, puller, runtimeConfig, assetService)
	})
}

func assertOperatorModelScopedCacheInspection(
	t *testing.T,
	puller AssetPuller,
	runtimeConfig *models.RuntimeConfig,
	assetService *recordingRuntimeCacheAssets,
	scope models.RuntimeScopeRef,
	modelName string,
) {
	t.Helper()
	inspection, err := puller.InspectRuntimeCache(t.Context(), runtimeConfig, modelName)
	if err != nil {
		t.Fatalf("InspectRuntimeCache operator model: %v", err)
	}
	if !inspection.Supported || !inspection.Installed || inspection.CachePath != "cached/qwen3-tts-0.6b" || inspection.InstalledFileCount != 2 {
		t.Fatalf("operator model inspection = %#v, want installed scoped cache facts", inspection)
	}
	if len(assetService.requests) != 1 || assetService.requests[0].Scope != scope || assetService.requests[0].Name != modelName {
		t.Fatalf("scoped inspection requests = %#v, want one name-only operator model request", assetService.requests)
	}
}

func assertOperatorModelReadyAndInstalled(
	t *testing.T, puller AssetPuller, runtimeConfig *models.RuntimeConfig, modelName string,
) {
	t.Helper()
	readiness, err := ManagedRuntimeReadinessForEffectiveDefinitionContext(
		t.Context(),
		models.Runtime{Identity: modelName, Locality: models.LocalityLocal},
		runtimeConfig,
		modelName,
		puller,
	)
	if err != nil {
		t.Fatalf("operator model readiness: %v", err)
	}
	if readiness.ReadinessState != models.ReadinessStateReady || readiness.LifecycleState != models.LifecycleStateInstalled {
		t.Fatalf("operator model readiness = (%s, %s), want READY/INSTALLED", readiness.ReadinessState, readiness.LifecycleState)
	}
}

func assertCloudWorkerSkipsCacheInspection(
	t *testing.T,
	puller AssetPuller,
	runtimeConfig *models.RuntimeConfig,
	assetService *recordingRuntimeCacheAssets,
) {
	t.Helper()
	skipped, err := puller.InspectRuntimeCache(t.Context(), runtimeConfig, "cloud-custom-model")
	if err != nil {
		t.Fatalf("InspectRuntimeCache cloud worker: %v", err)
	}
	if skipped.Supported || skipped.Installed || skipped.CachePath != "" {
		t.Fatalf("cloud worker inspection = %#v, want empty facts", skipped)
	}
	if len(assetService.requests) != 2 {
		t.Fatalf("scoped inspection requests = %#v, cloud worker should be skipped", assetService.requests)
	}
}
