package service

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
	modelhost "github.com/portpowered/infinite-you/pkg/services/models/internal/legacyhost"
	localmodels "github.com/portpowered/infinite-you/pkg/services/models/internal/local"
	scopedassets "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets"
	modelcatalog "github.com/portpowered/infinite-you/pkg/services/models/internal/services/catalog"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/catalog/wire"
	inference "github.com/portpowered/infinite-you/pkg/services/models/internal/services/inference"
	inferencewire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/inference/wire"
	runtimehost "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host"
	runtimehostwire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host/wire"
	runtimescopes "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes"
	runtimescopeswire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes/wire"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"net/http"
)

type constructionInvocationRuntime struct{}

func (constructionInvocationRuntime) Invoke(
	context.Context,
	inference.InvocationRuntimeRequest,
) (inference.InvocationRuntimeResult, error) {
	return inference.InvocationRuntimeResult{}, nil
}

func TestNewRootClassifiesMissingConstructionDependencies(t *testing.T) {
	t.Parallel()

	valid := newRootConstructionArgs(t)
	root, err := valid.build()
	if err != nil || root == nil {
		t.Fatalf("NewRoot valid dependencies = (%v, %v), want constructed root", root, err)
	}

	tests := []struct {
		name    string
		mutate  func(*rootConstructionArgs)
		message string
	}{
		{name: "process launcher", mutate: func(args *rootConstructionArgs) { args.processLauncher = nil }, message: "model host process launcher"},
		{name: "host HTTP client", mutate: func(args *rootConstructionArgs) { args.hostHTTP = nil }, message: "model host HTTP client"},
		{name: "host clock", mutate: func(args *rootConstructionArgs) { args.hostClock = nil }, message: "model host clock"},
		{name: "local runtime", mutate: func(args *rootConstructionArgs) { args.localRuntime = nil }, message: "local model runtime"},
		{name: "resource limiter", mutate: func(args *rootConstructionArgs) { args.resources = nil }, message: "local model resource limiter"},
		{name: "runtime scopes", mutate: func(args *rootConstructionArgs) { args.runtimeScopes = nil }, message: "Models Runtime Scopes service"},
		{name: "catalog", mutate: func(args *rootConstructionArgs) { args.catalog = nil }, message: "Models Catalog service"},
		{name: "assets", mutate: func(args *rootConstructionArgs) { args.assets = nil }, message: "Models Assets service"},
		{name: "runtime host", mutate: func(args *rootConstructionArgs) { args.runtimeHost = nil }, message: "Models Runtime Host service"},
		{name: "inference", mutate: func(args *rootConstructionArgs) { args.inference = nil }, message: "Models Inference service"},
		{name: "logger", mutate: func(args *rootConstructionArgs) { args.logger = nil }, message: "Models logger"},
		{name: "revision resolver", mutate: func(args *rootConstructionArgs) { args.revisionResolver = nil }, message: "Models revision resolver"},
		{name: "process clock", mutate: func(args *rootConstructionArgs) { args.now = nil }, message: "Models process clock"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			args := newRootConstructionArgs(t)
			test.mutate(&args)
			root, err := args.build()
			if root != nil || !errors.Is(err, ErrInvalidDependencies) || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("NewRoot missing %s = (%v, %v), want classified dependency error", test.name, root, err)
			}
		})
	}
}

type rootConstructionArgs struct {
	processLauncher  modelhost.ProcessLauncher
	hostHTTP         modelhost.HTTPDoer
	hostClock        modelhost.Clock
	localRuntime     localmodels.Runtime
	resources        *localmodels.ResourceLimiter
	runtimeScopes    runtimescopes.Service
	catalog          modelcatalog.Service
	assets           scopedassets.Service
	runtimeHost      runtimehost.Service
	inference        inference.Service
	logger           *zap.Logger
	now              func() time.Time
	revisionResolver func(context.Context, string) (string, error)
}

func (args rootConstructionArgs) build() (*Root, error) {
	execution, _ := NewScopedLocalExecution(args.runtimeScopes, args.assets, args.runtimeHost, args.localRuntime, args.resources, modelseffects.LocalRuntimeHooks{}, args.now)
	return NewRoot(
		args.processLauncher,
		args.hostHTTP,
		args.hostClock,
		args.localRuntime,
		args.resources, execution,
		args.runtimeScopes,
		args.catalog,
		args.assets,
		args.runtimeHost,
		args.inference,
		args.logger, args.now, nil, nil, nil, nil, modelseffects.LocalRuntimeHooks{},
		args.revisionResolver, nil, models.AssetHostPlatform{},
	)
}

func newRootConstructionArgs(t *testing.T) rootConstructionArgs {
	t.Helper()

	scopes, err := runtimescopeswire.NewService(func() string { return "root-construction-test" })
	if err != nil {
		t.Fatalf("construct runtime scopes: %v", err)
	}
	catalog, err := catalogwire.NewService(scopes, func(ctx context.Context, _ models.RuntimeScopeRef, _ models.RuntimeScopeConfig, detail models.Detail) (models.Runtime, error) {
		if err := ctx.Err(); err != nil {
			return models.Runtime{}, err
		}
		return detail.ManagedRuntime.Clone(), nil
	})
	if err != nil {
		t.Fatalf("construct catalog: %v", err)
	}
	events := []string{}
	return rootConstructionArgs{
		processLauncher: rootConstructionProcessLauncher{},
		hostHTTP:        http.DefaultClient,
		hostClock:       rootConstructionClock{},
		localRuntime:    &leaseTestRuntime{},
		resources:       mustResourceLimiter(t),
		runtimeScopes:   scopes,
		catalog:         catalog,
		assets:          inferenceRecordingAssetsService{},
		runtimeHost:     &joinedHostService{events: &events},
		inference:       &joinedInferenceService{events: &events},
		logger:          zap.NewNop(), now: time.Now,
		revisionResolver: func(context.Context, string) (string, error) { return "", models.ErrModelRevisionUnresolved },
	}
}

func TestNewRootResolvesScopedReferenceWithSelectedEffect(t *testing.T) {
	t.Parallel()
	args := newRootConstructionArgs(t)
	const revision = "0123456789abcdef0123456789abcdef01234567"
	calls := 0
	args.revisionResolver = func(ctx context.Context, source string) (string, error) {
		calls++
		if ctx != t.Context() || source != "hf://selected/repository@main" {
			t.Fatalf("resolver input = %v, %q, want selected context and source", ctx, source)
		}
		return revision, nil
	}
	root, err := args.build()
	if err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatal("construction invoked revision effect")
	}
	opened, err := root.OpenRuntimeScope(t.Context(), models.OpenRuntimeScopeRequest{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := root.ResolveModelReference(t.Context(), models.ResolveModelReferenceRequest{
		Scope: opened.Scope, Reference: models.ModelReference{NameOrURI: "hf://selected/repository@main"},
	})
	if err != nil || calls != 1 || result.Resolved.Provenance.ImmutableRevision != revision ||
		result.Resolved.Definition.Source != "hf://selected/repository@"+revision {
		t.Fatalf("resolution = %#v, %v, calls=%d, want selected immutable revision once", result, err, calls)
	}
}

func TestRootDelegatesInferenceThroughInjectedOwner(t *testing.T) {
	t.Parallel()

	privateInference := &delegatingInferenceService{}
	root := &Root{inference: privateInference}
	request := models.InvokeModelRequest{ModelName: "scoped-model", Operation: "generate"}

	_, err := root.InvokeModelWithLease(context.Background(), request)
	if !errors.Is(err, models.ErrUnsupportedOperation) {
		t.Fatalf("InvokeModelWithLease error = %v, want ErrUnsupportedOperation", err)
	}
	if !reflect.DeepEqual(privateInference.invokeRequest, request) {
		t.Fatalf("InvokeModelWithLease request = %#v, want %#v", privateInference.invokeRequest, request)
	}

	cancelRequest := models.CancelInvocationRequest{Invocation: mustInferenceInvocationRef(t, "inv-1")}
	_, err = root.CancelInvocation(context.Background(), cancelRequest)
	if !errors.Is(err, models.ErrUnsupportedOperation) {
		t.Fatalf("CancelInvocation error = %v, want ErrUnsupportedOperation", err)
	}
	if privateInference.cancelRequest != cancelRequest {
		t.Fatalf("CancelInvocation request = %#v, want %#v", privateInference.cancelRequest, cancelRequest)
	}
}

func TestBoundServiceContractOnlyOperationsFailExplicitly(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svc := &Service{}
	_, err := svc.ListCatalog(ctx, models.ListModelsRequest{})
	assertContractOnlyUnsupported(t, "ListCatalog", err)
	_, err = svc.GetCatalogModel(ctx, models.GetModelRequest{})
	assertContractOnlyUnsupported(t, "GetCatalogModel", err)
	_, err = svc.GetModelReadiness(ctx, models.GetModelReadinessRequest{})
	assertContractOnlyUnsupported(t, "GetModelReadiness", err)
	_, err = svc.PrepareModelAssets(ctx, models.PrepareModelAssetsRequest{})
	assertContractOnlyUnsupported(t, "PrepareModelAssets", err)
	_, err = svc.InspectModelAssets(ctx, models.InspectModelAssetsRequest{})
	assertContractOnlyUnsupported(t, "InspectModelAssets", err)
	_, err = svc.RemoveModelAssets(ctx, models.RemoveModelAssetsRequest{})
	assertContractOnlyUnsupported(t, "RemoveModelAssets", err)
	_, err = svc.EnsureModelHost(ctx, models.EnsureModelHostRequest{})
	assertContractOnlyUnsupported(t, "EnsureModelHost", err)
	_, err = svc.InspectModelHost(ctx, models.InspectModelHostRequest{})
	assertContractOnlyUnsupported(t, "InspectModelHost", err)
	_, err = svc.StopModelHost(ctx, models.StopModelHostRequest{})
	assertContractOnlyUnsupported(t, "StopModelHost", err)
	_, err = svc.AcquireModelLease(ctx, models.AcquireModelLeaseRequest{})
	assertContractOnlyUnsupported(t, "AcquireModelLease", err)
	_, err = svc.GetModelLease(ctx, models.GetModelLeaseRequest{})
	assertContractOnlyUnsupported(t, "GetModelLease", err)
	_, err = svc.ReleaseModelLease(ctx, models.ReleaseModelLeaseRequest{})
	assertContractOnlyUnsupported(t, "ReleaseModelLease", err)
	_, err = svc.InvokeModelWithLease(ctx, models.InvokeModelRequest{})
	assertContractOnlyUnsupported(t, "InvokeModelWithLease", err)
	_, err = svc.CancelInvocation(ctx, models.CancelInvocationRequest{})
	assertContractOnlyUnsupported(t, "CancelInvocation", err)
}

func TestRootCloseShutsDownRuntimeHost(t *testing.T) {
	t.Parallel()

	host := &shutdownTrackingRuntimeHost{}
	root := &Root{runtimeHost: host, resources: mustResourceLimiter(t), localExecution: inertScopedLocalExecution{}}
	if err := root.Close(context.Background()); err != nil {
		t.Fatalf("Root.Close() error = %v, want nil", err)
	}
	if host.shutdownCalls != 1 {
		t.Fatalf("runtime host shutdown calls = %d, want 1", host.shutdownCalls)
	}
}

type shutdownTrackingRuntimeHost struct {
	runtimehost.Service
	shutdownCalls int
}

func (host *shutdownTrackingRuntimeHost) Shutdown(context.Context) error {
	host.shutdownCalls++
	return nil
}

func TestScopedRuntimeResolutionDoesNotReplaceInjectedInferenceOwner(t *testing.T) {
	t.Parallel()

	scopes, err := runtimescopeswire.NewService(func() string { return "inference-root-test" })
	if err != nil {
		t.Fatalf("construct Runtime Scopes: %v", err)
	}
	privateRef, err := scopes.Open(models.RuntimeBinding{
		RuntimeConfig: func() *models.RuntimeConfig {
			return &models.RuntimeConfig{}
		},
	})
	if err != nil {
		t.Fatalf("open runtime scope: %v", err)
	}
	scope, err := (models.RuntimeScopeRef{}).Parse(string(privateRef))
	if err != nil {
		t.Fatalf("parse runtime scope: %v", err)
	}

	privateInference := &delegatingInferenceService{}
	root := &Root{
		runtimeScopes: scopes,
		assets:        inferenceRecordingAssetsService{},
		inference:     privateInference,
	}

	_, err = root.PullModelForScope(context.Background(), models.PullModelRequest{
		Scope: scope,
		Name:  "voice",
	})
	if err == nil {
		t.Fatal("PullModelForScope error = nil, want scoped runtime construction failure")
	}
	if root.inference != privateInference {
		t.Fatal("scoped runtime resolution replaced process-scoped inference owner")
	}
}

func TestInferenceWireConstructionIsInert(t *testing.T) {
	t.Parallel()

	scopes, err := runtimescopeswire.NewService(func() string { return "inference-inert-test" })
	if err != nil {
		t.Fatalf("construct Runtime Scopes: %v", err)
	}
	assets := inferenceRecordingAssetsService{}
	launcher := &inferenceRecordingProcessLauncher{}
	clock := &inferenceTestClock{}
	runtimeHost, err := newRuntimeHostFixture(scopes, assets, launcher, http.DefaultClient, clock, nil, nil, models.AssetHostPlatform{}, nil, nil, nil, nil, 0, 0)
	if err != nil {
		t.Fatalf("construct Runtime Host: %v", err)
	}
	catalog, err := catalogwire.NewService(scopes, func(ctx context.Context, _ models.RuntimeScopeRef, _ models.RuntimeScopeConfig, detail models.Detail) (models.Runtime, error) {
		if err := ctx.Err(); err != nil {
			return models.Runtime{}, err
		}
		return detail.ManagedRuntime.Clone(), nil
	})
	if err != nil {
		t.Fatalf("construct Catalog: %v", err)
	}
	registrar, err := inferencewire.NewInvocationArtifactRegistrar(inference.InertArtifactFileSystem{})
	if err != nil {
		t.Fatalf("construct artifact registrar: %v", err)
	}
	inference, err := inferencewire.NewService(
		scopes, assets, catalog, runtimeHost, constructionInvocationRuntime{},
		registrar, clock.Now, inferencewire.NewExecutionDeadline(),
	)
	if err != nil {
		t.Fatalf("construct Inference: %v", err)
	}
	if inference == nil {
		t.Fatal("construct Inference returned nil service")
	}
	if launcher.starts != 0 || clock.timerCalls != 0 {
		t.Fatalf(
			"inert construction side effects = (starts %d, timers %d), want 0/0",
			launcher.starts,
			clock.timerCalls,
		)
	}
}

func TestNewRootAcceptsComposedDependenciesAndSelectedLogger(t *testing.T) {
	t.Parallel()

	scopes, err := runtimescopeswire.NewService(func() string { return "root-construction-test" })
	if err != nil {
		t.Fatalf("construct Runtime Scopes: %v", err)
	}
	assets := inferenceRecordingAssetsService{}
	catalog, err := catalogwire.NewService(scopes, func(ctx context.Context, _ models.RuntimeScopeRef, _ models.RuntimeScopeConfig, detail models.Detail) (models.Runtime, error) {
		if err := ctx.Err(); err != nil {
			return models.Runtime{}, err
		}
		return detail.ManagedRuntime.Clone(), nil
	})
	if err != nil {
		t.Fatalf("construct Catalog: %v", err)
	}
	launcher := &inferenceRecordingProcessLauncher{}
	clock := &inferenceTestClock{}
	runtimeHost, err := newRuntimeHostFixture(scopes, assets, launcher, http.DefaultClient, clock, nil, nil, models.AssetHostPlatform{}, nil, nil, nil, nil, 0, 0)
	if err != nil {
		t.Fatalf("construct Runtime Host: %v", err)
	}
	registrar, err := inferencewire.NewInvocationArtifactRegistrar(inference.InertArtifactFileSystem{})
	if err != nil {
		t.Fatalf("construct artifact registrar: %v", err)
	}
	inferenceService, err := inferencewire.NewService(
		scopes, assets, catalog, runtimeHost, constructionInvocationRuntime{},
		registrar, clock.Now, inferencewire.NewExecutionDeadline(),
	)
	if err != nil {
		t.Fatalf("construct Inference: %v", err)
	}
	root, err := NewRoot(
		rootConstructionProcessLauncher{}, http.DefaultClient, rootConstructionClock{}, &leaseTestRuntime{},
		mustResourceLimiter(t), inertScopedLocalExecution{}, scopes, catalog, assets, runtimeHost, inferenceService,
		zap.NewNop(), time.Now, nil, nil, nil, nil, modelseffects.LocalRuntimeHooks{},
		func(context.Context, string) (string, error) { return "", models.ErrModelRevisionUnresolved }, nil, models.AssetHostPlatform{},
	)
	if err != nil {
		t.Fatalf("NewRoot: %v", err)
	}
	if root == nil || root.resolveHuggingFaceRevision == nil || root.logger == nil {
		t.Fatal("NewRoot did not retain a usable root with default logger/revision resolver")
	}
}

type rootConstructionCommandRunner struct{}

func (rootConstructionCommandRunner) Run(context.Context, platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	return platformprocess.CommandResult{}, nil
}

type rootConstructionProcessLauncher struct{}

func (rootConstructionProcessLauncher) Start(context.Context, modelhost.ProcessStartSpec) (modelhost.ManagedProcess, error) {
	return nil, nil
}

type rootConstructionClock struct{}

func (rootConstructionClock) Now() time.Time { return time.Unix(0, 0) }
func (rootConstructionClock) NewTimer(time.Duration) modelhost.Timer {
	return rootConstructionTimer{}
}

type rootConstructionTimer struct{}

func (rootConstructionTimer) C() <-chan time.Time { return nil }
func (rootConstructionTimer) Stop() bool          { return true }

type rootConstructionTempFile struct{}

func (rootConstructionTempFile) Close() error { return nil }
func (rootConstructionTempFile) Name() string { return "root-construction-temp" }

type delegatingInferenceService struct {
	invokeCalls   int
	invokeRequest models.InvokeModelRequest
	cancelRequest models.CancelInvocationRequest
}

func (service *delegatingInferenceService) InvokeModelWithLease(
	_ context.Context,
	request models.InvokeModelRequest,
) (models.InvokeModelResult, error) {
	service.invokeCalls++
	service.invokeRequest = request
	return models.InvokeModelResult{}, models.ErrUnsupportedOperation
}

func (service *delegatingInferenceService) CancelInvocation(
	_ context.Context,
	request models.CancelInvocationRequest,
) (models.CancelInvocationResult, error) {
	service.cancelRequest = request
	return models.CancelInvocationResult{}, models.ErrUnsupportedOperation
}

type inferenceRecordingProcessLauncher struct {
	starts int
}

func (launcher *inferenceRecordingProcessLauncher) Start(
	context.Context,
	modelseffects.HostProcessStartSpec,
) (modelseffects.HostManagedProcess, error) {
	launcher.starts++
	panic("process launcher called during inert inference composition")
}

type inferenceTestClock struct {
	timerCalls int
}

func (clock *inferenceTestClock) Now() time.Time {
	return time.Unix(0, 0)
}

func (clock *inferenceTestClock) NewTimer(time.Duration) modelseffects.HostTimer {
	clock.timerCalls++
	panic("host timer created during inert inference composition")
}

type inferenceRecordingAssetsService struct{}

var _ scopedassets.Service = inferenceRecordingAssetsService{}

func (inferenceRecordingAssetsService) PrepareModelAssets(
	context.Context,
	models.PrepareModelAssetsRequest,
) (models.PrepareModelAssetsResult, error) {
	return models.PrepareModelAssetsResult{}, models.ErrUnsupportedOperation
}

func (inferenceRecordingAssetsService) PreflightModelAssets(
	context.Context,
	models.PrepareModelAssetsRequest,
) (models.PreflightModelAssetsResult, error) {
	return models.PreflightModelAssetsResult{}, models.ErrUnsupportedOperation
}

func (inferenceRecordingAssetsService) InspectModelAssets(
	context.Context,
	models.InspectModelAssetsRequest,
) (models.InspectModelAssetsResult, error) {
	return models.InspectModelAssetsResult{}, models.ErrUnsupportedOperation
}

func (inferenceRecordingAssetsService) RemoveModelAssets(
	context.Context,
	models.RemoveModelAssetsRequest,
) (models.RemoveModelAssetsResult, error) {
	return models.RemoveModelAssetsResult{}, models.ErrUnsupportedOperation
}

func (inferenceRecordingAssetsService) ResolveRuntimeCache(
	context.Context,
	models.InspectModelAssetsRequest,
) (scopedassets.RuntimeCacheLayout, error) {
	return scopedassets.RuntimeCacheLayout{}, models.ErrUnsupportedOperation
}

func (inferenceRecordingAssetsService) InspectRuntimeCache(
	context.Context,
	models.InspectModelAssetsRequest,
) (scopedassets.RuntimeCacheInspection, error) {
	return scopedassets.RuntimeCacheInspection{}, models.ErrUnsupportedOperation
}

func TestRootInvokeModelJoinsStagesAndDoesNotDoubleRelease(t *testing.T) {
	t.Parallel()

	var events []string
	inference := &joinedInferenceService{
		events: &events,
		result: joinedCompletedResult(t),
	}
	root, scope, host := newJoinedInvocationRoot(t, &events, inference)
	core, observed := observer.New(zap.InfoLevel)
	root.logger = zap.New(core)
	root.now = func() time.Time { return time.Unix(123, 0) }

	result, err := root.InvokeModel(context.Background(), joinedInvocationRequest(scope))
	if err != nil {
		t.Fatalf("InvokeModel: %v", err)
	}
	assertJoinedInvocationResult(t, events, result, host, inference)
	assertJoinedInvocationEvidence(t, observed)
}

func assertJoinedInvocationResult(
	t *testing.T,
	events []string,
	result models.InvokeModelResult,
	host *joinedHostService,
	inference *joinedInferenceService,
) {
	t.Helper()
	if got, want := events, []string{"resolve", "preflight", "assets", "host", "lease", "invoke", "primitive-release"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("stage events = %#v, want %#v", got, want)
	}
	if result.Status != models.ModelInvocationStatusCompleted ||
		result.ModelName != "joined-model" || result.Operation != models.OperationOMNI {
		t.Fatalf("result identity = %#v, want completed joined result", result)
	}
	if len(result.Outputs) != 1 || result.Outputs[0].Name != "text" || result.Outputs[0].Content != "done" {
		t.Fatalf("result outputs = %#v, want one detached named output", result.Outputs)
	}
	if host.releaseCalls != 0 {
		t.Fatalf("root release calls = %d, want 0 after primitive release", host.releaseCalls)
	}
	if inference.invokeRequest.Inputs[0].Content == "" {
		t.Fatal("prepared invocation lost its ordered input")
	}
	if inference.invokeRequest.ModelName != "joined-model" || inference.invokeRequest.Operation != models.OperationOMNI {
		t.Fatalf("prepared request = %#v, want resolved identity", inference.invokeRequest)
	}
}

func assertJoinedInvocationEvidence(t *testing.T, observed *observer.ObservedLogs) {
	t.Helper()
	stageEntries := observed.FilterMessage("models invocation stage").All()
	wantStages := []string{
		joinedLifecycleStageArtifactProvision,
		joinedLifecycleStageBackendStart,
		joinedLifecycleStageHealth,
		joinedLifecycleStageInvoke,
		joinedLifecycleStageOutput,
		joinedLifecycleStageRelease,
	}
	if len(stageEntries) != len(wantStages) {
		t.Fatalf("lifecycle stage entries = %d, want %d", len(stageEntries), len(wantStages))
	}
	correlation := ""
	for index, entry := range stageEntries {
		assertJoinedStageFields(t, index, entry.ContextMap(), wantStages[index], &correlation)
	}
	terminalEntries := observed.FilterMessage("models invocation completed").All()
	if len(terminalEntries) != 1 {
		t.Fatalf("terminal entries = %d, want exactly one", len(terminalEntries))
	}
	assertJoinedTerminalFields(t, terminalEntries[0].ContextMap(), correlation)
}

func assertJoinedStageFields(
	t *testing.T,
	index int,
	fields map[string]interface{},
	wantStage string,
	correlation *string,
) {
	t.Helper()
	fieldString := func(name string) string {
		value, _ := fields[name].(string)
		return value
	}
	if got := fieldString("stage"); got != wantStage {
		t.Fatalf("lifecycle stage[%d] = %q, want %q", index, got, wantStage)
	}
	if fieldString("outcome") != joinedLifecycleOutcomeCompleted ||
		fieldString("model_name") != "joined-model" ||
		fieldString("backend") != "fixture-backend" ||
		fields["duration_millis"] == nil {
		t.Fatalf("lifecycle fields[%d] = %#v, want bounded completed identity", index, fields)
	}
	if index == 0 {
		*correlation = fieldString("correlation_id")
	}
	if fieldString("correlation_id") != *correlation {
		t.Fatalf("lifecycle correlation[%d] = %q, want %q", index, fieldString("correlation_id"), *correlation)
	}
	if strings.Contains(fieldString("operation"), "hello") || strings.Contains(fieldString("model_name"), "joined-holder") {
		t.Fatalf("lifecycle fields[%d] leaked invocation input: %#v", index, fields)
	}
}

func assertJoinedTerminalFields(t *testing.T, fields map[string]interface{}, correlation string) {
	t.Helper()
	fieldString := func(name string) string {
		value, _ := fields[name].(string)
		return value
	}
	if fieldString("stage") != joinedLifecycleStageTerminal ||
		fieldString("outcome") != joinedLifecycleOutcomeCompleted ||
		fieldString("correlation_id") != correlation ||
		fieldString("runtime_stage") != string(modelseffects.RuntimeStageInvoke) {
		t.Fatalf("terminal fields = %#v, want one correlated completed terminal", fields)
	}
}

func TestRootInvokeModelScopesTerminalEvidencePerInvocation(t *testing.T) {
	var events []string
	inference := &joinedInferenceService{
		events: &events,
		result: joinedCompletedResult(t),
	}
	root, scope, _ := newJoinedInvocationRoot(t, &events, inference)
	sink := &rootRuntimeEvidenceRecords{}
	root.runtimeEvidence = modelseffects.NewOrderedRuntimeEvidenceRecorder(sink)

	for attempt := 0; attempt < 2; attempt++ {
		if _, err := root.InvokeModel(context.Background(), joinedInvocationRequest(scope)); err != nil {
			t.Fatalf("InvokeModel attempt %d: %v", attempt+1, err)
		}
	}

	terminals := make([]modelseffects.RuntimeEvidenceRecord, 0, 2)
	for _, record := range sink.snapshot() {
		if record.Kind == modelseffects.RuntimeEvidenceKindTerminal {
			terminals = append(terminals, record)
		}
	}
	if len(terminals) != 2 {
		t.Fatalf("terminal evidence records = %d, want one per invocation: %#v", len(terminals), sink.snapshot())
	}
	for index, record := range terminals {
		if record.Stage != modelseffects.RuntimeStageInvoke || record.Outcome != modelseffects.RuntimeEvidenceOutcomeCompleted {
			t.Fatalf("terminal evidence[%d] = %#v, want completed invoke terminal", index, record)
		}
	}
}

func TestRootInvokeModelReleasesAndClearsPartialOutputOnInvocationFailure(t *testing.T) {
	var events []string
	inference := &joinedInferenceService{
		events: &events,
		result: models.InvokeModelResult{
			Status:           models.ModelInvocationStatusFailed,
			LeaseDisposition: models.InvocationLeaseRetained,
			Content:          []models.InferenceContent{{Name: "text", Content: "partial"}},
			Outputs:          []models.InferenceOutput{{Name: "text", Content: "partial"}},
		},
		err: models.ErrInferenceTimeout,
	}
	root, scope, host := newJoinedInvocationRoot(t, &events, inference)

	result, err := root.InvokeModel(context.Background(), joinedInvocationRequest(scope))
	if !errors.Is(err, models.ErrInferenceTimeout) {
		t.Fatalf("InvokeModel error = %v, want inference timeout", err)
	}
	if host.releaseCalls != 1 {
		t.Fatalf("lease release calls = %d, want exactly 1", host.releaseCalls)
	}
	if len(result.Content) != 0 || len(result.Artifacts) != 0 || len(result.Outputs) != 0 {
		t.Fatalf("failed result retained partial output: %#v", result)
	}
	if result.LeaseDisposition != models.InvocationLeaseReleased {
		t.Fatalf("failed lease disposition = %q, want RELEASED", result.LeaseDisposition)
	}
	if got, want := events, []string{"resolve", "preflight", "assets", "host", "lease", "invoke", "release"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("failure stage events = %#v, want %#v", got, want)
	}
}

func TestRootInvokeModelDoesNotRetryPrivateLeaseReleaseAttempt(t *testing.T) {
	var events []string
	inference := &joinedInferenceService{
		events:                    &events,
		markLeaseReleaseAttempted: true,
		result: models.InvokeModelResult{
			Status:           models.ModelInvocationStatusFailed,
			LeaseDisposition: models.InvocationLeaseRetained,
		},
		err: models.ErrInferenceTimeout,
	}
	root, scope, host := newJoinedInvocationRoot(t, &events, inference)

	result, err := root.InvokeModel(context.Background(), joinedInvocationRequest(scope))
	if !errors.Is(err, models.ErrInferenceTimeout) {
		t.Fatalf("InvokeModel error = %v, want inference timeout", err)
	}
	if host.releaseCalls != 0 {
		t.Fatalf("joined release calls = %d, want no retry after private attempt", host.releaseCalls)
	}
	if result.Status != models.ModelInvocationStatusFailed ||
		result.LeaseDisposition != models.InvocationLeaseRetained ||
		len(result.Content) != 0 || len(result.Artifacts) != 0 || len(result.Outputs) != 0 {
		t.Fatalf("failed result = %#v, want failed/retained with no output", result)
	}
	if got, want := events, []string{"resolve", "preflight", "assets", "host", "lease", "invoke"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("private-release failure events = %#v, want %#v", got, want)
	}
}

func TestRootInvokeModelNormalizesAtomicallyBeforeRelease(t *testing.T) {
	var events []string
	inference := &joinedInferenceService{
		events: &events,
		result: models.InvokeModelResult{
			Status:           models.ModelInvocationStatusCompleted,
			LeaseDisposition: models.InvocationLeaseRetained,
			Content:          []models.InferenceContent{{Name: "unknown", Content: "bad"}},
		},
	}
	root, scope, host := newJoinedInvocationRoot(t, &events, inference)

	result, err := root.InvokeModel(context.Background(), joinedInvocationRequest(scope))
	var failure *models.InvocationFailure
	if !errors.As(err, &failure) || failure.Class != models.InvocationFailureClassMalformedResponse {
		t.Fatalf("normalization error = %v, failure = %#v, want malformed response", err, failure)
	}
	if result.Status != models.ModelInvocationStatusFailed || len(result.Outputs) != 0 {
		t.Fatalf("normalized failure result = %#v, want failed atomic result", result)
	}
	if host.releaseCalls != 1 {
		t.Fatalf("normalization release calls = %d, want exactly 1", host.releaseCalls)
	}
}

func TestRootInvokeModelValidatesSlotsBeforeAssetAndHostEffects(t *testing.T) {
	var events []string
	inference := &joinedInferenceService{events: &events, result: joinedCompletedResult(t)}
	root, scope, host := newJoinedInvocationRoot(t, &events, inference)
	request := joinedInvocationRequest(scope)
	request.Inputs[0].Name = "unknown"

	_, err := root.InvokeModel(context.Background(), request)
	var failure *models.InvocationFailure
	if !errors.As(err, &failure) || failure.Class != models.InvocationFailureClassInvalidSlot {
		t.Fatalf("slot validation error = %v, failure = %#v, want INVALID_SLOT", err, failure)
	}
	if !reflect.DeepEqual(events, []string{"resolve"}) {
		t.Fatalf("effects after invalid slot = %#v, want only resolution", events)
	}
	if host.releaseCalls != 0 || inference.invokeCalls != 0 {
		t.Fatalf("invalid slot reached cleanup/invoke: releases=%d invokes=%d", host.releaseCalls, inference.invokeCalls)
	}
}

func TestRootCloseRuntimeScopePreventsConcurrentPullResolution(t *testing.T) {
	t.Parallel()
	scope, err := (models.RuntimeScopeRef{}).Parse("factory-session:test:close-race")
	if err != nil {
		t.Fatal(err)
	}
	scopes := newCloseRaceRuntimeScopes()
	args := newRootConstructionArgs(t)
	args.runtimeScopes = scopes
	root, err := args.build()
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		_, err := root.PullModelForScope(t.Context(), models.PullModelRequest{Scope: scope, Name: "voice"})
		result <- err
	}()
	awaitCloseRaceSignal(t, scopes.resolveStarted, "initial scope resolution")
	if _, err := root.CloseRuntimeScope(t.Context(), models.CloseRuntimeScopeRequest{Scope: scope}); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-result:
		if !errors.Is(err, models.ErrRuntimeScopeClosed) {
			t.Fatalf("late pull: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("late pull did not return")
	}
}

type closeRaceRuntimeScopes struct {
	mu             sync.Mutex
	resolveCalls   int
	closed         bool
	resolveStarted chan struct{}
	closeCompleted chan struct{}
	closeOnce      sync.Once
}

func newCloseRaceRuntimeScopes() *closeRaceRuntimeScopes {
	return &closeRaceRuntimeScopes{
		resolveStarted: make(chan struct{}),
		closeCompleted: make(chan struct{}),
	}
}

func (scopes *closeRaceRuntimeScopes) Open(models.RuntimeBinding) (runtimescopes.Reference, error) {
	return "", errors.New("unexpected runtime scope open")
}

func (scopes *closeRaceRuntimeScopes) Resolve(
	runtimescopes.Reference,
) (models.RuntimeBinding, error) {
	scopes.mu.Lock()
	scopes.resolveCalls++
	call := scopes.resolveCalls
	closed := scopes.closed
	scopes.mu.Unlock()
	if call == 1 {
		close(scopes.resolveStarted)
		<-scopes.closeCompleted
		return models.RuntimeBinding{}, nil
	}
	if closed {
		return models.RuntimeBinding{}, runtimescopes.ErrScopeClosed
	}
	return models.RuntimeBinding{}, nil
}

func (scopes *closeRaceRuntimeScopes) Close(runtimescopes.Reference) error {
	scopes.mu.Lock()
	scopes.closed = true
	scopes.mu.Unlock()
	scopes.closeOnce.Do(func() {
		close(scopes.closeCompleted)
	})
	return nil
}

func awaitCloseRaceSignal(t *testing.T, signal <-chan struct{}, description string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", description)
	}
}

func newRuntimeHostFixture(scopes runtimescopes.Service, assets scopedassets.Service, launcher modelseffects.HostProcessLauncher, httpDoer modelseffects.HostHTTPDoer, clock modelseffects.HostClock, logger modelseffects.HostDiagnosticLogger, metrics modelseffects.HostMetricsRecorder, platform models.AssetHostPlatform, protocol modelseffects.HostProtocolNegotiator, compatibility modelseffects.HostCompatibilityChecker, resolve modelseffects.HostResolveSymlinks, evidence modelseffects.RuntimeEvidenceRecorder, idle time.Duration, maximum int) (runtimehost.Service, error) {
	state := runtimehostwire.NewSlotState()
	facts := runtimehostwire.NewSlotFacts(scopes, assets, state)
	coordinator := runtimehostwire.NewSlotCoordinator(state, scopes, clock, logger, metrics, idle)
	leases, err := runtimehostwire.NewLeases(clock, facts, coordinator)
	if err != nil {
		return nil, err
	}
	return runtimehostwire.NewService(scopes, assets, leases, state, launcher, httpDoer, clock, logger, metrics, platform, protocol, compatibility, resolve, evidence, idle, maximum)
}

type inertScopedLocalExecution struct{}

func (inertScopedLocalExecution) PullModelForScope(context.Context, models.PullModelRequest) (models.PullResult, error) {
	return models.PullResult{}, models.ErrNotFound
}

func (inertScopedLocalExecution) InvokeLocal(context.Context, models.LocalInvocationRequest) (models.LocalInvocationResult, error) {
	return models.LocalInvocationResult{}, nil
}
func (inertScopedLocalExecution) CloseScope(models.RuntimeScopeRef) {}
func (inertScopedLocalExecution) Close()                            {}

type compatibilityLocalExecution struct{ *localExecutionFixture }

func (e compatibilityLocalExecution) CloseScope(scope models.RuntimeScopeRef) {
	e.local.CloseScope(scope)
}
func (e compatibilityLocalExecution) Close() { e.local.Close() }

// Configuration resolution is a component boundary: a successful lookup can
// complete after close, without permitting a new host or asset effect.
func TestRootScopedExecutionCloseWinningConfigurationResolution(t *testing.T) {
	t.Parallel()
	for _, operation := range []string{"invoke", "pull"} {
		t.Run(operation, func(t *testing.T) {
			t.Parallel()
			args := newRootConstructionArgs(t)
			gate := &gatedConfigurationScopes{Service: args.runtimeScopes, started: make(chan struct{}), resume: make(chan struct{})}
			args.runtimeScopes = gate
			assets := &preparationAssetService{}
			args.assets = assets
			root, err := args.build()
			if err != nil {
				t.Fatal(err)
			}
			opened, err := root.OpenRuntimeScope(t.Context(), models.OpenRuntimeScopeRequest{})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			var resumeOnce sync.Once
			resume := func() { resumeOnce.Do(func() { close(gate.resume) }) }
			defer resume()
			done := make(chan error, 1)
			go func() {
				if operation == "pull" {
					result, err := root.PullModelForScope(ctx, models.PullModelRequest{Scope: opened.Scope, Name: "voice"})
					if result.CachePath != "" {
						err = errors.New("closed pull published a cache")
					}
					done <- err
					return
				}
				request := scopedHandleRequest(t, "config-close")
				request.Scope = opened.Scope
				result, err := root.InvokeLocal(ctx, request)
				if result.Content != "" {
					err = errors.New("closed invocation published content")
				}
				done <- err
			}()
			awaitCloseRaceSignal(t, gate.started, "configuration lookup")
			if _, err := root.CloseRuntimeScope(ctx, models.CloseRuntimeScopeRequest{Scope: opened.Scope}); err != nil {
				t.Fatal(err)
			}
			resume()
			select {
			case err := <-done:
				if !errors.Is(err, models.ErrRuntimeScopeClosed) {
					t.Fatalf("late %s: %v", operation, err)
				}
			case <-ctx.Done():
				t.Fatal("late operation did not return")
			}
			if assets.request.Name != "" || args.localRuntime.(*leaseTestRuntime).loads != 0 {
				t.Fatal("closed configuration reached assets or runtime")
			}
		})
	}
}

type gatedConfigurationScopes struct {
	runtimescopes.Service
	started, resume chan struct{}
	once            sync.Once
}

func (s *gatedConfigurationScopes) Resolve(ref runtimescopes.Reference) (models.RuntimeBinding, error) {
	binding, err := s.Service.Resolve(ref)
	if err != nil {
		return binding, err
	}
	lookup := binding.RuntimeConfig
	binding.RuntimeConfig = func() *models.RuntimeConfig {
		config := lookup()
		s.once.Do(func() { close(s.started); <-s.resume })
		return config
	}
	return binding, nil
}
