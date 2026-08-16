package internal

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryhost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/host"
	runtimebuild "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/instance_host/build"
	runtime "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/scheduler"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

type ProgressPublisherFactory func(string) workers.ProgressPublisher
type DispatchCompletionFactory func(string) func(string)
type InitialFactorySnapshotFactory = factorydefinitions.InitialFactorySnapshotFactory

type runtimeWorkersServiceWithProgress struct {
	workers.Service
	publisher             workers.ProgressPublisher
	providerOverride      providers.Service
	commandRunnerOverride workers.CommandRunner
	replayCommandRunner   workers.CommandRunner
	clock                 workers.Clock
}

func (service runtimeWorkersServiceWithProgress) RuntimeProgressPublisher() workers.ProgressPublisher {
	return service.publisher
}

// Execute carries runtime-local effect substitutions into the shared Workers
// boundary. Replay uses these detached ports to reproduce provider and script
// outcomes without mutating the process-scoped Workers service.
func (service runtimeWorkersServiceWithProgress) Execute(
	ctx context.Context,
	request workers.ExecuteRequest,
) (workers.ExecuteResult, error) {
	if service.providerOverride != nil {
		request.Input.ProviderOverride = service.providerOverride
	}
	commandRunner := service.commandRunnerOverride
	if service.replayCommandRunner != nil {
		commandRunner = service.replayCommandRunner
	}
	if commandRunner != nil {
		// Session build specs may carry a raw injected command edge (or a
		// process-scoped logging runner). Clone logging runners before binding
		// the opened Runtime's sink so concurrent sessions never mutate shared
		// command-runner state.
		if request.Input.ExecutionLogger != nil && service.clock != nil {
			switch typed := commandRunner.(type) {
			case workers.LoggingCommandRunner:
				typed.Logger = request.Input.ExecutionLogger
				typed.Clock = service.clock
				commandRunner = typed
			case *workers.LoggingCommandRunner:
				if typed != nil {
					clone := *typed
					clone.Logger = request.Input.ExecutionLogger
					clone.Clock = service.clock
					commandRunner = &clone
				}
			default:
				commandRunner = workers.CommandRunnerWithLogging(
					commandRunner,
					request.Input.ExecutionLogger,
					service.clock,
				)
			}
		}
		request.Input.CommandRunnerOverride = commandRunner
	}
	return service.Service.Execute(ctx, request)
}

func (service runtimeWorkersServiceWithProgress) RuntimeOwnsModelEventRecording() bool {
	owner, ok := service.Service.(interface {
		RuntimeOwnsModelEventRecording() bool
	})
	return ok && owner.RuntimeOwnsModelEventRecording()
}

func (service runtimeWorkersServiceWithProgress) RenderPrompt(
	template string,
	tokens []workers.Token,
	workflowContext *workers.Context,
) (string, error) {
	renderer, ok := service.Service.(runtime.PromptRenderer)
	if !ok {
		return "", fmt.Errorf("render Worker prompt: renderer is unavailable")
	}
	return renderer.RenderPrompt(template, tokens, workflowContext)
}

func (service runtimeWorkersServiceWithProgress) ResolveTemplateFields(
	workingDirectory string,
	environment map[string]string,
	tokens []workers.Token,
	workflowContext *workers.Context,
	worktree string,
) (*workers.ResolvedTemplateFields, error) {
	resolver, ok := service.Service.(runtime.TemplateFieldResolver)
	if !ok {
		return nil, fmt.Errorf("resolve Worker template fields: resolver is unavailable")
	}
	return resolver.ResolveTemplateFields(
		workingDirectory, environment, tokens, workflowContext, worktree,
	)
}

type runtimeOpeningWithFlush struct {
	recordings.RuntimeOpening
	flushInterval time.Duration
}

func (opening runtimeOpeningWithFlush) OpenRuntime(
	ctx context.Context,
	request recordings.RuntimeScopeRequest,
) (recordings.RuntimeScopeResult, error) {
	request.FlushInterval = opening.flushInterval
	return opening.RuntimeOpening.OpenRuntime(ctx, request)
}

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
	providerOverride providers.Service,
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
	workerService workers.Service,
	workerSessionsFactory factory.WorkerSessionsFactory,
	providerInvocationFactory factory.ProviderInvocationExecutorFactory,
	runtimeExecutorsFactory factory.WorkersRuntimeExecutorsFactory,
	mockCommandRunnerFactory factory.WorkersMockCommandRunnerFactory,
	progressFactory ProgressPublisherFactory,
	completionFactory DispatchCompletionFactory,
	petriMutationRecorder factory.PetriMutationRecorder,
	worldStateProjector factory.WorldStateProjector,
	recordingsRuntime recordings.RuntimeOpening,
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
			var progressPublisher workers.ProgressPublisher
			if progressFactory != nil {
				progressPublisher = progressFactory(spec.SessionID)
			}
			// Workers and the runtime-owned Worker Sessions service are assembled
			// in opposite dependency order: Workers needs its progress publisher
			// before the workstation pool exists, while Worker Sessions needs that
			// pool. This bridge is bound exactly once when the Factory Runtime
			// creates Worker Sessions, ensuring reference-bearing progress cannot
			// reach the response stream before its Worker Session association.
			providerSessionProgress := workersessions.NewProviderSessionObservationPublisher(progressPublisher).WithUnassociatedProgressFallback()
			if workerService == nil {
				return nil, fmt.Errorf("Workers service is required")
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
				workerService,
				mockWorkersConfig,
				workerSessionsFactory,
				runtimeExecutorsFactory,
				providerSessionProgress,
				providerInvocationFactory,
				runtimeFactory,
				dispatchCompleted,
				worldStateProjector,
				recordingsRuntime,
				initialFactorySnapshot,
			)
		},
		petriMutationRecorder,
	)
}

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
	workerExecution workers.Service,
	mockWorkersConfig *workers.MockWorkersConfig,
	workerSessionsFactory factory.WorkerSessionsFactory,
	runtimeExecutorsFactory factory.WorkersRuntimeExecutorsFactory,
	providerSessionProgress *workersessions.ProviderSessionObservationPublisher,
	providerInvocationFactory factory.ProviderInvocationExecutorFactory,
	runtimeFactory *RuntimeFactory,
	dispatchCompleted func(string),
	worldStateProjector factory.WorldStateProjector,
	recordingsRuntime recordings.RuntimeOpening,
	initialFactorySnapshot InitialFactorySnapshotFactory,
) (*factoryhost.Bundle, error) {
	loadedFactoryCfg, sessionID, initialFactory, err := resolveBundleInputs(
		spec, defaultSessionID, recordingsRuntime, initialFactorySnapshot,
	)
	if err != nil {
		return nil, err
	}
	workerSessionsFactory, providerInvocation, err := prepareBundleExecution(
		workerSessionsFactory,
		providerSessionProgress,
		providerInvocationFactory,
		spec.ProviderCommandRunner,
	)
	if err != nil {
		return nil, err
	}
	workerServiceWithProgress := newRuntimeWorkersService(workerExecution, providerSessionProgress, spec)
	workerOptions := makeWorkerOptionsLoader(workerExecution, runtimeExecutorsFactory, spec, factoryRunnerID, verbose, skipRunnerPrerequisiteValidation, invocationSkipPermissionsOverride, providerSessionProgress.Publish, runtimeFactory.loggerFactory)
	bundle, err := runtimeFactory.Build(
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
		spec.RuntimeInstanceID,
		strings.TrimSpace(backendScopeID),
		spec.Clock,
		spec.RecordPath,
		initialFactory,
		spec.SubmissionHooks,
		spec.CompletionPlanner,
		spec.PetriMutationRecorder,
		worldStateProjector,
		runtimeOpeningWithFlush{
			RuntimeOpening: recordingsRuntime,
			flushInterval:  recordFlushInterval,
		},
		workerOptions,
		workerServiceWithProgress,
		providerInvocation,
		workerSessionsFactory,
		dispatchCompleted,
		mockWorkersConfig,
	)
	if err != nil {
		return nil, err
	}
	setReplayEvents(bundle.Factory, spec.ReplayEvents)
	setBundleProgressPublisher(bundle, providerSessionProgress.Publish)
	return bundle, nil
}

func newRuntimeWorkersService(
	workerExecution workers.Service,
	providerSessionProgress *workersessions.ProviderSessionObservationPublisher,
	spec runtimebuild.SessionBuildSpec,
) workers.Service {
	return workers.Service(runtimeWorkersServiceWithProgress{
		Service:               workerExecution,
		publisher:             providerSessionProgress.Publish,
		providerOverride:      spec.ProviderOverride,
		commandRunnerOverride: spec.CommandRunnerOverride,
		replayCommandRunner:   spec.ReplayCommandRunner,
		clock:                 spec.Clock,
	})
}

func setReplayEvents(
	bundleFactory factoryhost.Engine,
	events []factorydefinitions.FactoryEvent,
) {
	setter, ok := bundleFactory.(interface {
		SetReplayEvents([]factorydefinitions.FactoryEvent)
	})
	if ok {
		setter.SetReplayEvents(events)
	}
}

func prepareBundleExecution(
	workerSessionsFactory factory.WorkerSessionsFactory,
	providerSessionProgress *workersessions.ProviderSessionObservationPublisher,
	providerInvocationFactory factory.ProviderInvocationExecutorFactory,
	providerCommandRunner workers.CommandRunner,
) (factory.WorkerSessionsFactory, workers.WorkstationRequestExecutor, error) {
	if err := validateBundleDependencies(providerSessionProgress, workerSessionsFactory); err != nil {
		return nil, nil, err
	}
	workerSessionsFactory = bindProviderSessionProgress(workerSessionsFactory, providerSessionProgress)
	providerInvocation, err := buildProviderInvocation(
		providerInvocationFactory,
		providerCommandRunner,
		providerSessionProgress.Publish,
	)
	if err != nil {
		return nil, nil, err
	}
	return workerSessionsFactory, providerInvocation, nil
}

func makeWorkerOptionsLoader(
	workerExecution workers.Service,
	runtimeExecutorsFactory factory.WorkersRuntimeExecutorsFactory,
	spec runtimebuild.SessionBuildSpec,
	factoryRunnerID string,
	verbose bool,
	skipRunnerPrerequisiteValidation bool,
	invocationSkipPermissionsOverride *bool,
	progressPublisher workers.ProgressPublisher,
	loggerFactory factory.RuntimeLoggerFactory,
) func(recordings.WorkerEventRecorder, *zap.Logger) (map[string]workers.WorkerExecutor, error) {
	return func(history recordings.WorkerEventRecorder, logger *zap.Logger) (map[string]workers.WorkerExecutor, error) {
		runtimeService, ok := workerExecution.(workers.RuntimeService)
		if !ok || runtimeExecutorsFactory == nil {
			return nil, nil
		}
		return loadWorkerOptions(
			runtimeService,
			runtimeExecutorsFactory,
			spec,
			factoryRunnerID,
			verbose,
			skipRunnerPrerequisiteValidation,
			invocationSkipPermissionsOverride,
			progressPublisher,
			history,
			logger,
			loggerFactory,
		)
	}
}

func setBundleProgressPublisher(bundle *factoryhost.Bundle, publisher workers.ProgressPublisher) {
	if configurable, ok := bundle.Factory.(interface {
		SetProgressPublisher(workers.ProgressPublisher)
	}); ok {
		configurable.SetProgressPublisher(publisher)
	}
}

func resolveBundleInputs(
	spec runtimebuild.SessionBuildSpec,
	defaultSessionID string,
	recordingsRuntime recordings.RuntimeOpening,
	initialFactorySnapshot InitialFactorySnapshotFactory,
) (factorydefinitions.LoadedFactorySource, string, *factorydefinitions.FactorySnapshot, error) {
	loadedFactoryCfg, ok := spec.LoadedFactoryCfg.(factorydefinitions.LoadedFactorySource)
	if !ok || loadedFactoryCfg == nil {
		return nil, "", nil, fmt.Errorf("loaded Factory config is required")
	}
	sessionID := firstNonEmptySessionID(spec.SessionID, defaultSessionID)
	if sessionID == "" {
		return nil, "", nil, fmt.Errorf("default Factory Session ID is required")
	}
	if recordingsRuntime == nil {
		return nil, "", nil, fmt.Errorf("Recordings runtime opening is required")
	}
	var initialFactory *factorydefinitions.FactorySnapshot
	if initialFactorySnapshot == nil {
		return loadedFactoryCfg, sessionID, nil, nil
	}
	initialFactory, err := initialFactorySnapshot(loadedFactoryCfg)
	if err != nil {
		spec.BaseLogger.Warn(
			"editable factory event snapshot unavailable; using runtime-thin factory event payload",
			zap.Error(err),
		)
	}
	return loadedFactoryCfg, sessionID, initialFactory, nil
}

func firstNonEmptySessionID(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func validateBundleDependencies(
	providerSessionProgress *workersessions.ProviderSessionObservationPublisher,
	workerSessionsFactory factory.WorkerSessionsFactory,
) error {
	if providerSessionProgress == nil {
		return fmt.Errorf("Worker Session provider progress bridge is required")
	}
	if workerSessionsFactory == nil {
		return fmt.Errorf("Worker Sessions factory is required")
	}
	return nil
}

func buildProviderInvocation(
	factory factory.ProviderInvocationExecutorFactory,
	commandRunner workers.CommandRunner,
	publisher workers.ProgressPublisher,
) (workers.WorkstationRequestExecutor, error) {
	if factory == nil {
		return nil, nil
	}
	providerInvocation, err := factory(commandRunner, publisher)
	if err != nil {
		return nil, fmt.Errorf("construct provider-invocation Worker executor: %w", err)
	}
	return providerInvocation, nil
}

// bindProviderSessionProgress keeps the Workers progress bridge session-local:
// Factory Runtime creates the same Worker Sessions service that owns the pool
// and binds it before any dispatch can be admitted or produce output.
func bindProviderSessionProgress(
	workerSessionsFactory factory.WorkerSessionsFactory,
	publisher *workersessions.ProviderSessionObservationPublisher,
) factory.WorkerSessionsFactory {
	return func(boundary workers.WorkstationPoolBoundary, clock platformclock.Source) (workersessions.Service, error) {
		service, err := workerSessionsFactory(boundary, clock)
		if err != nil {
			return nil, err
		}
		publisher.Bind(service)
		return service, nil
	}
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
func buildRuntimeWorkstationBoundary(
	factory factory.WorkstationPoolBoundaryFactory,
	service runtimeWorkstationService,
	executors map[string]workers.WorkerExecutor,
	net *state.Net,
	requestExecutor workers.WorkstationRequestExecutor,
	providerInvocation workers.WorkstationRequestExecutor,
) workers.WorkstationPoolBoundary {
	if factory == nil || service == nil {
		return nil
	}
	return factory(workers.WorkstationPoolBoundaryConfig{
		Service:            service,
		Executors:          executors,
		RequestExecutor:    requestExecutor,
		RouteNames:         runtimeBoundaryRouteNames(net, executors),
		ProviderInvocation: providerInvocation,
		Async:              true,
	})
}

func runtimeBoundaryRouteNames(
	net *state.Net,
	executors map[string]workers.WorkerExecutor,
) []string {
	routes := make(map[string]struct{}, len(executors))
	for name := range executors {
		if name != "" {
			routes[name] = struct{}{}
		}
	}
	if net != nil {
		for id, transition := range net.Transitions {
			if id != "" {
				routes[id] = struct{}{}
			}
			if transition == nil {
				continue
			}
			if transition.Name != "" {
				routes[transition.Name] = struct{}{}
			}
			if transition.WorkerType != "" {
				routes[transition.WorkerType] = struct{}{}
			}
		}
	}
	names := make([]string, 0, len(routes))
	for name := range routes {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
