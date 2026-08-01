package wire

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	platformrandom "github.com/portpowered/infinite-you/pkg/platform/random"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
	"go.uber.org/zap"
)

func TestProductionCompositionRejectsScopesFromAnotherModelsAuthority(t *testing.T) {
	t.Parallel()

	entropy := &sequentialEntropySource{}
	now := func() time.Time { return time.Unix(123, 456) }
	left := newProductionTestServiceWithDependencies(t, now, entropy)
	right := newProductionTestServiceWithDependencies(t, now, entropy)
	config := models.RuntimeScopeConfig{Runtime: models.RuntimeConfig{
		Workers: []models.RuntimeWorker{{
			Name:          "voice-local",
			Type:          models.RuntimeWorkerTypeModel,
			Model:         "OMNIVOICE_Q4_K_M",
			ModelLocality: models.RuntimeModelLocalityLocal,
			Operations:    []models.RuntimeOperation{{Name: "TTS"}},
		}},
	}}
	leftScope, err := left.OpenRuntimeScope(
		context.Background(),
		models.OpenRuntimeScopeRequest{Config: config},
	)
	if err != nil {
		t.Fatalf("open left runtime scope: %v", err)
	}
	rightScope, err := right.OpenRuntimeScope(
		context.Background(),
		models.OpenRuntimeScopeRequest{Config: config},
	)
	if err != nil {
		t.Fatalf("open right runtime scope: %v", err)
	}
	if leftScope.Scope.String() == rightScope.Scope.String() {
		t.Fatalf("separate Models authorities issued the same first scope %q", leftScope.Scope.String())
	}

	assertForeignCatalogScope(t, "left rejects right", left, rightScope.Scope)
	assertForeignCatalogScope(t, "right rejects left", right, leftScope.Scope)
}

func TestProductionCompositionReportsCurrentScopedReadinessWithCompatibilityParity(t *testing.T) {
	t.Parallel()

	service := newProductionTestService(t)
	cacheDirectory := t.TempDir()
	runtimeConfig := models.RuntimeConfig{
		Workers: []models.RuntimeWorker{{
			Name:          "voice-local",
			Type:          models.RuntimeWorkerTypeModel,
			Model:         "OMNIVOICE_Q4_K_M",
			ModelLocality: models.RuntimeModelLocalityLocal,
			Operations:    []models.RuntimeOperation{{Name: "TTS"}},
			Resources: []models.RuntimeResource{{
				Name: "omnivoice-cache", Capacity: 1,
			}},
		}},
		Resources: []models.RuntimeResource{{
			Name:       "omnivoice-cache",
			Type:       models.RuntimeResourceTypeModel,
			Capacity:   1,
			Model:      "OMNIVOICE_Q4_K_M",
			Backend:    "GGUF",
			LoadPolicy: "ON_DEMAND",
		}},
	}
	opened, err := service.OpenRuntimeScope(context.Background(), models.OpenRuntimeScopeRequest{
		Config: models.RuntimeScopeConfig{
			CacheDirectory: cacheDirectory,
			Runtime:        runtimeConfig,
		},
	})
	if err != nil {
		t.Fatalf("OpenRuntimeScope: %v", err)
	}
	request := models.GetModelReadinessRequest{
		Scope: opened.Scope, Name: "OMNIVOICE_Q4_K_M", Operation: "TTS",
	}
	missing, err := service.GetModelReadiness(context.Background(), request)
	if err != nil {
		t.Fatalf("GetModelReadiness before cache transition: %v", err)
	}
	if missing.Readiness.ReadinessState != models.ReadinessStateMissing {
		t.Fatalf("initial scoped readiness = %s, want MISSING", missing.Readiness.ReadinessState)
	}

	revisionDirectory := filepath.Join(cacheDirectory, "OMNIVOICE_Q4_K_M", "rev-live")
	if err := os.MkdirAll(revisionDirectory, 0o755); err != nil {
		t.Fatalf("create model revision directory: %v", err)
	}
	for _, name := range []string{
		"omnivoice-base-Q4_K_M.gguf",
		"omnivoice-tokenizer-Q4_K_M.gguf",
	} {
		if err := os.WriteFile(filepath.Join(revisionDirectory, name), []byte("fixture"), 0o644); err != nil {
			t.Fatalf("write model cache file %s: %v", name, err)
		}
	}

	current, err := service.GetModelReadiness(context.Background(), request)
	if err != nil {
		t.Fatalf("GetModelReadiness after cache transition: %v", err)
	}
	if current.Readiness.ReadinessState != models.ReadinessStateReady ||
		current.Readiness.LifecycleState != models.LifecycleStateInstalled {
		t.Fatalf("current scoped readiness = %#v, want READY/INSTALLED", current.Readiness)
	}
	if current.Readiness.Identity != "OMNIVOICE_Q4_K_M" {
		t.Fatalf("current scoped readiness identity = %q, want model identity", current.Readiness.Identity)
	}
}

func TestProductionCompositionInspectsScopedAssetsThroughModelsRoot(t *testing.T) {
	t.Parallel()

	service := newProductionTestService(t)
	cacheDirectory := t.TempDir()
	config := models.RuntimeConfig{
		Resources: []models.RuntimeResource{{
			Name:     "omnivoice-cache",
			Type:     models.RuntimeResourceTypeModel,
			Model:    "OMNIVOICE_Q4_K_M",
			Provider: "MODELSCOPE",
		}},
	}
	opened, err := service.OpenRuntimeScope(context.Background(), models.OpenRuntimeScopeRequest{
		Config: models.RuntimeScopeConfig{CacheDirectory: cacheDirectory, Runtime: config},
	})
	if err != nil {
		t.Fatalf("OpenRuntimeScope: %v", err)
	}

	root := filepath.Join(cacheDirectory, "OMNIVOICE_Q4_K_M")
	revisionDirectory := filepath.Join(root, "rev-root")
	if err := os.MkdirAll(revisionDirectory, 0o755); err != nil {
		t.Fatalf("create revision directory: %v", err)
	}
	files := []string{"omnivoice-base-Q4_K_M.gguf", "omnivoice-tokenizer-Q4_K_M.gguf"}
	assetBody := []byte("asset")
	for _, name := range files {
		if err := os.WriteFile(filepath.Join(revisionDirectory, name), assetBody, 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	assetChecksum := fmt.Sprintf("%x", sha256.Sum256(assetBody))
	metadata, err := json.Marshal(map[string]any{
		"modelName": "OMNIVOICE_Q4_K_M",
		"revision":  "rev-root",
		"files": []map[string]any{
			{"path": files[0], "bytes": len(assetBody), "sha256": assetChecksum},
			{"path": files[1], "bytes": len(assetBody), "sha256": assetChecksum},
		},
	})
	if err != nil {
		t.Fatalf("marshal metadata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".managed-cache.json"), metadata, 0o644); err != nil {
		t.Fatalf("write metadata: %v", err)
	}

	result, err := service.InspectModelAssets(context.Background(), models.InspectModelAssetsRequest{
		Scope:           opened.Scope,
		Name:            "OMNIVOICE_Q4_K_M",
		VerifyIntegrity: true,
	})
	if err != nil {
		t.Fatalf("InspectModelAssets: %v", err)
	}
	if result.Asset.Readiness != models.AssetReadinessAvailable ||
		result.Asset.Integrity != models.AssetIntegrityVerified ||
		result.Asset.Source.Provider != "MANAGED_MIRROR" ||
		result.Asset.Revision != "rev-root" ||
		len(result.Asset.Artifacts) != 2 {
		t.Fatalf("InspectModelAssets = %#v", result)
	}
}

func TestProductionCompositionInspectsScopedHostThroughModelsRoot(t *testing.T) {
	t.Parallel()

	service := newProductionTestService(t)
	cacheDirectory := t.TempDir()
	config := models.RuntimeConfig{
		Resources: []models.RuntimeResource{{
			Name:     "omnivoice-cache",
			Type:     models.RuntimeResourceTypeModel,
			Model:    "OMNIVOICE_Q4_K_M",
			Provider: "MODELSCOPE",
		}},
	}
	opened, err := service.OpenRuntimeScope(context.Background(), models.OpenRuntimeScopeRequest{
		Config: models.RuntimeScopeConfig{CacheDirectory: cacheDirectory, Runtime: config},
	})
	if err != nil {
		t.Fatalf("OpenRuntimeScope: %v", err)
	}

	missing, err := service.InspectModelHost(context.Background(), models.InspectModelHostRequest{
		Scope: opened.Scope,
		Name:  "OMNIVOICE_Q4_K_M",
	})
	if err != nil {
		t.Fatalf("InspectModelHost before cache transition: %v", err)
	}
	if missing.Host.ReadinessState != models.ReadinessStateMissing {
		t.Fatalf("initial host readiness = %s, want MISSING", missing.Host.ReadinessState)
	}

	root := filepath.Join(cacheDirectory, "OMNIVOICE_Q4_K_M")
	revisionDirectory := filepath.Join(root, "rev-root")
	if err := os.MkdirAll(revisionDirectory, 0o755); err != nil {
		t.Fatalf("create revision directory: %v", err)
	}
	files := []string{"omnivoice-base-Q4_K_M.gguf", "omnivoice-tokenizer-Q4_K_M.gguf"}
	assetBody := []byte("asset")
	for _, name := range files {
		if err := os.WriteFile(filepath.Join(revisionDirectory, name), assetBody, 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	assetChecksum := fmt.Sprintf("%x", sha256.Sum256(assetBody))
	metadata, err := json.Marshal(map[string]any{
		"modelName": "OMNIVOICE_Q4_K_M",
		"revision":  "rev-root",
		"files": []map[string]any{
			{"path": files[0], "bytes": len(assetBody), "sha256": assetChecksum},
			{"path": files[1], "bytes": len(assetBody), "sha256": assetChecksum},
		},
	})
	if err != nil {
		t.Fatalf("marshal metadata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".managed-cache.json"), metadata, 0o644); err != nil {
		t.Fatalf("write metadata: %v", err)
	}

	ready, err := service.InspectModelHost(context.Background(), models.InspectModelHostRequest{
		Scope: opened.Scope,
		Name:  "OMNIVOICE_Q4_K_M",
	})
	if err != nil {
		t.Fatalf("InspectModelHost after cache transition: %v", err)
	}
	if ready.Host.ReadinessState != models.ReadinessStateReady ||
		ready.Host.LifecycleState != models.LifecycleStateInstalled {
		t.Fatalf("ready host snapshot = %#v, want READY/INSTALLED from assets facts", ready.Host)
	}
}

func TestProductionCompositionPreparesAssetsThroughModelsRoot(t *testing.T) {
	t.Parallel()

	baseBody := []byte("root base")
	tokenizerBody := []byte("root tokenizer")
	baseChecksum := fmt.Sprintf("%x", sha256.Sum256(baseBody))
	tokenizerChecksum := fmt.Sprintf("%x", sha256.Sum256(tokenizerBody))
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/models/Serveurperso/OmniVoice-GGUF":
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"sha": "root-prepared",
				"siblings": []map[string]any{
					{
						"rfilename": "omnivoice-base-Q4_K_M.gguf",
						"lfs":       map[string]any{"oid": baseChecksum, "size": len(baseBody)},
					},
					{
						"rfilename": "omnivoice-tokenizer-Q4_K_M.gguf",
						"lfs":       map[string]any{"oid": tokenizerChecksum, "size": len(tokenizerBody)},
					},
				},
			})
		case "/Serveurperso/OmniVoice-GGUF/resolve/root-prepared/omnivoice-base-Q4_K_M.gguf":
			_, _ = writer.Write(baseBody)
		case "/Serveurperso/OmniVoice-GGUF/resolve/root-prepared/omnivoice-tokenizer-Q4_K_M.gguf":
			_, _ = writer.Write(tokenizerBody)
		default:
			http.NotFound(writer, request)
		}
	}))
	t.Cleanup(server.Close)

	service := newProductionTestServiceWithAssetSource(
		t, server.Client(), models.RuntimeAssetEndpoints{
			BaseURL: server.URL, APIBaseURL: server.URL,
		},
	)
	opened, err := service.OpenRuntimeScope(context.Background(), models.OpenRuntimeScopeRequest{
		Config: models.RuntimeScopeConfig{
			CacheDirectory: t.TempDir(),
			Runtime: models.RuntimeConfig{Resources: []models.RuntimeResource{{
				Type: models.RuntimeResourceTypeModel, Model: "OMNIVOICE_Q4_K_M",
			}}},
		},
	})
	if err != nil {
		t.Fatalf("OpenRuntimeScope: %v", err)
	}
	result, err := service.PrepareModelAssets(context.Background(), models.PrepareModelAssetsRequest{
		Scope: opened.Scope, Name: "OMNIVOICE_Q4_K_M",
	})
	if err != nil {
		t.Fatalf("PrepareModelAssets: %v", err)
	}
	if result.Outcome != models.AssetPreparationPrepared ||
		result.Asset.Readiness != models.AssetReadinessAvailable ||
		result.Asset.Integrity != models.AssetIntegrityVerified ||
		result.Asset.Revision != "root-prepared" {
		t.Fatalf("PrepareModelAssets result = %#v", result)
	}
}

func TestProductionRuntimeCompatibilityPullUsesScopedAssetsService(t *testing.T) {
	t.Parallel()

	baseBody := []byte("runtime-root-base")
	tokenizerBody := []byte("runtime-root-tokenizer")
	baseChecksum := fmt.Sprintf("%x", sha256.Sum256(baseBody))
	tokenizerChecksum := fmt.Sprintf("%x", sha256.Sum256(tokenizerBody))
	var requestCount atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestCount.Add(1)
		switch request.URL.Path {
		case "/models/Serveurperso/OmniVoice-GGUF":
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"sha": "runtime-root",
				"siblings": []map[string]any{
					{
						"rfilename": "omnivoice-base-Q4_K_M.gguf",
						"lfs":       map[string]any{"oid": baseChecksum, "size": len(baseBody)},
					},
					{
						"rfilename": "omnivoice-tokenizer-Q4_K_M.gguf",
						"lfs":       map[string]any{"oid": tokenizerChecksum, "size": len(tokenizerBody)},
					},
				},
			})
		case "/Serveurperso/OmniVoice-GGUF/resolve/runtime-root/omnivoice-base-Q4_K_M.gguf":
			_, _ = writer.Write(baseBody)
		case "/Serveurperso/OmniVoice-GGUF/resolve/runtime-root/omnivoice-tokenizer-Q4_K_M.gguf":
			_, _ = writer.Write(tokenizerBody)
		default:
			http.NotFound(writer, request)
		}
	}))
	t.Cleanup(server.Close)

	service := newProductionTestServiceWithAssetSource(
		t,
		server.Client(),
		models.RuntimeAssetEndpoints{BaseURL: server.URL, APIBaseURL: server.URL},
	)
	runtimeConfig := models.RuntimeConfig{
		Workers: []models.RuntimeWorker{{
			Name:          "voice",
			Type:          models.RuntimeWorkerTypeModel,
			Model:         "OMNIVOICE_Q4_K_M",
			ModelLocality: models.RuntimeModelLocalityLocal,
			Operations:    []models.RuntimeOperation{{Name: "TTS"}},
		}},
		Resources: []models.RuntimeResource{{
			Name: "voice-assets", Type: models.RuntimeResourceTypeModel,
			Model: "OMNIVOICE_Q4_K_M", Backend: "GGUF", LoadPolicy: "ON_DEMAND",
		}},
	}
	opened, err := service.OpenRuntimeScope(context.Background(), models.OpenRuntimeScopeRequest{
		Config: models.RuntimeScopeConfig{CacheDirectory: t.TempDir(), Runtime: runtimeConfig},
	})
	if err != nil {
		t.Fatalf("OpenRuntimeScope: %v", err)
	}

	pullRequest := models.PullModelRequest{Scope: opened.Scope, Name: "OMNIVOICE_Q4_K_M"}
	prepared, err := service.PullModelForScope(context.Background(), pullRequest)
	if err != nil {
		t.Fatalf("PullModel prepared: %v", err)
	}
	if prepared.ManagedPullOutcome != "INSTALLED_SUCCESSFULLY" ||
		prepared.Revision != "runtime-root" ||
		len(prepared.DownloadedFiles) != 2 {
		t.Fatalf("prepared PullModel = %#v", prepared)
	}
	firstRequestCount := requestCount.Load()

	cached, err := service.PullModelForScope(context.Background(), pullRequest)
	if err != nil {
		t.Fatalf("PullModel cache hit: %v", err)
	}
	if cached.ManagedPullOutcome != "ALREADY_READY" ||
		cached.Outcome != "ALREADY_PRESENT" ||
		requestCount.Load() != firstRequestCount {
		t.Fatalf(
			"cached PullModel = %#v requests %d, want offline cache hit after %d",
			cached,
			requestCount.Load(),
			firstRequestCount,
		)
	}
}

func newProductionTestService(t *testing.T) models.Service {
	t.Helper()
	return newProductionTestServiceWithDependencies(t, time.Now, platformrandom.CryptoSource{})
}

func newProductionTestServiceWithDependencies(
	t *testing.T,
	now func() time.Time,
	issuerEntropy platformrandom.Source,
) models.Service {
	return newProductionTestServiceWithAssetEdges(
		t, http.DefaultClient, models.RuntimeAssetEndpoints{}, now, issuerEntropy,
	)
}

func newProductionTestServiceWithAssetSource(
	t *testing.T,
	client modelseffects.AssetHTTPDoer,
	endpoints models.RuntimeAssetEndpoints,
) models.Service {
	return newProductionTestServiceWithAssetEdges(
		t, client, endpoints, time.Now, platformrandom.CryptoSource{},
	)
}

func newProductionTestServiceWithAssetEdges(
	t *testing.T,
	client modelseffects.AssetHTTPDoer,
	endpoints models.RuntimeAssetEndpoints,
	now func() time.Time,
	issuerEntropy platformrandom.Source,
) models.Service {
	t.Helper()
	service, err := NewService(
		models.AssetHostPlatform{OperatingSystem: runtime.GOOS, Architecture: runtime.GOARCH},
		client,
		endpoints,
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
		inertProcessLauncher{},
		http.DefaultClient,
		inertHostClock{},
		inertCommandRunner{},
		http.DefaultClient,
		os.Stat,
		os.TempDir,
		func(dir, pattern string) (modelseffects.RuntimeTempFile, error) {
			return os.CreateTemp(dir, pattern)
		},
		zap.NewNop(),
		now,
		issuerEntropy,
		nil,
		nil,
		nil,
		modelseffects.LocalRuntimeHooks{},
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return service
}

type sequentialEntropySource struct {
	next int64
}

func (source *sequentialEntropySource) Int63n(upperBound int64) (int64, error) {
	source.next++
	return source.next % upperBound, nil
}

func assertForeignCatalogScope(
	t *testing.T,
	name string,
	service models.Service,
	foreignScope models.RuntimeScopeRef,
) {
	t.Helper()

	list, err := service.ListCatalog(
		context.Background(),
		models.ListModelsRequest{Scope: foreignScope},
	)
	if !errors.Is(err, models.ErrRuntimeScopeForeign) {
		t.Fatalf("%s ListCatalog error = %v, want ErrRuntimeScopeForeign", name, err)
	}
	if len(list.Models) != 0 {
		t.Fatalf("%s ListCatalog returned foreign models: %#v", name, list.Models)
	}

	get, err := service.GetCatalogModel(context.Background(), models.GetModelRequest{
		Scope: foreignScope, Name: "OMNIVOICE_Q4_K_M", Operation: "TTS",
	})
	if !errors.Is(err, models.ErrRuntimeScopeForeign) {
		t.Fatalf("%s GetCatalogModel error = %v, want ErrRuntimeScopeForeign", name, err)
	}
	if get.Model.Name != "" {
		t.Fatalf("%s GetCatalogModel returned foreign model: %#v", name, get.Model)
	}

	readiness, err := service.GetModelReadiness(context.Background(), models.GetModelReadinessRequest{
		Scope: foreignScope, Name: "OMNIVOICE_Q4_K_M", Operation: "TTS",
	})
	if !errors.Is(err, models.ErrRuntimeScopeForeign) {
		t.Fatalf("%s GetModelReadiness error = %v, want ErrRuntimeScopeForeign", name, err)
	}
	if readiness.ModelName != "" || readiness.Readiness.Identity != "" {
		t.Fatalf("%s GetModelReadiness returned foreign readiness: %#v", name, readiness)
	}
}

