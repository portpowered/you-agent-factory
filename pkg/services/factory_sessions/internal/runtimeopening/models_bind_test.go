package runtimeopening

import (
	"context"
	"errors"
	factorydefinitionswire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"
	"slices"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/automations"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"github.com/portpowered/infinite-you/pkg/services/models"
	operatorconfig "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

type recordingModelsService struct {
	openRequests    []models.OpenRuntimeScopeRequest
	closeRequests   []models.CloseRuntimeScopeRequest
	forRuntimeCalls int
	events          *[]string
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
		func() error { return nil },
	)

	if opened.application.HTTP.Models != root {
		t.Fatal("opened application runtime did not retain the process-scoped Models root")
	}
	if opened.application.HTTP.ModelsScope != scope {
		t.Fatalf("opened HTTP Models scope = %q, want %q", opened.application.HTTP.ModelsScope, scope)
	}
	if root.forRuntimeCalls != 0 {
		t.Fatalf("ForRuntime calls = %d, want 0", root.forRuntimeCalls)
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

func TestOpenRuntimeClosesModelsScopeExactlyOnceAfterLaterStepFails(t *testing.T) {
	t.Parallel()

	var events []string
	modelRoot := &recordingModelsService{events: &events}
	laterErr := errors.New("worker runtime opening failed")
	failure := &openingCoordinatorFailure{events: &events, err: laterErr}
	factory := &Factory{
		durableExecutionFactory:        openingCoordinatorDurableExecution,
		workerExecutionFactory:         failure.openWorkerExecution,
		modelService:                   modelRoot,
		factorySessionsService:         openingCoordinatorSessionsRoot{},
		recordingsProjectionFactory:    openingCoordinatorProjections,
		runtimeLedgerFactory:           openingCoordinatorLedgerFactory,
		runtimeRecorderFactory:         openingCoordinatorRecorder,
		automationHostedSourcesFactory: openingCoordinatorHostedPollers,
		factoryScaffoldInitializer:     openingCoordinatorInitializeScaffold,
		editableFactoryValidator:       openingCoordinatorValidateEditable,
		workService:                    work.MaterializationService(openingCoordinatorContentMaterializer{}),
		factoryDefinitionValidator:     openingCoordinatorValidator{},
		namedPaths:                     openingCoordinatorNamedPaths{},
		loadFactory:                    openingCoordinatorLoadFactory,
		resolveClock:                   openingCoordinatorResolveClock,
		newSessionLogger:               openingCoordinatorSessionLogger,
		adaptWorkerCommandRunner:       openingCoordinatorAdaptCommandRunner,
		generateRuntimeInstanceID:      func() string { return "runtime-opening-cleanup-test" },
		resolveHome:                    func() (string, error) { return t.TempDir(), nil },
		providerIdentities:             func(identity string) (string, error) { return identity, nil },
	}
	_, err := factory.openRuntime(
		context.Background(),
		&factorysessions.RuntimeOpeningRequest{
			FactoryDefinition: factorydefinitions.RuntimeOpeningRequest{Directory: t.TempDir()},
			FactorySession:    factorysessions.SessionRuntimeOpeningRequest{BackendScopeID: "test-scope"},
		},
		ExternalEffects{},
		zap.NewNop(),
	)
	if !errors.Is(err, laterErr) {
		t.Fatalf("openRuntime() error = %v, want later-step failure", err)
	}
	if !slices.Equal(events, []string{"models-open", "later-step-failed", "models-close"}) {
		t.Fatalf("opening events = %v, want scope cleanup after later-step failure", events)
	}
	if len(modelRoot.closeRequests) != 1 {
		t.Fatalf("CloseRuntimeScope requests = %d, want exactly 1", len(modelRoot.closeRequests))
	}
}

type openingCoordinatorFailure struct {
	events *[]string
	err    error
}

func (failure *openingCoordinatorFailure) openWorkerExecution(
	factoryruntime.RuntimeOpeningRequest,
	workers.RuntimeOpeningRequest,
	factoryruntime.Clock,
	*zap.Logger,
	workers.CommandRunner,
	workers.CommandRunner,
	workers.PTYAllocator,
	workers.Provider,
	roles.CurrentRuntimeResolver,
	models.Service,
	models.RuntimeScopeRef,
	work.Service,
	WorkersRuntimeFactory,
	[]operatorconfig.ACPIntegration,
) (workers.RuntimeService, error) {
	*failure.events = append(*failure.events, "later-step-failed")
	return nil, failure.err
}

func openingCoordinatorDurableExecution(
	_ factorydefinitions.RuntimeOpeningRequest,
	_ factorysessions.SessionRuntimeOpeningRequest,
	_ operatorconfig.ResolvedDefaults,
	_ RuntimeRoot,
	_ factoryruntime.Clock,
	_ workers.Provider,
	_ *workers.MockWorkersConfig,
	_ FactorySessionExecutionFactory,
	_ factorysessions.ProviderIdentityResolver,
) (DurableExecution, error) {
	return DurableExecution{}, nil
}

func openingCoordinatorProjections() recordings.ProjectionService {
	return openingCoordinatorProjection{}
}

func openingCoordinatorLedgerFactory() factoryruntime.RuntimeLedgerFactory {
	return func(
		recordings.InitialStructureSource,
		func() time.Time,
		factorydefinitions.RuntimeDefinitionLookup,
	) recordings.RuntimeEventLedger {
		return nil
	}
}

func openingCoordinatorRecorder(
	time.Duration,
	factorydefinitions.LoadedFactorySource,
	func() time.Time,
	string,
) (recordings.RuntimeRecorder, error) {
	return nil, nil
}

func openingCoordinatorHostedPollers(
	*zap.Logger,
	automations.HostedLinearClock,
	automations.HostedLinearHTTPDoer,
	automations.HostedLinearSecretResolver,
	string,
) automations.HostedPollers {
	return nil
}

func openingCoordinatorInitializeScaffold(string) error {
	return nil
}

func openingCoordinatorValidateEditable(
	context.Context,
	*factorydefinitions.FactorySnapshot,
	factorydefinitions.WorkstationLoader,
) error {
	return nil
}

func openingCoordinatorLoadFactory(
	string,
	factorydefinitions.WorkstationLoader,
) (factorydefinitions.MutableLoadedFactorySource, error) {
	return nil, nil
}

func openingCoordinatorResolveClock(clock factoryruntime.Clock) factoryruntime.Clock {
	if clock != nil {
		return clock
	}
	return openingCoordinatorClock{}
}

func openingCoordinatorSessionLogger(*zap.Logger, string, string, string) *zap.Logger {
	return zap.NewNop()
}

func openingCoordinatorAdaptCommandRunner(platformprocess.CommandRunner) workers.CommandRunner {
	return nil
}

type openingCoordinatorSessionsRoot struct {
	factorysessions.Service
}

func (openingCoordinatorSessionsRoot) ForRuntime(
	factorysessions.OpeningBindingRequest,
) (factorysessions.Service, error) {
	return openingCoordinatorBoundSessions{}, nil
}

type openingCoordinatorBoundSessions struct {
	factorysessions.Service
	roles.RuntimeAssembly
}

func (openingCoordinatorBoundSessions) CurrentRuntime() *factorysessions.LiveRuntime {
	return nil
}

type openingCoordinatorProjection struct {
	recordings.ProjectionService
}

type openingCoordinatorContentMaterializer struct {
	work.ContentMaterializer
}

type openingCoordinatorValidator struct {
	factorydefinitions.Validator
}

type openingCoordinatorNamedPaths struct {
	factorydefinitionswire.NamedPathResolver
}

func (openingCoordinatorNamedPaths) ResolveCurrentDir(rootDir string) (string, error) {
	return rootDir, nil
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
