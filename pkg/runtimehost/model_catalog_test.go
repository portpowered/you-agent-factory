package runtimehost

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"

	"github.com/jonboulle/clockwork"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factorysessions "github.com/portpowered/infinite-you/pkg/factory/sessions"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	sessioninvocation "github.com/portpowered/infinite-you/pkg/factory/sessions/invocation"
	modelhost "github.com/portpowered/infinite-you/pkg/models/host"
	localmodels "github.com/portpowered/infinite-you/pkg/models/local"
	managedruntime "github.com/portpowered/infinite-you/pkg/models/managedruntime"
	modelsservice "github.com/portpowered/infinite-you/pkg/models/service"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	workerapplication "github.com/portpowered/infinite-you/pkg/workers/application"
	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
	hostedworkers "github.com/portpowered/infinite-you/pkg/workers/hosted"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestHostDurableOperationsRequireInjectedExecution(t *testing.T) {
	t.Parallel()

	workflowName := "missing-execution"
	host := &Host{}
	_, startErr := host.StartDurableFactorySessionAsync(context.Background(), factoryapi.FactorySessionExecutionRequest{
		RequestId: "req-missing-durable-execution",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowName,
			WorkflowName: &workflowName,
		},
	})
	if !errors.Is(startErr, factorysessionexecution.ErrServiceNotConfigured) {
		t.Fatalf("StartDurableFactorySessionAsync error = %v, want missing execution error", startErr)
	}

	_, listErr := host.ListDurableExecutionSessions(context.Background(), factorysessionexecution.ListSessionsRequest{})
	if !errors.Is(listErr, factorysessionexecution.ErrServiceNotConfigured) {
		t.Fatalf("ListDurableExecutionSessions error = %v, want missing execution error", listErr)
	}
	if host.durableExecution != nil {
		t.Fatal("durable operation lazily created hidden execution state")
	}
}

func TestHostWithoutAttachedModelServiceReturnsConstructionError(t *testing.T) {
	var host *Host
	ctx := context.Background()
	_, listErr := host.ListModels(ctx)
	_, getErr := host.GetModel(ctx, "OMNIVOICE_Q4_K_M")
	_, pullErr := host.PullModel(ctx, "OMNIVOICE_Q4_K_M")
	_, invokeErr := host.InvokeModel(ctx, "OMNIVOICE_Q4_K_M", factoryapi.ModelInvocationRequest{Operation: "TTS"})
	for operation, err := range map[string]error{
		"list": listErr, "get": getErr, "pull": pullErr, "invoke": invokeErr,
	} {
		if !errors.Is(err, errModelServiceUnavailable) {
			t.Fatalf("nil Host %s error = %v, want unavailable model service", operation, err)
		}
	}
	if host.factoryRunnerID() != "" || host.providerOverride() != nil || host.invocationSkipPermissionsOverride() != nil {
		t.Fatal("nil Host returned model invocation policy overrides")
	}
}

func TestCoreModelServiceAttachmentIsVisibleToSnapshotsAndHostFacade(t *testing.T) {
	t.Parallel()

	if NewHostFromCore(nil) != nil {
		t.Fatal("NewHostFromCore(nil) returned a host")
	}
	var nilCore *Core
	if nilCore.ModelService() != nil {
		t.Fatal("nil core returned a model service")
	}
	stub := &catalogModelServiceStub{}
	core := NewCore(&Config{}, "", zap.NewNop(), nil, nil, nil,
		LocalModelDomain{}, hostedworkers.Config{}, nil, nil, zap.NewNop(), nil, nil, nil)
	AttachModelService(core, stub)
	if core.ModelService() != stub || !core.ComposeCollaboratorSnapshot().ModelServiceInitialized {
		t.Fatal("core did not retain the attached model service in its composition snapshot")
	}
	host := NewHostFromCore(core)
	if !host.ComposeCollaboratorSnapshot().ModelServiceInitialized {
		t.Fatal("host facade snapshot omitted the core-owned model service")
	}
	if _, err := host.ListModels(context.Background()); err != nil {
		t.Fatalf("host ListModels() error = %v", err)
	}
	if host.CurrentModelRuntimeConfig() != nil {
		t.Fatal("empty host returned a model runtime config")
	}
	if _, err := host.BuildModelInvocationExecutor(nil, nil, "worker"); err == nil {
		t.Fatal("host built a model invocation executor without runtime configuration")
	}
	if !reflect.DeepEqual(stub.calls, []string{"list"}) {
		t.Fatalf("forwarded model calls = %v, want ListModels once", stub.calls)
	}
}

func TestRuntimeHostModelServicePreservesCatalogPullObservabilityAndErrors(t *testing.T) {
	runtimeCfg := runtimeHostModelConfig(t)
	modelHost := &runtimeHostModelHost{readiness: modelhost.ReadinessSnapshot{
		Identity:       modelhost.Identity{Name: "voice-model", Locality: managedruntime.LocalityLocal},
		ReadinessState: managedruntime.ReadinessStateReady,
		LifecycleState: managedruntime.LifecycleStateLoaded,
	}, pull: modelhost.PullSnapshot{
		ReadinessSnapshot: modelhost.ReadinessSnapshot{
			Identity:       modelhost.Identity{Name: "voice-model", Locality: managedruntime.LocalityLocal},
			ReadinessState: managedruntime.ReadinessStateReady,
			LifecycleState: managedruntime.LifecycleStateLoaded,
		},
		PullOutcome:   managedruntime.PullOutcomeAlreadyReady,
		LegacyOutcome: "ALREADY_PRESENT",
		CachePath:     "/tmp/model",
		Revision:      "rev-1",
	}}
	puller := &runtimeHostModelPuller{result: apisurface.ModelPullResult{
		ModelName: "voice-model", Outcome: "ALREADY_PRESENT", CachePath: "/tmp/model", Revision: "rev-1",
	}, inspection: localmodels.RuntimeCacheInspection{
		Supported: true, Installed: true, CachePath: "/tmp/model", Revision: "rev-1",
	}}
	recorder := &runtimeHostPullMetricsRecorder{}
	logCore, logs := observer.New(zap.InfoLevel)
	logger := zap.New(logCore)
	host := runtimeHostModelFacade(t, runtimeCfg, modelHost, puller, &Config{Logger: logger, ModelPullMetricsRecorder: recorder})

	listed, err := host.ListModels(context.Background())
	if err != nil || len(listed.Results) != 1 || listed.Results[0].ManagedRuntime.ReadinessState != factoryapi.ManagedRuntimeReadinessStateREADY {
		t.Fatalf("ListModels = (%#v, %v), want one ready model", listed, err)
	}
	pulled, err := host.PullModel(context.Background(), "voice-model")
	if err != nil || pulled.ReadinessState != "READY" {
		t.Fatalf("PullModel = (%#v, %v), want ready result", pulled, err)
	}
	if got := recorder.names(); !reflect.DeepEqual(got, []string{modelPullMetricAttempts, modelPullMetricSuccess}) {
		t.Fatalf("pull metrics = %#v, want one attempt and one success", got)
	}
	if got := logs.FilterMessage("managed runtime pull completed").Len(); got != 1 {
		t.Fatalf("pull completion log count = %d, want 1", got)
	}

	sentinel := errors.New("readiness failed")
	modelHost.readinessErr = sentinel
	_, err = host.GetModel(context.Background(), "voice-model")
	if !errors.Is(err, sentinel) {
		t.Fatalf("GetModel error = %v, want readiness sentinel", err)
	}
}

func TestRuntimeHostModelServicePreservesPullFailureSignals(t *testing.T) {
	runtimeCfg := runtimeHostModelConfig(t)
	recorder := &runtimeHostPullMetricsRecorder{}
	logCore, logs := observer.New(zap.WarnLevel)
	puller := &runtimeHostModelPuller{}
	modelHost := &runtimeHostModelHost{pullErr: apisurface.ErrManagedRuntimeSourceFetchFailed}
	logger := zap.New(logCore)
	host := runtimeHostModelFacade(t, runtimeCfg, modelHost, puller, &Config{Logger: logger, ModelPullMetricsRecorder: recorder})

	_, err := host.PullModel(context.Background(), "voice-model")
	if !errors.Is(err, apisurface.ErrManagedRuntimeSourceFetchFailed) {
		t.Fatalf("PullModel error = %v, want source fetch failure", err)
	}
	if got := recorder.names(); !reflect.DeepEqual(got, []string{modelPullMetricAttempts, modelPullMetricFailure, modelPullMetricSourceFailure}) {
		t.Fatalf("pull metrics = %#v, want attempt, failure, and source failure", got)
	}
	if got := logs.FilterMessage("managed runtime pull failed").Len(); got != 1 {
		t.Fatalf("pull failure log count = %d, want 1", got)
	}
}

func TestRuntimeHostModelServicePreservesFactoryRunnerIdentityForInvocation(t *testing.T) {
	runtimeCfg := runtimeHostModelConfig(t)
	provider := &runtimeHostInvocationProvider{}
	cfg := &Config{RunnerID: "factory-runner", ProviderOverride: provider}
	components, err := workerapplication.New(zap.NewNop(), workerapplication.Edges{})
	if err != nil {
		t.Fatalf("construct worker application: %v", err)
	}
	cfg.WorkerApplication = components
	host := runtimeHostModelFacade(t, runtimeCfg, &runtimeHostModelHost{readiness: modelhost.ReadinessSnapshot{
		Identity:       modelhost.Identity{Name: "voice-model", Locality: managedruntime.LocalityLocal},
		ReadinessState: managedruntime.ReadinessStateReady,
		LifecycleState: managedruntime.LifecycleStateLoaded,
	}}, &runtimeHostModelPuller{}, cfg)

	result, err := host.InvokeModel(context.Background(), "voice-model", factoryapi.ModelInvocationRequest{Operation: "TTS"})
	if err != nil {
		t.Fatalf("InvokeModel: %v", err)
	}
	if result.ModelName != "voice-model" || result.Worker != "voice-worker" {
		t.Fatalf("InvokeModel result = %#v, want voice-model/voice-worker", result)
	}
	if got := provider.runnerIDs(); !reflect.DeepEqual(got, []string{"factory-runner"}) {
		t.Fatalf("provider runner IDs = %#v, want factory-runner", got)
	}
}

func runtimeHostModelConfig(t *testing.T) *factoryconfig.LoadedFactoryConfig {
	t.Helper()
	cfg, err := factoryconfig.NewLoadedFactoryConfig(t.TempDir(), &interfaces.FactoryConfig{
		Name: "runtime-host-models",
		Workers: []workerconfig.Config{{
			Name: "voice-worker", Type: interfaces.WorkerTypeModel, Model: "voice-model",
			ModelLocality: workerconfig.ModelLocalityLocal,
			Operations:    []workerconfig.ModelOperation{{Name: "TTS"}},
		}},
	}, nil)
	if err != nil {
		t.Fatalf("NewLoadedFactoryConfig: %v", err)
	}
	return cfg
}

func runtimeHostModelFacade(t *testing.T, runtimeCfg *factoryconfig.LoadedFactoryConfig, host modelhost.Host, puller modelAssetPuller, cfg *Config) *Host {
	t.Helper()
	facade := &Host{
		cfg: cfg, policy: CoordinatorPolicyFromConfig(cfg), modelAssets: puller, logger: cfg.Logger,
		clock:         clockwork.NewRealClock(),
		startupBundle: &factoryRuntimeBundle{RuntimeCfg: runtimeCfg, ModelHost: host, ModelAssets: puller},
	}
	modelAPI, err := modelsservice.NewService(modelsservice.Dependencies{
		RuntimeConfig: facade.currentRuntimeConfig, ModelHost: facade.modelHost(),
		ModelAssetPuller: puller, Logger: cfg.Logger, Clock: facade.clock.Now,
		ModelPullMetrics:        runtimeHostModelMetricsAdapter{inner: cfg.ModelPullMetricsRecorder},
		ModelInvocationExecutor: facade.modelInvocationExecutor,
		FactoryRunnerID:         cfg.RunnerID,
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	facade.modelService = modelAPI
	return facade
}

type runtimeHostModelMetricsAdapter struct {
	inner ModelPullMetricsRecorder
}

func (a runtimeHostModelMetricsAdapter) RecordModelPullMetric(metric modelsservice.PullMetric) {
	if a.inner != nil {
		a.inner.RecordModelPullMetric(InvocationMetric{Name: metric.Name, Labels: metric.Labels})
	}
}

type runtimeHostModelPuller struct {
	result     apisurface.ModelPullResult
	inspection localmodels.RuntimeCacheInspection
	err        error
}

func (p *runtimeHostModelPuller) PullModel(context.Context, *factoryconfig.LoadedFactoryConfig, string) (apisurface.ModelPullResult, error) {
	return p.result, p.err
}
func (p *runtimeHostModelPuller) EnsureModelAvailable(context.Context, *factoryconfig.LoadedFactoryConfig, *workerconfig.Config) error {
	return nil
}
func (p *runtimeHostModelPuller) ResolveModelCache(context.Context, *factoryconfig.LoadedFactoryConfig, *workerconfig.Config) (localmodels.CacheLayout, error) {
	return localmodels.CacheLayout{}, nil
}
func (p *runtimeHostModelPuller) InspectRuntimeCache(context.Context, *factoryconfig.LoadedFactoryConfig, string) (localmodels.RuntimeCacheInspection, error) {
	return p.inspection, nil
}

type runtimeHostModelHost struct {
	readiness    modelhost.ReadinessSnapshot
	readinessErr error
	pull         modelhost.PullSnapshot
	pullErr      error
}

func (h *runtimeHostModelHost) ResolveIdentity(context.Context, *factoryconfig.LoadedFactoryConfig, string) (modelhost.Identity, error) {
	return h.readiness.Identity, nil
}
func (h *runtimeHostModelHost) InspectReadiness(context.Context, *factoryconfig.LoadedFactoryConfig, string) (modelhost.ReadinessSnapshot, error) {
	return h.readiness, h.readinessErr
}
func (h *runtimeHostModelHost) Pull(context.Context, *factoryconfig.LoadedFactoryConfig, string) (modelhost.PullSnapshot, error) {
	return h.pull, h.pullErr
}
func (*runtimeHostModelHost) AcquireLease(context.Context, *factoryconfig.LoadedFactoryConfig, string, modelhost.LeaseOptions) (modelhost.Lease, error) {
	return modelhost.Lease{}, nil
}
func (*runtimeHostModelHost) ReleaseLease(context.Context, string) error { return nil }
func (*runtimeHostModelHost) Unload(context.Context, *factoryconfig.LoadedFactoryConfig, string) error {
	return nil
}

type runtimeHostPullMetricsRecorder struct {
	mu      sync.Mutex
	metrics []InvocationMetric
}

func (r *runtimeHostPullMetricsRecorder) RecordModelPullMetric(metric InvocationMetric) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.metrics = append(r.metrics, metric)
}
func (r *runtimeHostPullMetricsRecorder) names() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	names := make([]string, len(r.metrics))
	for i := range r.metrics {
		names[i] = r.metrics[i].Name
	}
	return names
}

type runtimeHostInvocationProvider struct {
	mu       sync.Mutex
	requests []workerexecution.ProviderInferenceRequest
}

func (p *runtimeHostInvocationProvider) Infer(_ context.Context, request workerexecution.ProviderInferenceRequest) (workerexecution.InferenceResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.requests = append(p.requests, request)
	return workerexecution.InferenceResponse{Content: "spoken response"}, nil
}
func (p *runtimeHostInvocationProvider) runnerIDs() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	ids := make([]string, len(p.requests))
	for i := range p.requests {
		ids[i] = p.requests[i].RunnerID
	}
	return ids
}

func TestHostModelMethodsForwardContextResultsAndErrorsUnchanged(t *testing.T) {
	t.Parallel()

	type contextKey string
	ctx := context.WithValue(context.Background(), contextKey("request"), "catalog-request")
	listErr := errors.New("list sentinel")
	getErr := fmt.Errorf("requested model: %w", apisurface.ErrModelNotFound)
	pullErr := &apisurface.ManagedRuntimePullError{
		Result: apisurface.ModelPullResult{ModelName: "pull-result", ManagedPullOutcome: "TIMED_OUT"},
		Cause:  errors.New("pull sentinel"),
	}
	stub := &catalogModelServiceStub{
		listResult: factoryapi.ListModelsResponse{Results: []factoryapi.ModelSummary{{Name: "list-result"}}},
		listErr:    listErr,
		getResult:  factoryapi.ModelDetail{Name: "detail-result"},
		getErr:     getErr,
		pullResult: apisurface.ModelPullResult{ModelName: "pull-result", ManagedPullOutcome: "TIMED_OUT"},
		pullErr:    pullErr,
	}
	host := &Host{modelService: stub}

	listed, gotListErr := host.ListModels(ctx)
	detail, gotGetErr := host.GetModel(ctx, "requested-model")
	pulled, gotPullErr := host.PullModel(ctx, "pull-model")

	if !reflect.DeepEqual(listed, stub.listResult) || gotListErr != listErr {
		t.Fatalf("ListModels = (%#v, %v), want exact result and sentinel error", listed, gotListErr)
	}
	if detail.Name != "detail-result" || gotGetErr != getErr {
		t.Fatalf("GetModel = (%#v, %v), want exact result and sentinel error", detail, gotGetErr)
	}
	if !reflect.DeepEqual(pulled, stub.pullResult) || gotPullErr != pullErr {
		t.Fatalf("PullModel = (%#v, %v), want exact result and sentinel error", pulled, gotPullErr)
	}
	if !errors.Is(gotGetErr, apisurface.ErrModelNotFound) || !apisurface.IsManagedRuntimePullError(gotPullErr) {
		t.Fatalf("typed errors = (%v, %v), want model-not-found and managed-runtime-pull errors", gotGetErr, gotPullErr)
	}
	assertCatalogCallsForwardedOnce(t, stub, ctx)
}

func TestHostInvokeModelForwardsContextRequestResultAndErrorUnchanged(t *testing.T) {
	t.Parallel()

	type contextKey string
	ctx := context.WithValue(context.Background(), contextKey("request"), "invoke-request")
	invokeErr := &apisurface.ManagedRuntimeInvocationError{
		Identity:       "invoke-model",
		ReadinessState: managedruntime.ReadinessStateMissing,
		Cause:          apisurface.ErrManagedRuntimeMissing,
	}
	request := factoryapi.ModelInvocationRequest{Operation: "TTS"}
	stub := &catalogModelServiceStub{
		invokeResult: apisurface.ModelInvocationResult{ModelName: "invoke-result", Operation: "TTS"},
		invokeErr:    invokeErr,
	}

	result, err := (&Host{modelService: stub}).InvokeModel(ctx, "invoke-model", request)
	if !reflect.DeepEqual(result, stub.invokeResult) || err != invokeErr {
		t.Fatalf("InvokeModel = (%#v, %v), want exact result and sentinel error", result, err)
	}
	if !apisurface.IsManagedRuntimeMissing(err) {
		t.Fatalf("InvokeModel error = %v, want typed unavailable-runtime error", err)
	}
	if len(stub.contexts) != 1 || stub.contexts[0] != ctx || len(stub.modelNames) != 1 || stub.modelNames[0] != "invoke-model" {
		t.Fatalf("forwarded context/model = (%#v, %#v), want original context and invoke-model", stub.contexts, stub.modelNames)
	}
	if len(stub.requests) != 1 || !reflect.DeepEqual(stub.requests[0], request) {
		t.Fatalf("invoke requests = %#v, want exact TTS request", stub.requests)
	}
	if !reflect.DeepEqual(stub.calls, []string{"invoke"}) {
		t.Fatalf("model calls = %#v, want invoke exactly once", stub.calls)
	}
}

func assertCatalogCallsForwardedOnce(t *testing.T, stub *catalogModelServiceStub, ctx context.Context) {
	t.Helper()
	if len(stub.contexts) != 3 || stub.contexts[0] != ctx || stub.contexts[1] != ctx || stub.contexts[2] != ctx {
		t.Fatalf("model contexts = %#v, want original context three times", stub.contexts)
	}
	if len(stub.modelNames) != 2 || stub.modelNames[0] != "requested-model" || stub.modelNames[1] != "pull-model" {
		t.Fatalf("model names = %#v, want requested-model then pull-model", stub.modelNames)
	}
	if !reflect.DeepEqual(stub.calls, []string{"list", "get", "pull"}) {
		t.Fatalf("model calls = %#v, want each operation exactly once", stub.calls)
	}
}

type catalogModelServiceStub struct {
	listResult   factoryapi.ListModelsResponse
	listErr      error
	getResult    factoryapi.ModelDetail
	getErr       error
	pullResult   apisurface.ModelPullResult
	pullErr      error
	invokeResult apisurface.ModelInvocationResult
	invokeErr    error
	contexts     []context.Context
	modelNames   []string
	requests     []factoryapi.ModelInvocationRequest
	calls        []string
}

func (s *catalogModelServiceStub) ListModels(ctx context.Context) (factoryapi.ListModelsResponse, error) {
	s.calls = append(s.calls, "list")
	s.contexts = append(s.contexts, ctx)
	return s.listResult, s.listErr
}

func (s *catalogModelServiceStub) GetModel(ctx context.Context, modelName string) (factoryapi.ModelDetail, error) {
	s.calls = append(s.calls, "get")
	s.contexts = append(s.contexts, ctx)
	s.modelNames = append(s.modelNames, modelName)
	return s.getResult, s.getErr
}

func (s *catalogModelServiceStub) PullModel(ctx context.Context, modelName string) (apisurface.ModelPullResult, error) {
	s.calls = append(s.calls, "pull")
	s.contexts = append(s.contexts, ctx)
	s.modelNames = append(s.modelNames, modelName)
	return s.pullResult, s.pullErr
}

func (s *catalogModelServiceStub) InvokeModel(ctx context.Context, modelName string, request factoryapi.ModelInvocationRequest) (apisurface.ModelInvocationResult, error) {
	s.calls = append(s.calls, "invoke")
	s.contexts = append(s.contexts, ctx)
	s.modelNames = append(s.modelNames, modelName)
	s.requests = append(s.requests, request)
	return s.invokeResult, s.invokeErr
}

type compatibilitySessionGateway struct {
	SessionGateway
	getFactorySession func(context.Context, string) (factorysessions.ProjectionContext, error)
}

func (f compatibilitySessionGateway) GetFactorySession(ctx context.Context, sessionID string) (factorysessions.ProjectionContext, error) {
	return f.getFactorySession(ctx, sessionID)
}

type compatibilityModelAPI struct {
	apisurface.ModelAPI
	getModel func(context.Context, string) (factoryapi.ModelDetail, error)
}

func (f compatibilityModelAPI) GetModel(ctx context.Context, modelName string) (factoryapi.ModelDetail, error) {
	return f.getModel(ctx, modelName)
}

type compatibilityFactorySave struct {
	save func(context.Context, string, factoryapi.FactorySaveMode, factoryapi.Factory) (factoryapi.Factory, error)
}

func (f compatibilityFactorySave) Save(ctx context.Context, sessionID string, mode factoryapi.FactorySaveMode, request factoryapi.Factory) (factoryapi.Factory, error) {
	return f.save(ctx, sessionID, mode, request)
}

type compatibilityInvocationAPI struct {
	invoke func(context.Context, string, sessioninvocation.InvocationRequest) (apisurface.FactoryInvocationResult, error)
}

func (f compatibilityInvocationAPI) InvokeFactorySession(ctx context.Context, sessionID string, request sessioninvocation.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
	return f.invoke(ctx, sessionID, request)
}

type compatibilityDurableExecutionAPI struct {
	apisurface.DurableSessionAPI
	startAsync func(context.Context, factoryapi.FactorySessionExecutionRequest) (factoryapi.FactorySessionExecutionResponse, error)
	pause      func(context.Context, string, factoryapi.FactorySessionLifecycleControlRequest) (factoryapi.FactorySessionLifecycleControlResponse, error)
}

func (f compatibilityDurableExecutionAPI) PauseDurableFactorySession(ctx context.Context, sessionID string, request factoryapi.FactorySessionLifecycleControlRequest) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	return f.pause(ctx, sessionID, request)
}

func (f compatibilityDurableExecutionAPI) StartDurableFactorySessionAsync(ctx context.Context, request factoryapi.FactorySessionExecutionRequest) (factoryapi.FactorySessionExecutionResponse, error) {
	return f.startAsync(ctx, request)
}

func TestHostCompatibilityFacadeForwardsToCanonicalCollaborators(t *testing.T) {
	t.Parallel()

	ctx := context.WithValue(context.Background(), struct{}{}, "compatibility-context")
	sentinel := errors.New("typed collaborator outcome")
	requestFactory := factoryapi.Factory{Name: "submitted"}
	calls := map[string]int{}

	host := &Host{}
	host.sessionGateway = compatibilitySessionGateway{getFactorySession: func(gotCtx context.Context, sessionID string) (factorysessions.ProjectionContext, error) {
		calls["session"]++
		if gotCtx != ctx || sessionID != "missing-session" {
			t.Fatalf("session args = (%v, %q)", gotCtx, sessionID)
		}
		return factorysessions.ProjectionContext{}, sentinel
	}}
	host.modelService = compatibilityModelAPI{getModel: func(gotCtx context.Context, modelName string) (factoryapi.ModelDetail, error) {
		calls["model"]++
		if gotCtx != ctx || modelName != "missing-model" {
			t.Fatalf("model args = (%v, %q)", gotCtx, modelName)
		}
		return factoryapi.ModelDetail{}, sentinel
	}}
	host.factorySave = compatibilityFactorySave{save: func(gotCtx context.Context, sessionID string, mode factoryapi.FactorySaveMode, request factoryapi.Factory) (factoryapi.Factory, error) {
		calls["factory-definition"]++
		if gotCtx != ctx || sessionID != "session-1" || mode != factoryapi.FactorySaveModeReplaceCurrent || request.Name != requestFactory.Name {
			t.Fatalf("factory-definition args = (%v, %q, %q, %#v)", gotCtx, sessionID, mode, request)
		}
		return factoryapi.Factory{}, sentinel
	}}
	host.sessionInvoker = compatibilityInvocationAPI{invoke: func(gotCtx context.Context, sessionID string, request sessioninvocation.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
		calls["invocation"]++
		if gotCtx != ctx || sessionID != "session-1" {
			t.Fatalf("invocation args = (%v, %q, %#v)", gotCtx, sessionID, request)
		}
		return apisurface.FactoryInvocationResult{}, sentinel
	}}
	host.durableExecutionAPI = compatibilityDurableExecutionAPI{startAsync: func(gotCtx context.Context, request factoryapi.FactorySessionExecutionRequest) (factoryapi.FactorySessionExecutionResponse, error) {
		calls["durable-execution"]++
		if gotCtx != ctx {
			t.Fatalf("durable context was not preserved")
		}
		return factoryapi.FactorySessionExecutionResponse{}, sentinel
	}}

	_, sessionErr := host.GetFactorySession(ctx, "missing-session")
	_, modelErr := host.GetModel(ctx, "missing-model")
	_, definitionErr := host.SaveFactoryForSession(ctx, "session-1", factoryapi.FactorySaveModeReplaceCurrent, requestFactory)
	_, invocationErr := host.InvokeFactorySession(ctx, "session-1", factoryapi.InvocationRequest{})
	_, durableErr := host.StartDurableFactorySessionAsync(ctx, factoryapi.FactorySessionExecutionRequest{})
	for role, err := range map[string]error{"session": sessionErr, "model": modelErr, "factory-definition": definitionErr, "invocation": invocationErr, "durable-execution": durableErr} {
		if !errors.Is(err, sentinel) || calls[role] != 1 {
			t.Errorf("%s result = (%v, %d calls), want unchanged error and one call", role, err, calls[role])
		}
	}
}

func TestHostCompatibilityFacadePreservesTypedOutcomes(t *testing.T) {
	t.Parallel()

	ctx := context.WithValue(context.Background(), struct{ name string }{"typed"}, "outcomes")
	notFound := errors.New("missing host session")
	validation := &apisurface.RequestValidationError{Message: "invalid factory definition"}
	wantInvocation := apisurface.FactoryInvocationResult{
		RequestID: "request-typed", TraceID: "trace-typed",
		Status: "COMPLETED",
	}
	wantLifecycle := factoryapi.FactorySessionLifecycleControlResponse{
		SessionId: "durable-1", Operation: factoryapi.FactorySessionLifecycleControlKindPause,
		Status: factoryapi.FactorySessionDurableLifecycleStatusPaused,
	}
	calls := map[string]int{}

	host := &Host{
		sessionGateway: compatibilitySessionGateway{getFactorySession: func(gotCtx context.Context, sessionID string) (factorysessions.ProjectionContext, error) {
			calls["not-found"]++
			requireHostCompatibility(t, gotCtx == ctx && sessionID == "missing", "session args = (%v, %q)", gotCtx, sessionID)
			return factorysessions.ProjectionContext{}, errors.Join(apisurface.ErrFactorySessionNotFound, notFound)
		}},
		factorySave: compatibilityFactorySave{save: func(gotCtx context.Context, sessionID string, mode factoryapi.FactorySaveMode, request factoryapi.Factory) (factoryapi.Factory, error) {
			calls["validation"]++
			requireHostCompatibility(t, gotCtx == ctx && sessionID == "session-1" && mode == factoryapi.FactorySaveModeReplaceCurrent, "factory-definition args = (%v, %q, %q)", gotCtx, sessionID, mode)
			return factoryapi.Factory{}, validation
		}},
		sessionInvoker: compatibilityInvocationAPI{invoke: func(gotCtx context.Context, sessionID string, request sessioninvocation.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
			calls["invocation"]++
			requireHostCompatibility(t, gotCtx == ctx && sessionID == "session-1", "invocation args = (%v, %q)", gotCtx, sessionID)
			return wantInvocation, nil
		}},
	}
	host.durableExecutionAPI = compatibilityDurableExecutionAPI{pause: func(gotCtx context.Context, sessionID string, request factoryapi.FactorySessionLifecycleControlRequest) (factoryapi.FactorySessionLifecycleControlResponse, error) {
		calls["lifecycle"]++
		requireHostCompatibility(t, gotCtx == ctx && sessionID == "durable-1", "lifecycle args = (%v, %q)", gotCtx, sessionID)
		return wantLifecycle, nil
	}}

	_, sessionErr := host.GetFactorySession(ctx, "missing")
	_, validationErr := host.SaveFactoryForSession(ctx, "session-1", factoryapi.FactorySaveModeReplaceCurrent, factoryapi.Factory{})
	invocation, invocationErr := host.InvokeFactorySession(ctx, "session-1", factoryapi.InvocationRequest{})
	lifecycle, lifecycleErr := host.PauseDurableFactorySession(ctx, "durable-1", factoryapi.FactorySessionLifecycleControlRequest{})
	requireHostCompatibility(t, errors.Is(sessionErr, apisurface.ErrFactorySessionNotFound) && errors.Is(sessionErr, notFound), "session error = %v, want typed not-found", sessionErr)
	var gotValidation *apisurface.RequestValidationError
	requireHostCompatibility(t, errors.As(validationErr, &gotValidation) && gotValidation == validation, "validation error = %#v, want unchanged %#v", validationErr, validation)
	requireHostCompatibility(t, invocationErr == nil && reflect.DeepEqual(invocation, wantInvocation), "invocation = (%#v, %v), want %#v", invocation, invocationErr, wantInvocation)
	requireHostCompatibility(t, lifecycleErr == nil && reflect.DeepEqual(lifecycle, wantLifecycle), "lifecycle = (%#v, %v), want %#v", lifecycle, lifecycleErr, wantLifecycle)
	for outcome, count := range calls {
		requireHostCompatibility(t, count == 1, "%s calls = %d, want 1", outcome, count)
	}
}

func requireHostCompatibility(t *testing.T, condition bool, format string, args ...any) {
	t.Helper()
	if !condition {
		t.Fatalf(format, args...)
	}
}

var _ apisurface.APISurface = (*Host)(nil)
var _ apisurface.SessionAPISurface = (*Host)(nil)
