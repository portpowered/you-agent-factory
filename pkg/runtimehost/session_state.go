package runtimehost

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/portpowered/infinite-you/pkg/cli/dashboardrender"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factory/projections"
	factoryservice "github.com/portpowered/infinite-you/pkg/factory/service"
	"github.com/portpowered/infinite-you/pkg/factorysessions"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/service/runtimebuild"
	"github.com/portpowered/infinite-you/pkg/workers"
	"go.uber.org/zap"
)


type liveSessionState struct {
	bundle                *factoryRuntimeBundle
	handle                *liveRuntimeHandle
	spec                  *runtimebuild.SessionBuildSpec
	javascriptCheckpoints *factorysessions.JavaScriptCheckpointStore
	responseStreamsOnce   sync.Once
	responseStreams       *factorysessions.SessionResponseStreamSet
}

// NewStartupLiveSessionHandle constructs the default session handle attached
// during startup core composition.
func NewStartupLiveSessionHandle(bundle *factoryservice.Bundle, spec *runtimebuild.SessionBuildSpec) any {
	if spec == nil {
		return &liveSessionState{bundle: bundle}
	}
	copied := *spec
	return &liveSessionState{bundle: bundle, spec: &copied}
}

type hostCoordinatorPolicy struct {
	dir                           string
	executionBaseDir              string
	runtimeMode                   interfaces.RuntimeMode
	port                          int
	verbose                       bool
	runtimeInstanceID             string
	workFile                      string
	workflowID                    string
	mockWorkersConfig             *factoryconfig.MockWorkersConfig
	simpleDashboardRenderer       SimpleDashboardRenderer
	apiServerStarter              APIServerStarter
	apiServerReady                <-chan struct{}
	workstationLoader             factoryconfig.WorkstationLoader
	modelCacheDir                 string
	runnerID                      string
	providerOverride              workers.Provider
	providerCommandRunnerOverride workers.CommandRunner
	commandRunnerOverride         workers.CommandRunner
}

const (
	runtimeMetricLifecycleStarted     = "runtime.lifecycle.started"
	runtimeMetricLifecycleStopped     = "runtime.lifecycle.stopped"
	runtimeMetricStateActive          = "runtime.state.active"
	runtimeMetricStateIdle            = "runtime.state.idle"
	runtimeMetricStatePaused          = "runtime.state.paused"
	runtimeMetricStateFailed          = "runtime.state.failed"
	runtimeMetricQueueInFlight        = "runtime.queue.in_flight"
	runtimeMetricQueueSubmissionCount = "queue.submission_count"
	runtimeMetricDispatchStarted      = "dispatch.started"
	runtimeMetricDispatchComplete     = "dispatch.completed"
	runtimeMetricDispatchDuration     = "dispatch.duration"
	runtimeMetricDispatchRetries      = "dispatch.retry_count"
	runtimeMetricDispatchCost         = "dispatch.cost"
	runtimeMetricProviderRequest      = "provider.requested"
	runtimeMetricProviderComplete     = "provider.completed"
	runtimeMetricProviderFailed       = "provider.failed"
	runtimeMetricProviderDuration     = "provider.duration"
	runtimeMetricProviderInputTok     = "provider.input_tokens"
	runtimeMetricProviderOutputTok    = "provider.output_tokens"
	runtimeMetricProviderCost         = "provider.cost"
	runtimeMetricScriptStarted        = "script.started"
	runtimeMetricScriptComplete       = "script.completed"
	runtimeMetricScriptDuration       = "script.duration"
	runtimeMetricScriptTimedOut       = "script.timed_out"
	runtimeMetricScriptFailed         = "script.failed"
)

// CoordinatorPolicy captures normalized host coordinator settings derived from Config.
type CoordinatorPolicy = hostCoordinatorPolicy

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
		policy.providerOverride != nil ||
		policy.providerCommandRunnerOverride != nil ||
		policy.commandRunnerOverride != nil
}

func CoordinatorPolicyFromConfig(cfg *Config) hostCoordinatorPolicy {
	if cfg == nil {
		return hostCoordinatorPolicy{}
	}
	return hostCoordinatorPolicy{
		dir:                           cfg.Dir,
		executionBaseDir:              cfg.ExecutionBaseDir,
		runtimeMode:                   cfg.RuntimeMode,
		port:                          cfg.Port,
		verbose:                       cfg.Verbose,
		runtimeInstanceID:             cfg.RuntimeInstanceID,
		workFile:                      cfg.WorkFile,
		workflowID:                    cfg.WorkflowID,
		mockWorkersConfig:             cfg.MockWorkersConfig,
		simpleDashboardRenderer:       cfg.SimpleDashboardRenderer,
		apiServerStarter:              cfg.APIServerStarter,
		apiServerReady:                cfg.APIServerReady,
		workstationLoader:             cfg.WorkstationLoader,
		modelCacheDir:                 cfg.ModelCacheDir,
		runnerID:                      cfg.RunnerID,
		providerOverride:              cfg.ProviderOverride,
		providerCommandRunnerOverride: cfg.ProviderCommandRunnerOverride,
		commandRunnerOverride:         cfg.CommandRunnerOverride,
	}
}

func (h *Host) dashboardLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.renderDashboard(ctx)
		}
	}
}

func (h *Host) renderDashboard(ctx context.Context) {
	now := factory.EnsureClock(h.clock).Now()
	input, err := h.buildSimpleDashboardRenderInput(ctx, now)
	if err != nil {
		if h.logger != nil {
			h.logger.Error("simple dashboard render failed", zap.Error(err))
		}
		return
	}
	h.cfg.SimpleDashboardRenderer(input)
}

func (h *Host) buildSimpleDashboardRenderInput(ctx context.Context, now time.Time) (SimpleDashboardRenderInput, error) {
	es, err := h.GetEngineStateSnapshot(ctx)
	if err != nil {
		return SimpleDashboardRenderInput{}, err
	}
	renderData, err := h.simpleDashboardRenderData(ctx, es.TickCount, es.ActiveThrottlePauses)
	if err != nil {
		return SimpleDashboardRenderInput{}, err
	}
	return SimpleDashboardRenderInput{
		EngineState: *es,
		RenderData:  renderData,
		Now:         now,
	}, nil
}

func (h *Host) simpleDashboardRenderData(
	ctx context.Context,
	selectedTick int,
	activeThrottlePauses []interfaces.ActiveThrottlePause,
) (dashboardrender.SimpleDashboardRenderData, error) {
	events, err := h.GetFactoryEvents(ctx)
	if err != nil {
		return dashboardrender.SimpleDashboardRenderData{}, err
	}
	worldState, err := projections.ReconstructFactoryWorldState(events, selectedTick)
	if err != nil {
		return dashboardrender.SimpleDashboardRenderData{}, err
	}
	renderData := dashboardrender.SimpleDashboardRenderDataFromWorldState(worldState)
	renderData.ActiveThrottlePauses = projections.ProjectActiveThrottlePauses(worldState.Topology, activeThrottlePauses)
	return renderData, nil
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
		_ = h.stopLiveRuntime(currentRuntime)
		return nil
	}
	h.clearRunState()
	h.unregisterLiveSession(DefaultFactorySessionID)
	stopErr := h.stopLiveRuntime(currentRuntime)
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

const (
	modelPullMetricAttempts      = "managed_runtime.pull.attempts"
	modelPullMetricSuccess       = "managed_runtime.pull.success"
	modelPullMetricFailure       = "managed_runtime.pull.failure"
	modelPullMetricSourceFailure = "managed_runtime.pull.source_failure"
)

func (h *Host) modelPullMetricsRecorder() ModelPullMetricsRecorder {
	if h == nil || h.cfg == nil {
		return nil
	}
	return h.cfg.ModelPullMetricsRecorder
}
