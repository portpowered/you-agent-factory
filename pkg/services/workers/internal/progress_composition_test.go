package internal

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	platformrandom "github.com/portpowered/infinite-you/pkg/platform/random"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workeragentrun "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/executor/agentrun"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

var testRetryRandom = platformrandom.SourceFunc(func(int64) (int64, error) {
	return 0, nil
})

func testProgressPublisher(workers.ProgressFragment) {}

type testProvidersService struct {
	providers.Service
}

func (testProvidersService) ListProviders(context.Context, providers.ListProvidersRequest) (providers.ListProvidersResult, error) {
	return providers.ListProvidersResult{}, nil
}
func (testProvidersService) GetProvider(context.Context, providers.GetProviderRequest) (providers.GetProviderResult, error) {
	return providers.GetProviderResult{}, providers.ErrUnknownProvider
}
func (testProvidersService) Execute(context.Context, providers.ExecuteRequest) (providers.ExecuteResult, error) {
	return providers.ExecuteResult{Content: "ok"}, nil
}

// pkgmaintcheck:ignore-cyclomatic-complexity service-ownership migration preserves this decision flow; simplify branches and remove this exemption.
func TestNewRequiresCompositionSelectedWorkerEffects(t *testing.T) {
	base := func(provider, script workers.CommandRunner, publisher workers.ProgressPublisher, allocator workers.PTYAllocator, logger *zap.Logger, now func() time.Time) error {
		_, err := New(
			inertCurrentRuntimeResolver{}, testModelsService{}, testProvidersService{}, provider, script, publisher, allocator,
			logger, false, "", "", nil, nil, now, os.Environ, os.Getwd, nil, nil, nil, nil,
			testFactoryDocsLoader, testResolveSymlinks, platformprocess.HostExecutableLocator{}, platformfilesystem.Local{}, platformfilesystem.Local{}, "linux", testFactoryWorktreePreparer{}, workeragentrun.NewLibraryHarnessAdapter(platformfilesystem.Local{}),
			testRetryRandom,
			platformfilesystem.Local{},
			platformfilesystem.Local{},
		)
		return err
	}
	validRunner := injectedProviderRunner{}
	validPublisher := workers.ProgressPublisher(testProgressPublisher)
	validAllocator := &workers.MockPTYAllocator{}
	for _, testCase := range []struct {
		name string
		err  error
		want string
	}{
		{name: "provider runner", err: base(nil, validRunner, validPublisher, validAllocator, zap.NewNop(), time.Now), want: "provider command runner is required"},
		{name: "script runner", err: base(validRunner, nil, validPublisher, validAllocator, zap.NewNop(), time.Now), want: "script command runner is required"},
		{name: "progress publisher", err: base(validRunner, validRunner, nil, validAllocator, zap.NewNop(), time.Now), want: "progress publisher is required"},
		{name: "PTY allocator", err: base(validRunner, validRunner, validPublisher, nil, zap.NewNop(), time.Now), want: "Agy PTY allocator is required"},
		{name: "logger", err: base(validRunner, validRunner, validPublisher, validAllocator, nil, time.Now), want: "logger is required"},
		{name: "clock", err: base(validRunner, validRunner, validPublisher, validAllocator, zap.NewNop(), nil), want: "clock is required"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if testCase.err == nil || !strings.Contains(testCase.err.Error(), testCase.want) {
				t.Fatalf("error = %v, want %q", testCase.err, testCase.want)
			}
		})
	}
	_, err := New(
		inertCurrentRuntimeResolver{}, testModelsService{}, testProvidersService{}, validRunner, validRunner, validPublisher, validAllocator,
		zap.NewNop(), false, "", "", nil, nil, time.Now, os.Environ, os.Getwd, nil, nil, nil, nil,
		testFactoryDocsLoader, testResolveSymlinks, platformprocess.HostExecutableLocator{}, platformfilesystem.Local{}, platformfilesystem.Local{}, "linux", nil, workeragentrun.NewLibraryHarnessAdapter(platformfilesystem.Local{}),
		testRetryRandom,
		platformfilesystem.Local{},
		platformfilesystem.Local{},
	)
	if err == nil || !strings.Contains(err.Error(), "worktree preparer is required") {
		t.Fatalf("missing worktree preparer error = %v", err)
	}
	_, err = New(
		inertCurrentRuntimeResolver{}, testModelsService{}, testProvidersService{}, validRunner, validRunner, validPublisher, validAllocator,
		zap.NewNop(), false, "", "", nil, nil, time.Now, os.Environ, os.Getwd, nil, nil, nil, nil,
		testFactoryDocsLoader, testResolveSymlinks, platformprocess.HostExecutableLocator{}, platformfilesystem.Local{}, platformfilesystem.Local{}, "linux", testFactoryWorktreePreparer{}, nil,
		testRetryRandom,
		platformfilesystem.Local{},
		platformfilesystem.Local{},
	)
	if err == nil || !strings.Contains(err.Error(), "agent-run harness is required") {
		t.Fatalf("missing agent-run harness error = %v", err)
	}
	_, err = New(
		inertCurrentRuntimeResolver{}, testModelsService{}, testProvidersService{}, validRunner, validRunner, validPublisher, validAllocator,
		zap.NewNop(), false, "", "", nil, nil, time.Now, os.Environ, os.Getwd, nil, nil, nil, nil,
		testFactoryDocsLoader, testResolveSymlinks, platformprocess.HostExecutableLocator{}, platformfilesystem.Local{}, platformfilesystem.Local{}, "linux", testFactoryWorktreePreparer{}, workeragentrun.NewLibraryHarnessAdapter(platformfilesystem.Local{}),
		nil,
		platformfilesystem.Local{},
		platformfilesystem.Local{},
	)
	if err == nil || !strings.Contains(err.Error(), "provider retry random source is required") {
		t.Fatalf("missing retry random source error = %v", err)
	}
	_, err = New(
		inertCurrentRuntimeResolver{}, testModelsService{}, testProvidersService{}, validRunner, validRunner, validPublisher, validAllocator,
		zap.NewNop(), false, "", "", nil, nil, time.Now, os.Environ, os.Getwd, nil, nil, nil, nil,
		testFactoryDocsLoader, testResolveSymlinks, platformprocess.HostExecutableLocator{}, platformfilesystem.Local{}, platformfilesystem.Local{}, "linux", testFactoryWorktreePreparer{}, workeragentrun.NewLibraryHarnessAdapter(platformfilesystem.Local{}),
		testRetryRandom,
		nil,
		platformfilesystem.Local{},
	)
	if err == nil || !strings.Contains(err.Error(), "workstation filesystem is required") {
		t.Fatalf("missing workstation filesystem error = %v", err)
	}
	_, err = New(
		inertCurrentRuntimeResolver{}, testModelsService{}, testProvidersService{}, validRunner, validRunner, validPublisher, validAllocator,
		zap.NewNop(), false, "", "", nil, nil, time.Now, os.Environ, os.Getwd, nil, nil, nil, nil,
		testFactoryDocsLoader, testResolveSymlinks, platformprocess.HostExecutableLocator{}, platformfilesystem.Local{}, platformfilesystem.Local{}, "linux", testFactoryWorktreePreparer{}, workeragentrun.NewLibraryHarnessAdapter(platformfilesystem.Local{}),
		testRetryRandom,
		platformfilesystem.Local{},
		nil,
	)
	if err == nil || !strings.Contains(err.Error(), "provider temporary filesystem is required") {
		t.Fatalf("missing provider temporary filesystem error = %v", err)
	}
}

func TestNewInvocationRequiresCompositionSelectedWorkerEffects(t *testing.T) {
	clock := workers.ClockFunc(time.Now)
	if _, err := NewInvocation(testProvidersService{}, nil, clock, &workers.MockPTYAllocator{}, testResolveSymlinks, platformprocess.HostExecutableLocator{}, platformfilesystem.Local{}, platformfilesystem.Local{}, "linux"); err == nil || !strings.Contains(err.Error(), "command runner is required") {
		t.Fatalf("missing command runner error = %v", err)
	}
	if _, err := NewInvocation(testProvidersService{}, injectedProviderRunner{}, clock, nil, testResolveSymlinks, platformprocess.HostExecutableLocator{}, platformfilesystem.Local{}, platformfilesystem.Local{}, "linux"); err == nil || !strings.Contains(err.Error(), "PTY allocator is required") {
		t.Fatalf("missing allocator error = %v", err)
	}
	if _, err := NewInvocation(testProvidersService{}, injectedProviderRunner{}, nil, &workers.MockPTYAllocator{}, testResolveSymlinks, platformprocess.HostExecutableLocator{}, platformfilesystem.Local{}, platformfilesystem.Local{}, "linux"); err == nil || !strings.Contains(err.Error(), "command clock is required") {
		t.Fatalf("missing command clock error = %v", err)
	}
	if _, err := NewInvocation(nil, injectedProviderRunner{}, clock, &workers.MockPTYAllocator{}, testResolveSymlinks, platformprocess.HostExecutableLocator{}, platformfilesystem.Local{}, platformfilesystem.Local{}, "linux"); err == nil || !strings.Contains(err.Error(), "Providers service is required") {
		t.Fatalf("missing Providers service error = %v", err)
	}
}

// TestNewRuntimeConstructionPreservesInjectedLoggerForWorkstationPool proves
// the logger supplied at construction keeps reaching a freshly constructed
// runtime's workstation pool. Every Factory Session build reaches its Workers
// runtime through this same construction path (workers.SessionBuildFactory,
// reached via the Workers wire construction boundary from
// factory_runtime/internal/runtime_build.go), so this generic construction
// proof also covers session-build construction.
func TestNewRuntimeConstructionPreservesInjectedLoggerForWorkstationPool(t *testing.T) {
	t.Parallel()

	core, logs := observer.New(zapcore.InfoLevel)
	runtime := newTestFullRuntimeService(t, zap.New(core))

	if _, err := runtime.StartWorkstationPool(
		context.Background(),
		workers.WorkstationPoolStartRequest{Bindings: []workers.AssembledRuntimeBinding{
			{RoleName: "review", RoleKind: workers.RuntimeBuildRoleKindWorkstation},
		}},
	); err != nil {
		t.Fatalf("StartWorkstationPool() error = %v", err)
	}

	entries := logs.FilterMessage("workers workstation pool start").All()
	if len(entries) != 1 {
		t.Fatalf(
			"observed logs = %#v, want exactly one workstation pool start record surviving construction",
			logs.All(),
		)
	}
}

type inertCurrentRuntimeResolver struct{}

func (inertCurrentRuntimeResolver) CurrentRuntime() *factorysessions.LiveRuntime {
	return nil
}

type testModelsService struct {
	testRuntimeScopeUnsupported
}

type testRuntimeScopeUnsupported struct{}

func (testRuntimeScopeUnsupported) OpenRuntimeScope(
	context.Context,
	models.OpenRuntimeScopeRequest,
) (models.OpenRuntimeScopeResult, error) {
	return models.OpenRuntimeScopeResult{}, models.ErrUnsupportedOperation
}

func (testRuntimeScopeUnsupported) CloseRuntimeScope(
	context.Context,
	models.CloseRuntimeScopeRequest,
) (models.CloseRuntimeScopeResult, error) {
	return models.CloseRuntimeScopeResult{}, models.ErrUnsupportedOperation
}

func (testRuntimeScopeUnsupported) PrepareModelAssets(
	context.Context,
	models.PrepareModelAssetsRequest,
) (models.PrepareModelAssetsResult, error) {
	return models.PrepareModelAssetsResult{}, models.ErrUnsupportedOperation
}

func (testRuntimeScopeUnsupported) InspectModelAssets(
	context.Context,
	models.InspectModelAssetsRequest,
) (models.InspectModelAssetsResult, error) {
	return models.InspectModelAssetsResult{}, models.ErrUnsupportedOperation
}

func (testRuntimeScopeUnsupported) RemoveModelAssets(
	context.Context,
	models.RemoveModelAssetsRequest,
) (models.RemoveModelAssetsResult, error) {
	return models.RemoveModelAssetsResult{}, models.ErrUnsupportedOperation
}

func (testRuntimeScopeUnsupported) InvokeModelWithLease(
	context.Context,
	models.InvokeModelRequest,
) (models.InvokeModelResult, error) {
	return models.InvokeModelResult{}, models.ErrUnsupportedOperation
}

func (testRuntimeScopeUnsupported) CancelInvocation(
	context.Context,
	models.CancelInvocationRequest,
) (models.CancelInvocationResult, error) {
	return models.CancelInvocationResult{}, models.ErrUnsupportedOperation
}

func (s testModelsService) ForRuntime(models.RuntimeBinding) (models.Service, error) {
	return s, nil
}

type injectedProviderRunner struct{}

type testFactoryWorktreePreparer struct{}

func (testFactoryWorktreePreparer) Prepare(context.Context, string, string) (workers.FactoryWorktreePreparation, error) {
	return workers.FactoryWorktreePreparation{}, nil
}

func testFactoryDocsLoader(string) (map[string]string, error) { return map[string]string{}, nil }

func testResolveSymlinks(path string) (string, error) { return path, nil }

func (injectedProviderRunner) Run(context.Context, workers.CommandRequest) (workers.CommandResult, error) {
	return workers.CommandResult{}, nil
}

func (testModelsService) ListModels(context.Context) (models.List, error) {
	return models.List{}, nil
}
func (testModelsService) ListCatalog(context.Context, models.ListModelsRequest) (models.ListModelsResult, error) {
	return models.ListModelsResult{}, models.ErrUnsupportedOperation
}
func (testModelsService) GetCatalogModel(context.Context, models.GetModelRequest) (models.GetModelResult, error) {
	return models.GetModelResult{}, models.ErrUnsupportedOperation
}
func (testModelsService) GetModelReadiness(context.Context, models.GetModelReadinessRequest) (models.GetModelReadinessResult, error) {
	return models.GetModelReadinessResult{}, models.ErrUnsupportedOperation
}
func (testModelsService) GetModel(context.Context, string) (models.Detail, error) {
	return models.Detail{}, nil
}
func (testModelsService) PullModel(context.Context, string) (models.PullResult, error) {
	return models.PullResult{}, nil
}
func (testModelsService) PullModelForScope(
	context.Context,
	models.PullModelRequest,
) (models.PullResult, error) {
	return models.PullResult{}, nil
}
func (testModelsService) InspectRuntime(context.Context, string) (models.Runtime, error) {
	return models.Runtime{}, nil
}

func (testModelsService) AcquireLease(context.Context, models.AcquireLeaseRequest) (models.HostLease, error) {
	return models.HostLease{}, nil
}

func (testModelsService) ReleaseLease(context.Context, models.ReleaseLeaseRequest) error {
	return nil
}

func (testModelsService) EnsureModelHost(
	context.Context,
	models.EnsureModelHostRequest,
) (models.EnsureModelHostResult, error) {
	return models.EnsureModelHostResult{}, models.ErrUnsupportedOperation
}

func (testModelsService) InspectModelHost(
	context.Context,
	models.InspectModelHostRequest,
) (models.InspectModelHostResult, error) {
	return models.InspectModelHostResult{}, models.ErrUnsupportedOperation
}

func (testModelsService) StopModelHost(
	context.Context,
	models.StopModelHostRequest,
) (models.StopModelHostResult, error) {
	return models.StopModelHostResult{}, models.ErrUnsupportedOperation
}

func (testModelsService) AcquireModelLease(
	context.Context,
	models.AcquireModelLeaseRequest,
) (models.AcquireModelLeaseResult, error) {
	return models.AcquireModelLeaseResult{}, models.ErrUnsupportedOperation
}

func (testModelsService) GetModelLease(
	context.Context,
	models.GetModelLeaseRequest,
) (models.GetModelLeaseResult, error) {
	return models.GetModelLeaseResult{}, models.ErrUnsupportedOperation
}

func (testModelsService) ReleaseModelLease(
	context.Context,
	models.ReleaseModelLeaseRequest,
) (models.ReleaseModelLeaseResult, error) {
	return models.ReleaseModelLeaseResult{}, models.ErrUnsupportedOperation
}

func (testModelsService) InvokeLocal(context.Context, models.LocalInvocationRequest) (models.LocalInvocationResult, error) {
	return models.LocalInvocationResult{}, nil
}

// taggedCommandRunner is a distinguishable CommandRunner fake so construction
// tests can assert exact instance retention rather than mere non-nilness.
type taggedCommandRunner struct{ tag string }

func (taggedCommandRunner) Run(context.Context, workers.CommandRequest) (workers.CommandResult, error) {
	return workers.CommandResult{}, nil
}

// recordingProgressPublisher returns a ProgressPublisher plus a counter
// pointer, so tests can prove which exact publisher instance a Service
// invokes without relying on function-value equality (Go func values are
// only comparable to nil).
func recordingProgressPublisher() (workers.ProgressPublisher, *int) {
	calls := new(int)
	return func(workers.ProgressFragment) { *calls++ }, calls
}

func newTestServiceWithDependencies(
	t *testing.T,
	providerRunner, scriptRunner workers.CommandRunner,
	progressPublisher workers.ProgressPublisher,
	logger *zap.Logger,
) *Service {
	t.Helper()
	service, err := New(
		inertCurrentRuntimeResolver{},
		testModelsService{},
		testProvidersService{},
		providerRunner,
		scriptRunner,
		progressPublisher,
		&workers.MockPTYAllocator{},
		logger,
		false,
		"",
		"",
		nil,
		nil,
		time.Now,
		os.Environ,
		os.Getwd,
		nil,
		nil,
		nil,
		nil,
		testFactoryDocsLoader,
		testResolveSymlinks,
		platformprocess.HostExecutableLocator{},
		platformfilesystem.Local{},
		platformfilesystem.Local{},
		"linux",
		testFactoryWorktreePreparer{},
		workeragentrun.NewLibraryHarnessAdapter(platformfilesystem.Local{}),
		testRetryRandom,
		platformfilesystem.Local{},
		platformfilesystem.Local{},
	)
	if err != nil {
		t.Fatalf("construct Worker service: %v", err)
	}
	return service
}

// TestNewRetainsExactSuppliedDependencies proves the runtime returned by New
// retains the exact provider runner, script runner, and progress publisher
// instances supplied at construction, with no supported operation to replace
// them afterward.
func TestNewRetainsExactSuppliedDependencies(t *testing.T) {
	t.Parallel()

	providerRunner := taggedCommandRunner{tag: "provider"}
	scriptRunner := taggedCommandRunner{tag: "script"}
	publisher, calls := recordingProgressPublisher()
	service := newTestServiceWithDependencies(t, providerRunner, scriptRunner, publisher, zap.NewNop())

	if service.ProviderCommandRunner() != workers.CommandRunner(providerRunner) {
		t.Fatalf("ProviderCommandRunner() = %#v, want the exact supplied instance %#v", service.ProviderCommandRunner(), providerRunner)
	}
	if service.ScriptCommandRunner() != workers.CommandRunner(scriptRunner) {
		t.Fatalf("ScriptCommandRunner() = %#v, want the exact supplied instance %#v", service.ScriptCommandRunner(), scriptRunner)
	}
	service.progressPublisher(workers.ProgressFragment{})
	if *calls != 1 {
		t.Fatalf("retained progress publisher calls = %d, want 1 (the exact supplied publisher was not retained)", *calls)
	}
}

// TestNewConstructsIndependentRuntimes proves two separately constructed
// runtimes retain independent dependency identity: operating on one cannot
// observe or affect the other's constructed dependencies.
func TestNewConstructsIndependentRuntimes(t *testing.T) {
	t.Parallel()

	firstProviderRunner := taggedCommandRunner{tag: "first-provider"}
	secondProviderRunner := taggedCommandRunner{tag: "second-provider"}
	firstPublisher, firstCalls := recordingProgressPublisher()
	secondPublisher, secondCalls := recordingProgressPublisher()

	first := newTestServiceWithDependencies(t, firstProviderRunner, injectedProviderRunner{}, firstPublisher, zap.NewNop())
	second := newTestServiceWithDependencies(t, secondProviderRunner, injectedProviderRunner{}, secondPublisher, zap.NewNop())

	if first.ProviderCommandRunner() == second.ProviderCommandRunner() {
		t.Fatal("two independently constructed runtimes retained the same provider command runner instance")
	}

	second.progressPublisher(workers.ProgressFragment{})
	if *firstCalls != 0 {
		t.Fatalf("first runtime's progress publisher observed %d calls from the second runtime's activity, want 0", *firstCalls)
	}
	if *secondCalls != 1 {
		t.Fatalf("second runtime's progress publisher calls = %d, want 1", *secondCalls)
	}

	if first.ProviderCommandRunner() != workers.CommandRunner(firstProviderRunner) {
		t.Fatal("first runtime's provider command runner changed after constructing an unrelated second runtime")
	}
}
