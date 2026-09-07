package service

import (
	"context"
	"errors"
	"net/http"
	"os"
	"reflect"
	"strings"
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
		{name: "runtime command runner", mutate: func(args *rootConstructionArgs) { args.runtimeRunner = nil }, message: "model runtime command runner"},
		{name: "runtime HTTP client", mutate: func(args *rootConstructionArgs) { args.runtimeHTTP = nil }, message: "model runtime HTTP client"},
		{name: "runtime file inspector", mutate: func(args *rootConstructionArgs) { args.runtimeInspect = nil }, message: "model runtime file inspector"},
		{name: "runtime temp directory", mutate: func(args *rootConstructionArgs) { args.runtimeTempDir = nil }, message: "model runtime temporary directory resolver"},
		{name: "runtime temp file", mutate: func(args *rootConstructionArgs) { args.runtimeTempFile = nil }, message: "model runtime temporary file creator"},
		{name: "runtime scopes", mutate: func(args *rootConstructionArgs) { args.runtimeScopes = nil }, message: "Models Runtime Scopes service"},
		{name: "catalog", mutate: func(args *rootConstructionArgs) { args.catalog = nil }, message: "Models Catalog service"},
		{name: "assets", mutate: func(args *rootConstructionArgs) { args.assets = nil }, message: "Models Assets service"},
		{name: "runtime host", mutate: func(args *rootConstructionArgs) { args.runtimeHost = nil }, message: "Models Runtime Host service"},
		{name: "inference", mutate: func(args *rootConstructionArgs) { args.inference = nil }, message: "Models Inference service"},
		{name: "process clock", mutate: func(args *rootConstructionArgs) { args.process.Clock = nil }, message: "Models process clock"},
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
	processLauncher modelhost.ProcessLauncher
	hostHTTP        modelhost.HTTPDoer
	hostClock       modelhost.Clock
	runtimeRunner   platformprocess.CommandRunner
	runtimeHTTP     localmodels.HTTPDoer
	runtimeInspect  localmodels.InspectFile
	runtimeTempDir  localmodels.TempDirectory
	runtimeTempFile localmodels.CreateTempFile
	runtimeScopes   runtimescopes.Service
	catalog         modelcatalog.Service
	assets          scopedassets.Service
	runtimeHost     runtimehost.Service
	inference       inference.Service
	process         modelseffects.ProcessDependencies
}

func (args rootConstructionArgs) build() (*Root, error) {
	return NewRoot(
		args.processLauncher,
		args.hostHTTP,
		args.hostClock,
		args.runtimeRunner,
		args.runtimeHTTP,
		args.runtimeInspect,
		args.runtimeTempDir,
		args.runtimeTempFile,
		args.runtimeScopes,
		args.catalog,
		args.assets,
		args.runtimeHost,
		args.inference,
		args.process,
	)
}

func newRootConstructionArgs(t *testing.T) rootConstructionArgs {
	t.Helper()

	scopes, err := runtimescopeswire.NewService(func() string { return "root-construction-test" })
	if err != nil {
		t.Fatalf("construct runtime scopes: %v", err)
	}
	catalog, err := catalogwire.NewService(scopes)
	if err != nil {
		t.Fatalf("construct catalog: %v", err)
	}
	events := []string{}
	return rootConstructionArgs{
		processLauncher: rootConstructionProcessLauncher{},
		hostHTTP:        http.DefaultClient,
		hostClock:       rootConstructionClock{},
		runtimeRunner:   rootConstructionCommandRunner{},
		runtimeHTTP:     http.DefaultClient,
		runtimeInspect:  os.Stat,
		runtimeTempDir:  os.TempDir,
		runtimeTempFile: func(dir, pattern string) (localmodels.TempFile, error) {
			return os.CreateTemp(dir, pattern)
		},
		runtimeScopes: scopes,
		catalog:       catalog,
		assets:        inferenceRecordingAssetsService{},
		runtimeHost:   &joinedHostService{events: &events},
		inference:     &joinedInferenceService{events: &events},
		process: modelseffects.ProcessDependencies{
			Logger: zap.NewNop(), Clock: time.Now,
		},
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
	root := &Root{runtimeHost: host}
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
		runtimeScopes:  scopes,
		assets:         inferenceRecordingAssetsService{},
		inference:      privateInference,
		runtimeByScope: make(map[models.RuntimeScopeRef]models.Service),
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
	runtimeHost, err := runtimehostwire.NewService(
		scopes,
		assets,
		launcher,
		http.DefaultClient,
		clock,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("construct Runtime Host: %v", err)
	}
	catalog, err := catalogwire.NewService(scopes)
	if err != nil {
		t.Fatalf("construct Catalog: %v", err)
	}
	inference, err := inferencewire.NewService(
		scopes,
		assets,
		catalog,
		runtimeHost,
		constructionInvocationRuntime{},
		inference.InertArtifactFileSystem{},
		clock.Now,
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

func TestNewRootAcceptsComposedDependenciesAndDefaultsLogger(t *testing.T) {
	t.Parallel()

	scopes, err := runtimescopeswire.NewService(func() string { return "root-construction-test" })
	if err != nil {
		t.Fatalf("construct Runtime Scopes: %v", err)
	}
	assets := inferenceRecordingAssetsService{}
	catalog, err := catalogwire.NewService(scopes)
	if err != nil {
		t.Fatalf("construct Catalog: %v", err)
	}
	launcher := &inferenceRecordingProcessLauncher{}
	clock := &inferenceTestClock{}
	runtimeHost, err := runtimehostwire.NewService(scopes, assets, launcher, http.DefaultClient, clock, nil, nil)
	if err != nil {
		t.Fatalf("construct Runtime Host: %v", err)
	}
	inferenceService, err := inferencewire.NewService(
		scopes, assets, catalog, runtimeHost, constructionInvocationRuntime{},
		inference.InertArtifactFileSystem{}, clock.Now,
	)
	if err != nil {
		t.Fatalf("construct Inference: %v", err)
	}
	root, err := NewRoot(
		rootConstructionProcessLauncher{}, http.DefaultClient, rootConstructionClock{}, rootConstructionCommandRunner{},
		http.DefaultClient, os.Stat, os.TempDir,
		func(string, string) (localmodels.TempFile, error) { return rootConstructionTempFile{}, nil },
		scopes, catalog, assets, runtimeHost, inferenceService,
		modelseffects.ProcessDependencies{Clock: time.Now},
	)
	if err != nil {
		t.Fatalf("NewRoot: %v", err)
	}
	if root == nil || root.resolveHuggingFaceRevision == nil || root.process.Logger == nil {
		t.Fatal("NewRoot did not retain a usable root with default logger/revision resolver")
	}
}

func TestNewRootRejectsMissingRequiredComposedDependencies(t *testing.T) {
	t.Parallel()

	validLauncher := rootConstructionProcessLauncher{}
	validHTTP := http.DefaultClient
	validClock := rootConstructionClock{}
	validRunner := rootConstructionCommandRunner{}
	validTempFile := func(string, string) (localmodels.TempFile, error) {
		return rootConstructionTempFile{}, nil
	}
	construct := func(
		processLauncher modelhost.ProcessLauncher,
		hostHTTP modelhost.HTTPDoer,
		hostClock modelhost.Clock,
		runtimeRunner platformprocess.CommandRunner,
		runtimeHTTP localmodels.HTTPDoer,
		runtimeInspect localmodels.InspectFile,
		runtimeTempDir localmodels.TempDirectory,
		runtimeTempFile localmodels.CreateTempFile,
	) error {
		_, err := NewRoot(
			processLauncher, hostHTTP, hostClock, runtimeRunner, runtimeHTTP, runtimeInspect,
			runtimeTempDir, runtimeTempFile, nil, nil, nil, nil, nil,
			modelseffects.ProcessDependencies{Clock: time.Now},
		)
		return err
	}

	missing := []struct {
		name string
		call func() error
	}{
		{name: "model host process launcher", call: func() error {
			return construct(nil, validHTTP, validClock, validRunner, validHTTP, os.Stat, os.TempDir, validTempFile)
		}},
		{name: "model host HTTP client", call: func() error {
			return construct(validLauncher, nil, validClock, validRunner, validHTTP, os.Stat, os.TempDir, validTempFile)
		}},
		{name: "model host clock", call: func() error {
			return construct(validLauncher, validHTTP, nil, validRunner, validHTTP, os.Stat, os.TempDir, validTempFile)
		}},
		{name: "model runtime command runner", call: func() error {
			return construct(validLauncher, validHTTP, validClock, nil, validHTTP, os.Stat, os.TempDir, validTempFile)
		}},
		{name: "model runtime HTTP client", call: func() error {
			return construct(validLauncher, validHTTP, validClock, validRunner, nil, os.Stat, os.TempDir, validTempFile)
		}},
		{name: "model runtime file inspector", call: func() error {
			return construct(validLauncher, validHTTP, validClock, validRunner, validHTTP, nil, os.TempDir, validTempFile)
		}},
		{name: "model runtime temporary directory resolver", call: func() error {
			return construct(validLauncher, validHTTP, validClock, validRunner, validHTTP, os.Stat, nil, validTempFile)
		}},
		{name: "model runtime temporary file creator", call: func() error {
			return construct(validLauncher, validHTTP, validClock, validRunner, validHTTP, os.Stat, os.TempDir, nil)
		}},
	}
	for _, test := range missing {
		t.Run(test.name, func(t *testing.T) {
			if err := test.call(); !errors.Is(err, ErrInvalidDependencies) {
				t.Fatalf("NewRoot missing %s error = %v, want ErrInvalidDependencies", test.name, err)
			}
		})
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
	root.process = modelseffects.ProcessDependencies{
		Logger: zap.New(core),
		Clock:  func() time.Time { return time.Unix(123, 0) },
	}

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
	root.process = modelseffects.ProcessDependencies{
		RuntimeEvidence: modelseffects.NewOrderedRuntimeEvidenceRecorder(sink),
	}

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
