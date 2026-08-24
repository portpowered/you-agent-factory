package internal

import (
	"context"
	"fmt"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/automations"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	instancehost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/instance_host"
	instancehostwire "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/instance_host/wire"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/replayhooks"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

// Assembly owns the product-policy dependencies used to assemble each
// session-owned Factory Runtime.
type Assembly struct {
	runtimeFactory        *RuntimeFactory
	workerSessionsFactory factoryruntime.WorkerSessionsFactory
	workerService         workers.Service
	metricsClock          platformclock.TimerSource
}

// NewAssembly constructs the inert Factory Runtime assembly service selected
// by Wire. It does not start a runtime or sidecar. workerSessionsFactory is
// the one directly injected per-session Worker Sessions construction path
// (W4 dispatch cutover); Wire owns composing it over worker_sessions/wire so
// Factory Runtime never imports that peer service's wire package directly.
func NewAssembly(
	runtimeFactory *RuntimeFactory,
	workerSessionsFactory factoryruntime.WorkerSessionsFactory,
	workerService workers.Service,
	metricsClock platformclock.TimerSource,
) (*Assembly, error) {
	if runtimeFactory == nil {
		return nil, fmt.Errorf("Factory Runtime factory is required")
	}
	if workerSessionsFactory == nil {
		return nil, fmt.Errorf("Worker Sessions factory is required")
	}
	if workerService == nil {
		return nil, fmt.Errorf("Workers service is required")
	}
	return &Assembly{
		runtimeFactory: runtimeFactory, workerSessionsFactory: workerSessionsFactory,
		workerService: workerService, metricsClock: metricsClock,
	}, nil
}

// Assemble creates one session-owned runtime from invocation values and the
// product-policy dependencies already selected by Wire.
// backendsizecheck:ignore-function service-ownership migration preserves this orchestration flow; extract focused helpers and remove this exemption.
// pkgmaintcheck:ignore-function-lines service-ownership migration preserves this orchestration flow; extract focused helpers and remove this exemption.
func (a *Assembly) Assemble(
	ctx context.Context,
	defaultWorkerModelProvider string,
	defaultWorkerModel string,
	applyOperatorDefaults bool,
	recordPath string,
	workflowID string,
	defaultSessionID string,
	workstationLoader factorydefinitions.WorkstationLoader,
	loadFactory factoryruntime.LoadedFactoryLoader,
	providerOverride providers.Service,
	providerCommandRunner platformprocess.CommandRunner,
	scriptCommandRunner platformprocess.CommandRunner,
	mockWorkersConfig *workers.MockWorkersConfig,
	runtimeMode factorydefinitions.RuntimeMode,
	runtimeScheduler factoryruntime.Scheduler,
	inlineDispatch bool,
	submissionRecorder recordings.SubmissionRecorder,
	dispatchRecorder recordings.DispatchRecorder,
	runtimeLogDir string,
	runtimeLogConfig factoryruntime.RuntimeLogStorageConfig,
	runtimeFileLoggingPolicy factoryruntime.RuntimeFileLoggingPolicy,
	runtimeMetricsPolicy factoryruntime.RuntimeMetricsPolicy,
	runtimeMetricsDir string,
	runtimeMetricsConfig factoryruntime.RuntimeMetricsStorageConfig,
	recordFlushInterval time.Duration,
	backendScopeID string,
	factoryRunnerID string,
	verbose bool,
	skipBuiltInPrerequisiteValidation bool,
	invocationSkipPermissionsOverride *bool,
	clock factoryruntime.Clock,
	baseLogger *zap.Logger,
	mockCommandRunnerFactory factoryruntime.WorkersMockCommandRunnerFactory,
	progressFactory func(string) workers.ProgressPublisher,
	completionFactory func(string) func(string),
	petriMutationRecorder factoryruntime.PetriMutationRecorder,
	worldStateProjector factoryruntime.WorldStateProjector,
	recordingsRuntime recordings.RuntimeOpening,
	initialFactorySnapshot factorydefinitions.InitialFactorySnapshotFactory,
	dir string,
	factoryRootDir string,
	executionBaseDir string,
	loadedFactory factorydefinitions.MutableLoadedFactorySource,
	runtimeInstanceID string,
	replayArtifact *factorydefinitions.ReplayArtifact,
	resumeInput *recordings.LoadResumeInputResult,
	restoredWorldState *factorydefinitions.FactoryWorldState,
	automationService automations.Service,
	serviceMode bool,
) (
	factoryruntime.RuntimeReplacementBuilder,
	factoryruntime.RuntimeRecord,
	factoryruntime.SessionBuildSpec,
	factoryruntime.RuntimeLifecycle,
	factoryruntime.RuntimeSidecars,
	error,
) {
	if a == nil || a.runtimeFactory == nil {
		return nil, nil, factoryruntime.SessionBuildSpec{}, nil, nil,
			fmt.Errorf("Factory Runtime assembly service is required")
	}
	if a.workerService == nil {
		return nil, nil, factoryruntime.SessionBuildSpec{}, nil, nil,
			fmt.Errorf("Workers service is required")
	}
	builder, err := NewRuntimeBuild(
		defaultWorkerModelProvider,
		defaultWorkerModel,
		applyOperatorDefaults,
		recordPath,
		workflowID,
		defaultSessionID,
		workstationLoader,
		providerOverride,
		providerCommandRunner,
		scriptCommandRunner,
		mockWorkersConfig,
		runtimeMode,
		runtimeScheduler,
		inlineDispatch,
		submissionRecorder,
		dispatchRecorder,
		runtimeLogDir,
		runtimeLogConfig,
		runtimeFileLoggingPolicy,
		runtimeMetricsPolicy,
		runtimeMetricsDir,
		runtimeMetricsConfig,
		recordFlushInterval,
		backendScopeID,
		factoryRunnerID,
		verbose,
		skipBuiltInPrerequisiteValidation,
		invocationSkipPermissionsOverride,
		clock,
		baseLogger,
		a.runtimeFactory,
		a.workerService,
		a.workerSessionsFactory,
		mockCommandRunnerFactory,
		progressFactory,
		completionFactory,
		petriMutationRecorder,
		worldStateProjector,
		recordingsRuntime,
		loadFactory,
		initialFactorySnapshot,
	)
	if err != nil {
		return nil, nil, factoryruntime.SessionBuildSpec{}, nil, nil, err
	}
	if recordingsRuntime == nil {
		return nil, nil, factoryruntime.SessionBuildSpec{}, nil, nil, fmt.Errorf(
			"Recordings runtime opening is required",
		)
	}
	replayProvider, replayProcessRunner, replayHooks, completionPlanner, err := recordingsRuntime.ReplayExecution(
		replayArtifact,
	)
	if err != nil {
		return nil, nil, factoryruntime.SessionBuildSpec{}, nil, nil, err
	}
	// Recordings owns replay as a platform process effect. Factory Runtime keeps
	// that low-level effect at the composition boundary and Workers adapts it
	// privately when Execute receives the runtime-scoped override.
	replayCommandRunner := replayProcessRunner
	spec, err := builder.BuildSpec(
		ctx,
		dir,
		factoryRootDir,
		defaultSessionID,
		executionBaseDir,
		loadedFactory,
		runtimeInstanceID,
		replayProvider,
		replayCommandRunner,
		replayhooks.Adapt(replayHooks),
		completionPlanner,
		true,
	)
	if err != nil {
		return nil, nil, factoryruntime.SessionBuildSpec{}, nil, nil, err
	}
	if _, ok := replayProvider.(interface {
		InvokeModel(context.Context, models.InvokeModelRequest) (models.InvokeModelResult, error)
	}); ok {
		spec.ModelInvocationOverride = replayProvider
	}
	spec.ReplayEvents = cloneReplayArtifactEvents(replayArtifact)
	if err := a.configureRestoredWorldState(
		&spec,
		replayArtifact,
		resumeInput,
		restoredWorldState,
		recordingsRuntime,
	); err != nil {
		return nil, nil, factoryruntime.SessionBuildSpec{}, nil, nil, err
	}
	instance, err := builder.Build(ctx, spec)
	if err != nil {
		return nil, nil, factoryruntime.SessionBuildSpec{}, nil, nil, err
	}
	if instance == nil {
		return nil, nil, factoryruntime.SessionBuildSpec{}, nil, nil, fmt.Errorf(
			"default runtime instance is required",
		)
	}
	attachInvocationScheduleFactory(ctx, automationService, instance)
	lifecycle, err := instancehostwire.New(instancehost.Dependencies{Clock: clock})
	if err != nil {
		return nil, nil, factoryruntime.SessionBuildSpec{}, nil, nil, err
	}
	return builder,
		instance,
		spec,
		lifecycle,
		NewRuntimeSidecars(automationService, serviceMode, a.metricsClock),
		nil
}

func (a *Assembly) configureRestoredWorldState(
	spec *factoryruntime.SessionBuildSpec,
	replayArtifact *factorydefinitions.ReplayArtifact,
	resumeInput *recordings.LoadResumeInputResult,
	restoredWorldState *factorydefinitions.FactoryWorldState,
	recordingsRuntime recordings.RuntimeOpening,
) error {
	if spec == nil {
		return fmt.Errorf("Factory Runtime build spec is required")
	}
	// Deterministic replay also needs a detached starting board: replay hooks
	// reproduce the event history, while the runtime marking supplies the Work
	// identities that those hooks observe. It is not a daemon restart, so its
	// recorded active dispatches must not be interrupted or re-armed.
	if replayArtifact != nil {
		restored, err := reconstructRestoredWorldState(recordingsRuntime, spec.ReplayEvents)
		if err != nil {
			return err
		}
		spec.RestoredWorldState = restored
		spec.SkipRestoredDispatchReconciliation = true
		return nil
	}
	// Live daemon openings supply detached current-board state explicitly.
	if restoredWorldState != nil {
		spec.RestoredWorldState = restoredWorldState
		return nil
	}
	// Resume is a live continuation: reconstruct the successor's starting
	// state from the Recordings-selected history while retaining that prefix in
	// the successor ledger for public reconnect cursors.
	restoredEvents, err := restoredEventsForOpening(spec.ReplayEvents, resumeInput)
	if err != nil {
		return err
	}
	if resumeInput != nil {
		spec.ResumeCanonicalEvents = cloneFactoryEvents(restoredEvents)
	}
	restored, err := reconstructRestoredWorldState(recordingsRuntime, restoredEvents)
	if err != nil {
		return err
	}
	spec.RestoredWorldState = restored
	return nil
}

func cloneReplayArtifactEvents(artifact *factorydefinitions.ReplayArtifact) []factorydefinitions.FactoryEvent {
	if artifact == nil || len(artifact.Events) == 0 {
		return nil
	}
	return cloneFactoryEvents(artifact.Events)
}

func cloneFactoryEvents(events []factorydefinitions.FactoryEvent) []factorydefinitions.FactoryEvent {
	if len(events) == 0 {
		return nil
	}
	cloned := make([]factorydefinitions.FactoryEvent, len(events))
	for index, event := range events {
		cloned[index] = event.Clone()
	}
	return cloned
}

func restoredEventsForOpening(
	replayEvents []factorydefinitions.FactoryEvent,
	resumeInput *recordings.LoadResumeInputResult,
) ([]factorydefinitions.FactoryEvent, error) {
	if resumeInput == nil {
		return replayEvents, nil
	}
	if resumeInput.Input.Legacy == nil {
		return nil, fmt.Errorf("resume recording does not contain Factory event history")
	}
	events := cloneReplayArtifactEvents(resumeInput.Input.Legacy)
	if len(events) == 0 {
		return nil, fmt.Errorf("resume recording contains no Factory events")
	}
	return events, nil
}

func reconstructRestoredWorldState(
	opening recordings.RuntimeOpening,
	events []factorydefinitions.FactoryEvent,
) (*factorydefinitions.FactoryWorldState, error) {
	if len(events) == 0 {
		return nil, nil
	}
	if opening == nil {
		return nil, fmt.Errorf("Recordings runtime opening is required to reconstruct restored world state")
	}
	selectedTick := restoredWorldStateTick(events)
	worldState, err := opening.ReconstructCanonicalFactoryWorldState(events, selectedTick)
	if err != nil {
		return nil, fmt.Errorf("reconstruct restored Factory world state: %w", err)
	}
	return &worldState, nil
}

func restoredWorldStateTick(events []factorydefinitions.FactoryEvent) int {
	latestTick := events[0].Context.Tick
	for _, event := range events[1:] {
		if event.Context.Tick > latestTick {
			latestTick = event.Context.Tick
		}
	}
	if successorRecordingRestartsLogicalClock(events) {
		// A resumed runtime starts its logical tick counter again. The
		// predecessor prefix can therefore contain a larger tick than the
		// successor's final event; use event order after the interruption so
		// projection does not restore the stale predecessor state.
		return events[len(events)-1].Context.Tick
	}
	return latestTick
}

func successorRecordingRestartsLogicalClock(events []factorydefinitions.FactoryEvent) bool {
	interruptionSeen := false
	previousTick := events[0].Context.Tick
	for _, event := range events {
		if event.Type == factorydefinitions.FactoryEventTypeDispatchInterrupted {
			interruptionSeen = true
		}
		if interruptionSeen && event.Context.Tick < previousTick {
			return true
		}
		previousTick = event.Context.Tick
	}
	return false
}
