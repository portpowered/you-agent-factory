package internal

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/automations"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	instancehost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/instance_host"
	runtimebuild "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/instance_host/build"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/replayhooks"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

// Assembly owns the product-policy dependencies used to assemble each
// session-owned Factory Runtime.
type Assembly struct {
	bundleOpening     BundleOpeningOperation
	sidecars          *SidecarOpening
	instanceHost      instancehost.Service
	preparation       runtimebuild.ExecutionPreparation
	recordingsRuntime recordings.RuntimeScopeService
	automationService automations.Service
	progressFactory   func(*zap.Logger) func(string) workers.ProgressPublisher
	completionFactory func(string) func(string)
}

// NewAssembly constructs the inert assembly selected by Wire.
// Opening and replacement consume fixed process behavior. Only invocation
// selections and scoped resources are allocated when a session opens.
func NewAssembly(
	bundleOpening BundleOpeningOperation,
	sidecars *SidecarOpening,
	instanceHost instancehost.Service,
	preparation runtimebuild.ExecutionPreparation,
	recordingsRuntime recordings.RuntimeScopeService,
	automationService automations.Service,
	progressFactory func(*zap.Logger) func(string) workers.ProgressPublisher,
	completionFactory func(string) func(string),
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
		recordingsRuntime: recordingsRuntime,
		automationService: automationService,
		progressFactory:   progressFactory, completionFactory: completionFactory,
	}, nil
}

// AssembleInitial derives only scoped selections from the initial value request.
func (a *Assembly) AssembleInitial(ctx context.Context, request factoryruntime.RuntimeActivationRequest,
	loaded factorydefinitions.MutableLoadedFactorySource, clock factoryruntime.Clock, logger *zap.Logger,
	observations factoryruntime.SessionObservations,
) (*factoryruntime.RuntimeInitialOpening, error) {
	recovery := request.Inputs.RecoveryInput
	if clock == nil && recovery.ReplayArtifact != nil {
		clock = a.recordingsRuntime.ReplayClock(recovery.ReplayArtifact)
	}
	mockWorkersConfig := activationMockWorkers(request.Inputs.Workers.MockWorkers)
	publishRuntimeStreams := !recovery.CheckpointContinuation
	var petriMutationRecorder factoryruntime.PetriMutationRecorder
	if observations != nil {
		petriMutationRecorder = observations.RecordPetriTokenMutations
	}
	spec, err := a.prepareInitialOpening(ctx, request, loaded, clock, logger, petriMutationRecorder, mockWorkersConfig)
	if err != nil {
		return nil, err
	}
	// The callback retains this session's selections, not a secondary service
	// graph. Both initial and replacement resources use the fixed opening owner.
	open := func(ctx context.Context, spec factoryruntime.SessionBuildSpec) (factoryruntime.RuntimeRecord, error) {
		progressPublisher := a.sessionProgressPublisher(spec.SessionID, logger, publishRuntimeStreams, observations)
		var dispatchCompleted func(string)
		if publishRuntimeStreams && a.completionFactory != nil {
			dispatchCompleted = a.completionFactory(spec.SessionID)
		}
		return a.bundleOpening(
			ctx, spec, request.Runtime.LogDirectory, request.Runtime.LogConfig, request.Runtime.FileLoggingPolicy,
			request.Runtime.MetricsPolicy, request.Runtime.MetricsDirectory, request.Runtime.MetricsConfig, request.Inputs.Recordings.FlushInterval,
			request.FactorySessionID, request.Runtime.Mode, nil, false,
			request.Inputs.Session.BackendScopeID, request.Inputs.Workers.RunnerID, request.Runtime.Verbose,
			request.Inputs.Workers.SkipBuiltInPrerequisiteValidation, request.Inputs.Workers.InvocationSkipPermissionsOverride, mockWorkersConfig,
			progressPublisher, dispatchCompleted,
		)
	}
	builder := runtimeReplacementOperation(func(ctx context.Context, folderPath, factoryDir, sessionID, executionBaseDir string) (factoryruntime.RuntimeRecord, error) {
		replacementSpec, err := a.prepareOpeningSpec(ctx,
			runtimebuild.BuildDefaults{
				WorkerModelProvider: request.Inputs.OperatorDefaults.WorkerModelProvider, WorkerModel: request.Inputs.OperatorDefaults.WorkerModel,
				ApplyOperatorDefaults: recovery.ReplayArtifact == nil && !recovery.CheckpointContinuation, RecordPath: request.Inputs.Recordings.RecordPath, WorkflowID: request.Inputs.Recordings.WorkflowID,
			},
			runtimebuild.SessionBuildValues{
				Dir: factoryDir, FolderPath: folderPath, SessionID: sessionID, ExecutionBaseDir: executionBaseDir,
			},
			factoryruntime.SessionBuildSpec{
				Clock: clock, BaseLogger: logger, PetriMutationRecorder: petriMutationRecorder,
			},
			mockWorkersConfig)
		if err != nil {
			return nil, err
		}
		return open(ctx, replacementSpec)
	})
	instance, err := open(ctx, spec)
	result, err := initialRuntimeOpening(instance, spec, builder, clock, err)
	if err != nil {
		return result, err
	}
	attachInvocationScheduleFactory(ctx, a.automationService, instance)
	result.Lifecycle = a.instanceHost.Scope(clock)
	result.Sidecars = a.sidecars.Scope(request.Runtime.Mode == factorydefinitions.RuntimeModeService && !recovery.CheckpointContinuation)
	return result, nil
}

// prepareInitialOpening preserves detached replay history and initial invocation paths.
func (a *Assembly) prepareInitialOpening(ctx context.Context, request factoryruntime.RuntimeActivationRequest,
	loaded factorydefinitions.MutableLoadedFactorySource, clock factoryruntime.Clock, logger *zap.Logger,
	petriMutationRecorder factoryruntime.PetriMutationRecorder, mockWorkersConfig *workers.MockWorkersConfig,
) (factoryruntime.SessionBuildSpec, error) {
	recovery := request.Inputs.RecoveryInput
	var resumeInput *recordings.LoadResumeInputResult
	if request.Inputs.Recordings.ResumePath != "" {
		resumeInput = &request.Inputs.ResumeInput
	}
	metricsID := request.Inputs.Session.CanonicalSessionID
	if metricsID == "" {
		metricsID = request.FactorySessionID
	}
	// Replay hooks consume the same detached event history as world-state
	// reconstruction. A successor recording can legitimately reset its local
	// logical clock, but the replay engine must observe that history in one
	// monotonic generation order. Keep spec.ReplayEvents raw below so the
	// read-only canonical ledger remains byte-equivalent to the source.
	replayExecutionArtifact := normalizedReplayArtifactForExecution(recovery.ReplayArtifact)
	replayProvider, replayProcessRunner, replayHooks, completionPlanner, err := a.recordingsRuntime.ReplayExecution(
		replayExecutionArtifact,
	)
	if err != nil {
		return factoryruntime.SessionBuildSpec{}, err
	}
	// Recordings owns replay as a platform process effect. Factory Runtime keeps
	// that low-level effect at the composition boundary and Workers adapts it
	// privately when Execute receives the runtime-scoped override.
	replayCommandRunner := replayProcessRunner
	spec, err := a.prepareOpeningSpec(ctx,
		runtimebuild.BuildDefaults{
			WorkerModelProvider: request.Inputs.OperatorDefaults.WorkerModelProvider, WorkerModel: request.Inputs.OperatorDefaults.WorkerModel,
			ApplyOperatorDefaults: recovery.ReplayArtifact == nil && !recovery.CheckpointContinuation, RecordPath: request.Inputs.Recordings.RecordPath, WorkflowID: request.Inputs.Recordings.WorkflowID,
		},
		runtimebuild.SessionBuildValues{
			Dir: request.Inputs.Definition.Directory, FolderPath: request.Inputs.Definition.Directory, SessionID: request.FactorySessionID,
			ExecutionBaseDir: request.Inputs.Definition.ExecutionBaseDir, LoadedFactoryCfg: loaded,
			RuntimeInstanceID: request.RuntimeID,
		},
		factoryruntime.SessionBuildSpec{
			BaseLogger: logger, Clock: clock, ProviderOverride: replayProvider,
			ReplayCommandRunner: replayCommandRunner, SubmissionHooks: replayhooks.Adapt(replayHooks),
			CompletionPlanner: completionPlanner, PetriMutationRecorder: petriMutationRecorder,
		},
		mockWorkersConfig)
	if err != nil {
		return factoryruntime.SessionBuildSpec{}, err
	}
	// Restore and output must address the same session-resolved destination.
	// Resolving an explicit session's placeholder as ~default writes a different
	// board from the one validated during opening.
	spec.RecordPath = runtimebuild.SessionScopedRecordPath(request.Inputs.Recordings.RecordPath, "~default")
	if strings.Contains(request.Inputs.Recordings.RecordPath, "__factory_session_id__") {
		spec.RecordPath = runtimebuild.SessionScopedRecordPath(request.Inputs.Recordings.RecordPath, request.FactorySessionID)
	}
	spec.MetricsSessionID = firstNonEmptySessionID(metricsID, request.FactorySessionID)
	if resumeInput != nil {
		spec.ResumeSourceCanonicalSessionID = strings.TrimSpace(resumeInput.SourceCanonicalSessionID)
	}
	if _, ok := replayProvider.(interface {
		InvokeModel(context.Context, models.InvokeModelRequest) (models.InvokeModelResult, error)
	}); ok {
		spec.ModelInvocationOverride = replayProvider
	}
	spec.ReplayEvents = cloneReplayArtifactEvents(recovery.ReplayArtifact)
	if err := a.configureRestoredWorldState(
		&spec,
		recovery.ReplayArtifact,
		resumeInput,
		recovery.WorldState,
		recovery.EventHistory,
		a.recordingsRuntime,
	); err != nil {
		return factoryruntime.SessionBuildSpec{}, err
	}
	return spec, nil
}

// sessionProgressPublisher retains only the addressed opening's observations.
// Runtime publication precedes durable observation, preserving event ordering.
func (a *Assembly) sessionProgressPublisher(
	sessionID string,
	logger *zap.Logger,
	publishRuntimeStreams bool,
	observations factoryruntime.SessionObservations,
) workers.ProgressPublisher {
	var next workers.ProgressPublisher
	if publishRuntimeStreams && a.progressFactory != nil {
		if publishers := a.progressFactory(logger); publishers != nil {
			next = publishers(sessionID)
		}
	}
	if observations == nil {
		return next
	}
	return func(fragment workers.ProgressFragment) {
		if next != nil {
			next(fragment)
		}
		observations.PublishWorkerProgress(fragment)
	}
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
	// Live openings with recorded history use the same generation-aware
	// reconstruction as resume. The historical tick-ordered view can otherwise
	// select predecessor state after the successor's logical clock restarts.
	if restoredWorldState != nil && len(restoredEventHistory) == 0 {
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
	if resumeInput == nil && len(restoredEventHistory) > 0 {
		restoredEvents = restoredEventHistory
	}
	if resumeInput != nil || len(restoredEventHistory) > 0 {
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
	sessionStartSeen := false
	previousTick := events[0].Context.Tick
	for _, event := range events {
		if event.Type == factorydefinitions.FactoryEventTypeSessionStarted {
			// A graceful live reopen starts a new execution without a resume or
			// interruption marker. Two canonical starts prove that generation
			// boundary; one initial start must not classify async tick regression
			// within a single execution as a restart.
			if sessionStartSeen {
				return true
			}
			sessionStartSeen = true
		} else if isRestoredLogicalClockBoundary(event) {
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
	return isDaemonRestartInterruption(event) || event.Type == factorydefinitions.FactoryEventTypeSessionResumed ||
		event.Type == factorydefinitions.FactoryEventTypeSessionStarted
}

// initialRuntimeOpening declares publication and retains partial ownership
// before inspecting the opening error. No failed candidate exposes a service.
func initialRuntimeOpening(record factoryruntime.RuntimeRecord, spec factoryruntime.SessionBuildSpec,
	builder factoryruntime.RuntimeReplacementBuilder, clock factoryruntime.Clock, openingErr error,
) (*factoryruntime.RuntimeInitialOpening, error) {
	result := &factoryruntime.RuntimeInitialOpening{
		Record: record, Completion: factoryruntime.RuntimeInitialCompletion{
			SessionID: spec.SessionID, MetricsSessionID: spec.MetricsSessionID,
			CanonicalSessionIDGenerated:    spec.CanonicalSessionIDGenerated,
			ResumeSourceCanonicalSessionID: spec.ResumeSourceCanonicalSessionID,
		}, ReplacementBuilder: builder,
		Activation: &factoryruntime.RuntimeActivation{},
	}
	result.Activation.Close = initialRuntimeCloser(record, clock)
	if openingErr != nil {
		return result, openingErr
	}
	if record == nil {
		return result, fmt.Errorf("activate Factory Runtime: opened Runtime engine service is required")
	}
	service := record.RuntimeService()
	if service == nil {
		return result, fmt.Errorf("activate Factory Runtime: opened Runtime engine service is required")
	}
	ingress, ok := service.(factoryruntime.APIFactory)
	if !ok {
		return result, fmt.Errorf("activate Factory Runtime: opened runtime Work submission and event subscription are required until Recordings migration")
	}
	result.Activation.Service = service
	result.Activation.WorkAndEventIngress = ingress
	return result, nil
}

// initialRuntimeCloser serializes cleanup, remembers successful releases and
// retains failed releases for the addressed activation's next cleanup attempt.
func initialRuntimeCloser(record factoryruntime.RuntimeRecord, clock factoryruntime.Clock) func(context.Context) error {
	var mu sync.Mutex
	finalized, artifactsClosed := false, false
	return func(context.Context) error {
		mu.Lock()
		defer mu.Unlock()
		if record == nil {
			return nil
		}
		var finalizationErr, artifactsErr error
		if !finalized {
			if finalizer, ok := record.(interface{ FinalizeRecording(time.Time) error }); ok {
				finalizationErr = finalizer.FinalizeRecording(clock.Now().UTC())
			}
			finalized = finalizationErr == nil
		}
		if !artifactsClosed {
			artifactsErr = record.CloseArtifacts()
			artifactsClosed = artifactsErr == nil
		}
		return errors.Join(finalizationErr, artifactsErr)
	}
}

// prepareOpeningSpec sends only candidate facts to fixed preparation; legacy
// engine hooks remain owned by this opening, never by its Wire role.
func (a *Assembly) prepareOpeningSpec(ctx context.Context, defaults runtimebuild.BuildDefaults,
	values runtimebuild.SessionBuildValues, selections factoryruntime.SessionBuildSpec, mockWorkersConfig *workers.MockWorkersConfig,
) (factoryruntime.SessionBuildSpec, error) {
	prepared, err := a.preparation.PrepareExecutionValues(ctx, defaults, values, selections.BaseLogger,
		selections.ProviderOverride, selections.ReplayCommandRunner, mockWorkersConfig)
	if err != nil {
		return factoryruntime.SessionBuildSpec{}, err
	}
	return runtimebuild.PreparedOpeningSpec(prepared, selections), nil
}
