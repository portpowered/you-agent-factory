package runtimehost

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory/sessions"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/service/runtimebuild"
	"github.com/portpowered/infinite-you/pkg/workers"
)

// LiveSessionState tracks per-session runtime state attached to live session handles.
type LiveSessionState = liveSessionState

type liveSessionState struct {
	bundle                *factoryRuntimeBundle
	handle                *liveRuntimeHandle
	spec                  *runtimebuild.SessionBuildSpec
	javascriptCheckpoints *factorysessions.JavaScriptCheckpointStore
	responseStreamsOnce   sync.Once
	responseStreams       *factorysessions.SessionResponseStreamSet
}

// NewLiveSessionState constructs the runtimehost-owned handle stored on a live
// Factory Session. Composition roots must not manufacture a service-owned
// lookalike because runtimehost session operations require this concrete type.
func NewLiveSessionState(bundle *factoryRuntimeBundle, spec *runtimebuild.SessionBuildSpec) *LiveSessionState {
	state := &liveSessionState{bundle: bundle, spec: spec}
	if bundle != nil {
		state.handle = &liveRuntimeHandle{Bundle: bundle}
	}
	return state
}

type hostCoordinatorPolicy struct {
	dir                     string
	executionBaseDir        string
	runtimeMode             interfaces.RuntimeMode
	port                    int
	verbose                 bool
	runtimeInstanceID       string
	workFile                string
	workflowID              string
	mockWorkersConfig       *factoryconfig.MockWorkersConfig
	simpleDashboardRenderer SimpleDashboardRenderer
	apiServerStarter        APIServerStarter
	apiServerReady          <-chan struct{}
	workstationLoader       factoryconfig.WorkstationLoader
	modelCacheDir           string
	runnerID                string
	providerOverride        workers.Provider
}

// CoordinatorPolicy captures normalized host coordinator settings derived from Config.
type CoordinatorPolicy = hostCoordinatorPolicy

// FactoryDir returns the coordinator factory root directory.
func (p CoordinatorPolicy) FactoryDir() string {
	return p.dir
}

// MockWorkersConfig returns mock worker config from coordinator policy for tests.
func (p CoordinatorPolicy) MockWorkersConfig() *factoryconfig.MockWorkersConfig {
	return p.mockWorkersConfig
}

func (h *Host) coordinatorPolicy() hostCoordinatorPolicy {
	if h == nil {
		return hostCoordinatorPolicy{}
	}
	if hasExplicitHostCoordinatorPolicy(h.policy) {
		return h.policy
	}
	return CoordinatorPolicyFromConfig(h.cfg)
}

func hasExplicitHostCoordinatorPolicy(policy hostCoordinatorPolicy) bool {
	return hasExplicitHostCoordinatorValuePolicy(policy) || hasExplicitHostCoordinatorReferencePolicy(policy)
}

func hasExplicitHostCoordinatorValuePolicy(policy hostCoordinatorPolicy) bool {
	return policy.dir != "" ||
		policy.executionBaseDir != "" ||
		policy.runtimeMode != "" ||
		policy.port != 0 ||
		policy.verbose ||
		policy.runtimeInstanceID != "" ||
		policy.workFile != "" ||
		policy.workflowID != "" ||
		policy.modelCacheDir != "" ||
		policy.runnerID != ""
}

func hasExplicitHostCoordinatorReferencePolicy(policy hostCoordinatorPolicy) bool {
	return policy.mockWorkersConfig != nil ||
		policy.simpleDashboardRenderer != nil ||
		policy.apiServerStarter != nil ||
		policy.apiServerReady != nil ||
		policy.workstationLoader != nil ||
		policy.providerOverride != nil
}

func CoordinatorPolicyFromConfig(cfg *Config) hostCoordinatorPolicy {
	if cfg == nil {
		return hostCoordinatorPolicy{}
	}
	return hostCoordinatorPolicy{
		dir:                     cfg.Dir,
		executionBaseDir:        cfg.ExecutionBaseDir,
		runtimeMode:             cfg.RuntimeMode,
		port:                    cfg.Port,
		verbose:                 cfg.Verbose,
		runtimeInstanceID:       cfg.RuntimeInstanceID,
		workFile:                cfg.WorkFile,
		workflowID:              cfg.WorkflowID,
		mockWorkersConfig:       cfg.MockWorkersConfig,
		simpleDashboardRenderer: cfg.SimpleDashboardRenderer,
		apiServerStarter:        cfg.APIServerStarter,
		apiServerReady:          cfg.APIServerReady,
		workstationLoader:       cfg.WorkstationLoader,
		modelCacheDir:           cfg.ModelCacheDir,
		runnerID:                cfg.RunnerID,
		providerOverride:        cfg.ProviderOverride,
	}
}

// RuntimeLogDiagnostics describes the active runtime log selected during host construction.
type RuntimeLogDiagnostics struct {
	Path                string
	RootDir             string
	StartTimeUTC        time.Time
	MetricsPath         string
	MetricsRootDir      string
	MetricsStartTimeUTC time.Time
}

// RuntimeLogDiagnostics returns the selected runtime log metadata for startup
// diagnostics without exposing the sink writer.
func (h *Host) RuntimeLogDiagnostics() RuntimeLogDiagnostics {
	bundle := h.currentRuntimeBundle()
	if bundle == nil || bundle.LogSink == nil {
		return RuntimeLogDiagnostics{}
	}
	return RuntimeLogDiagnostics{
		Path:                bundle.LogSink.Path(),
		RootDir:             bundle.LogSink.RootDir(),
		StartTimeUTC:        bundle.LogSink.StartTimeUTC(),
		MetricsPath:         runtimeMetricsPath(bundle.MetricsSink),
		MetricsRootDir:      runtimeMetricsRootDir(bundle.MetricsSink),
		MetricsStartTimeUTC: runtimeMetricsStartTime(bundle.MetricsSink),
	}
}

func runtimeMetricsPath(sink *logging.RuntimeMetricsSink) string {
	if sink == nil {
		return ""
	}
	return sink.Path()
}

func runtimeMetricsRootDir(sink *logging.RuntimeMetricsSink) string {
	if sink == nil {
		return ""
	}
	return sink.RootDir()
}

func runtimeMetricsStartTime(sink *logging.RuntimeMetricsSink) time.Time {
	if sink == nil {
		return time.Time{}
	}
	return sink.StartTimeUTC()
}

func (h *Host) defaultSessionClosedDuringStartup() bool {
	if h == nil || runtimeModeOrDefault(h.cfg.RuntimeMode) != interfaces.RuntimeModeService {
		return false
	}
	return h.sessionByID(DefaultFactorySessionID) == nil
}

func (h *Host) handleDefaultRuntimeStartFailure(
	ctx context.Context,
	currentRuntime *liveRuntimeHandle,
	startErr error,
) error {
	if h.defaultSessionClosedDuringStartup() {
		h.clearRunState()
		_ = h.StopLiveRuntime(currentRuntime)
		return nil
	}
	h.clearRunState()
	h.unregisterLiveSession(DefaultFactorySessionID)
	stopErr := h.StopLiveRuntime(currentRuntime)
	if isCanceledServiceStartup(ctx, startErr) {
		if stopErr != nil && !errors.Is(stopErr, context.Canceled) {
			return stopErr
		}
		return nil
	}
	if stopErr != nil && !errors.Is(stopErr, context.Canceled) {
		return errors.Join(fmt.Errorf("start runtime: %w", startErr), stopErr)
	}
	return fmt.Errorf("start runtime: %w", startErr)
}
