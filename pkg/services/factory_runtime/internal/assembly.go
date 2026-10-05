package internal

import (
	"context"
	"fmt"
	"strings"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/automations"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	instancehost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/instance_host"
	runtimebuild "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/instance_host/build"
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
	bundleOpening          BundleOpeningOperation
	sidecars               *SidecarOpening
	instanceHost           instancehost.Service
	preparation            *runtimebuild.Service
	recordingsRuntime      recordings.RuntimeScopeService
	initialFactorySnapshot factorydefinitions.InitialFactorySnapshotFactory
}

// NewAssembly constructs the inert compatibility assembly selected by Wire.
// Opening and replacement consume fixed process behavior. Only invocation
// selections and scoped resources are allocated when a session opens.
func NewAssembly(
	bundleOpening BundleOpeningOperation,
	sidecars *SidecarOpening,
	instanceHost instancehost.Service,
	preparation *runtimebuild.Service,
	recordingsRuntime recordings.RuntimeScopeService,
	initialFactorySnapshot factorydefinitions.InitialFactorySnapshotFactory,
) (*Assembly, error) {
	if bundleOpening == nil {
		return nil, fmt.Errorf("factory runtime bundle opening is required")
	}
	if recordingsRuntime == nil {
		return nil, fmt.Errorf("recordings runtime opening is required")
	}
	return &Assembly{
		bundleOpening: bundleOpening, sidecars: sidecars, instanceHost: instanceHost,
		preparation:       preparation,
		recordingsRuntime: recordingsRuntime, initialFactorySnapshot: initialFactorySnapshot,
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
	metricsSessionID string,
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
	dir string,
	factoryRootDir string,
	executionBaseDir string,
	loadedFactory factorydefinitions.MutableLoadedFactorySource,
	runtimeInstanceID string,
	replayArtifact *factorydefinitions.ReplayArtifact,
	resumeInput *recordings.LoadResumeInputResult,
	restoredWorldState *factorydefinitions.FactoryWorldState,
	restoredEventHistory []factorydefinitions.FactoryEvent,
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
	if a == nil || a.bundleOpening == nil {
		return nil, nil, factoryruntime.SessionBuildSpec{}, nil, nil,
			fmt.Errorf("Factory Runtime assembly service is required")
	}
	// Replay hooks consume the same detached event history as world-state
	// reconstruction. A successor recording can legitimately reset its local
	// logical clock, but the replay engine must observe that history in one
	// monotonic generation order. Keep spec.ReplayEvents raw below so the
	// read-only canonical ledger remains byte-equivalent to the source.
	replayExecutionArtifact := normalizedReplayArtifactForExecution(replayArtifact)
	replayProvider, replayProcessRunner, replayHooks, completionPlanner, err := a.recordingsRuntime.ReplayExecution(
		replayExecutionArtifact,
	)
	if err != nil {
		return nil, nil, factoryruntime.SessionBuildSpec{}, nil, nil, err
	}
	// Recordings owns replay as a platform process effect. Factory Runtime keeps
	// that low-level effect at the composition boundary and Workers adapts it
	// privately when Execute receives the runtime-scoped override.
	replayCommandRunner := replayProcessRunner
	spec, err := a.preparation.PrepareExecutionSpec(ctx, runtimebuild.BuildDefaults{
		WorkerModelProvider: defaultWorkerModelProvider, WorkerModel: defaultWorkerModel,
		ApplyOperatorDefaults: applyOperatorDefaults, RecordPath: recordPath, WorkflowID: workflowID,
	}, runtimebuild.SessionBuildValues{
		Dir: dir, FolderPath: factoryRootDir, SessionID: defaultSessionID,
		ExecutionBaseDir: executionBaseDir, LoadedFactoryCfg: loadedFactory,
		RuntimeInstanceID: runtimeInstanceID, PreserveCompatibilityDefaultRecordPath: true,
	}, factoryruntime.SessionBuildSpec{
		BaseLogger: baseLogger, Clock: clock, ProviderOverride: replayProvider,
		ReplayCommandRunner: replayCommandRunner, SubmissionHooks: replayhooks.Adapt(replayHooks),
		CompletionPlanner: completionPlanner, PetriMutationRecorder: petriMutationRecorder,
	}, providerOverride, providerCommandRunner, scriptCommandRunner, mockWorkersConfig,
		runtimebuild.MockCommandRunnerFactory(mockCommandRunnerFactory))
	if err != nil {
		return nil, nil, factoryruntime.SessionBuildSpec{}, nil, nil, err
	}
	spec.MetricsSessionID = firstNonEmptySessionID(metricsSessionID, defaultSessionID)
	if resumeInput != nil {
		spec.ResumeSourceCanonicalSessionID = strings.TrimSpace(resumeInput.SourceCanonicalSessionID)
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
		restoredEventHistory,
		a.recordingsRuntime,
	); err != nil {
		return nil, nil, factoryruntime.SessionBuildSpec{}, nil, nil, err
	}
	// The callback retains this session's selections, not a secondary service
	// graph. Both initial and replacement resources use the fixed opening owner.
	open := func(ctx context.Context, spec factoryruntime.SessionBuildSpec) (factoryruntime.RuntimeRecord, error) {
		var progressPublisher workers.ProgressPublisher
		if progressFactory != nil {
			progressPublisher = progressFactory(spec.SessionID)
		}
		var dispatchCompleted func(string)
		if completionFactory != nil {
			dispatchCompleted = completionFactory(spec.SessionID)
		}
		return a.bundleOpening(
			ctx, spec, runtimeLogDir, runtimeLogConfig, runtimeFileLoggingPolicy,
			runtimeMetricsPolicy, runtimeMetricsDir, runtimeMetricsConfig, recordFlushInterval,
			defaultSessionID, runtimeMode, runtimeScheduler, inlineDispatch,
			submissionRecorder, dispatchRecorder, backendScopeID, factoryRunnerID, verbose,
			skipBuiltInPrerequisiteValidation, invocationSkipPermissionsOverride, mockWorkersConfig,
			progressPublisher, dispatchCompleted, worldStateProjector, a.recordingsRuntime, a.initialFactorySnapshot,
		)
	}
	builder := runtimeReplacementOperation(func(ctx context.Context, folderPath, factoryDir, sessionID, executionBaseDir string) (factoryruntime.RuntimeRecord, error) {
		replacementSpec, err := a.preparation.PrepareExecutionSpec(ctx, runtimebuild.BuildDefaults{
			WorkerModelProvider: defaultWorkerModelProvider, WorkerModel: defaultWorkerModel,
			ApplyOperatorDefaults: applyOperatorDefaults, RecordPath: recordPath, WorkflowID: workflowID,
		}, runtimebuild.SessionBuildValues{
			Dir: factoryDir, FolderPath: folderPath, SessionID: sessionID, ExecutionBaseDir: executionBaseDir,
		}, factoryruntime.SessionBuildSpec{
			Clock: clock, BaseLogger: baseLogger, PetriMutationRecorder: petriMutationRecorder,
		}, providerOverride, providerCommandRunner, scriptCommandRunner, mockWorkersConfig,
			runtimebuild.MockCommandRunnerFactory(mockCommandRunnerFactory))
		if err != nil {
			return nil, err
		}
		return open(ctx, replacementSpec)
	})
	instance, err := open(ctx, spec)
	if err != nil {
		// Preserve partial resource ownership for Sessions even though no
		// lifecycle or runnable generation can be published.
		return builder, instance, spec, nil, nil, err
	}
	if instance == nil {
		return nil, nil, factoryruntime.SessionBuildSpec{}, nil, nil, fmt.Errorf(
			"default runtime instance is required",
		)
	}
	attachInvocationScheduleFactory(ctx, automationService, instance)
	lifecycle := a.instanceHost.Scope(clock)
	return builder,
		instance,
		spec,
		lifecycle,
		a.sidecars.Scope(serviceMode),
		nil
}

// runtimeReplacementOperation is one session's addressed replacement capability.
// The captured invocation selections have the same lifetime as that session.
type runtimeReplacementOperation func(context.Context, string, string, string, string) (factoryruntime.RuntimeRecord, error)

func (operation runtimeReplacementOperation) BuildReplacement(ctx context.Context, folderPath, factoryDir, sessionID, executionBaseDir string) (factoryruntime.RuntimeRecord, error) {
	return operation(ctx, folderPath, factoryDir, sessionID, executionBaseDir)
}

func (a *Assembly) configureRestoredWorldState(
	spec *factoryruntime.SessionBuildSpec,
	replayArtifact *factorydefinitions.ReplayArtifact,
	resumeInput *recordings.LoadResumeInputResult,
	restoredWorldState *factorydefinitions.FactoryWorldState,
	restoredEventHistory []factorydefinitions.FactoryEvent,
	recordingsRuntime recordings.RuntimeScopeService,
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
		spec.ResumeCanonicalEvents = cloneFactoryEvents(restoredEventHistory)
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
	restored, err := reconstructRestoredWorldStateForResume(recordingsRuntime, restoredEvents)
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

func normalizedReplayArtifactForExecution(
	artifact *factorydefinitions.ReplayArtifact,
) *factorydefinitions.ReplayArtifact {
	if artifact == nil {
		return nil
	}
	clone := *artifact
	clone.Events = normalizeRestoredEventTicks(cloneFactoryEvents(artifact.Events))
	return &clone
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
	opening recordings.RuntimeScopeService,
	events []factorydefinitions.FactoryEvent,
) (*factorydefinitions.FactoryWorldState, error) {
	// A replay artifact may contain predecessor and successor generations even
	// though replay itself is not a live daemon restart. Project its detached
	// seed in append order so a reset logical clock cannot mix an early
	// predecessor tick with the successor's final state.
	return reconstructRestoredWorldStateEvents(opening, normalizeRestoredEventTicks(events))
}

func reconstructRestoredWorldStateForResume(
	opening recordings.RuntimeScopeService,
	events []factorydefinitions.FactoryEvent,
) (*factorydefinitions.FactoryWorldState, error) {
	return reconstructRestoredWorldStateEvents(opening, normalizeRestoredEventTicks(events))
}

func reconstructRestoredWorldStateEvents(
	opening recordings.RuntimeScopeService,
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

// normalizeRestoredEventTicks gives each proven successor generation a
// detached monotonic tick range. The projection reducer can then preserve the
// canonical ledger order instead of sorting low successor ticks ahead of the
// predecessor events that caused them.
func normalizeRestoredEventTicks(events []factorydefinitions.FactoryEvent) []factorydefinitions.FactoryEvent {
	if !successorRecordingRestartsLogicalClock(events) {
		return events
	}
	normalized := cloneFactoryEvents(events)
	offset := 0
	previousRawTick := events[0].Context.Tick
	previousNormalizedTick := previousRawTick
	restartBoundarySeen := isRestoredLogicalClockBoundary(events[0])
	for index := 1; index < len(events); index++ {
		rawTick := events[index].Context.Tick
		if restartBoundarySeen && rawTick < previousRawTick {
			offset = previousNormalizedTick + 1 - rawTick
			restartBoundarySeen = false
		}
		normalizedTick := rawTick + offset
		// Async responses retain the tick of the dispatch that started them, so
		// their raw tick can move backwards even inside one successor process.
		// Resume reconstruction needs append order after a proven daemon restart;
		// keep that order monotonic without inventing another restart boundary.
		if normalizedTick < previousNormalizedTick {
			normalizedTick = previousNormalizedTick
		}
		normalized[index].Context.Tick = normalizedTick
		if isRestoredLogicalClockBoundary(events[index]) {
			restartBoundarySeen = true
		}
		previousRawTick = rawTick
		previousNormalizedTick = normalized[index].Context.Tick
	}
	return normalized
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
	if len(events) == 0 {
		return false
	}
	restartBoundarySeen := false
	previousTick := events[0].Context.Tick
	for _, event := range events {
		if isRestoredLogicalClockBoundary(event) {
			restartBoundarySeen = true
		}
		if restartBoundarySeen && event.Context.Tick < previousTick {
			return true
		}
		previousTick = event.Context.Tick
	}
	return false
}

func isDaemonRestartInterruption(event factorydefinitions.FactoryEvent) bool {
	return event.Type == factorydefinitions.FactoryEventTypeDispatchInterrupted &&
		event.Context.Source != nil && strings.EqualFold(strings.TrimSpace(*event.Context.Source), "daemon-restart")
}

func isRestoredLogicalClockBoundary(event factorydefinitions.FactoryEvent) bool {
	return isDaemonRestartInterruption(event) || event.Type == factorydefinitions.FactoryEventTypeSessionResumed
}
