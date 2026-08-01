package local

import (
	"io"
	"net/http"
	"os"
	"runtime"
	"testing"
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	assetswire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets/wire"
	runtimescopeswire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes/wire"
	"go.uber.org/zap"
)

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
		platformfilesystem.Local{}.RemoveTree,
		os.ReadFile,
		os.ReadDir,
		func(path string) (io.WriteCloser, error) { return os.Create(path) },
		func(path string) (io.ReadCloser, error) { return os.Open(path) },
		zap.NewNop(),
		time.Now,
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
