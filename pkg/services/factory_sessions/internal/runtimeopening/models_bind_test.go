package runtimeopening

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"go.uber.org/zap"
)

type recordingModelsService struct {
	openRequests  []models.OpenRuntimeScopeRequest
	closeRequests []models.CloseRuntimeScopeRequest
	events        *[]string
}

func (fake *recordingModelsService) OpenRuntimeScope(
	_ context.Context,
	request models.OpenRuntimeScopeRequest,
) (models.OpenRuntimeScopeResult, error) {
	fake.openRequests = append(fake.openRequests, request)
	if fake.events != nil {
		*fake.events = append(*fake.events, "models-open")
	}
	scope, err := (models.RuntimeScopeRef{}).Parse("factory-session:test:1")
	if err != nil {
		return models.OpenRuntimeScopeResult{}, err
	}
	return models.OpenRuntimeScopeResult{Scope: scope}, nil
}

func (fake *recordingModelsService) CloseRuntimeScope(
	_ context.Context,
	request models.CloseRuntimeScopeRequest,
) (models.CloseRuntimeScopeResult, error) {
	fake.closeRequests = append(fake.closeRequests, request)
	if fake.events != nil {
		*fake.events = append(*fake.events, "models-close")
	}
	return models.CloseRuntimeScopeResult{Scope: request.Scope, Closed: true}, nil
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

func (fake *recordingModelsService) ResolveModelReference(
	context.Context,
	models.ResolveModelReferenceRequest,
) (models.ResolveModelReferenceResult, error) {
	return models.ResolveModelReferenceResult{}, models.ErrUnsupportedOperation
}

func (fake *recordingModelsService) PreflightModelAssets(
	context.Context,
	models.PrepareModelAssetsRequest,
) (models.PreflightModelAssetsResult, error) {
	return models.PreflightModelAssetsResult{}, models.ErrUnsupportedOperation
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

func (fake *recordingModelsService) InvokeModel(
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

func (fake *recordingModelsService) PullModelForScope(
	context.Context,
	models.PullModelRequest,
) (models.PullResult, error) {
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

func TestBindModelsRuntimeScopeOpensDetachedScope(t *testing.T) {
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
		nil,
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
		context.Background(),
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
		func() error { return nil },
	)

	if opened.application.Models != root {
		t.Fatal("opened application runtime did not retain the process-scoped Models root")
	}
	if opened.application.ModelsScope != scope {
		t.Fatalf("opened Models scope = %q, want %q", opened.application.ModelsScope, scope)
	}
}

func TestAssembleRuntimeProductsBindsHostBoundFactorySessionsGatewayForApplicationRoles(t *testing.T) {
	t.Parallel()

	gateway := &runtimeProductsSessionsRole{readSession: func(string) factorysessions.SessionReadResult {
		return factorysessions.SessionReadResult{SessionID: "host-bound-gateway"}
	}, readLiveSession: func(string) factorysessions.LiveControlSnapshot {
		return factorysessions.LiveControlSnapshot{Context: factorysessions.ProjectionContext{FactorySessionID: "host-bound-gateway"}}
	}}
	opened := assembleRuntimeProducts(
		context.Background(),
		nil,
		gateway,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		modelsRuntimeBind{},
		nil,
		inertHostedInstance{},
		nil,
		nil,
		nil,
		nil,
		"/factory",
		"runtime-1",
		"backend-1",
		func() error { return nil },
	)

	got, err := opened.application.FactorySessions.GetSession(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("FactorySessions.GetSession() error = %v, want host-bound gateway read", err)
	}
	if got.SessionID != "host-bound-gateway" {
		t.Fatalf("FactorySessions.GetSession() = %q, want host-bound gateway result", got.SessionID)
	}
	liveRead, err := opened.application.LiveControl.GetFactorySession(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("LiveControl.GetSession() error = %v, want host-bound gateway read", err)
	}
	if liveRead.Context.FactorySessionID != "host-bound-gateway" {
		t.Fatalf("LiveControl.GetFactorySession() = %q, want host-bound gateway result", liveRead.Context.FactorySessionID)
	}
}

func TestAssembledRuntimeResourcesCloseAcquiredResourcesInReverseOrder(t *testing.T) {
	t.Parallel()

	var events []string
	cleanup := &runtimeOpeningCleanup{}
	cleanup.Add(func() error {
		events = append(events, "models-close")
		return nil
	})
	cleanup.Add(func() error {
		events = append(events, "workers-close")
		return nil
	})

	opened := assembleRuntimeProducts(
		context.Background(),
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		modelsRuntimeBind{},
		nil,
		inertHostedInstance{},
		nil,
		nil,
		nil,
		nil,
		"/factory",
		"runtime-1",
		"backend-1",
		cleanup.Close,
	)
	if err := opened.application.Resources.Close(); err != nil {
		t.Fatalf("opened runtime resource Close() error = %v, want nil", err)
	}
	if !slices.Equal(events, []string{"workers-close", "models-close"}) {
		t.Fatalf("runtime close events = %v, want reverse acquisition order", events)
	}
	if err := opened.application.Resources.Close(); err != nil {
		t.Fatalf("second opened runtime resource Close() error = %v, want nil", err)
	}
	if !slices.Equal(events, []string{"workers-close", "models-close"}) {
		t.Fatalf("runtime close events after second close = %v, want each resource closed once", events)
	}
}

func TestRuntimeOpeningCleanupClosesModelsScopeAfterLaterResourceOnFailure(t *testing.T) {
	t.Parallel()

	var events []string
	root := &recordingModelsService{events: &events}
	bind, err := bindModelsRuntimeScope(
		context.Background(),
		root,
		"/cache/models",
		func() *models.RuntimeConfig { return &models.RuntimeConfig{} },
		nil,
	)
	if err != nil {
		t.Fatalf("bindModelsRuntimeScope() error = %v, want nil", err)
	}

	cleanup := &runtimeOpeningCleanup{}
	cleanup.OwnModelsScope(context.Background(), bind)
	cleanup.Add(func() error {
		events = append(events, "later-close")
		return nil
	})
	openingErr := errors.New("later opening step failed")
	if err := cleanup.Unwind(openingErr); !errors.Is(err, openingErr) {
		t.Fatalf("Unwind() error = %v, want opening failure", err)
	}
	if !slices.Equal(events, []string{"models-open", "later-close", "models-close"}) {
		t.Fatalf("cleanup events = %v, want reverse acquisition order", events)
	}
	if len(root.closeRequests) != 1 || root.closeRequests[0].Scope != bind.Scope {
		t.Fatalf("CloseRuntimeScope requests = %#v, want issued scope exactly once", root.closeRequests)
	}

	if err := cleanup.Close(); err != nil {
		t.Fatalf("second Close() error = %v, want nil", err)
	}
	if len(root.closeRequests) != 1 {
		t.Fatalf("CloseRuntimeScope requests after second close = %d, want 1", len(root.closeRequests))
	}
}

func TestRuntimeOpeningCleanupPreservesPrimaryErrorAndAggregatesCleanupFailures(t *testing.T) {
	t.Parallel()

	var events []string
	firstCleanupErr := errors.New("first cleanup failed")
	secondCleanupErr := errors.New("second cleanup failed")
	openingErr := errors.New("runtime assembly failed")
	cleanup := &runtimeOpeningCleanup{}
	cleanup.Add(func() error {
		events = append(events, "first-close")
		return firstCleanupErr
	})
	cleanup.Add(func() error {
		events = append(events, "second-close")
		return secondCleanupErr
	})

	err := cleanup.Unwind(openingErr)
	for _, expected := range []error{openingErr, firstCleanupErr, secondCleanupErr} {
		if !errors.Is(err, expected) {
			t.Fatalf("Unwind() error = %v, want to retain %v", err, expected)
		}
	}
	if !slices.Equal(events, []string{"second-close", "first-close"}) {
		t.Fatalf("cleanup events = %v, want reverse acquisition order", events)
	}

	if err := cleanup.Close(); !errors.Is(err, firstCleanupErr) || !errors.Is(err, secondCleanupErr) {
		t.Fatalf("second Close() error = %v, want previously aggregated cleanup errors", err)
	}
	if !slices.Equal(events, []string{"second-close", "first-close"}) {
		t.Fatalf("cleanup events after second Close() = %v, want each resource closed once", events)
	}
}

type runtimeProductsSessionsRole struct {
	factorysessions.Service
	readSession     func(string) factorysessions.SessionReadResult
	readLiveSession func(string) factorysessions.LiveControlSnapshot
}

func (role *runtimeProductsSessionsRole) GetSession(_ context.Context, sessionID string) (factorysessions.SessionReadResult, error) {
	return role.readSession(sessionID), nil
}

func (role *runtimeProductsSessionsRole) GetFactorySession(_ context.Context, sessionID string) (factorysessions.LiveControlSnapshot, error) {
	return role.readLiveSession(sessionID), nil
}

func (role *runtimeProductsSessionsRole) OpenFactorySession(context.Context, factorysessions.LiveControlOpenRequest) (*factorysessions.LiveControlOpenResult, error) {
	return nil, errors.New("not implemented")
}

func (role *runtimeProductsSessionsRole) ListFactorySessions(context.Context) ([]factorysessions.LiveControlListItem, error) {
	return nil, errors.New("not implemented")
}

func (role *runtimeProductsSessionsRole) PauseLiveFactorySession(context.Context, string, factorysessions.LiveControlRequest) (factorysessions.LiveControlResult, error) {
	return factorysessions.LiveControlResult{}, errors.New("not implemented")
}

func (role *runtimeProductsSessionsRole) ResumeLiveFactorySession(context.Context, string, factorysessions.LiveControlRequest) (factorysessions.LiveControlResult, error) {
	return factorysessions.LiveControlResult{}, errors.New("not implemented")
}

func (role *runtimeProductsSessionsRole) CloseFactorySession(context.Context, string) error {
	return errors.New("not implemented")
}

type openingCoordinatorProjection struct {
	recordings.ProjectionService
}

type openingCoordinatorClock struct{}

func (openingCoordinatorClock) Now() time.Time {
	return time.Unix(1, 0)
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
