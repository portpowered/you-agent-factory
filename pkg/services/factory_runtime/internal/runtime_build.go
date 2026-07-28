package internal

import (
	"context"
	"fmt"
	"strings"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	runtimebuild "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/instance_host/build"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/scheduler"
	factoryhost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/host"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

type ProgressPublisherFactory func(string) workers.ProgressPublisher
type DispatchCompletionFactory func(string) func(string)
type InitialFactorySnapshotFactory = factorydefinitions.InitialFactorySnapshotFactory

// newRuntimeBuild constructs the canonical runtime-build service from decomposed process
// configuration and domain collaborators.
// backendsizecheck:ignore-function service-ownership migration preserves this orchestration flow; extract focused helpers and remove this exemption.
// pkgmaintcheck:ignore-function-lines service-ownership migration preserves this orchestration flow; extract focused helpers and remove this exemption.
func NewRuntimeBuild(
	defaultWorkerModelProvider string,
	defaultWorkerModel string,
	applyOperatorDefaults bool,
	recordPath string,
	workflowID string,
	defaultSessionID string,
	workstationLoader factorydefinitions.WorkstationLoader,
	providerOverride workers.Provider,
	providerCommandRunner workers.CommandRunner,
	scriptCommandRunner workers.CommandRunner,
	mockWorkersConfig *workers.MockWorkersConfig,
	runtimeMode factorydefinitions.RuntimeMode,
	runtimeScheduler scheduler.Scheduler,
	workerExecutors map[string]workers.WorkerExecutor,
	workerExecutorDecorator func(string, workers.WorkerExecutor) workers.WorkerExecutor,
	inlineDispatch bool,
	submissionRecorder recordings.SubmissionRecorder,
	dispatchRecorder recordings.DispatchRecorder,
	runtimeLogDir string,
	runtimeLogConfig factory.RuntimeLogStorageConfig,
	runtimeFileLoggingPolicy RuntimeFileLoggingPolicy,
	runtimeMetricsPolicy RuntimeMetricsPolicy,
	runtimeMetricsDir string,
	runtimeMetricsConfig factory.RuntimeMetricsStorageConfig,
	recordFlushInterval time.Duration,
	backendScopeID string,
	factoryRunnerID string,
	verbose bool,
	skipRunnerPrerequisiteValidation bool,
	invocationSkipPermissionsOverride *bool,
	clock factory.Clock,
	baseLogger *zap.Logger,
	runtimeFactory *RuntimeFactory,
	workerExecution workers.RuntimeService,
	runtimeExecutorsFactory factory.WorkersRuntimeExecutorsFactory,
	mockCommandRunnerFactory factory.WorkersMockCommandRunnerFactory,
	progressFactory ProgressPublisherFactory,
	completionFactory DispatchCompletionFactory,
	petriMutationRecorder factory.PetriMutationRecorder,
	worldStateProjector factory.WorldStateProjector,
	runtimeLedgerFactory factory.RuntimeLedgerFactory,
	runtimeRecorderFactory recordings.RuntimeRecorderFactory,
	loadFactory factory.LoadedFactoryLoader,
	initialFactorySnapshot InitialFactorySnapshotFactory,
) (*runtimebuild.Service, error) {
	if runtimeFactory == nil {
		return nil, fmt.Errorf("Factory Runtime factory is required")
	}
	return runtimebuild.New(
		defaultWorkerModelProvider,
		defaultWorkerModel,
		applyOperatorDefaults,
		recordPath,
		workflowID,
		workstationLoader,
		loadFactory,
		providerOverride,
		providerCommandRunner,
		scriptCommandRunner,
		mockWorkersConfig,
		func(
			config *workers.MockWorkersConfig,
			runtimeConfig factorydefinitions.RuntimeDefinitionLookup,
			next workers.CommandRunner,
		) workers.CommandRunner {
			if mockCommandRunnerFactory == nil {
				return next
			}
			return mockCommandRunnerFactory(config, runtimeConfig, next)
		},
		clock,
		runtimeFactory.newID,
		baseLogger,
		func(ctx context.Context, spec runtimebuild.SessionBuildSpec) (*factoryhost.Bundle, error) {
			providerCommandRunner := spec.ProviderCommandRunner
			var progressPublisher workers.ProgressPublisher
			progressPublisherSet := false
			if progressFactory != nil {
				progressPublisher = progressFactory(spec.SessionID)
				progressPublisherSet = true
			}
			runtimeWorkers, err := workerServiceWithProgress(
				workerExecution,
				spec.ProviderCommandRunner,
				spec.CommandRunnerOverride,
				providerCommandRunner,
				progressPublisher,
				progressPublisherSet,
				spec.BaseLogger,
				verbose,
				runtimeFactory.loggerFactory,
			)
			if err != nil {
				return nil, fmt.Errorf("construct runtime Worker service: %w", err)
			}
			var dispatchCompleted func(string)
			if completionFactory != nil {
				dispatchCompleted = completionFactory(spec.SessionID)
			}
			return buildBundle(
				ctx,
				spec,
				runtimeLogDir,
				runtimeLogConfig,
				runtimeFileLoggingPolicy,
				runtimeMetricsPolicy,
				runtimeMetricsDir,
				runtimeMetricsConfig,
				recordFlushInterval,
				defaultSessionID,
				runtimeMode,
				runtimeScheduler,
				workerExecutors,
				workerExecutorDecorator,
				inlineDispatch,
				submissionRecorder,
				dispatchRecorder,
				backendScopeID,
				factoryRunnerID,
				verbose,
				skipRunnerPrerequisiteValidation,
				invocationSkipPermissionsOverride,
				runtimeWorkers,
				runtimeExecutorsFactory,
				progressPublisher,
				runtimeFactory,
				dispatchCompleted,
				worldStateProjector,
				runtimeLedgerFactory,
				runtimeRecorderFactory,
				initialFactorySnapshot,
			)
		},
		petriMutationRecorder,
	)
}

// backendsizecheck:ignore-function service-ownership migration preserves this orchestration flow; extract focused helpers and remove this exemption.
// pkgmaintcheck:ignore-function-lines service-ownership migration preserves this orchestration flow; extract focused helpers and remove this exemption.
func buildBundle(
	ctx context.Context,
	spec runtimebuild.SessionBuildSpec,
	runtimeLogDir string,
	runtimeLogConfig factory.RuntimeLogStorageConfig,
	runtimeFileLoggingPolicy RuntimeFileLoggingPolicy,
	runtimeMetricsPolicy RuntimeMetricsPolicy,
	runtimeMetricsDir string,
	runtimeMetricsConfig factory.RuntimeMetricsStorageConfig,
	recordFlushInterval time.Duration,
	defaultSessionID string,
	runtimeMode factorydefinitions.RuntimeMode,
	runtimeScheduler scheduler.Scheduler,
	workerExecutors map[string]workers.WorkerExecutor,
	workerExecutorDecorator func(string, workers.WorkerExecutor) workers.WorkerExecutor,
	inlineDispatch bool,
	submissionRecorder recordings.SubmissionRecorder,
	dispatchRecorder recordings.DispatchRecorder,
	backendScopeID string,
	factoryRunnerID string,
	verbose bool,
	skipRunnerPrerequisiteValidation bool,
	invocationSkipPermissionsOverride *bool,
	workerExecution workers.RuntimeService,
	runtimeExecutorsFactory factory.WorkersRuntimeExecutorsFactory,
	progressPublisher workers.ProgressPublisher,
	runtimeFactory *RuntimeFactory,
	dispatchCompleted func(string),
	worldStateProjector factory.WorldStateProjector,
	runtimeLedgerFactory factory.RuntimeLedgerFactory,
	runtimeRecorderFactory recordings.RuntimeRecorderFactory,
	initialFactorySnapshot InitialFactorySnapshotFactory,
) (*factoryhost.Bundle, error) {
	loadedFactoryCfg, ok := spec.LoadedFactoryCfg.(factorydefinitions.LoadedFactorySource)
	if !ok || loadedFactoryCfg == nil {
		return nil, fmt.Errorf("loaded Factory config is required")
	}
	sessionID := strings.TrimSpace(spec.SessionID)
	if sessionID == "" {
		sessionID = strings.TrimSpace(defaultSessionID)
	}
	if sessionID == "" {
		return nil, fmt.Errorf("default Factory Session ID is required")
	}
	if runtimeRecorderFactory == nil {
		return nil, fmt.Errorf("Recordings runtime recorder factory is required")
	}
	recording, err := runtimeRecorderFactory(
		recordFlushInterval,
		loadedFactoryCfg,
		spec.Clock.Now,
		spec.RecordPath,
	)
	if err != nil {
		return nil, err
	}
	var initialFactory *factorydefinitions.FactorySnapshot
	var snapshotErr error
	if initialFactorySnapshot != nil {
		initialFactory, snapshotErr = initialFactorySnapshot(loadedFactoryCfg)
	}
	if snapshotErr != nil {
		spec.BaseLogger.Warn(
			"editable factory event snapshot unavailable; using runtime-thin factory event payload",
			zap.Error(snapshotErr),
		)
	}
	return runtimeFactory.Build(
		ctx,
		spec.Dir,
		spec.FolderPath,
		sessionID,
		factoryRunnerID,
		runtimeMode,
		verbose,
		runtimeScheduler,
		workerExecutors,
		workerExecutorDecorator,
		inlineDispatch,
		submissionRecorder,
		dispatchRecorder,
		runtimeLogDir,
		runtimeLogConfig,
		runtimeFileLoggingPolicy,
		runtimeMetricsPolicy,
		runtimeMetricsDir,
		runtimeMetricsConfig,
		loadedFactoryCfg,
		spec.BaseLogger,
		spec.RuntimeInstanceID,
		strings.TrimSpace(backendScopeID),
		spec.Clock,
		spec.RecordPath,
		recording,
		initialFactory,
		spec.SubmissionHooks,
		spec.CompletionPlanner,
		spec.PetriMutationRecorder,
		worldStateProjector,
		runtimeLedgerFactory,
		func(history recordings.WorkerEventRecorder, logger *zap.Logger) (map[string]workers.WorkerExecutor, error) {
			return loadWorkerOptions(
				workerExecution,
				runtimeExecutorsFactory,
				spec,
				factoryRunnerID,
				verbose,
				skipRunnerPrerequisiteValidation,
				invocationSkipPermissionsOverride,
				progressPublisher,
				history,
				logger,
				runtimeFactory.loggerFactory,
			)
		},
		workerExecution,
		dispatchCompleted,
	)
}

func loadWorkerOptions(
	workerExecution workers.RuntimeService,
	runtimeExecutorsFactory factory.WorkersRuntimeExecutorsFactory,
	spec runtimebuild.SessionBuildSpec,
	factoryRunnerID string,
	verbose bool,
	skipRunnerPrerequisiteValidation bool,
	invocationSkipPermissionsOverride *bool,
	progressPublisher workers.ProgressPublisher,
	history recordings.WorkerEventRecorder,
	logger *zap.Logger,
	loggerFactory factory.RuntimeLoggerFactory,
) (map[string]workers.WorkerExecutor, error) {
	if workerExecution == nil {
		return nil, fmt.Errorf("Worker execution service is required")
	}
	if runtimeExecutorsFactory == nil {
		return nil, fmt.Errorf("Workers runtime executors factory is required")
	}
	executors, err := runtimeExecutorsFactory(
		workerExecution,
		spec.LoadedFactoryCfg,
		spec.LoadedFactoryCfg.FactoryConfig(),
		factoryRunnerID,
		RuntimeWorkflowContext(spec.LoadedFactoryCfg.FactoryConfig(), spec.SessionID),
		loggerFactory(logger, verbose),
		skipRunnerPrerequisiteValidation,
		invocationSkipPermissionsOverride,
		spec.ProviderOverride,
		progressPublisher,
		history.RecordScriptEvent,
		history.RecordInferenceEvent,
		history.RecordModelEvent,
		history.RecordAgentRunEvent,
		spec.Clock.Now,
	)
	if err != nil {
		logger.Error("failed to load workers from config", zap.Error(err))
		return nil, fmt.Errorf("load workers: %w", err)
	}
	return executors, nil
}

func workerServiceWithProgress(
	workerExecution workers.RuntimeService,
	providerRunner workers.CommandRunner,
	scriptRunner workers.CommandRunner,
	progressRunner workers.CommandRunner,
	publisher workers.ProgressPublisher,
	publisherSet bool,
	baseLogger *zap.Logger,
	verbose bool,
	loggerFactory factory.RuntimeLoggerFactory,
) (workers.RuntimeService, error) {
	service, err := workerExecution.WithCommandRunners(
		providerRunner,
		scriptRunner,
	)
	if err != nil {
		return nil, err
	}
	var logger factory.Logger = factory.NoopLogger{}
	if baseLogger != nil {
		logger = loggerFactory(baseLogger, verbose)
	}
	return service.WithProgressPublisher(
		progressRunner,
		publisher,
		publisherSet,
		logger,
	)
}
