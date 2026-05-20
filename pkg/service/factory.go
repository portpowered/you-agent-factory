package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/cli/dashboardrender"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/listeners"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/replay"
	"github.com/portpowered/infinite-you/pkg/workers"

	"go.uber.org/zap"
)

// SimpleDashboardRenderInput carries the low-level engine snapshot that powers
// runtime diagnostics together with the dedicated event-first render DTO used
// for dashboard session accounting.
type SimpleDashboardRenderInput struct {
	EngineState interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]
	RenderData  dashboardrender.SimpleDashboardRenderData
	Now         time.Time
}

// SimpleDashboardRenderer is a callback that formats and prints dashboard
// output. Callers (e.g. CLI) provide their own rendering implementation.
type SimpleDashboardRenderer func(input SimpleDashboardRenderInput)

// APIServerStarter is a callback that starts an API server for the factory.
// It receives the API surface, port, and logger, and should block until ctx is
// cancelled. Callers (e.g. CLI) provide their own implementation to avoid
// import cycles between service and api packages.
type APIServerStarter func(ctx context.Context, runtime apisurface.APISurface, port int, logger *zap.Logger) error

// ErrFactoryActivationRequiresIdle reports that runtime replacement was
// attempted while the current runtime still had active work.
var ErrFactoryActivationRequiresIdle = apisurface.ErrFactoryActivationRequiresIdle

// ErrInvalidNamedFactoryName reports that the requested named-factory name is
// not a safe canonical layout segment.
var ErrInvalidNamedFactoryName = apisurface.ErrInvalidNamedFactoryName

// ErrInvalidNamedFactory reports that the submitted named-factory payload could
// not be persisted or validated as a runnable runtime config.
var ErrInvalidNamedFactory = apisurface.ErrInvalidNamedFactory

// ErrCurrentNamedFactoryNotFound reports that no durable current-factory
// pointer could be resolved for named-factory readback.
var ErrCurrentNamedFactoryNotFound = apisurface.ErrCurrentNamedFactoryNotFound

type replacementFactoryRuntime struct {
	dir          string
	eventHistory *factory.FactoryEventHistory
	factory      factory.Factory
	listener     *listeners.FileWatcher
	net          *state.Net
	runtimeCfg   *factoryconfig.LoadedFactoryConfig
}

type liveRuntimeHandle struct {
	runtime       *replacementFactoryRuntime
	runCancel     context.CancelFunc
	runDone       chan struct{}
	sidecarCancel context.CancelFunc
	sidecars      sync.WaitGroup
	runErrMu      sync.RWMutex
	runErr        error
	sidecarMu     sync.Mutex
}

type serviceRunState struct {
	ctx     context.Context
	runtime *liveRuntimeHandle
}

// FactoryService is an instantiation of a factory along with its runtime
// concerns: file watcher, dashboard, API server. It owns the full lifecycle
// so that CLI and other entry points remain thin wrappers.
type FactoryService struct {
	runtimeMu      sync.RWMutex
	activationMu   sync.RWMutex
	runMu          sync.RWMutex
	runState       *serviceRunState
	factoryRootDir string
	factory        factory.Factory
	listener       *listeners.FileWatcher
	net            *state.Net
	cfg            *FactoryServiceConfig
	runtimeCfg     *factoryconfig.LoadedFactoryConfig
	eventHistory   *factory.FactoryEventHistory
	logger         *zap.Logger
	startTime      time.Time
	clock          factory.Clock
	recording      *replay.Recorder
	logSink        *logging.RuntimeLogSink
}

var _ factory.APIFactory = (*FactoryService)(nil)
var _ apisurface.APISurface = (*FactoryService)(nil)

// FactoryServiceConfig holds all parameters needed to build and run a factory.
type FactoryServiceConfig struct {
	// Dir is the factory root directory containing factory.json and inputs/.
	Dir string
	// RunnerID sets the factory-level runner override used when a workstation
	// does not declare its own runner selection.
	RunnerID string
	// ExecutionBaseDir overrides the base directory used to resolve relative
	// runtime execution paths such as workstation workingDirectory values.
	// Empty defaults to the loaded factory directory.
	ExecutionBaseDir string
	// RuntimeMode controls whether the runtime exits on idle completion or
	// stays alive until its context is canceled. Empty defaults to batch mode.
	RuntimeMode interfaces.RuntimeMode
	// Port is the REST API server port. 0 disables the API server.
	Port int
	// Logger is the structured logger. Nil uses a production default.
	Logger *zap.Logger
	// Verbose enables additional runtime diagnostic records. The runtime file
	// log remains enabled regardless of this setting.
	Verbose bool
	// RuntimeInstanceID identifies this runtime process for file-backed logs.
	// Empty generates a UUID.
	RuntimeInstanceID string
	// RuntimeLogDir optionally overrides the default ~/.agent-factory/logs
	// directory. Tests use this to keep file-backed logs isolated.
	RuntimeLogDir string
	// RuntimeLogConfig controls bounded runtime file logging behavior.
	// Zero values use defaults that match the package rolling policy.
	RuntimeLogConfig logging.RuntimeLogConfig
	// WorkFile is an optional path to a FACTORY_REQUEST_BATCH JSON file
	// containing initial work to submit when the factory starts.
	WorkFile string
	// RecordPath is an optional path where the service writes a replay artifact
	// for the current run.
	RecordPath string
	// ReplayPath is an optional path to a replay artifact whose embedded config
	// should be used instead of local factory files.
	ReplayPath string
	// WorkflowID is optional metadata recorded into replay artifacts when the
	// caller selected a specific workflow.
	WorkflowID string
	// MockWorkersConfig is the normalized mock-worker run configuration loaded
	// by the CLI when --with-mock-workers is enabled.
	MockWorkersConfig *factoryconfig.MockWorkersConfig
	// RecordFlushInterval controls how often dirty record-mode artifacts are
	// flushed during execution. Empty uses replay.DefaultRecordFlushInterval.
	RecordFlushInterval time.Duration
	// Clock is an optional runtime time source. Replay mode defaults to a
	// deterministic logical clock when no explicit clock is supplied.
	Clock factory.Clock
	// ExtraOptions are additional factory.FactoryOption values applied when
	// constructing the factory (e.g. factory.WithWorkerExecutor for tests).
	ExtraOptions []factory.FactoryOption
	// SimpleDashboardRenderer is an optional callback for rendering dashboard
	// output from the aggregate runtime snapshot and event-first world view.
	// If nil, no dashboard output is produced.
	SimpleDashboardRenderer SimpleDashboardRenderer
	// APIServerStarter is an optional callback that starts an API server.
	// If nil, no API server is started.
	APIServerStarter APIServerStarter
	// ProviderOverride, when non-nil, replaces the default
	// ScriptWrapProvider for MODEL_WORKER executors. This allows tests
	// to inject a mock Provider and exercise the full worker pipeline
	// (prompt rendering, AgentExecutor, stop-token evaluation) without
	// shelling out to a real CLI tool.
	ProviderOverride workers.Provider
	// ProviderCommandRunnerOverride, when non-nil, is injected into the
	// ScriptWrapProvider used by MODEL_WORKER executors. This preserves the
	// real provider request construction while letting tests fake the CLI
	// subprocess boundary and assert command details, env, stdin, stdout,
	// stderr, and exit failures.
	ProviderCommandRunnerOverride workers.CommandRunner
	// SkipBuiltInRunnerPrerequisiteValidation disables PATH-style built-in
	// runner prerequisite checks during startup. Tests that replace execution
	// with mocks or custom executors use this to exercise service wiring
	// without requiring local AI runner binaries to exist.
	SkipBuiltInRunnerPrerequisiteValidation bool
	// WorkstationLoader, when non-nil, is consulted before falling back
	// to disk when loading workstation AGENTS.md files. Returning
	// (nil, nil) from Load signals "no config available" and the
	// workstation is skipped. Tests use this to inject workstation
	// definitions without requiring files on disk.
	WorkstationLoader factoryconfig.WorkstationLoader
	// CommandRunnerOverride, when non-nil, is injected into SCRIPT_WORKER
	// executors instead of the default ExecCommandRunner. This allows
	// tests to mock os/exec at the CommandRunner level while still
	// exercising the full ScriptExecutor pipeline (arg templates, env
	// merging, exit-code routing).
	CommandRunnerOverride workers.CommandRunner
}

// SubmitWorkRequest submits a canonical work request batch to the factory.
func (fs *FactoryService) SubmitWorkRequest(ctx context.Context, request interfaces.WorkRequest) (interfaces.WorkRequestSubmitResult, error) {
	fs.activationMu.RLock()
	defer fs.activationMu.RUnlock()

	activeFactory := fs.currentFactory()
	if activeFactory == nil {
		return interfaces.WorkRequestSubmitResult{}, fmt.Errorf("factory service runtime is not available")
	}
	return activeFactory.SubmitWorkRequest(ctx, request)
}

// SubscribeFactoryEvents returns canonical factory event history followed by
// live events from the current service-owned runtime.
func (fs *FactoryService) SubscribeFactoryEvents(ctx context.Context) (*interfaces.FactoryEventStream, error) {
	activeFactory := fs.currentFactory()
	if activeFactory == nil {
		return nil, fmt.Errorf("factory service runtime is not available")
	}
	stream, err := activeFactory.SubscribeFactoryEvents(ctx)
	if err != nil {
		return nil, fmt.Errorf("subscribe factory events: %w", err)
	}
	return stream, nil
}

// WaitToComplete returns a channel that is closed when all tokens reach
// terminal or failed places and no dispatches are in flight. Delegates to
// the underlying factory's termination signal.
func (fs *FactoryService) WaitToComplete() <-chan struct{} {
	activeFactory := fs.currentFactory()
	if activeFactory == nil {
		ch := make(chan struct{})
		close(ch)
		return ch
	}
	return activeFactory.WaitToComplete()
}

// GetEngineStateSnapshot returns the factory boundary's aggregate
// observability snapshot.
func (fs *FactoryService) GetEngineStateSnapshot(ctx context.Context) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error) {
	activeFactory := fs.currentFactory()
	if activeFactory == nil {
		return nil, fmt.Errorf("factory service runtime is not available")
	}
	snap, err := activeFactory.GetEngineStateSnapshot(ctx)
	if err != nil {
		return nil, fmt.Errorf("get engine state snapshot: %w", err)
	}
	return snap, nil
}

// Pause pauses the current runtime instance.
func (fs *FactoryService) Pause(ctx context.Context) error {
	activeFactory := fs.currentFactory()
	if activeFactory == nil {
		return fmt.Errorf("factory service runtime is not available")
	}
	if err := activeFactory.Pause(ctx); err != nil {
		return fmt.Errorf("pause factory: %w", err)
	}
	return nil
}

// GetFactoryEvents returns the canonical factory event history.
func (fs *FactoryService) GetFactoryEvents(ctx context.Context) ([]factoryapi.FactoryEvent, error) {
	activeFactory := fs.currentFactory()
	if activeFactory == nil {
		return nil, fmt.Errorf("factory service runtime is not available")
	}
	events, err := activeFactory.GetFactoryEvents(ctx)
	if err != nil {
		return nil, fmt.Errorf("get factory events: %w", err)
	}
	return events, nil
}

func (fs *FactoryService) currentFactory() factory.Factory {
	if fs == nil {
		return nil
	}
	fs.runtimeMu.RLock()
	defer fs.runtimeMu.RUnlock()
	return fs.factory
}

func (fs *FactoryService) currentRuntimeConfig() *factoryconfig.LoadedFactoryConfig {
	if fs == nil {
		return nil
	}
	fs.runtimeMu.RLock()
	defer fs.runtimeMu.RUnlock()
	return fs.runtimeCfg
}

func (fs *FactoryService) workflowID() string {
	if fs == nil || fs.cfg == nil {
		return ""
	}
	fs.runtimeMu.RLock()
	defer fs.runtimeMu.RUnlock()
	return fs.cfg.WorkflowID
}
