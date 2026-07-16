package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory"
	factory_context "github.com/portpowered/infinite-you/pkg/factory/context"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factoryevents "github.com/portpowered/infinite-you/pkg/factory/events"
	factoryingest "github.com/portpowered/infinite-you/pkg/factory/ingest"
	"github.com/portpowered/infinite-you/pkg/factory/replay"
	"github.com/portpowered/infinite-you/pkg/factory/runtime"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	modelhost "github.com/portpowered/infinite-you/pkg/models/host"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformmetrics "github.com/portpowered/infinite-you/pkg/platform/metrics"
	"github.com/portpowered/infinite-you/pkg/workers"
	workerprovider "github.com/portpowered/infinite-you/pkg/workers/provider"
	workerrunner "github.com/portpowered/infinite-you/pkg/workers/runner"
	"go.uber.org/zap"
)

const defaultSessionID = "~default"

// BuildInput is the immutable input for constructing one hosted runtime bundle.
type BuildInput struct {
	Dir                           string
	FolderPath                    string
	SessionID                     string
	Config                        Config
	LoadedFactoryCfg              *factoryconfig.LoadedFactoryConfig
	BaseLogger                    *zap.Logger
	RuntimeInstanceID             string
	BackendScopeID                string
	Clock                         factory.Clock
	RecordPath                    string
	WorkflowID                    string
	ProviderOverride              workers.Provider
	ProviderCommandRunner         workers.CommandRunner
	CommandRunnerOverride         workers.CommandRunner
	AdditionalFactoryOpts         []factory.FactoryOption
	LoadWorkerOpts                func(*factoryevents.FactoryEventHistory, *zap.Logger) ([]factory.FactoryOption, error)
	PrefetchedLocalModels         LocalModelDomain
	InferenceProgressPublisher    workerprovider.InferenceProgressPublisher
	InferenceProgressPublisherSet bool
	DispatchCompleted             func(string)
}

// Build constructs one hosted runtime bundle from an explicit build input.
func Build(ctx context.Context, input BuildInput) (*Bundle, error) {
	sessionID := strings.TrimSpace(input.SessionID)
	if sessionID == "" {
		sessionID = defaultSessionID
	}
	logSink, runtimeInstanceID, err := buildRuntimeLogSink(input.Config, input.BaseLogger, input.RuntimeInstanceID)
	if err != nil {
		return nil, err
	}
	logger := newSessionLogger(
		runtimeSessionBaseLogger(input.BaseLogger, logSink),
		sessionID,
		input.FolderPath,
		input.Dir,
	)
	metricsSink, err := buildRuntimeMetricsSink(
		input.Config,
		sessionID,
		runtimeInstanceID,
		input.FolderPath,
		input.Dir,
	)
	if err != nil {
		logger.Warn(
			"runtime metrics sink unavailable; continuing without metrics",
			zap.Error(err),
			zap.String("runtime_instance_id", runtimeInstanceID),
			zap.String("runtime_metrics_root_dir", strings.TrimSpace(input.Config.RuntimeMetricsDir)),
		)
		metricsSink = nil
	}
	bundleBuilt := false
	defer func() {
		if !bundleBuilt {
			_ = CloseBundleSinks(logSink, metricsSink)
		}
	}()
	if runtimeInstanceID != "" {
		input.RuntimeInstanceID = runtimeInstanceID
	}

	mapper := factoryconfig.ConfigMapper{}
	net, err := mapper.Map(ctx, input.LoadedFactoryCfg.FactoryConfig())
	if err != nil {
		logger.Error("failed to map factory config", zap.Error(err))
		return nil, fmt.Errorf("map factory config: %w", err)
	}

	effectiveFactoryRunnerID := effectiveFactoryRunnerID(input.Config.RunnerID, input.LoadedFactoryCfg.FactoryConfig())
	eventHistory := factoryevents.NewFactoryEventHistory(net, input.Clock.Now, input.LoadedFactoryCfg)
	eventHistory.SetFactoryRunnerOverride(effectiveFactoryRunnerID)
	if editableFactory, err := editableEventFactorySnapshot(input); err != nil {
		logger.Warn("editable factory event snapshot unavailable; using runtime-thin factory event payload", zap.Error(err))
	} else {
		eventHistory.SetInitialStructureFactory(editableFactory)
	}
	localModels := input.PrefetchedLocalModels
	if localModels.Manager == nil {
		localModels, err = modelhost.NewLocalDomain(LocalModelDomainDependencies(input.Config))
		if err != nil {
			return nil, err
		}
	}

	var workerOpts []factory.FactoryOption
	if input.LoadWorkerOpts != nil {
		workerOpts, err = input.LoadWorkerOpts(eventHistory, logger)
		if err != nil {
			return nil, err
		}
	}

	bundleBuilt = true
	return assembleRuntimeBundle(input, logger, logSink, metricsSink, net, eventHistory, localModels, workerOpts)
}

func editableEventFactorySnapshot(input BuildInput) (*interfaces.FactorySnapshot, error) {
	if input.LoadedFactoryCfg == nil || input.LoadedFactoryCfg.FactoryConfig() == nil {
		return nil, fmt.Errorf("loaded factory config is unavailable")
	}
	factoryCfg, err := factoryconfig.CloneFactoryConfig(input.LoadedFactoryCfg.FactoryConfig())
	if err != nil {
		return nil, fmt.Errorf("clone factory config: %w", err)
	}
	if err := factoryconfig.ApplySupportedPortableBundledFiles(input.LoadedFactoryCfg.FactoryDir(), factoryCfg, true, false); err != nil {
		return nil, fmt.Errorf("inline portable bundled files: %w", err)
	}
	if err := factoryconfig.ApplySharedFactoryStarterWork(input.LoadedFactoryCfg.FactoryDir(), factoryCfg); err != nil {
		return nil, fmt.Errorf("inline shared factory starter work: %w", err)
	}
	snapshot, err := replay.FactorySnapshotFromRuntimeConfig(
		input.LoadedFactoryCfg.FactoryDir(),
		factoryCfg,
		input.LoadedFactoryCfg,
		replay.WithFactorySnapshotSourceDirectory(input.LoadedFactoryCfg.FactoryDir()),
		replay.WithFactorySnapshotWorkflowID(input.WorkflowID),
	)
	if err != nil {
		return nil, err
	}
	return snapshot, nil
}

func assembleRuntimeBundle(
	input BuildInput,
	logger *zap.Logger,
	logSink *logging.RuntimeLogSink,
	metricsSink *platformmetrics.RuntimeMetricsSink,
	net *state.Net,
	eventHistory *factoryevents.FactoryEventHistory,
	localModels LocalModelDomain,
	workerOpts []factory.FactoryOption,
) (*Bundle, error) {
	recording, err := buildRuntimeRecorder(
		input.Config,
		input.LoadedFactoryCfg.FactoryDir(),
		input.LoadedFactoryCfg.FactoryConfig(),
		input.LoadedFactoryCfg,
		input.Clock,
		input.RecordPath,
		input.WorkflowID,
	)
	if err != nil {
		return nil, err
	}

	bundle := &Bundle{
		Dir:               input.Dir,
		FolderPath:        input.FolderPath,
		RuntimeInstanceID: input.RuntimeInstanceID,
		BackendScopeID:    strings.TrimSpace(input.BackendScopeID),
		StartedAtUTC:      input.Clock.Now().UTC(),
		EventHistory:      eventHistory,
		Net:               net,
		RuntimeCfg:        input.LoadedFactoryCfg,
		ModelResources:    localModels.Resources,
		ModelAssets:       localModels.Assets,
		LocalModels:       localModels.Manager,
		LocalModelRuntime: localModels.Runtime,
		ModelHost:         localModels.Host,
		LeaseExecution:    localModels.LeaseExecution,
		Logger:            logger,
		LogSink:           logSink,
		MetricsSink:       metricsSink,
		Recording:         recording,
		RecordPath:        input.RecordPath,
		dispatchCompleted: input.DispatchCompleted,
	}
	opts := []factory.FactoryOption{
		factory.WithNet(net),
		factory.WithRuntimeMode(input.Config.RuntimeMode),
		factory.WithLogger(logging.NewZapLogger(logger, input.Config.Verbose)),
		factory.WithRuntimeConfig(input.LoadedFactoryCfg),
		factory.WithWorkflowContext(runtimeWorkflowContext(input.LoadedFactoryCfg.FactoryConfig(), input.SessionID)),
		factory.WithClock(input.Clock),
		factory.WithFactoryEventHistory(eventHistory),
		factory.WithSubmissionRecorder(bundle.recordSubmissionMetric),
		factory.WithDispatchRecorder(bundle.recordDispatchMetric),
		factory.WithCompletionRecorder(bundle.recordCompletionMetrics),
	}
	if input.RecordPath != "" {
		opts = append(opts, factory.WithFactoryEventRecorder(func(event interfaces.FactoryEvent) {
			if recording == nil {
				return
			}
			recording.RecordEvent(event)
		}))
	}
	opts = append(opts, input.AdditionalFactoryOpts...)
	opts = append(opts, workerOpts...)
	opts = append(opts, input.Config.ExtraOptions...)

	activeFactory, err := runtime.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("create factory: %w", err)
	}
	listener, err := buildRuntimeListener(input.Dir, activeFactory, logger, net)
	if err != nil {
		return nil, err
	}

	bundle.Factory = activeFactory
	bundle.Listener = listener
	return bundle, nil
}

func buildRuntimeListener(
	factoryDir string,
	activeFactory factory.Factory,
	logger *zap.Logger,
	net *state.Net,
) (*factoryingest.FileWatcher, error) {
	inputsDir := filepath.Join(factoryDir, interfaces.InputsDir)
	if !dirExists(inputsDir) {
		if err := os.MkdirAll(inputsDir, 0o755); err != nil {
			return nil, fmt.Errorf("create inputs dir: %w", err)
		}
	} else {
		logger.Info("using inputs/ directory", zap.String("dir", inputsDir))
	}
	return factoryingest.NewFileWatcher(
		inputsDir,
		activeFactory,
		logger,
		factoryingest.WithKnownWorkStates(state.ValidStatesByType(net.WorkTypes)),
	), nil
}

func newSessionLogger(base *zap.Logger, sessionID string, folderPath string, factoryDir string) *zap.Logger {
	if base == nil {
		base = zap.NewNop()
	}
	return base.With(
		zap.String("session_id", sessionID),
		zap.String("folder_path", folderPath),
		zap.String("factory_dir", factoryDir),
	)
}

func runtimeWorkflowContext(cfg *interfaces.FactoryConfig, sessionID string) *factory_context.FactoryContext {
	projectID := factory_context.DefaultProjectID
	if cfg != nil && cfg.Project != "" {
		projectID = factory_context.ResolveProjectID(cfg.Project, nil, nil)
	}
	if strings.TrimSpace(sessionID) == "" {
		sessionID = defaultSessionID
	}
	return &factory_context.FactoryContext{
		ProjectID: projectID,
		EnvVars:   make(map[string]string),
		SessionID: sessionID,
	}
}

func effectiveFactoryRunnerID(override string, factoryCfg *interfaces.FactoryConfig) string {
	if runner := workerrunner.NormalizeRunnerID(override); runner != "" {
		return runner
	}
	if factoryCfg == nil {
		return ""
	}
	return workerrunner.NormalizeRunnerID(factoryCfg.Runner)
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func buildRuntimeLogSink(
	cfg Config,
	baseLogger *zap.Logger,
	runtimeInstanceID string,
) (*logging.RuntimeLogSink, string, error) {
	if baseLogger == nil {
		baseLogger = zap.NewNop()
	}
	if strings.TrimSpace(runtimeInstanceID) == "" {
		runtimeInstanceID = uuid.NewString()
	}
	if !runtimeFileLoggingEnabled(cfg.RuntimeFileLoggingPolicy) {
		return nil, runtimeInstanceID, nil
	}
	logSink, err := logging.BuildRuntimeLogger(baseLogger, runtimeInstanceID, cfg.RuntimeLogDir, cfg.RuntimeLogConfig)
	if err != nil {
		return nil, runtimeInstanceID, fmt.Errorf("build runtime logger: %w", err)
	}
	return logSink, runtimeInstanceID, nil
}

func runtimeFileLoggingEnabled(policy RuntimeFileLoggingPolicy) bool {
	switch policy {
	case "", RuntimeFileLoggingPolicyEnabled:
		return true
	case RuntimeFileLoggingPolicyDisabled:
		return false
	default:
		return true
	}
}

func runtimeMetricsEnabled(policy RuntimeMetricsPolicy) bool {
	switch policy {
	case "", RuntimeMetricsPolicyEnabled:
		return true
	case RuntimeMetricsPolicyDisabled:
		return false
	default:
		return true
	}
}

func runtimeSessionBaseLogger(baseLogger *zap.Logger, logSink *logging.RuntimeLogSink) *zap.Logger {
	if logSink != nil {
		return logSink.Logger()
	}
	if baseLogger != nil {
		return baseLogger
	}
	return zap.NewNop()
}

func buildRuntimeMetricsSink(
	cfg Config,
	sessionID string,
	runtimeInstanceID string,
	folderPath string,
	factoryDir string,
) (*platformmetrics.RuntimeMetricsSink, error) {
	if !runtimeMetricsEnabled(cfg.RuntimeMetricsPolicy) {
		return nil, nil
	}
	metricsSink, err := platformmetrics.BuildRuntimeMetricsSink(
		sessionID,
		runtimeInstanceID,
		folderPath,
		factoryDir,
		cfg.RuntimeMetricsDir,
		cfg.RuntimeMetricsConfig,
	)
	if err != nil {
		return nil, fmt.Errorf("build runtime metrics sink: %w", err)
	}
	return metricsSink, nil
}

// CloseBundleSinks closes runtime log and metrics sinks created during bundle build.
func CloseBundleSinks(logSink *logging.RuntimeLogSink, metricsSink *platformmetrics.RuntimeMetricsSink) error {
	var errs []error
	if logSink != nil {
		if err := logSink.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if metricsSink != nil {
		if err := metricsSink.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func buildRuntimeRecorder(
	cfg Config,
	factoryDir string,
	factoryCfg *interfaces.FactoryConfig,
	runtimeCfg interfaces.RuntimeDefinitionLookup,
	clock factory.Clock,
	recordPath string,
	workflowID string,
) (*replay.Recorder, error) {
	recordingArtifact, err := newRecordingArtifact(recordPath, workflowID, factoryDir, factoryCfg, runtimeCfg, clock)
	if err != nil || recordingArtifact == nil {
		return nil, err
	}
	recording, err := replay.NewRecorder(
		recordPath,
		recordingArtifact,
		replay.WithFlushInterval(cfg.RecordFlushInterval),
	)
	if err != nil {
		return nil, fmt.Errorf("create replay recorder: %w", err)
	}
	return recording, nil
}

func newRecordingArtifact(
	recordPath string,
	workflowID string,
	factoryDir string,
	factoryCfg *interfaces.FactoryConfig,
	runtimeCfg interfaces.RuntimeDefinitionLookup,
	clock factory.Clock,
) (*interfaces.ReplayArtifact, error) {
	if recordPath == "" {
		return nil, nil
	}
	now := factory.EnsureClock(clock).Now().UTC()
	factorySnapshot, err := replay.FactorySnapshotFromRuntimeConfig(
		factoryDir,
		factoryCfg,
		runtimeCfg,
		replay.WithFactorySnapshotSourceDirectory(factoryDir),
		replay.WithFactorySnapshotWorkflowID(workflowID),
	)
	if err != nil {
		return nil, fmt.Errorf("build replay artifact config: %w", err)
	}
	return replay.NewEventLogArtifact(now, factorySnapshot, &interfaces.ReplayWallClockMetadata{
		StartedAt: now,
	}, interfaces.ReplayDiagnostics{})
}

// RuntimeLogger returns the bundle logger or a nop logger when unset.
func (r *Bundle) RuntimeLogger() *zap.Logger {
	if r == nil || r.Logger == nil {
		return zap.NewNop()
	}
	return r.Logger
}
