package wire

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	"go.uber.org/zap"
)

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

func newProductionTestService(t *testing.T) models.Service {
	t.Helper()
	service, err := NewService(
		models.AssetHostPlatform{OperatingSystem: runtime.GOOS, Architecture: runtime.GOARCH},
		http.DefaultClient,
		models.RuntimeAssetEndpoints{},
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
		time.Now,
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
