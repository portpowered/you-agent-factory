package runtimeopening

import (
	"context"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"go.uber.org/zap"
)

type recordingModelsService struct {
	openRequests    []models.OpenRuntimeScopeRequest
	forRuntimeCalls int
}

func (fake *recordingModelsService) OpenRuntimeScope(
	_ context.Context,
	request models.OpenRuntimeScopeRequest,
) (models.OpenRuntimeScopeResult, error) {
	fake.openRequests = append(fake.openRequests, request)
	scope, err := (models.RuntimeScopeRef{}).Parse("factory-session:test:1")
	if err != nil {
		return models.OpenRuntimeScopeResult{}, err
	}
	return models.OpenRuntimeScopeResult{Scope: scope}, nil
}

func (fake *recordingModelsService) CloseRuntimeScope(
	context.Context,
	models.CloseRuntimeScopeRequest,
) (models.CloseRuntimeScopeResult, error) {
	return models.CloseRuntimeScopeResult{}, models.ErrUnsupportedOperation
}

func (fake *recordingModelsService) ForRuntime(models.RuntimeBinding) (models.Service, error) {
	fake.forRuntimeCalls++
	return fake, nil
}

func (fake *recordingModelsService) ListCatalog(
	context.Context,
	models.ListModelsRequest,
) (models.ListModelsResult, error) {
	return models.ListModelsResult{}, models.ErrUnsupportedOperation
}

func (fake *recordingModelsService) GetCatalogModel(
	context.Context,
	models.GetModelRequest,
) (models.GetModelResult, error) {
	return models.GetModelResult{}, models.ErrUnsupportedOperation
}

func (fake *recordingModelsService) GetModelReadiness(
	context.Context,
	models.GetModelReadinessRequest,
) (models.GetModelReadinessResult, error) {
	return models.GetModelReadinessResult{}, models.ErrUnsupportedOperation
}

func (fake *recordingModelsService) PrepareModelAssets(
	context.Context,
	models.PrepareModelAssetsRequest,
) (models.PrepareModelAssetsResult, error) {
	return models.PrepareModelAssetsResult{}, models.ErrUnsupportedOperation
}

func (fake *recordingModelsService) InspectModelAssets(
	context.Context,
	models.InspectModelAssetsRequest,
) (models.InspectModelAssetsResult, error) {
	return models.InspectModelAssetsResult{}, models.ErrUnsupportedOperation
}

func (fake *recordingModelsService) RemoveModelAssets(
	context.Context,
	models.RemoveModelAssetsRequest,
) (models.RemoveModelAssetsResult, error) {
	return models.RemoveModelAssetsResult{}, models.ErrUnsupportedOperation
}

func (fake *recordingModelsService) EnsureModelHost(
	context.Context,
	models.EnsureModelHostRequest,
) (models.EnsureModelHostResult, error) {
	return models.EnsureModelHostResult{}, models.ErrUnsupportedOperation
}

func (fake *recordingModelsService) InspectModelHost(
	context.Context,
	models.InspectModelHostRequest,
) (models.InspectModelHostResult, error) {
	return models.InspectModelHostResult{}, models.ErrUnsupportedOperation
}

func (fake *recordingModelsService) StopModelHost(
	context.Context,
	models.StopModelHostRequest,
) (models.StopModelHostResult, error) {
	return models.StopModelHostResult{}, models.ErrUnsupportedOperation
}

func (fake *recordingModelsService) AcquireModelLease(
	context.Context,
	models.AcquireModelLeaseRequest,
) (models.AcquireModelLeaseResult, error) {
	return models.AcquireModelLeaseResult{}, models.ErrUnsupportedOperation
}

func (fake *recordingModelsService) GetModelLease(
	context.Context,
	models.GetModelLeaseRequest,
) (models.GetModelLeaseResult, error) {
	return models.GetModelLeaseResult{}, models.ErrUnsupportedOperation
}

func (fake *recordingModelsService) ReleaseModelLease(
	context.Context,
	models.ReleaseModelLeaseRequest,
) (models.ReleaseModelLeaseResult, error) {
	return models.ReleaseModelLeaseResult{}, models.ErrUnsupportedOperation
}

func (fake *recordingModelsService) InvokeModelWithLease(
	context.Context,
	models.InvokeModelRequest,
) (models.InvokeModelResult, error) {
	return models.InvokeModelResult{}, models.ErrUnsupportedOperation
}

func (fake *recordingModelsService) CancelInvocation(
	context.Context,
	models.CancelInvocationRequest,
) (models.CancelInvocationResult, error) {
	return models.CancelInvocationResult{}, models.ErrUnsupportedOperation
}

func (fake *recordingModelsService) ListModels(context.Context) (models.List, error) {
	return models.List{}, models.ErrUnsupportedOperation
}

func (fake *recordingModelsService) GetModel(context.Context, string) (models.Detail, error) {
	return models.Detail{}, models.ErrUnsupportedOperation
}

func (fake *recordingModelsService) PullModel(context.Context, string) (models.PullResult, error) {
	return models.PullResult{}, models.ErrUnsupportedOperation
}

func (fake *recordingModelsService) InspectRuntime(context.Context, string) (models.Runtime, error) {
	return models.Runtime{}, models.ErrUnsupportedOperation
}

func (fake *recordingModelsService) AcquireLease(
	context.Context,
	models.AcquireLeaseRequest,
) (models.HostLease, error) {
	return models.HostLease{}, models.ErrUnsupportedOperation
}

func (fake *recordingModelsService) ReleaseLease(
	context.Context,
	models.ReleaseLeaseRequest,
) error {
	return models.ErrUnsupportedOperation
}

func (fake *recordingModelsService) InvokeLocal(
	context.Context,
	models.LocalInvocationRequest,
) (models.LocalInvocationResult, error) {
	return models.LocalInvocationResult{}, models.ErrUnsupportedOperation
}

func TestBindModelsRuntimeScopeOpensDetachedScopeWithoutForRuntime(t *testing.T) {
	t.Parallel()

	fake := &recordingModelsService{}
	runtimeConfig := &models.RuntimeConfig{
		FactoryDirectory: "/factory",
		BaseDirectory:    "/runtime",
	}
	bind, err := bindModelsRuntimeScope(
		context.Background(),
		fake,
		"/cache/models",
		func() *models.RuntimeConfig { return runtimeConfig },
	)
	if err != nil {
		t.Fatalf("bindModelsRuntimeScope() error = %v, want nil", err)
	}
	if root, ok := bind.Root.(*recordingModelsService); !ok || root != fake {
		t.Fatal("bindModelsRuntimeScope() did not keep the process-scoped Models root")
	}
	if bind.Scope.IsZero() {
		t.Fatal("bindModelsRuntimeScope() returned zero runtime scope")
	}
	if fake.forRuntimeCalls != 0 {
		t.Fatalf("ForRuntime calls = %d, want 0", fake.forRuntimeCalls)
	}
	if len(fake.openRequests) != 1 {
		t.Fatalf("OpenRuntimeScope requests = %d, want 1", len(fake.openRequests))
	}
	got := fake.openRequests[0].Config
	if got.CacheDirectory != "/cache/models" {
		t.Fatalf("scope cache directory = %q, want /cache/models", got.CacheDirectory)
	}
	if got.Runtime.FactoryDirectory != runtimeConfig.FactoryDirectory {
		t.Fatalf("scope runtime factory directory = %q, want %q", got.Runtime.FactoryDirectory, runtimeConfig.FactoryDirectory)
	}
}

func TestAssembleRuntimeProductsCarriesModelsRootAndScopeIntoOpenedRuntime(t *testing.T) {
	t.Parallel()

	root := &recordingModelsService{}
	scope, err := (models.RuntimeScopeRef{}).Parse("factory-session:test:assembled")
	if err != nil {
		t.Fatalf("parse Models scope: %v", err)
	}

	opened := assembleRuntimeProducts(
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		modelsRuntimeBind{Root: root, Scope: scope},
		nil,
		inertHostedInstance{},
		nil,
		nil,
		nil,
		nil,
		"/factory",
		"runtime-1",
		"backend-1",
	)

	if opened.application.HTTP.Models != root {
		t.Fatal("opened application runtime did not retain the process-scoped Models root")
	}
	if opened.modelsScope != scope {
		t.Fatalf("opened Models scope = %q, want %q", opened.modelsScope.String(), scope.String())
	}
	if root.forRuntimeCalls != 0 {
		t.Fatalf("ForRuntime calls = %d, want 0", root.forRuntimeCalls)
	}
}

type inertHostedInstance struct{}

func (inertHostedInstance) RuntimeService() factoryruntime.Service { return nil }
func (inertHostedInstance) Directory() string                      { return "" }
func (inertHostedInstance) FolderDirectory() string                { return "" }
func (inertHostedInstance) BackendScope() string                   { return "" }
func (inertHostedInstance) StartTime() time.Time                   { return time.Time{} }
func (inertHostedInstance) LoadedRuntimeConfig() factoryruntime.LoadedConfig {
	return nil
}
func (inertHostedInstance) CanonicalEvents() []factorydefinitions.FactoryEvent { return nil }
func (inertHostedInstance) AddEventTypeRecorder(func(factorydefinitions.FactoryEventType)) {
}
func (inertHostedInstance) StreamGeneration() string { return "" }
func (inertHostedInstance) RuntimeLogger() *zap.Logger {
	return zap.NewNop()
}
func (inertHostedInstance) RuntimeMetrics() factoryruntime.MetricsEmitter { return nil }
func (inertHostedInstance) RuntimeDiagnostics() factoryruntime.RuntimeLogDiagnostics {
	return factoryruntime.RuntimeLogDiagnostics{}
}
func (inertHostedInstance) RecordingLedger() recordings.Ledger { return nil }
func (inertHostedInstance) CloseArtifacts() error              { return nil }
