package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	assetswire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets/wire"
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

func newTestRuntimeHost(t *testing.T, launcher *recordingProcessLauncher) runtimehost.Service {
	t.Helper()
	scopes := newScopes(t, t.Name())
	return newTestRuntimeHostWithScopes(t, scopes, launcher)
}

func newTestRuntimeHostWithScopes(
	t *testing.T,
	scopes runtimescopes.Service,
	launcher *recordingProcessLauncher,
) runtimehost.Service {
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
	return internalservice.New(
		scopes,
		assets,
		launcher,
		http.DefaultClient,
		testHostClock{},
		nil,
		nil,
	)
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
