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
	"strings"
	"sync/atomic"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	platformrandom "github.com/portpowered/infinite-you/pkg/platform/random"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	"go.uber.org/zap"
)

func TestNewServiceRejectsMissingConstructionPorts(t *testing.T) {
	t.Parallel()

	edges := validConstructionEdges()
	tests := []struct {
		name   string
		mutate func(*constructionEdges)
		want   string
	}{
		{
			name: "issuer entropy",
			mutate: func(edges *constructionEdges) {
				edges.issuerEntropy = nil
			},
			want: "issuer entropy is required",
		},
		{
			name: "asset host platform",
			mutate: func(edges *constructionEdges) {
				edges.assetPlatform = models.AssetHostPlatform{}
			},
			want: "asset host platform is required",
		},
		{
			name: "asset HTTP client",
			mutate: func(edges *constructionEdges) {
				edges.assetHTTP = nil
			},
			want: "asset HTTP client is required",
		},
		{
			name: "asset make-directories effect",
			mutate: func(edges *constructionEdges) {
				edges.assetMkdirAll = nil
			},
			want: "asset make-directories effect is required",
		},
		{
			name: "asset inspect-path effect",
			mutate: func(edges *constructionEdges) {
				edges.assetStat = nil
			},
			want: "asset inspect-path effect is required",
		},
		{
			name: "asset resolve-home effect",
			mutate: func(edges *constructionEdges) {
				edges.assetHome = nil
			},
			want: "asset resolve-home effect is required",
		},
		{
			name: "asset write-file effect",
			mutate: func(edges *constructionEdges) {
				edges.assetWriteFile = nil
			},
			want: "asset write-file effect is required",
		},
		{
			name: "asset rename-path effect",
			mutate: func(edges *constructionEdges) {
				edges.assetRename = nil
			},
			want: "asset rename-path effect is required",
		},
		{
			name: "asset remove-path effect",
			mutate: func(edges *constructionEdges) {
				edges.assetRemove = nil
			},
			want: "asset remove-path effect is required",
		},
		{
			name: "asset read-file effect",
			mutate: func(edges *constructionEdges) {
				edges.assetReadFile = nil
			},
			want: "asset read-file effect is required",
		},
		{
			name: "asset read-directory effect",
			mutate: func(edges *constructionEdges) {
				edges.assetReadDir = nil
			},
			want: "asset read-directory effect is required",
		},
		{
			name: "asset create-file effect",
			mutate: func(edges *constructionEdges) {
				edges.assetCreate = nil
			},
			want: "asset create-file effect is required",
		},
		{
			name: "asset open-file effect",
			mutate: func(edges *constructionEdges) {
				edges.assetOpen = nil
			},
			want: "asset open-file effect is required",
		},
		{
			name: "process launcher",
			mutate: func(edges *constructionEdges) {
				edges.processLauncher = nil
			},
			want: "model host process launcher is required",
		},
		{
			name: "host HTTP client",
			mutate: func(edges *constructionEdges) {
				edges.hostHTTP = nil
			},
			want: "model host HTTP client is required",
		},
		{
			name: "host clock",
			mutate: func(edges *constructionEdges) {
				edges.hostClock = nil
			},
			want: "model host clock is required",
		},
		{
			name: "runtime command runner",
			mutate: func(edges *constructionEdges) {
				edges.runtimeRunner = nil
			},
			want: "model runtime command runner is required",
		},
		{
			name: "runtime HTTP client",
			mutate: func(edges *constructionEdges) {
				edges.runtimeHTTP = nil
			},
			want: "model runtime HTTP client is required",
		},
		{
			name: "runtime file inspector",
			mutate: func(edges *constructionEdges) {
				edges.runtimeInspect = nil
			},
			want: "model runtime file inspector is required",
		},
		{
			name: "runtime temporary directory resolver",
			mutate: func(edges *constructionEdges) {
				edges.runtimeTempDir = nil
			},
			want: "model runtime temporary directory resolver is required",
		},
		{
			name: "runtime temporary file creator",
			mutate: func(edges *constructionEdges) {
				edges.runtimeTempFile = nil
			},
			want: "model runtime temporary file creator is required",
		},
		{
			name: "process clock",
			mutate: func(edges *constructionEdges) {
				edges.now = nil
			},
			want: "process clock is required",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			current := edges
			test.mutate(&current)
			service, err := current.newService()
			if service != nil {
				t.Fatal("NewService() returned non-nil service, want nil")
			}
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("NewService() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestNewServiceReturnsPublishedRootInterface(t *testing.T) {
	t.Parallel()

	var service models.Service = mustNewWireService(t)
	if service == nil {
		t.Fatal("NewService() returned nil service")
	}
}

func TestNewInvocationArtifactExporterConstructsPublishedExporter(t *testing.T) {
	t.Parallel()

	exporter, err := NewInvocationArtifactExporter(invocationArtifactFileSystemStub{})
	if err != nil {
		t.Fatalf("NewInvocationArtifactExporter() error = %v", err)
	}
	if exporter == nil {
		t.Fatal("NewInvocationArtifactExporter() returned nil exporter")
	}
}

func TestNewServiceHonorsRuntimeAssetEndpointOverrides(t *testing.T) {
	t.Parallel()

	edges := validConstructionEdges()
	edges.assetEndpoints = models.RuntimeAssetEndpoints{
		BaseURL:    "https://example.test/models",
		APIBaseURL: "https://example.test/api",
	}
	service, err := edges.newService()
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if service == nil {
		t.Fatal("NewService() returned nil service")
	}
}

type invocationArtifactFileSystemStub struct{}

func (invocationArtifactFileSystemStub) Open(string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}

func (invocationArtifactFileSystemStub) Create(string) (io.WriteCloser, error) {
	return nopWriteCloser{}, nil
}

type nopWriteCloser struct{}

func (nopWriteCloser) Write(p []byte) (int, error) { return len(p), nil }
func (nopWriteCloser) Close() error                { return nil }

func TestNewServiceConstructsInertRoot(t *testing.T) {
	t.Parallel()

	assetHTTP := &recordingHTTPDoer{name: "asset HTTP"}
	hostHTTP := &recordingHTTPDoer{name: "host HTTP"}
	runtimeHTTP := &recordingHTTPDoer{name: "runtime HTTP"}
	processLauncher := &recordingProcessLauncher{}
	hostClock := &recordingHostClock{}
	runtimeRunner := &recordingCommandRunner{}
	assetMkdirAll := &recordingAssetMkdirAll{}
	assetStat := &recordingAssetStat{}
	assetHome := &recordingAssetHome{}
	assetWriteFile := &recordingAssetWriteFile{}
	assetRename := &recordingAssetRename{}
	assetRemove := &recordingAssetRemove{}
	assetReadFile := &recordingAssetReadFile{}
	assetReadDir := &recordingAssetReadDir{}
	assetCreate := &recordingAssetCreate{}
	assetOpen := &recordingAssetOpen{}
	runtimeInspect := &recordingRuntimeInspect{}
	runtimeTempDir := &recordingRuntimeTempDir{}
	runtimeTempFile := &recordingRuntimeTempFile{}
	processClock := &recordingProcessClock{}

	runtime.GC()
	time.Sleep(20 * time.Millisecond)
	baseline := runtime.NumGoroutine()

	service, err := NewService(
		models.AssetHostPlatform{OperatingSystem: runtime.GOOS, Architecture: runtime.GOARCH},
		assetHTTP,
		models.RuntimeAssetEndpoints{},
		assetMkdirAll.mkdirAll,
		assetStat.stat,
		assetHome.home,
		assetWriteFile.write,
		assetRename.rename,
		assetRemove.remove,
		assetReadFile.read,
		assetReadDir.readDir,
		assetCreate.create,
		assetOpen.open,
		processLauncher,
		hostHTTP,
		hostClock,
		runtimeRunner,
		runtimeHTTP,
		runtimeInspect.inspect,
		runtimeTempDir.tempDir,
		runtimeTempFile.create,
		zap.NewNop(),
		processClock.now,
		platformrandom.CryptoSource{},
		nil,
		nil,
		nil,
		models.LocalRuntimeHooks{},
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if service == nil {
		t.Fatal("NewService() returned nil service")
	}
	var peer models.Service = service
	if peer == nil {
		t.Fatal("constructed value is not assignable to models.Service")
	}

	assertInertConstruction(t, "asset HTTP", assetHTTP.calls)
	assertInertConstruction(t, "host HTTP", hostHTTP.calls)
	assertInertConstruction(t, "runtime HTTP", runtimeHTTP.calls)
	assertInertConstruction(t, "process launcher", processLauncher.starts)
	assertInertConstruction(t, "host clock", hostClock.nowCalls+hostClock.timerCalls)
	assertInertConstruction(t, "runtime command runner", runtimeRunner.calls)
	assertInertConstruction(t, "asset mkdir", assetMkdirAll.calls)
	assertInertConstruction(t, "asset stat", assetStat.calls)
	assertInertConstruction(t, "asset home", assetHome.calls)
	assertInertConstruction(t, "asset write", assetWriteFile.calls)
	assertInertConstruction(t, "asset rename", assetRename.calls)
	assertInertConstruction(t, "asset remove", assetRemove.calls)
	assertInertConstruction(t, "asset read file", assetReadFile.calls)
	assertInertConstruction(t, "asset read dir", assetReadDir.calls)
	assertInertConstruction(t, "asset create", assetCreate.calls)
	assertInertConstruction(t, "asset open", assetOpen.calls)
	assertInertConstruction(t, "runtime inspect", runtimeInspect.calls)
	assertInertConstruction(t, "runtime temp dir", runtimeTempDir.calls)
	assertInertConstruction(t, "runtime temp file", runtimeTempFile.calls)
	assertInertConstruction(t, "process clock", processClock.calls)

	runtime.GC()
	time.Sleep(20 * time.Millisecond)
	if leaked := runtime.NumGoroutine() - baseline; leaked > 4 {
		t.Fatalf(
			"goroutine leak after construction: baseline=%d current=%d delta=%d, want no lifecycle goroutines",
			baseline, runtime.NumGoroutine(), leaked,
		)
	}
}

func assertInertConstruction(t *testing.T, effect string, calls int) {
	t.Helper()
	if calls != 0 {
		t.Fatalf("construction invoked %s %d times, want inert construction", effect, calls)
	}
}

type constructionEdges struct {
	assetPlatform   models.AssetHostPlatform
	assetHTTP       models.AssetHTTPDoer
	assetEndpoints  models.RuntimeAssetEndpoints
	assetMkdirAll   models.AssetMakeDirectories
	assetStat       models.AssetInspectPath
	assetHome       models.AssetResolveHomeDirectory
	assetWriteFile  models.AssetWriteFile
	assetRename     models.AssetRenamePath
	assetRemove     models.AssetRemovePath
	assetReadFile   models.AssetReadFile
	assetReadDir    models.AssetReadDirectory
	assetCreate     models.AssetCreateFile
	assetOpen       models.AssetOpenFile
	processLauncher models.HostProcessLauncher
	hostHTTP        models.HostHTTPDoer
	hostClock       models.HostClock
	runtimeRunner   platformprocess.CommandRunner
	runtimeHTTP     models.RuntimeHTTPDoer
	runtimeInspect  models.RuntimeInspectFile
	runtimeTempDir  models.RuntimeTempDirectory
	runtimeTempFile models.RuntimeCreateTempFile
	now             func() time.Time
	issuerEntropy   platformrandom.Source
}

func validConstructionEdges() constructionEdges {
	return constructionEdges{
		assetPlatform: models.AssetHostPlatform{
			OperatingSystem: runtime.GOOS,
			Architecture:    runtime.GOARCH,
		},
		assetHTTP:       http.DefaultClient,
		assetMkdirAll:   os.MkdirAll,
		assetStat:       os.Stat,
		assetHome:       os.UserHomeDir,
		assetWriteFile:  os.WriteFile,
		assetRename:     os.Rename,
		assetRemove:     os.Remove,
		assetReadFile:   os.ReadFile,
		assetReadDir:    os.ReadDir,
		assetCreate:     func(path string) (io.WriteCloser, error) { return os.Create(path) },
		assetOpen:       func(path string) (io.ReadCloser, error) { return os.Open(path) },
		processLauncher: inertProcessLauncher{},
		hostHTTP:        http.DefaultClient,
		hostClock:       inertHostClock{},
		runtimeRunner:   inertCommandRunner{},
		runtimeHTTP:     http.DefaultClient,
		runtimeInspect:  os.Stat,
		runtimeTempDir:  os.TempDir,
		runtimeTempFile: func(dir, pattern string) (models.RuntimeTempFile, error) {
			return os.CreateTemp(dir, pattern)
		},
		now:           func() time.Time { return time.Unix(123, 456) },
		issuerEntropy: platformrandom.CryptoSource{},
	}
}

func (edges constructionEdges) newService() (models.Service, error) {
	return NewService(
		edges.assetPlatform,
		edges.assetHTTP,
		edges.assetEndpoints,
		edges.assetMkdirAll,
		edges.assetStat,
		edges.assetHome,
		edges.assetWriteFile,
		edges.assetRename,
		edges.assetRemove,
		edges.assetReadFile,
		edges.assetReadDir,
		edges.assetCreate,
		edges.assetOpen,
		edges.processLauncher,
		edges.hostHTTP,
		edges.hostClock,
		edges.runtimeRunner,
		edges.runtimeHTTP,
		edges.runtimeInspect,
		edges.runtimeTempDir,
		edges.runtimeTempFile,
		zap.NewNop(),
		edges.now,
		edges.issuerEntropy,
		nil,
		nil,
		nil,
		models.LocalRuntimeHooks{},
	)
}

func mustNewWireService(t *testing.T) models.Service {
	t.Helper()
	service, err := validConstructionEdges().newService()
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return service
}

func TestNewServiceServesPublishedModelsPeerBehavior(t *testing.T) {
	t.Parallel()

	var service models.Service = mustNewWireService(t)
	config := models.RuntimeScopeConfig{
		CacheDirectory: t.TempDir(),
		Runtime: models.RuntimeConfig{
			Workers: []models.RuntimeWorker{{
				Name:          "voice-local",
				Type:          models.RuntimeWorkerTypeModel,
				Model:         "OMNIVOICE_Q4_K_M",
				ModelLocality: models.RuntimeModelLocalityLocal,
				Operations:    []models.RuntimeOperation{{Name: "TTS"}},
			}},
		},
	}
	opened, err := service.OpenRuntimeScope(
		context.Background(),
		models.OpenRuntimeScopeRequest{Config: config},
	)
	if err != nil {
		t.Fatalf("OpenRuntimeScope: %v", err)
	}

	readiness, err := service.GetModelReadiness(context.Background(), models.GetModelReadinessRequest{
		Scope: opened.Scope, Name: "OMNIVOICE_Q4_K_M", Operation: "TTS",
	})
	if err != nil {
		t.Fatalf("GetModelReadiness: %v", err)
	}
	if readiness.Readiness.ReadinessState != models.ReadinessStateMissing {
		t.Fatalf("scoped readiness = %s, want MISSING", readiness.Readiness.ReadinessState)
	}

	other := mustNewWireService(t)
	foreign, err := other.OpenRuntimeScope(
		context.Background(),
		models.OpenRuntimeScopeRequest{Config: config},
	)
	if err != nil {
		t.Fatalf("open foreign scope: %v", err)
	}
	if opened.Scope.String() == foreign.Scope.String() {
		t.Fatalf("separate Wire-built authorities issued the same scope %q", opened.Scope.String())
	}

	_, err = service.ListCatalog(
		context.Background(),
		models.ListModelsRequest{Scope: foreign.Scope},
	)
	if !errors.Is(err, models.ErrRuntimeScopeForeign) {
		t.Fatalf("ListCatalog foreign scope error = %v, want ErrRuntimeScopeForeign", err)
	}
}

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
	bound, err := service.ForRuntime(models.RuntimeBinding{
		CacheDirectory: t.TempDir(),
		RuntimeConfig:  func() *models.RuntimeConfig { return &runtimeConfig },
	})
	if err != nil {
		t.Fatalf("ForRuntime: %v", err)
	}

	prepared, err := bound.PullModel(context.Background(), "OMNIVOICE_Q4_K_M")
	if err != nil {
		t.Fatalf("PullModel prepared: %v", err)
	}
	if prepared.ManagedPullOutcome != "INSTALLED_SUCCESSFULLY" ||
		prepared.Revision != "runtime-root" ||
		len(prepared.DownloadedFiles) != 2 {
		t.Fatalf("prepared PullModel = %#v", prepared)
	}
	firstRequestCount := requestCount.Load()

	cached, err := bound.PullModel(context.Background(), "OMNIVOICE_Q4_K_M")
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

type recordingHTTPDoer struct {
	name  string
	calls int
}

func (doer *recordingHTTPDoer) Do(*http.Request) (*http.Response, error) {
	doer.calls++
	panic(doer.name + " client invoked during inert construction")
}

type recordingProcessLauncher struct{ starts int }

func (launcher *recordingProcessLauncher) Start(
	context.Context,
	models.HostProcessStartSpec,
) (models.HostManagedProcess, error) {
	launcher.starts++
	panic("process launcher invoked during inert construction")
}

type recordingHostClock struct {
	nowCalls   int
	timerCalls int
}

func (clock *recordingHostClock) Now() time.Time {
	clock.nowCalls++
	panic("host clock invoked during inert construction")
}

func (clock *recordingHostClock) NewTimer(time.Duration) models.HostTimer {
	clock.timerCalls++
	panic("host timer created during inert construction")
}

type recordingCommandRunner struct{ calls int }

func (runner *recordingCommandRunner) Run(
	context.Context,
	platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	runner.calls++
	panic("runtime command runner invoked during inert construction")
}

type recordingAssetMkdirAll struct{ calls int }

func (effect *recordingAssetMkdirAll) mkdirAll(string, os.FileMode) error {
	effect.calls++
	panic("asset mkdir invoked during inert construction")
}

type recordingAssetStat struct{ calls int }

func (effect *recordingAssetStat) stat(string) (os.FileInfo, error) {
	effect.calls++
	panic("asset stat invoked during inert construction")
}

type recordingAssetHome struct{ calls int }

func (effect *recordingAssetHome) home() (string, error) {
	effect.calls++
	panic("asset home invoked during inert construction")
}

type recordingAssetWriteFile struct{ calls int }

func (effect *recordingAssetWriteFile) write(string, []byte, os.FileMode) error {
	effect.calls++
	panic("asset write invoked during inert construction")
}

type recordingAssetRename struct{ calls int }

func (effect *recordingAssetRename) rename(string, string) error {
	effect.calls++
	panic("asset rename invoked during inert construction")
}

type recordingAssetRemove struct{ calls int }

func (effect *recordingAssetRemove) remove(string) error {
	effect.calls++
	panic("asset remove invoked during inert construction")
}

type recordingAssetReadFile struct{ calls int }

func (effect *recordingAssetReadFile) read(string) ([]byte, error) {
	effect.calls++
	panic("asset read file invoked during inert construction")
}

type recordingAssetReadDir struct{ calls int }

func (effect *recordingAssetReadDir) readDir(string) ([]os.DirEntry, error) {
	effect.calls++
	panic("asset read dir invoked during inert construction")
}

type recordingAssetCreate struct{ calls int }

func (effect *recordingAssetCreate) create(string) (io.WriteCloser, error) {
	effect.calls++
	panic("asset create invoked during inert construction")
}

type recordingAssetOpen struct{ calls int }

func (effect *recordingAssetOpen) open(string) (io.ReadCloser, error) {
	effect.calls++
	panic("asset open invoked during inert construction")
}

type recordingRuntimeInspect struct{ calls int }

func (effect *recordingRuntimeInspect) inspect(string) (os.FileInfo, error) {
	effect.calls++
	panic("runtime inspect invoked during inert construction")
}

type recordingRuntimeTempDir struct{ calls int }

func (effect *recordingRuntimeTempDir) tempDir() string {
	effect.calls++
	panic("runtime temp dir invoked during inert construction")
}

type recordingRuntimeTempFile struct{ calls int }

func (effect *recordingRuntimeTempFile) create(string, string) (models.RuntimeTempFile, error) {
	effect.calls++
	panic("runtime temp file invoked during inert construction")
}

type recordingProcessClock struct{ calls int }

func (clock *recordingProcessClock) now() time.Time {
	clock.calls++
	panic("process clock invoked during inert construction")
}
