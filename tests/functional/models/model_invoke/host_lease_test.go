package model_invoke_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"runtime"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessionwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelswire "github.com/portpowered/infinite-you/pkg/services/models/wire"
	workersservice "github.com/portpowered/infinite-you/pkg/services/workers/service"
	"go.uber.org/zap"
)

func TestBoundModelsServiceHostLeaseAndValidatorPaths(t *testing.T) {
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, request)
	}))
	t.Cleanup(modelServer.Close)

	home := t.TempDir()
	writeReadyOmniVoiceCache(t, home)
	assetFiles := processModelAssetFileSystem{home: home}
	runtimeRunner, err := platformprocess.NewExecCommandRunner(exec.Command, platformclock.Real{}, nil)
	if err != nil {
		t.Fatalf("NewExecCommandRunner: %v", err)
	}

	rootService, err := modelswire.NewService(
		models.AssetHostPlatform{OperatingSystem: runtime.GOOS, Architecture: runtime.GOARCH},
		modelServer.Client(),
		models.RuntimeAssetEndpoints{},
		assetFiles.MkdirAll,
		assetFiles.Stat,
		assetFiles.UserHomeDir,
		assetFiles.WriteFile,
		assetFiles.Rename,
		assetFiles.Remove,
		assetFiles.ReadFile,
		assetFiles.ReadDir,
		assetFiles.Create,
		assetFiles.Open,
		&processModelLauncher{endpoint: modelServer.URL},
		modelServer.Client(),
		functionalModelsClock{},
		runtimeRunner,
		modelServer.Client(),
		assetFiles.Stat,
		os.TempDir,
		func(dir, pattern string) (models.RuntimeTempFile, error) { return os.CreateTemp(dir, pattern) },
		zap.NewNop(),
		time.Now,
		nil,
		factorysessionwire.ModelHostDiagnosticLogger(zap.NewNop()),
		factorysessionwire.ModelHostDiagnosticMetrics(nil),
		workersservice.LocalRuntimeHooks(),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	bound, err := rootService.ForRuntime(models.RuntimeBinding{
		CacheDirectory: home,
		RuntimeConfig: func() *models.RuntimeConfig {
			return &models.RuntimeConfig{
				FactoryDirectory: home,
				Workers: []models.RuntimeWorker{{
					Name:          "voice-local",
					Type:          factorydefinitions.WorkerTypeModel,
					Model:         "OMNIVOICE_Q4_K_M",
					ModelLocality: factorydefinitions.ModelLocalityLocal,
				}},
			}
		},
	})
	if err != nil {
		t.Fatalf("ForRuntime: %v", err)
	}

	ctx := context.Background()
	if _, err := bound.AcquireLease(ctx, models.AcquireLeaseRequest{}); !errors.Is(err, models.ErrNotFound) {
		t.Fatalf("AcquireLease empty model = %v, want ErrNotFound", err)
	}
	if err := bound.ReleaseLease(ctx, models.ReleaseLeaseRequest{}); !errors.Is(err, models.ErrHostLeaseNotFound) {
		t.Fatalf("ReleaseLease empty lease id = %v, want ErrHostLeaseNotFound", err)
	}
	if _, err := bound.GetModel(ctx, " "); !errors.Is(err, models.ErrNotFound) {
		t.Fatalf("GetModel empty name = %v, want ErrNotFound", err)
	}
	if _, err := bound.PullModel(ctx, ""); !errors.Is(err, models.ErrNotFound) {
		t.Fatalf("PullModel empty name = %v, want ErrNotFound", err)
	}
	if _, err := bound.InspectRuntime(ctx, ""); !errors.Is(err, models.ErrNotFound) {
		t.Fatalf("InspectRuntime empty name = %v, want ErrNotFound", err)
	}

	lease, err := bound.AcquireLease(ctx, models.AcquireLeaseRequest{
		ModelName: "OMNIVOICE_Q4_K_M",
		Holder:    "functional-test",
	})
	if err == nil {
		if err := bound.ReleaseLease(ctx, models.ReleaseLeaseRequest{LeaseID: lease.ID}); err != nil {
			t.Fatalf("ReleaseLease after successful acquire: %v", err)
		}
	} else if !errors.Is(err, models.ErrHostMissingAssets) && !errors.Is(err, models.ErrHostRuntimeNotReady) {
		t.Fatalf("AcquireLease missing-ready model = %v, want missing-assets or runtime-not-ready", err)
	}
}

type functionalModelsClock struct{}

func (functionalModelsClock) Now() time.Time { return time.Now() }

func (functionalModelsClock) NewTimer(duration time.Duration) models.HostTimer {
	return functionalModelsTimer{Timer: time.NewTimer(duration)}
}

type functionalModelsTimer struct{ *time.Timer }

func (timer functionalModelsTimer) C() <-chan time.Time { return timer.Timer.C }
