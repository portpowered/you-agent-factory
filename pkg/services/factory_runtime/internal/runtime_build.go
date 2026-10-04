package internal

import (
	"context"
	"fmt"
	"strings"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryhost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/host"
	runtimebuild "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/instance_host/build"
	runtime "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/scheduler"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

type ProgressPublisherFactory func(string) workers.ProgressPublisher
type DispatchCompletionFactory func(string) func(string)
type InitialFactorySnapshotFactory = factorydefinitions.InitialFactorySnapshotFactory

// TODO: these command runner overrides adn what else, should not exist within the factory runtime
// Rather the invocation overridess should not be passed in to the commadn runner at all, they should be provideed logically
// inside of the service.
// factory session ID for the runtiem workers or whatever shoudl not be in the purview of the system here
// It should be done within the context of the factory runtime/session
type runtimeWorkersServiceWithProgress struct {
	workers.Service
	publisher                         workers.ProgressPublisher
	providerOverride                  providers.Service
	commandRunnerOverride             platformprocess.CommandRunner
	replayCommandRunner               platformprocess.CommandRunner
	modelInvocationOverride           any
	skipBuiltInPrerequisiteValidation bool
	invocationSkipPermissionsOverride *bool
	workstationResolver               runtime.WorkstationExecutionResolver
	factorySessionID                  string
	runtimeID                         string
	recordingID                       string
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
	correlation := request.Correlation
	progress := request.Input.ProgressPublisher
	observer := request.Input.ProcessLifecycleObserver
	if service.workstationResolver != nil && targetNeedsRuntimeResolution(request.Target) {
		resolved, err := service.workstationResolver.ResolveExecutionRequest(
			workstationExecutionRequestFromExecute(
				request,
				service.factorySessionID,
				service.runtimeID,
				service.recordingID,
			),
		)
		if err != nil {
			return workers.ExecuteResult{}, err
		}
		request = resolved
		if progress != nil {
			request.Correlation = correlation
			request.Input.ProgressPublisher = progress
		}
		request.Input.ProcessLifecycleObserver = observer
	}
	if service.providerOverride != nil {
		request.Input.ProviderOverride = service.providerOverride
	}
	if service.modelInvocationOverride != nil {
		request.Input.ModelInvocationOverride = service.modelInvocationOverride
	}
	if service.publisher != nil && request.Input.ProgressPublisher == nil {
		request.Input.ProgressPublisher = service.publisher
	}
	request.Input.SkipBuiltInPrerequisiteValidation = service.skipBuiltInPrerequisiteValidation
	if service.invocationSkipPermissionsOverride != nil {
		value := *service.invocationSkipPermissionsOverride
		request.Input.InvocationSkipPermissionsOverride = &value
	}
	commandRunner := service.commandRunnerOverride
	if service.replayCommandRunner != nil {
		commandRunner = service.replayCommandRunner
	}
	if commandRunner != nil {
		request.Input.CommandRunnerOverride = commandRunner
	}
	return service.Service.Execute(ctx, request)
}

func targetNeedsRuntimeResolution(target workers.ExecutionTarget) bool {
	return !target.Noop &&
		strings.TrimSpace(target.RunnerID) == "" &&
		strings.TrimSpace(target.Provider.ID) == "" &&
		strings.TrimSpace(target.Provider.Alias) == "" &&
		strings.TrimSpace(target.Model.Name) == ""
}

func workstationExecutionRequestFromExecute(
	request workers.ExecuteRequest,
	factorySessionID string,
	runtimeID string,
	recordingID string,
) workers.WorkstationExecutionRequest {
	target := request.Target
	factorySessionID = strings.TrimSpace(factorySessionID)
	if factorySessionID == "" {
		factorySessionID = request.Correlation.FactorySessionID
	}
	runtimeID = strings.TrimSpace(runtimeID)
	if runtimeID == "" {
		runtimeID = request.Correlation.RuntimeID
	}
	generationID := runtimeID
	if generationID == "" {
		generationID = request.Correlation.GenerationID
	}
	if strings.TrimSpace(recordingID) == "" {
		recordingID = request.Input.RecordingID
	}
	var continuation *workers.ProviderContinuationRef
	if request.Input.Resume != nil {
		value := *request.Input.Resume
		continuation = &value
	}
	return workers.WorkstationExecutionRequest{
		Dispatch:                    request.Input.Dispatch,
		WorkerName:                  target.WorkerName,
		WorkerType:                  target.WorkerType,
		WorkstationType:             target.WorkstationName,
		RunnerID:                    target.RunnerID,
		ExecutorProvider:            target.ExecutorProvider,
		FactorySessionID:            factorySessionID,
		RuntimeID:                   runtimeID,
		RecordingID:                 recordingID,
		GenerationID:                generationID,
		Capabilities:                target.Capabilities,
		ModelOperation:              request.Input.ModelOperation,
		ModelBindings:               workers.CloneResolvedModelOperationBindings(request.Input.ModelBindings),
		Model:                       target.Model.Name,
		ModelProvider:               target.Model.Provider,
		ReasoningEffort:             target.Model.ReasoningEffort,
		Command:                     target.Command,
		Args:                        append([]string(nil), target.Args...),
		FactoryDirectory:            target.FactoryDirectory,
		OutputFormat:                target.Output.Format,
		StopToken:                   target.Output.StopToken,
		DecisionEnvelope:            target.Output.DecisionEnvelope,
		GoalRoutingDecisionEnvelope: target.Output.GoalRoutingDecisionEnvelope,
		SystemPrompt:                target.Prompt.SystemPrompt,
		UserMessage:                 target.Prompt.UserMessage,
		OutputSchema:                target.Prompt.OutputSchema,
		Timeout:                     target.Timeout,
		EnvVars:                     target.Environment.Vars,
		ProcessEnvironment:          append([]string(nil), target.Environment.ProcessEnvironment...),
		Worktree:                    target.Workspace.Worktree,
		WorkingDirectory:            target.Environment.WorkingDirectory,
		WorkingDirectoryAuthored:    target.Environment.WorkingDirectorySet,
		WorkflowContext:             request.Input.WorkflowContext.Clone(),
		Continuation:                continuation,
		SkipPermissions:             target.Permissions.SkipPermissions,
	}
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

type runtimeScopeWithFlush struct {
	recordings.RuntimeScopeService
	flushInterval         time.Duration
	resumeCanonicalEvents []factorydefinitions.FactoryEvent
}

func (opening runtimeScopeWithFlush) OpenRuntime(
	ctx context.Context,
	request recordings.RuntimeScopeRequest,
) (recordings.RuntimeScopeResult, error) {
	request.FlushInterval = opening.flushInterval
	request.ReplayEvents = cloneFactoryEvents(opening.resumeCanonicalEvents)
	return opening.RuntimeScopeService.OpenRuntime(ctx, request)
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
	providerCommandRunner platformprocess.CommandRunner,
	scriptCommandRunner platformprocess.CommandRunner,
	mockWorkersConfig *workers.MockWorkersConfig,
	runtimeMode factorydefinitions.RuntimeMode,
	runtimeScheduler scheduler.Scheduler,
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
	skipBuiltInPrerequisiteValidation bool,
	invocationSkipPermissionsOverride *bool,
	clock factory.Clock,
	baseLogger *zap.Logger,
	runtimeFactory *RuntimeFactory,
	workerService workers.Service,
	workerSessions workersessions.Service,
	workerAttempts factory.WorkerAttemptOpener,
	mockCommandRunnerFactory factory.WorkersMockCommandRunnerFactory,
	progressFactory ProgressPublisherFactory,
	completionFactory DispatchCompletionFactory,
	petriMutationRecorder factory.PetriMutationRecorder,
	worldStateProjector factory.WorldStateProjector,
	recordingsRuntime recordings.RuntimeScopeService,
	loadFactory factory.LoadedFactoryLoader,
	initialFactorySnapshot InitialFactorySnapshotFactory,
	preparation *runtimebuild.Service,
	requestResolver *runtime.WorkstationRequestExecutor,
) (*runtimebuild.CompatibilityBuild, error) {
	if runtimeFactory == nil {
		return nil, fmt.Errorf("Factory Runtime factory is required")
	}
	return runtimebuild.BindCompatibility(preparation, runtimebuild.BuildDefaults{
		WorkerModelProvider: defaultWorkerModelProvider, WorkerModel: defaultWorkerModel,
		ApplyOperatorDefaults: applyOperatorDefaults, RecordPath: recordPath, WorkflowID: workflowID,
	},
		providerOverride,
		providerCommandRunner,
		scriptCommandRunner,
		mockWorkersConfig,
		func(
			config *workers.MockWorkersConfig,
			runtimeConfig factorydefinitions.RuntimeDefinitionLookup,
			next platformprocess.CommandRunner,
		) platformprocess.CommandRunner {
			if mockCommandRunnerFactory == nil {
				return next
			}
			return mockCommandRunnerFactory(config, runtimeConfig, next)
		},
		clock,
		baseLogger,
		func(ctx context.Context, spec runtimebuild.SessionBuildSpec) (*factoryhost.Bundle, error) {
			var progressPublisher workers.ProgressPublisher
			if progressFactory != nil {
				progressPublisher = progressFactory(spec.SessionID)
			}
			providerSessionProgress := workersessions.RuntimeProgressPublisher(workerAttempts.PublishRuntimeProgress).ForRuntime(spec.RuntimeInstanceID, progressPublisher)
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
				inlineDispatch,
				submissionRecorder,
				dispatchRecorder,
				backendScopeID,
				factoryRunnerID,
				verbose,
				skipBuiltInPrerequisiteValidation,
				invocationSkipPermissionsOverride,
				workerService,
				mockWorkersConfig,
				workerSessions,
				workerAttempts,
				providerSessionProgress,
				runtimeFactory,
				dispatchCompleted,
				worldStateProjector,
				recordingsRuntime,
				initialFactorySnapshot,
				requestResolver,
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
	inlineDispatch bool,
	submissionRecorder recordings.SubmissionRecorder,
	dispatchRecorder recordings.DispatchRecorder,
	backendScopeID string,
	factoryRunnerID string,
	verbose bool,
	skipBuiltInPrerequisiteValidation bool,
	invocationSkipPermissionsOverride *bool,
	workerExecution workers.Service,
	mockWorkersConfig *workers.MockWorkersConfig,
	workerSessions workersessions.Service,
	workerAttempts factory.WorkerAttemptOpener,
	providerSessionProgress workers.ProgressPublisher,
	runtimeFactory *RuntimeFactory,
	dispatchCompleted func(string),
	worldStateProjector factory.WorldStateProjector,
	recordingsRuntime recordings.RuntimeScopeService,
	initialFactorySnapshot InitialFactorySnapshotFactory,
	requestResolver *runtime.WorkstationRequestExecutor,
) (*factoryhost.Bundle, error) {
	loadedFactoryCfg, sessionID, initialFactory, err := resolveBundleInputs(
		spec, defaultSessionID, recordingsRuntime, initialFactorySnapshot,
	)
	if err != nil {
		return nil, err
	}
	metricsSessionID := firstNonEmptySessionID(spec.MetricsSessionID, sessionID)
	workerServiceWithProgress := newRuntimeWorkersService(
		workerExecution, providerSessionProgress, spec, sessionID,
		skipBuiltInPrerequisiteValidation, invocationSkipPermissionsOverride,
		requestResolver, mockWorkersConfig,
	)
	bundle, err := runtimeFactory.Build(
		ctx,
		spec.Dir,
		spec.FolderPath,
		sessionID,
		metricsSessionID,
		factoryRunnerID,
		runtimeMode,
		verbose,
		runtimeScheduler,
		inlineDispatch,
		submissionRecorder,
		dispatchRecorder,
		runtimeLogDir,
		runtimeLogConfig, runtimeFileLoggingPolicy,
		runtimeMetricsPolicy,
		runtimeMetricsDir,
		runtimeMetricsConfig,
		loadedFactoryCfg,
		spec.RuntimeInstanceID,
		strings.TrimSpace(backendScopeID),
		spec.Clock,
		spec.RecordPath,
		initialFactory,
		spec.RestoredWorldState,
		spec.SkipRestoredDispatchReconciliation,
		spec.SubmissionHooks,
		spec.CompletionPlanner,
		spec.PetriMutationRecorder,
		worldStateProjector,
		runtimeScopeWithFlush{
			RuntimeScopeService:   recordingsRuntime,
			flushInterval:         recordFlushInterval,
			resumeCanonicalEvents: cloneFactoryEvents(spec.ResumeCanonicalEvents),
		},
		workerServiceWithProgress,
		runtimeWorkerSessionBoundary{Service: workerSessions, opener: workerAttempts, execution: workerServiceWithProgress, clock: spec.Clock, scheduler: runtimeFactory.workerAttemptScheduler, runtimeID: spec.RuntimeInstanceID},
		workerAttempts,
		dispatchCompleted,
		mockWorkersConfig,
	)
	if err != nil {
		return nil, err
	}
	setReplayEvents(bundle.Factory, spec.ReplayEvents)
	setBundleProgressPublisher(bundle, providerSessionProgress)
	return bundle, nil
}

func newRuntimeWorkersService(
	workerExecution workers.Service,
	providerSessionProgress workers.ProgressPublisher,
	spec runtimebuild.SessionBuildSpec,
	sessionID string,
	skipBuiltInPrerequisiteValidation bool,
	invocationSkipPermissionsOverride *bool,
	requestResolver *runtime.WorkstationRequestExecutor,
	mockWorkersConfig *workers.MockWorkersConfig,
) *runtimeWorkersServiceWithProgress {
	canonicalSessionID := firstNonEmptySessionID(spec.MetricsSessionID, sessionID)
	var invocationOverride *bool
	if invocationSkipPermissionsOverride != nil {
		value := *invocationSkipPermissionsOverride
		invocationOverride = &value
	}
	service := &runtimeWorkersServiceWithProgress{
		Service:                           workerExecution,
		publisher:                         providerSessionProgress,
		providerOverride:                  spec.ProviderOverride,
		commandRunnerOverride:             spec.CommandRunnerOverride,
		replayCommandRunner:               spec.ReplayCommandRunner,
		modelInvocationOverride:           spec.ModelInvocationOverride,
		skipBuiltInPrerequisiteValidation: skipBuiltInPrerequisiteValidation,
		invocationSkipPermissionsOverride: invocationOverride,
		factorySessionID:                  canonicalSessionID,
		runtimeID:                         spec.RuntimeInstanceID,
		recordingID:                       workerRecordingIdentity(spec.RuntimeInstanceID),
	}
	if requestResolver != nil && spec.LoadedFactoryCfg != nil {
		service.workstationResolver = runtime.BindWorkstationRequests(requestResolver, runtime.WorkstationRequestValues{
			RuntimeDefinitions: spec.LoadedFactoryCfg,
			WorkflowContext:    RuntimeWorkflowContext(spec.LoadedFactoryCfg.FactoryConfig(), canonicalSessionID).Clone(),
			FactorySessionID:   canonicalSessionID, RuntimeID: spec.RuntimeInstanceID,
			RecordingID: workerRecordingIdentity(spec.RuntimeInstanceID), GenerationID: spec.RuntimeInstanceID,
			FactoryDirectory: spec.LoadedFactoryCfg.FactoryDir(), RuntimeBaseDir: spec.ExecutionBaseDir,
			MockWorkers: mockWorkersConfig.Clone(),
		})
	}

	return service
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
	recordingsRuntime recordings.RuntimeScopeService,
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

// runtimeWorkerSessionBoundary supplies selected runtime effects to the direct
// HTTP admission boundary. Reads and controls keep the canonical supervisor.
type runtimeWorkerSessionBoundary struct {
	workersessions.Service
	opener    factory.WorkerAttemptOpener
	execution *runtimeWorkersServiceWithProgress
	clock     platformclock.Source
	scheduler platformclock.TimerSource
	runtimeID string
}

func (boundary runtimeWorkerSessionBoundary) Start(ctx context.Context, request workersessions.StartRequest) (workersessions.StartResult, error) {
	request.Execution.Execution = workers.CloneWorkstationExecutionRequest(request.Execution.Execution)
	if supplied := strings.TrimSpace(request.Execution.Execution.RuntimeID); supplied != "" && supplied != boundary.runtimeID {
		return workersessions.StartResult{}, workersessions.ErrProviderSessionAssociationAttemptMismatch
	}
	request.Execution.Execution.RuntimeID = boundary.runtimeID
	if strings.TrimSpace(request.Execution.Execution.FactorySessionID) == "" {
		request.Execution.Execution.FactorySessionID = boundary.execution.factorySessionID
	}
	if strings.TrimSpace(request.Execution.Execution.GenerationID) == "" {
		request.Execution.Execution.GenerationID = boundary.runtimeID
	}
	return boundary.opener.AdmitRuntimeAttemptAsync(ctx, request, boundary.execution, boundary.clock, boundary.scheduler)
}
