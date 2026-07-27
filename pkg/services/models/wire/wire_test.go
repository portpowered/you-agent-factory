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
	"reflect"
	"runtime"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	platformrandom "github.com/portpowered/infinite-you/pkg/platform/random"
	models "github.com/portpowered/infinite-you/pkg/services/models"
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

	bound, err := service.ForRuntime(models.RuntimeBinding{
		CacheDirectory: cacheDirectory,
		RuntimeConfig:  func() *models.RuntimeConfig { return &runtimeConfig },
	})
	if err != nil {
		t.Fatalf("ForRuntime: %v", err)
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
	compatibility, err := bound.InspectRuntime(context.Background(), "OMNIVOICE_Q4_K_M")
	if err != nil {
		t.Fatalf("InspectRuntime after cache transition: %v", err)
	}
	assertReadinessParity(t, current.Readiness, compatibility)
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
	for _, name := range files {
		if err := os.WriteFile(filepath.Join(revisionDirectory, name), []byte("asset"), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	metadata, err := json.Marshal(map[string]any{
		"modelName": "OMNIVOICE_Q4_K_M",
		"revision":  "rev-root",
		"files": []map[string]any{
			{"path": files[0], "sha256": "aaa"},
			{"path": files[1], "sha256": "bbb"},
		},
	})
	if err != nil {
		t.Fatalf("marshal metadata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".managed-cache.json"), metadata, 0o644); err != nil {
		t.Fatalf("write metadata: %v", err)
	}

	result, err := service.InspectModelAssets(context.Background(), models.InspectModelAssetsRequest{
		Scope: opened.Scope,
		Name:  "OMNIVOICE_Q4_K_M",
	})
	if err != nil {
		t.Fatalf("InspectModelAssets: %v", err)
	}
	if result.Asset.Readiness != models.AssetReadinessAvailable ||
		result.Asset.Source.Provider != "MANAGED_MIRROR" ||
		result.Asset.Revision != "rev-root" ||
		len(result.Asset.Artifacts) != 2 {
		t.Fatalf("InspectModelAssets = %#v", result)
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
	client models.AssetHTTPDoer,
	endpoints models.RuntimeAssetEndpoints,
) models.Service {
	return newProductionTestServiceWithAssetEdges(
		t, client, endpoints, time.Now, platformrandom.CryptoSource{},
	)
}

func newProductionTestServiceWithAssetEdges(
	t *testing.T,
	client models.AssetHTTPDoer,
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
		func(dir, pattern string) (models.RuntimeTempFile, error) {
			return os.CreateTemp(dir, pattern)
		},
		zap.NewNop(),
		now,
		issuerEntropy,
		nil,
		nil,
		nil,
		models.LocalRuntimeHooks{},
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

func assertReadinessParity(t *testing.T, scoped, compatibility models.Runtime) {
	t.Helper()
	if scoped.Identity != compatibility.Identity ||
		scoped.ReadinessState != compatibility.ReadinessState ||
		scoped.LifecycleState != compatibility.LifecycleState ||
		scoped.Locality != compatibility.Locality ||
		!reflect.DeepEqual(scoped.SupportedOperations, compatibility.SupportedOperations) {
		t.Fatalf(
			"production readiness parity = (scoped %#v, compatibility %#v)",
			scoped,
			compatibility,
		)
	}
	for _, key := range []string{
		"cachePath", "installedFileCount", "revision",
		"sourceId", "sourceKind", "resolverNotes",
	} {
		if scoped.Diagnostics[key] != compatibility.Diagnostics[key] {
			t.Fatalf(
				"production readiness diagnostic %q = (scoped %q, compatibility %q)",
				key,
				scoped.Diagnostics[key],
				compatibility.Diagnostics[key],
			)
		}
	}
}

type inertProcessLauncher struct{}

func (inertProcessLauncher) Start(
	context.Context,
	models.HostProcessStartSpec,
) (models.HostManagedProcess, error) {
	panic("process launcher called during readiness inspection")
}

type inertHostClock struct{}

func (inertHostClock) Now() time.Time {
	return time.Unix(0, 0)
}

func (inertHostClock) NewTimer(time.Duration) models.HostTimer {
	panic("host timer created during readiness inspection")
}

type inertCommandRunner struct{}

func (inertCommandRunner) Run(
	context.Context,
	platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	panic("runtime command called during readiness inspection")
}
