package wire_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"runtime"
	"testing"
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	platformrandom "github.com/portpowered/infinite-you/pkg/platform/random"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelswire "github.com/portpowered/infinite-you/pkg/services/models/wire"
	"go.uber.org/zap"
)

// TestModelsRootWireBehavioralBoundaryPreservesScopeFailures constructs the
// published Models root through models/wire and checks the detached readiness,
// foreign-scope, and closed-scope outcomes that peers rely on.
func TestModelsRootWireBehavioralBoundaryPreservesScopeFailures(t *testing.T) {
	t.Parallel()

	service := newModelsRootWireService(t)
	config := models.RuntimeScopeConfig{
		CacheDirectory: t.TempDir(),
		Runtime: models.RuntimeConfig{
			Workers: []models.RuntimeWorker{{
				Name:          "seal-model",
				Type:          models.RuntimeWorkerTypeModel,
				Model:         "SEAL_MODEL",
				ModelLocality: models.RuntimeModelLocalityLocal,
				Operations:    []models.RuntimeOperation{{Name: "CHAT"}},
			}},
		},
	}
	opened, err := service.OpenRuntimeScope(context.Background(), models.OpenRuntimeScopeRequest{Config: config})
	if err != nil {
		t.Fatalf("OpenRuntimeScope() error = %v", err)
	}

	readiness, err := service.GetModelReadiness(context.Background(), models.GetModelReadinessRequest{
		Scope: opened.Scope, Name: "SEAL_MODEL", Operation: "CHAT",
	})
	if err != nil {
		t.Fatalf("GetModelReadiness() error = %v", err)
	}
	if readiness.Readiness.ReadinessState != models.ReadinessStateMissing {
		t.Fatalf("readiness state = %q, want %q", readiness.Readiness.ReadinessState, models.ReadinessStateMissing)
	}

	foreignService := newModelsRootWireService(t)
	foreign, err := foreignService.OpenRuntimeScope(context.Background(), models.OpenRuntimeScopeRequest{Config: config})
	if err != nil {
		t.Fatalf("foreign OpenRuntimeScope() error = %v", err)
	}
	if _, err := service.ListCatalog(context.Background(), models.ListModelsRequest{Scope: foreign.Scope}); !errors.Is(err, models.ErrRuntimeScopeForeign) {
		t.Fatalf("foreign ListCatalog() error = %v, want ErrRuntimeScopeForeign", err)
	}

	if _, err := service.CloseRuntimeScope(context.Background(), models.CloseRuntimeScopeRequest{Scope: opened.Scope}); err != nil {
		t.Fatalf("CloseRuntimeScope() error = %v", err)
	}
	if _, err := service.GetModelReadiness(context.Background(), models.GetModelReadinessRequest{
		Scope: opened.Scope, Name: "SEAL_MODEL", Operation: "CHAT",
	}); !errors.Is(err, models.ErrRuntimeScopeClosed) {
		t.Fatalf("closed GetModelReadiness() error = %v, want ErrRuntimeScopeClosed", err)
	}
}

func newModelsRootWireService(t *testing.T) models.Service {
	t.Helper()

	service, err := modelswire.NewService(
		models.AssetHostPlatform{OperatingSystem: runtime.GOOS, Architecture: runtime.GOARCH},
		http.DefaultClient,
		models.RuntimeAssetEndpoints{},
		os.MkdirAll,
		os.Stat,
		os.UserHomeDir,
		os.WriteFile,
		os.Rename,
		os.Remove,
		modelsRootAssetRemoveTree,
		os.ReadFile,
		os.ReadDir,
		func(path string) (io.WriteCloser, error) { return os.Create(path) },
		func(path string) (io.ReadCloser, error) { return os.Open(path) },
		modelsRootInertProcessLauncher{},
		http.DefaultClient,
		modelsRootInertHostClock{},
		modelsRootInertCommandRunner{},
		http.DefaultClient,
		os.Stat,
		os.TempDir,
		func(dir, pattern string) (modelswire.RuntimeTempFile, error) { return os.CreateTemp(dir, pattern) },
		zap.NewNop(),
		time.Now,
		platformrandom.CryptoSource{},
		nil,
		nil,
		nil,
		modelswire.LocalRuntimeHooks{},
	)
	if err != nil {
		t.Fatalf("models/wire.NewService() error = %v", err)
	}
	if service == nil {
		t.Fatal("models/wire.NewService() returned nil service")
	}
	return service
}

func modelsRootAssetRemoveTree(
	ctx context.Context,
	parent string,
	target string,
) (modelswire.AssetRemoveTreeResult, error) {
	result, err := (platformfilesystem.Local{}).RemoveTree(ctx, parent, target)
	return modelswire.AssetRemoveTreeResult{
		State: modelswire.AssetRemoveTreeState(result.State),
	}, err
}

type modelsRootInertProcessLauncher struct{}

func (modelsRootInertProcessLauncher) Start(context.Context, modelswire.HostProcessStartSpec) (modelswire.HostManagedProcess, error) {
	panic("Models host process launched during root boundary readiness proof")
}

type modelsRootInertHostClock struct{}

func (modelsRootInertHostClock) Now() time.Time { return time.Unix(0, 0) }

func (modelsRootInertHostClock) NewTimer(time.Duration) modelswire.HostTimer {
	panic("Models host timer created during root boundary readiness proof")
}

type modelsRootInertCommandRunner struct{}

func (modelsRootInertCommandRunner) Run(context.Context, platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	panic("Models runtime command called during root boundary readiness proof")
}
