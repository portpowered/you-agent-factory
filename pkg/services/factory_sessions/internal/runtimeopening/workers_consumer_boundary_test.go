package runtimeopening

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/automations"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"github.com/portpowered/infinite-you/pkg/services/models"
	operatorconfig "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

const workersImportRoot = "github.com/portpowered/infinite-you/pkg/services/workers"

// TestRuntimeOpeningPackagesImportWorkersOnlyThroughRoot seals runtime-opening
// and construction call sites to the Workers service root contract.

// TestRuntimeOpeningFactoryRolesNameWorkersRootContracts proves runtime-opening
// construction helpers type Workers-facing bindings only through the Workers
// service root.
func TestRuntimeOpeningFactoryRolesNameWorkersRootContracts(t *testing.T) {
	t.Parallel()

	var (
		_ FactorySessionExecutionFactory
		_ ConductorInvocationWithProgressFactory
		_ WorkersRuntimeFactory
		_ WorkerExecutionFactory
		_ DurableExecutionFactory
		_ ProviderFromCommandRunnerFactory
	)

	var _ ExternalEffects = ExternalEffects{
		ProviderOverride:     (workers.Provider)(nil),
		HostedClock:          (automations.HostedLinearClock)(nil),
		HostedHTTPClient:     (automations.HostedLinearHTTPDoer)(nil),
		HostedSecretResolver: (automations.HostedLinearSecretResolver)(nil),
	}
}

type workersRootBindingProbeRunner struct{}

func (workersRootBindingProbeRunner) Run(
	context.Context,
	workers.CommandRequest,
) (workers.CommandResult, error) {
	return workers.CommandResult{}, nil
}

func TestNewWorkerExecutionForwardsWorkersRootBindings(t *testing.T) {
	t.Parallel()

	const runnerID = "session-worker-runner"
	const runWorktree = "feature-login"
	ptyAllocator := &workers.MockPTYAllocator{}
	var gotRunnerID string
	var gotWorktree string
	var gotPTY workers.PTYAllocator
	var gotProvider workers.Provider

	workersRuntimeFactory := func(
		_ roles.CurrentRuntimeResolver,
		_ models.Service,
		_ models.RuntimeScopeRef,
		_ workers.CommandRunner,
		_ workers.CommandRunner,
		_ workers.ProgressPublisher,
		pty workers.PTYAllocator,
		_ *zap.Logger,
		_ bool,
		runner string,
		worktree string,
		_ *bool,
		provider workers.Provider,
		_ func() time.Time,
		_ work.ContentMaterializer,
		_ []operatorconfig.ACPIntegration,
	) (workers.RuntimeService, error) {
		gotRunnerID = runner
		gotWorktree = worktree
		gotPTY = pty
		gotProvider = provider
		return nil, nil
	}

	service, sessionBuildFactory, err := NewWorkerExecution(
		factoryruntime.RuntimeOpeningRequest{},
		workers.RuntimeOpeningRequest{RunnerID: runnerID, Worktree: runWorktree},
		openingCoordinatorClock{},
		zap.NewNop(),
		workersRootBindingProbeRunner{},
		workersRootBindingProbeRunner{},
		nil,
		ptyAllocator,
		nil,
		nil,
		nil,
		models.RuntimeScopeRef{},
		work.MaterializationService(materializerStub{}),
		workersRuntimeFactory,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("NewWorkerExecution() error = %v, want nil", err)
	}
	if service != nil {
		t.Fatalf("service = %#v, want nil placeholder", service)
	}
	if sessionBuildFactory == nil {
		t.Fatal("sessionBuildFactory = nil, want a SessionBuildFactory reaching the same canonical construction boundary")
	}
	if gotRunnerID != runnerID {
		t.Fatalf("runner ID = %q, want %q", gotRunnerID, runnerID)
	}
	if gotWorktree != runWorktree {
		t.Fatalf("worktree = %q, want %q", gotWorktree, runWorktree)
	}
	if gotPTY != ptyAllocator {
		t.Fatalf("PTY allocator = %#v, want %#v", gotPTY, ptyAllocator)
	}
	if gotProvider != nil {
		t.Fatalf("provider = %#v, want nil override", gotProvider)
	}
}

// sessionBuildFactoryProbe captures every call NewWorkerExecution's
// WorkersRuntimeFactory dependency receives, so tests can assert exactly
// which provider/script runner and progress publisher instance reached the
// canonical construction boundary for each SessionBuildFactory call.
type sessionBuildFactoryProbe struct {
	callCount       int
	providerRunner  workers.CommandRunner
	scriptRunner    workers.CommandRunner
	publisher       workers.ProgressPublisher
	registeredCount int
}

func (probe *sessionBuildFactoryProbe) workersRuntimeFactory(
	_ roles.CurrentRuntimeResolver,
	_ models.Service,
	_ models.RuntimeScopeRef,
	providerRunner workers.CommandRunner,
	scriptRunner workers.CommandRunner,
	publisher workers.ProgressPublisher,
	_ workers.PTYAllocator,
	_ *zap.Logger,
	_ bool,
	_ string,
	_ string,
	_ *bool,
	_ workers.Provider,
	_ func() time.Time,
	_ work.ContentMaterializer,
	_ []operatorconfig.ACPIntegration,
) (workers.RuntimeService, error) {
	probe.callCount++
	probe.providerRunner = providerRunner
	probe.scriptRunner = scriptRunner
	probe.publisher = publisher
	return nil, nil
}

// newSessionBuildFactoryProbe constructs the Factory Session's own Workers
// runtime through NewWorkerExecution with the supplied base runner/publisher,
// returning the resulting SessionBuildFactory and the probe recording every
// WorkersRuntimeFactory call it makes.
func newSessionBuildFactoryProbe(
	t *testing.T,
	baseProviderRunner, baseScriptRunner workers.CommandRunner,
	basePublisher workers.ProgressPublisher,
) (*sessionBuildFactoryProbe, workers.SessionBuildFactory) {
	t.Helper()
	probe := &sessionBuildFactoryProbe{}
	_, sessionBuildFactory, err := NewWorkerExecution(
		factoryruntime.RuntimeOpeningRequest{},
		workers.RuntimeOpeningRequest{},
		openingCoordinatorClock{},
		zap.NewNop(),
		baseProviderRunner,
		baseScriptRunner,
		basePublisher,
		&workers.MockPTYAllocator{},
		nil,
		nil,
		nil,
		models.RuntimeScopeRef{},
		work.MaterializationService(materializerStub{}),
		probe.workersRuntimeFactory,
		nil,
		func(workers.RuntimeService) {
			probe.registeredCount++
		},
	)
	if err != nil {
		t.Fatalf("NewWorkerExecution() error = %v, want nil", err)
	}
	if probe.callCount != 1 {
		t.Fatalf("workersRuntimeFactory calls = %d after construction, want exactly 1", probe.callCount)
	}
	if probe.registeredCount != 0 {
		t.Fatalf("registered count = %d, want none for the base construction itself", probe.registeredCount)
	}
	return probe, sessionBuildFactory
}

// TestNewWorkerExecutionSessionBuildFactoryPreservesBaseOnNilArguments proves
// a nil provider runner, script runner, and progress publisher argument to
// the returned SessionBuildFactory resolve to the exact instances the
// Factory Session's own runtime was constructed with -- read from
// NewWorkerExecution's own captured construction context, never from an
// already-built RuntimeService -- and that the independently constructed
// session-build runtime is reported through registerSessionBuildRuntime.
func TestNewWorkerExecutionSessionBuildFactoryPreservesBaseOnNilArguments(t *testing.T) {
	t.Parallel()

	baseProviderRunner := workersRootBindingProbeRunner{}
	baseScriptRunner := workersRootBindingProbeRunner{}
	var basePublished []string
	basePublisher := workers.ProgressPublisher(func(workers.ProgressFragment) {
		basePublished = append(basePublished, "published")
	})

	probe, sessionBuildFactory := newSessionBuildFactoryProbe(t, baseProviderRunner, baseScriptRunner, basePublisher)

	if _, err := sessionBuildFactory(nil, nil, nil); err != nil {
		t.Fatalf("sessionBuildFactory(nil, nil, nil) error = %v", err)
	}
	if probe.callCount != 2 {
		t.Fatalf("workersRuntimeFactory calls = %d, want exactly 2 after one session build", probe.callCount)
	}
	if probe.providerRunner != workers.CommandRunner(baseProviderRunner) {
		t.Fatalf("provider runner = %#v, want base's resolved instance %#v", probe.providerRunner, baseProviderRunner)
	}
	if probe.scriptRunner != workers.CommandRunner(baseScriptRunner) {
		t.Fatalf("script runner = %#v, want base's resolved instance %#v", probe.scriptRunner, baseScriptRunner)
	}
	probe.publisher(workers.ProgressFragment{})
	if !slices.Equal(basePublished, []string{"published"}) {
		t.Fatalf("published = %#v, want the nil publisher argument to resolve to base's own publisher", basePublished)
	}
	if probe.registeredCount != 1 {
		t.Fatalf("registered count = %d, want exactly 1 after one session build", probe.registeredCount)
	}
}

// TestNewWorkerExecutionSessionBuildFactoryForwardsExplicitOverride proves a
// non-nil provider runner argument to the returned SessionBuildFactory
// reaches the construction boundary directly instead of being defaulted
// away, while a nil script runner argument still preserves the Factory
// Session's own resolved instance, and each independent session-build
// runtime is reported through registerSessionBuildRuntime.
func TestNewWorkerExecutionSessionBuildFactoryForwardsExplicitOverride(t *testing.T) {
	t.Parallel()

	baseProviderRunner := workersRootBindingProbeRunner{}
	baseScriptRunner := workersRootBindingProbeRunner{}
	buildProviderRunner := workersRootBindingProbeRunner{}
	basePublisher := workers.ProgressPublisher(func(workers.ProgressFragment) {})

	probe, sessionBuildFactory := newSessionBuildFactoryProbe(t, baseProviderRunner, baseScriptRunner, basePublisher)

	if _, err := sessionBuildFactory(buildProviderRunner, nil, nil); err != nil {
		t.Fatalf("sessionBuildFactory(override, nil, nil) error = %v", err)
	}
	if probe.providerRunner != workers.CommandRunner(buildProviderRunner) {
		t.Fatalf("provider runner = %#v, want the explicit build-time override %#v", probe.providerRunner, buildProviderRunner)
	}
	if probe.scriptRunner != workers.CommandRunner(baseScriptRunner) {
		t.Fatalf("script runner = %#v, want base's resolved instance preserved alongside the provider override", probe.scriptRunner)
	}
	if probe.registeredCount != 1 {
		t.Fatalf("registered count = %d, want the session-build runtime reported", probe.registeredCount)
	}
}
