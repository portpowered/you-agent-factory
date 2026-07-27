package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	assetswire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets/wire"
	scopedassets "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets"
	runtimescopes "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes"
	runtimescopeswire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes/wire"
	internalservice "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host/internal/service"
	runtimehost "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host"
)

func TestConstructionAllocatesHostStateWithoutLaunchingProcess(t *testing.T) {
	t.Parallel()

	launcher := &recordingProcessLauncher{}
	service := newTestRuntimeHost(t, launcher)
	if service == nil {
		t.Fatal("New returned nil service")
	}
	if launcher.starts != 0 {
		t.Fatalf("process starts during construction = %d, want 0", launcher.starts)
	}
}

func TestInspectModelHostReportsMissingAssetsFromAcceptedAssetsFacts(t *testing.T) {
	t.Parallel()

	cacheDirectory := t.TempDir()
	scopes := newScopes(t, "missing-assets")
	ref := openScope(t, scopes, cacheDirectory, runtimeConfig())
	service := newTestRuntimeHostWithScopes(t, scopes, &recordingProcessLauncher{})

	result, err := service.InspectModelHost(context.Background(), models.InspectModelHostRequest{
		Scope: ref,
		Name:  "OMNIVOICE_Q4_K_M",
	})
	if err != nil {
		t.Fatalf("InspectModelHost: %v", err)
	}
	if result.Host.ReadinessState != models.ReadinessStateMissing ||
		result.Host.LifecycleState != models.LifecycleStateNotInstalled ||
		result.Host.ModelName != "OMNIVOICE_Q4_K_M" {
		t.Fatalf("host snapshot = %#v, want missing/not-installed", result.Host)
	}
}

func TestInspectModelHostReportsInstalledAssetsWithoutSupervisedProcess(t *testing.T) {
	t.Parallel()

	cacheDirectory := t.TempDir()
	writeCacheFixture(t, cacheDirectory, true)
	scopes := newScopes(t, "installed-assets")
	ref := openScope(t, scopes, cacheDirectory, runtimeConfig())
	launcher := &recordingProcessLauncher{}
	service := newTestRuntimeHostWithScopes(t, scopes, launcher)

	result, err := service.InspectModelHost(context.Background(), models.InspectModelHostRequest{
		Scope: ref,
		Name:  "OMNIVOICE_Q4_K_M",
	})
	if err != nil {
		t.Fatalf("InspectModelHost: %v", err)
	}
	if result.Host.ReadinessState != models.ReadinessStateReady ||
		result.Host.LifecycleState != models.LifecycleStateInstalled {
		t.Fatalf("host snapshot = %#v, want ready/installed from assets facts", result.Host)
	}
	if launcher.starts != 0 {
		t.Fatalf("process starts during inspect = %d, want 0", launcher.starts)
	}
}

func TestOpenRuntimeScopeDoesNotConstructAnotherRuntimeHost(t *testing.T) {
	t.Parallel()

	launcher := &recordingProcessLauncher{}
	scopes := newScopes(t, "scope-binding")
	service := newTestRuntimeHostWithScopes(t, scopes, launcher)
	_, err := scopes.Open(models.RuntimeBinding{
		CacheDirectory: t.TempDir(),
		RuntimeConfig: func() *models.RuntimeConfig {
			return &models.RuntimeConfig{FactoryDirectory: "factory"}
		},
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if service == nil {
		t.Fatal("runtime host service is nil after scope open")
	}
	if launcher.starts != 0 {
		t.Fatalf("process starts after scope open = %d, want 0", launcher.starts)
	}
}

func TestInspectModelHostRejectsForeignScope(t *testing.T) {
	t.Parallel()

	left := newScopes(t, "left")
	right := newScopes(t, "right")
	ref, err := right.Open(models.RuntimeBinding{
		CacheDirectory: t.TempDir(),
		RuntimeConfig: func() *models.RuntimeConfig {
			return &models.RuntimeConfig{FactoryDirectory: "factory"}
		},
	})
	if err != nil {
		t.Fatalf("Open foreign scope: %v", err)
	}
	service := newTestRuntimeHostWithScopes(t, left, &recordingProcessLauncher{})
	_, err = service.InspectModelHost(context.Background(), models.InspectModelHostRequest{
		Scope: mustParseScope(t, ref),
		Name:  "OMNIVOICE_Q4_K_M",
	})
	if !errors.Is(err, models.ErrRuntimeScopeForeign) {
		t.Fatalf("InspectModelHost error = %v, want ErrRuntimeScopeForeign", err)
	}
}

func TestEnsureModelHostHealthyStartupReturnsReadyWithEndpoint(t *testing.T) {
	t.Parallel()

	healthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(healthServer.Close)

	cacheDirectory := t.TempDir()
	writeCacheFixture(t, cacheDirectory, true)
	scopes := newScopes(t, "healthy-startup")
	ref := openScope(t, scopes, cacheDirectory, supervisedRuntimeConfig())
	launcher := &fakeProcessLauncher{
		newProcess: func(spec models.HostProcessStartSpec) *fakeManagedProcess {
			return newFakeManagedProcess(healthServer.URL, nil)
		},
	}
	service := newTestRuntimeHostWithScopesAndClock(t, scopes, launcher, realHostClock{})

	result, err := service.EnsureModelHost(context.Background(), models.EnsureModelHostRequest{
		Scope: ref,
		Name:  "OMNIVOICE_Q4_K_M",
	})
	if err != nil {
		t.Fatalf("EnsureModelHost: %v", err)
	}
	if result.Outcome != models.HostEnsureBecameReady ||
		result.Host.ReadinessState != models.ReadinessStateReady ||
		result.Host.LifecycleState != models.LifecycleStateLoaded {
		t.Fatalf("ensure result = %#v, want became-ready/ready/loaded", result)
	}
	if result.Host.Diagnostics["endpoint"] != healthServer.URL {
		t.Fatalf("endpoint = %q, want %q", result.Host.Diagnostics["endpoint"], healthServer.URL)
	}
	if launcher.startCount() != 1 {
		t.Fatalf("process starts = %d, want 1", launcher.startCount())
	}
}

func TestEnsureModelHostConcurrentReuseSharesOneSupervisedProcess(t *testing.T) {
	t.Parallel()

	healthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(healthServer.Close)

	cacheDirectory := t.TempDir()
	writeCacheFixture(t, cacheDirectory, true)
	scopes := newScopes(t, "concurrent-reuse")
	ref := openScope(t, scopes, cacheDirectory, supervisedRuntimeConfig())
	launcher := &fakeProcessLauncher{
		newProcess: func(spec models.HostProcessStartSpec) *fakeManagedProcess {
			return newFakeManagedProcess(healthServer.URL, nil)
		},
	}
	service := newTestRuntimeHostWithScopesAndClock(t, scopes, launcher, realHostClock{})

	const workers = 8
	var wg sync.WaitGroup
	errCh := make(chan error, workers)
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := service.EnsureModelHost(context.Background(), models.EnsureModelHostRequest{
				Scope: ref,
				Name:  "OMNIVOICE_Q4_K_M",
			})
			errCh <- err
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("EnsureModelHost: %v", err)
		}
	}
	if launcher.startCount() != 1 {
		t.Fatalf("process starts = %d, want 1 shared supervised process", launcher.startCount())
	}
}

func TestEnsureModelHostReturnsDetachedSnapshotWithoutPrivateHandles(t *testing.T) {
	t.Parallel()

	healthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(healthServer.Close)

	cacheDirectory := t.TempDir()
	writeCacheFixture(t, cacheDirectory, true)
	scopes := newScopes(t, "detached-snapshot")
	ref := openScope(t, scopes, cacheDirectory, supervisedRuntimeConfig())
	launcher := &fakeProcessLauncher{
		newProcess: func(spec models.HostProcessStartSpec) *fakeManagedProcess {
			return newFakeManagedProcess(healthServer.URL, nil)
		},
	}
	service := newTestRuntimeHostWithScopesAndClock(t, scopes, launcher, realHostClock{})

	result, err := service.EnsureModelHost(context.Background(), models.EnsureModelHostRequest{
		Scope: ref,
		Name:  "OMNIVOICE_Q4_K_M",
	})
	if err != nil {
		t.Fatalf("EnsureModelHost: %v", err)
	}
	result.Host.Diagnostics["mutated"] = "peer"
	inspected, err := service.InspectModelHost(context.Background(), models.InspectModelHostRequest{
		Scope: ref,
		Name:  "OMNIVOICE_Q4_K_M",
	})
	if err != nil {
		t.Fatalf("InspectModelHost: %v", err)
	}
	if inspected.Host.Diagnostics["mutated"] != "" {
		t.Fatal("peer mutation of ensure result changed retained host state")
	}
	if inspected.Host.Diagnostics["endpoint"] != healthServer.URL {
		t.Fatalf("endpoint = %q, want %q", inspected.Host.Diagnostics["endpoint"], healthServer.URL)
	}
}

func newTestRuntimeHost(t *testing.T, launcher *recordingProcessLauncher) runtimehost.Service {
	t.Helper()
	scopes := newScopes(t, t.Name())
	return newTestRuntimeHostWithScopes(t, scopes, launcher)
}

func newTestRuntimeHostWithScopes(
	t *testing.T,
	scopes runtimescopes.Service,
	launcher models.HostProcessLauncher,
) runtimehost.Service {
	return newTestRuntimeHostWithScopesAndClock(t, scopes, launcher, testHostClock{})
}

func newTestRuntimeHostWithScopesAndClock(
	t *testing.T,
	scopes runtimescopes.Service,
	launcher models.HostProcessLauncher,
	clock models.HostClock,
) runtimehost.Service {
	t.Helper()
	return internalservice.New(
		scopes,
		mustAssetsService(t, scopes),
		launcher,
		http.DefaultClient,
		clock,
		nil,
		nil,
	)
}

func mustAssetsService(t *testing.T, scopes runtimescopes.Service) scopedassets.Service {
	t.Helper()
	assets, err := assetswire.NewService(
		scopes,
		models.AssetHostPlatform{OperatingSystem: "linux", Architecture: "amd64"},
		http.DefaultClient,
		models.RuntimeAssetEndpoints{
			BaseURL: "https://assets.example.test", APIBaseURL: "https://api.example.test",
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
	)
	if err != nil {
		t.Fatalf("construct assets: %v", err)
	}
	return assets
}

type recordingProcessLauncher struct {
	starts int
}

func (launcher *recordingProcessLauncher) Start(
	context.Context,
	models.HostProcessStartSpec,
) (models.HostManagedProcess, error) {
	launcher.starts++
	panic("process launcher called during inert runtime host")
}

type testHostClock struct{}

func (testHostClock) Now() time.Time { return time.Unix(0, 0) }
func (testHostClock) NewTimer(time.Duration) models.HostTimer {
	panic("host timer created during inert runtime host")
}

func newScopes(t *testing.T, issuer string) runtimescopes.Service {
	t.Helper()
	scopes, err := runtimescopeswire.NewService(func() string { return issuer })
	if err != nil {
		t.Fatalf("construct runtime scopes: %v", err)
	}
	return scopes
}

func openScope(
	t *testing.T,
	scopes runtimescopes.Service,
	cacheDirectory string,
	config models.RuntimeConfig,
) models.RuntimeScopeRef {
	t.Helper()
	ref, err := scopes.Open(models.RuntimeBinding{
		CacheDirectory: cacheDirectory,
		RuntimeConfig:  func() *models.RuntimeConfig { return &config },
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return mustParseScope(t, ref)
}

func mustParseScope(t *testing.T, ref runtimescopes.Reference) models.RuntimeScopeRef {
	t.Helper()
	scope, err := (models.RuntimeScopeRef{}).Parse(string(ref))
	if err != nil {
		t.Fatalf("Parse scope: %v", err)
	}
	return scope
}

func runtimeConfig() models.RuntimeConfig {
	return models.RuntimeConfig{
		Resources: []models.RuntimeResource{{
			Name:     "omnivoice-cache",
			Type:     models.RuntimeResourceTypeModel,
			Model:    "OMNIVOICE_Q4_K_M",
			Provider: "MODELSCOPE",
		}},
	}
}

func supervisedRuntimeConfig() models.RuntimeConfig {
	cfg := runtimeConfig()
	cfg.Resources[0].Backend = "LLAMACPP"
	cfg.Workers = []models.RuntimeWorker{{
		Name:          "omnivoice-worker",
		Model:         "OMNIVOICE_Q4_K_M",
		ModelLocality: models.RuntimeModelLocalityLocal,
		Command:       "fake-llamacpp-server",
		Args: []string{
			"--health-endpoint",
			"http://127.0.0.1:1",
		},
	}}
	return cfg
}

const metadataFileName = ".managed-cache.json"

type cacheMetadata struct {
	ModelName string         `json:"modelName"`
	Revision  string         `json:"revision"`
	Files     []metadataFile `json:"files"`
}

type metadataFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

func writeCacheFixture(t *testing.T, cacheDirectory string, includeMetadata bool) {
	t.Helper()
	root := filepath.Join(cacheDirectory, "OMNIVOICE_Q4_K_M")
	revisionDirectory := filepath.Join(root, "rev-test")
	if err := os.MkdirAll(revisionDirectory, 0o755); err != nil {
		t.Fatalf("create cache fixture: %v", err)
	}
	files := []metadataFile{
		{Path: "omnivoice-base-Q4_K_M.gguf", SHA256: "aaa"},
		{Path: "omnivoice-tokenizer-Q4_K_M.gguf", SHA256: "bbb"},
	}
	for index, file := range files {
		content := []byte{byte(index + 1), byte(index + 2)}
		if err := os.WriteFile(filepath.Join(revisionDirectory, file.Path), content, 0o644); err != nil {
			t.Fatalf("write cache artifact: %v", err)
		}
	}
	if !includeMetadata {
		return
	}
	body, err := json.Marshal(cacheMetadata{
		ModelName: "OMNIVOICE_Q4_K_M",
		Revision:  "rev-test",
		Files:     files,
	})
	if err != nil {
		t.Fatalf("marshal metadata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, metadataFileName), body, 0o644); err != nil {
		t.Fatalf("write metadata: %v", err)
	}
}

type fakeProcessLauncher struct {
	mu         sync.Mutex
	starts     int
	newProcess func(spec models.HostProcessStartSpec) *fakeManagedProcess
}

func (f *fakeProcessLauncher) Start(
	_ context.Context,
	spec models.HostProcessStartSpec,
) (models.HostManagedProcess, error) {
	f.mu.Lock()
	f.starts++
	newProcess := f.newProcess
	f.mu.Unlock()
	if newProcess == nil {
		return nil, errors.New("fake process launcher is not configured")
	}
	return newProcess(spec), nil
}

func (f *fakeProcessLauncher) startCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.starts
}

type fakeManagedProcess struct {
	endpoint string
	exitCh   chan error
	stopCh   chan struct{}
	stopOnce sync.Once
	stopFn   func() error
}

func newFakeManagedProcess(endpoint string, exitCh chan error) *fakeManagedProcess {
	if exitCh == nil {
		exitCh = make(chan error, 1)
	}
	return &fakeManagedProcess{
		endpoint: endpoint,
		exitCh:   exitCh,
		stopCh:   make(chan struct{}),
	}
}

func (p *fakeManagedProcess) HealthEndpoint() string {
	return p.endpoint
}

func (p *fakeManagedProcess) Wait() error {
	return <-p.exitCh
}

func (p *fakeManagedProcess) Stop(context.Context) error {
	if p.stopFn != nil {
		return p.stopFn()
	}
	return p.defaultStop()
}

func (p *fakeManagedProcess) defaultStop() error {
	p.stopOnce.Do(func() {
		close(p.stopCh)
		p.exitCh <- errors.New("stopped")
	})
	return nil
}

type realHostClock struct{}

func (realHostClock) Now() time.Time {
	return time.Now()
}

func (realHostClock) NewTimer(duration time.Duration) models.HostTimer {
	return realHostTimer{timer: time.NewTimer(duration)}
}

type realHostTimer struct {
	timer *time.Timer
}

func (t realHostTimer) C() <-chan time.Time {
	return t.timer.C
}

func (t realHostTimer) Stop() bool {
	return t.timer.Stop()
}
