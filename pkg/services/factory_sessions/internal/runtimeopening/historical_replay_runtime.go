package runtimeopening

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/automations"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimeports"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

// historicalReplayProcessRuntime completes the process lifecycle for an
// inspection-only portable recording. It intentionally starts neither a live
// Factory runtime nor worker sidecars nor an HTTP host.
type historicalReplayProcessRuntime struct{}

func (historicalReplayProcessRuntime) Start(context.Context, context.Context) error { return nil }

func (historicalReplayProcessRuntime) StartWorkers(context.Context) (factorysessions.RuntimeStop, error) {
	return func(context.Context) error { return nil }, nil
}

func (historicalReplayProcessRuntime) RunTransport(context.Context, http.Handler) error { return nil }

func (historicalReplayProcessRuntime) Stop(context.Context) error { return nil }

type portableReplayDurableOwner struct {
	durableexecution.Service
	prepareOnce sync.Once
	prepare     func(context.Context) error
	prepareErr  error
}

func (owner *portableReplayDurableOwner) HasRestorableState(
	ctx context.Context,
	sessionID string,
) (bool, error) {
	if owner == nil || owner.Service == nil {
		return false, nil
	}
	probe, ok := owner.Service.(interface {
		HasRestorableState(context.Context, string) (bool, error)
	})
	if !ok {
		return false, nil
	}
	available, err := probe.HasRestorableState(ctx, sessionID)
	if err != nil || !available {
		return available, err
	}
	owner.prepareOnce.Do(func() {
		if owner.prepare != nil {
			owner.prepareErr = owner.prepare(ctx)
		}
	})
	if owner.prepareErr != nil {
		return false, owner.prepareErr
	}
	return true, nil
}

// SubscribeResponseEvents forwards the optional response-event capability
// through the replay owner wrapper after the explicit handoff.
func (owner *portableReplayDurableOwner) SubscribeResponseEvents(
	ctx context.Context,
	sessionID string,
	request factorysessions.ResponseEventSubscriptionRequest,
) (*factorysessions.ResponseEventCursor, error) {
	if owner == nil || owner.Service == nil {
		return nil, factorysessions.ErrRuntimeNotAvailable
	}
	subscriber, ok := owner.Service.(interface {
		SubscribeResponseEvents(context.Context, string, factorysessions.ResponseEventSubscriptionRequest) (*factorysessions.ResponseEventCursor, error)
	})
	if !ok {
		return nil, factorysessions.ErrRuntimeNotAvailable
	}
	return subscriber.SubscribeResponseEvents(ctx, sessionID, request)
}

// Close forwards the optional execution-owner shutdown boundary through the
// replay handoff. The durable execution service contract remains focused on
// customer operations; only implementations that own asynchronous work need
// to expose this private lifecycle capability.
func (owner *portableReplayDurableOwner) Close() error {
	if owner == nil || owner.Service == nil {
		return nil
	}
	closer, ok := owner.Service.(interface{ Close() error })
	if !ok {
		return nil
	}
	return closer.Close()
}

type portableReplayRuntimeCleanup struct {
	mu       sync.Mutex
	owner    interface{ Close() error }
	runtime  runtimeports.RuntimeInstance
	closed   bool
	closeErr error
}

func newPortableReplayRuntimeCleanup() *portableReplayRuntimeCleanup {
	return &portableReplayRuntimeCleanup{}
}

func (cleanup *portableReplayRuntimeCleanup) SetOwner(owner interface{ Close() error }) {
	if cleanup == nil || owner == nil {
		return
	}
	cleanup.mu.Lock()
	defer cleanup.mu.Unlock()
	if cleanup.closed {
		cleanup.closeErr = errors.Join(cleanup.closeErr, owner.Close())
		return
	}
	cleanup.owner = owner
}

func (cleanup *portableReplayRuntimeCleanup) Set(runtime runtimeports.RuntimeInstance) {
	if cleanup == nil || runtime == nil {
		return
	}
	cleanup.mu.Lock()
	defer cleanup.mu.Unlock()
	if cleanup.closed {
		cleanup.closeErr = errors.Join(cleanup.closeErr, runtime.CloseArtifacts())
		return
	}
	cleanup.runtime = runtime
}

func (cleanup *portableReplayRuntimeCleanup) Close() error {
	if cleanup == nil {
		return nil
	}
	cleanup.mu.Lock()
	defer cleanup.mu.Unlock()
	if cleanup.closed {
		return cleanup.closeErr
	}
	cleanup.closed = true
	if cleanup.owner != nil {
		cleanup.closeErr = errors.Join(cleanup.closeErr, cleanup.owner.Close())
		cleanup.owner = nil
	}
	if cleanup.runtime != nil {
		cleanup.closeErr = errors.Join(cleanup.closeErr, cleanup.runtime.CloseArtifacts())
		cleanup.runtime = nil
	}
	return cleanup.closeErr
}

// openPortableReplayDurableOwner constructs the existing durable execution
// owner for a checkpoint-bearing replay. Runtime assembly is deferred until
// HasRestorableState confirms that the durable owner can actually resume; a
// public checkpoint summary alone must remain inspection-only.
func openPortableReplayDurableOwner(
	configured preparedRuntime,
	root RuntimeRoot,
	logger *zap.Logger,
	clockEdge factoryruntime.Clock,
	providerOverride providers.Service,
	providerCommandRunner platformprocess.CommandRunner,
	scriptCommandRunner platformprocess.CommandRunner,
	workerService workers.Service,
	workersMockCommandRunnerFactory factoryruntime.WorkersMockCommandRunnerFactory,
	providerFromCommandRunnerFactory ProviderFromCommandRunnerFactory,
	durableExecutionFactory DurableExecutionFactory,
	factorySessionExecutionFactory FactorySessionExecutionFactory,
	providerIdentities factorysessions.ProviderIdentityResolver,
	resolveClock factoryruntime.ClockResolver,
	factoryRuntimeAssembler FactoryRuntimeAssembler,
	recordingsRuntime recordings.RuntimeOpening,
	initialFactorySnapshotFactory factorydefinitions.InitialFactorySnapshotFactory,
	loadFactory factorydefinitions.LoadedFactoryLoader,
	automationService automations.Service,
	submissionRecorder recordings.SubmissionRecorder,
	dispatchRecorder recordings.DispatchRecorder,
) (durableexecution.Service, func() error, error) {
	if durableExecutionFactory == nil {
		return nil, nil, fmt.Errorf("construct portable replay runtime: durable execution operation is required")
	}
	clock, err := clockForReplay(clockEdge, nil, nil, resolveClock)
	if err != nil {
		return nil, nil, err
	}
	durable, providerForDurable, err := constructPortableReplayDurableOwner(
		configured,
		root,
		clock,
		providerOverride,
		providerCommandRunner,
		workersMockCommandRunnerFactory,
		providerFromCommandRunnerFactory,
		durableExecutionFactory,
		factorySessionExecutionFactory,
		providerIdentities,
	)
	if err != nil {
		return nil, nil, err
	}
	if durable.Service == nil {
		return nil, nil, fmt.Errorf("construct portable replay runtime: durable execution owner is required")
	}
	cleanup := newPortableReplayRuntimeCleanup()
	owner := &portableReplayDurableOwner{
		Service: durable.Service,
		prepare: func(probeContext context.Context) error {
			runtime, err := preparePortableReplayRuntime(
				probeContext,
				configured,
				root,
				clock,
				logger,
				factoryRuntimeAssembler,
				durable.Service,
				workerService,
				providerForDurable,
				providerOverride,
				providerCommandRunner,
				scriptCommandRunner,
				workersMockCommandRunnerFactory,
				submissionRecorder,
				dispatchRecorder,
				recordingsRuntime,
				initialFactorySnapshotFactory,
				loadFactory,
				automationService,
			)
			if err != nil {
				return err
			}
			cleanup.Set(runtime)
			return nil
		},
	}
	cleanup.SetOwner(owner)
	return owner, cleanup.Close, nil
}

func preparePortableReplayRuntime(
	ctx context.Context,
	configured preparedRuntime,
	root RuntimeRoot,
	clock factoryruntime.Clock,
	logger *zap.Logger,
	factoryRuntimeAssembler FactoryRuntimeAssembler,
	durableOwner durableexecution.Service,
	workerService workers.Service,
	providerForDurable providers.Service,
	providerOverride providers.Service,
	providerCommandRunner platformprocess.CommandRunner,
	scriptCommandRunner platformprocess.CommandRunner,
	workersMockCommandRunnerFactory factoryruntime.WorkersMockCommandRunnerFactory,
	submissionRecorder recordings.SubmissionRecorder,
	dispatchRecorder recordings.DispatchRecorder,
	recordingsRuntime recordings.RuntimeOpening,
	initialFactorySnapshotFactory factorydefinitions.InitialFactorySnapshotFactory,
	loadFactory factorydefinitions.LoadedFactoryLoader,
	automationService automations.Service,
) (runtimeports.RuntimeInstance, error) {
	runtime, err := assemblePortableReplayRuntime(
		ctx,
		configured,
		root,
		clock,
		logger,
		factoryRuntimeAssembler,
		durableOwner,
		providerOverride,
		providerCommandRunner,
		scriptCommandRunner,
		workersMockCommandRunnerFactory,
		submissionRecorder,
		dispatchRecorder,
		recordingsRuntime,
		initialFactorySnapshotFactory,
		loadFactory,
		automationService,
	)
	if err != nil {
		return nil, err
	}
	runtimeService := runtime.RuntimeService()
	var resourceLeaseAdmission factoryruntime.ResourceCapacityLeaseAdmission
	if admission, ok := runtimeService.(factoryruntime.ResourceCapacityLeaseAdmission); ok {
		resourceLeaseAdmission = admission
	}
	if err := bindDurableExecutionCapabilities(
		configured.Session.FactorySessionID,
		durableOwner,
		workerService,
		runtimeService,
		resourceLeaseAdmission,
		configured.Runtime.RuntimeInstanceID,
		runtime.StreamGeneration(),
		runtime.RecordingLedger(),
		providerForDurable,
		configured.Workers.MockWorkers,
		providerCommandRunner,
		runtimeProgressPublisher(runtime),
		runtimeWorkerAttemptStarter(runtime),
	); err != nil {
		runtime.CloseArtifacts()
		return nil, err
	}
	return runtime, nil
}

func constructPortableReplayDurableOwner(
	configured preparedRuntime,
	root RuntimeRoot,
	clock factoryruntime.Clock,
	providerOverride providers.Service,
	providerCommandRunner platformprocess.CommandRunner,
	workersMockCommandRunnerFactory factoryruntime.WorkersMockCommandRunnerFactory,
	providerFromCommandRunnerFactory ProviderFromCommandRunnerFactory,
	durableExecutionFactory DurableExecutionFactory,
	factorySessionExecutionFactory FactorySessionExecutionFactory,
	providerIdentities factorysessions.ProviderIdentityResolver,
) (DurableExecution, providers.Service, error) {
	providerForDurable, err := resolveDurableExecutionProvider(
		providerOverride,
		configured.Workers.MockWorkers,
		nil,
		providerCommandRunner,
		workersMockCommandRunnerFactory,
		providerFromCommandRunnerFactory,
	)
	if err != nil {
		return DurableExecution{}, nil, err
	}
	durable, err := durableExecutionFactory(
		configured.Definition,
		configured.Session,
		configured.OperatorDefaults,
		root,
		clock,
		providerForDurable,
		configured.Workers.MockWorkers,
		factorySessionExecutionFactory,
		providerIdentities,
	)
	if err != nil {
		return DurableExecution{}, nil, err
	}
	return durable, providerForDurable, nil
}

func assemblePortableReplayRuntime(
	ctx context.Context,
	configured preparedRuntime,
	root RuntimeRoot,
	clock factoryruntime.Clock,
	logger *zap.Logger,
	factoryRuntimeAssembler FactoryRuntimeAssembler,
	durableOwner durableexecution.Service,
	providerOverride providers.Service,
	providerCommandRunner platformprocess.CommandRunner,
	scriptCommandRunner platformprocess.CommandRunner,
	workersMockCommandRunnerFactory factoryruntime.WorkersMockCommandRunnerFactory,
	submissionRecorder recordings.SubmissionRecorder,
	dispatchRecorder recordings.DispatchRecorder,
	recordingsRuntime recordings.RuntimeOpening,
	initialFactorySnapshotFactory factorydefinitions.InitialFactorySnapshotFactory,
	loadFactory factorydefinitions.LoadedFactoryLoader,
	automationService automations.Service,
) (runtimeports.RuntimeInstance, error) {
	if factoryRuntimeAssembler == nil {
		return nil, fmt.Errorf("construct portable replay runtime: Factory Runtime assembler is required")
	}
	if recordingsRuntime == nil {
		return nil, fmt.Errorf("construct portable replay runtime: Recordings runtime opening is required")
	}
	projection := recordingsRuntime.Projection()
	if projection == nil {
		return nil, fmt.Errorf("construct portable replay runtime: Recordings projection is unavailable")
	}
	mutationOwner, ok := durableOwner.(interface {
		RecordPetriTokenMutations(string, []factorydefinitions.TokenMutationRecord) error
	})
	if !ok {
		return nil, fmt.Errorf("construct portable replay runtime: durable execution owner does not record Petri mutations")
	}
	progressFactory := fanOutWorkerProgress(nil, durableOwner)
	_, runtime, _, _, _, err := factoryRuntimeAssembler.Assemble(
		ctx,
		configured.OperatorDefaults.WorkerModelProvider,
		configured.OperatorDefaults.WorkerModel,
		false,
		configured.Recordings.RecordPath,
		configured.Recordings.WorkflowID,
		configured.Session.FactorySessionID,
		nil,
		loadFactory,
		providerOverride,
		providerCommandRunner,
		scriptCommandRunner,
		configured.Workers.MockWorkers,
		configured.Runtime.Mode,
		factoryruntime.Scheduler(nil),
		false,
		submissionRecorder,
		dispatchRecorder,
		configured.Runtime.LogDirectory,
		configured.Runtime.LogConfig,
		factoryruntime.RuntimeFileLoggingPolicy(configured.Runtime.FileLoggingPolicy),
		factoryruntime.RuntimeMetricsPolicy(configured.Runtime.MetricsPolicy),
		configured.Runtime.MetricsDirectory,
		configured.Runtime.MetricsConfig,
		configured.Recordings.FlushInterval,
		configured.Session.BackendScopeID,
		configured.Workers.RunnerID,
		configured.Runtime.Verbose,
		configured.Workers.SkipBuiltInPrerequisiteValidation,
		configured.Workers.InvocationSkipPermissionsOverride,
		clock,
		logger,
		workersMockCommandRunnerFactory,
		progressFactory,
		nil,
		mutationOwner.RecordPetriTokenMutations,
		projection.ReconstructFactoryWorldState,
		recordingsRuntime,
		initialFactorySnapshotFactory,
		configured.Definition.Directory,
		root.FactoryRootDir,
		configured.Definition.ExecutionBaseDir,
		nil,
		configured.Runtime.RuntimeInstanceID,
		nil,
		nil,
		nil,
		nil,
		automationService,
		false,
	)
	if err != nil {
		return nil, fmt.Errorf("construct portable replay runtime: %w", err)
	}
	if runtime == nil {
		return nil, fmt.Errorf("construct portable replay runtime: runtime instance is required")
	}
	return runtime, nil
}

func bindDurableExecutionCapabilities(
	sessionID string,
	execution durableexecution.Service,
	workerService workers.Service,
	invoker factoryruntime.Service,
	admission factoryruntime.ResourceCapacityLeaseAdmission,
	runtimeID string,
	generationID string,
	recordingLedger recordings.Ledger,
	providerOverride providers.Service,
	mockWorkers *workers.MockWorkersConfig,
	commandRunner platformprocess.CommandRunner,
	progressPublisher workers.ProgressPublisher,
	attemptStarter func(context.Context, workers.ExecuteRequest) (func(context.Context, workers.ExecuteResult, error) error, error),
) error {
	setWorkerInvoker(execution, invoker)
	setDispatchDurability(execution, recordingLedger, generationID)
	if err := setWorkerExecution(
		sessionID,
		execution,
		workerService,
		admission,
		runtimeID,
		generationID,
		providerOverride,
		mockWorkers,
		commandRunner,
	); err != nil {
		return err
	}
	setWorkerProgressPublisher(execution, progressPublisher)
	setWorkerAttemptStarter(execution, attemptStarter)
	return nil
}

func setDispatchDurability(
	execution durableexecution.Service,
	ledger recordings.Ledger,
	generationID string,
) {
	reader, _ := ledger.(recordings.CompletedFlushWatermarkReader)
	setter, ok := execution.(interface {
		SetDispatchDurability(recordings.CompletedFlushWatermarkReader, string)
	})
	if !ok {
		return
	}
	setter.SetDispatchDurability(reader, generationID)
}

type durableSessionStateReader interface {
	HasDurableState(context.Context, string) (bool, error)
}

type currentBoardHistoryOpening struct {
	allowMissingHistory bool
	hasDurableState     bool
}

const (
	currentBoardRecordingFailureCode    = "CURRENT_BOARD_RECORDING_FAILURE"
	currentBoardRecordingMissingCode    = "CURRENT_BOARD_RECORDING_MISSING"
	currentBoardRecordingCorruptCode    = "CURRENT_BOARD_RECORDING_CORRUPT"
	currentBoardRecordingUnreadableCode = "CURRENT_BOARD_RECORDING_UNREADABLE"
)

// currentBoardHistoryRestoreError is also a safe CLI error. The method names
// intentionally match the transport's narrow coded-error interface without
// making runtime opening depend on the CLI package. Its Error method never
// includes the underlying cause, which keeps startup diagnostics safe while
// Unwrap retains typed failure matching for service callers.
type currentBoardHistoryRestoreError struct {
	code    string
	message string
	cause   error
}

func (err *currentBoardHistoryRestoreError) Error() string {
	if err == nil {
		return ""
	}
	return err.message
}

func (err *currentBoardHistoryRestoreError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.cause
}

func (err *currentBoardHistoryRestoreError) CLIErrorCode() string {
	if err == nil {
		return ""
	}
	return err.code
}

func (err *currentBoardHistoryRestoreError) CLIErrorMessage() string {
	if err == nil {
		return ""
	}
	return err.message
}

func logCurrentBoardHistoryFailure(
	logger *zap.Logger,
	sessionID string,
	recordPath string,
	err error,
) {
	if logger == nil {
		return
	}
	kind := "UNREADABLE_OR_CORRUPT_RECORDING"
	var queryErr *recordings.HistoricalRecordingQueryError
	if errors.As(err, &queryErr) && queryErr.Kind != "" {
		kind = string(queryErr.Kind)
	}
	logger.Error(
		"current Factory Session board recording could not be restored; startup aborted",
		zap.String("session_id", sessionID),
		zap.String("recording_path", recordPath),
		zap.String("failure", kind),
	)
}

// inspectCurrentBoardHistory makes the missing-history escape hatch explicit.
// A successful persistence probe is required before a missing board recording
// can be treated as either a fresh opening or an interrupted write. The latter
// is observable through hasDurableState so the caller can warn that the board
// was lost while preserving the durable snapshot.
func inspectCurrentBoardHistory(
	ctx context.Context,
	service any,
	sessionID string,
) (currentBoardHistoryOpening, error) {
	if service == nil {
		return currentBoardHistoryOpening{}, fmt.Errorf("inspect current Factory Session board history: durable session state probe is unavailable")
	}
	probe, ok := service.(durableSessionStateReader)
	if !ok {
		return currentBoardHistoryOpening{}, fmt.Errorf("inspect current Factory Session board history: durable session state probe is unavailable")
	}
	hasDurableState, err := probe.HasDurableState(ctx, sessionID)
	if err != nil {
		return currentBoardHistoryOpening{}, fmt.Errorf("inspect current Factory Session board history initialization: %w", err)
	}
	return currentBoardHistoryOpening{
		allowMissingHistory: true,
		hasDurableState:     hasDurableState,
	}, nil
}

type currentBoardHistory struct {
	state  *factorydefinitions.FactoryWorldState
	events []factorydefinitions.FactoryEvent
}

// restoreCurrentBoardHistory loads both the detached board state and the
// canonical Factory event prefix that produced it. The live successor seeds
// its new Recordings ledger with that prefix so historical dispatch/Worker
// Session associations remain available after a process replacement.
func restoreCurrentBoardHistory(
	service historicalRecordingReader,
	recordPath string,
	sessionID string,
	allowMissingHistory bool,
) (*currentBoardHistory, error) {
	recordPath = strings.TrimSpace(recordPath)
	if recordPath == "" {
		return nil, nil
	}
	recordPath = factoryruntime.RecordingPath(recordPath).ForSession(sessionID)
	if service == nil {
		return nil, fmt.Errorf("restore current Factory Session board: Recordings history is unavailable")
	}
	scope := recordings.CanonicalEventScope{FactorySessionID: strings.TrimSpace(sessionID)}
	result, err := service.QueryHistoricalRecording(recordings.HistoricalRecordingQueryRequest{
		Recording: recordings.HistoricalRecordingIdentity{
			RecordingID: recordings.RecordingID("current-board/" + scope.FactorySessionID),
			Artifact:    recordings.RecordingArtifactReference(recordPath),
			Scope:       scope,
		},
	})
	if err != nil {
		var queryErr *recordings.HistoricalRecordingQueryError
		if errors.As(err, &queryErr) && queryErr.Kind == recordings.HistoricalRecordingQueryErrorMissingHistory {
			if allowMissingHistory {
				return nil, nil
			}
			return nil, currentBoardHistoryFailure(
				recordPath,
				sessionID,
				"MISSING_HISTORY: durable state exists but the current-board recording is missing; preserve the durable snapshot and restore the recording from a trusted backup without deleting the snapshot",
				err,
			)
		}
		if errors.As(err, &queryErr) && queryErr.Kind == recordings.HistoricalRecordingQueryErrorCorruptHistory {
			return nil, currentBoardHistoryFailure(
				recordPath,
				sessionID,
				"CORRUPT_HISTORY: the current-board recording is corrupt or incompatible; preserve the artifact for investigation and repair or replace it from a trusted backup before retrying",
				err,
			)
		}
		if errors.As(err, &queryErr) && queryErr.Kind == recordings.HistoricalRecordingQueryErrorUnavailable {
			return nil, currentBoardHistoryFailure(
				recordPath,
				sessionID,
				"UNREADABLE_RECORDING: the current-board recording is present but unreadable; preserve the artifact for investigation and repair access or replace it from a trusted backup before retrying",
				err,
			)
		}
		return nil, currentBoardHistoryFailure(
			recordPath,
			sessionID,
			"UNREADABLE_OR_CORRUPT_RECORDING: the current-board recording is unreadable or corrupt; preserve the artifact for investigation and repair it or replace it from a trusted backup before retrying",
			err,
		)
	}
	view := result.WorldState
	if view.SchemaVersion != recordings.WorldStateViewSchemaV1 || strings.TrimSpace(view.Payload) == "" {
		return nil, currentBoardHistoryFailure(
			recordPath,
			sessionID,
			"CORRUPT_HISTORY: Recordings returned an incompatible or empty world-state view; preserve the artifact for investigation and repair or replace it from a trusted backup before retrying",
			nil,
		)
	}
	if view.Scope != scope {
		return nil, currentBoardHistoryFailure(
			recordPath,
			sessionID,
			fmt.Sprintf(
				"CORRUPT_HISTORY: world-state scope %#v does not match %#v; preserve the artifact for investigation and repair or replace it from a trusted backup before retrying",
				view.Scope,
				scope,
			),
			nil,
		)
	}
	var state factorydefinitions.FactoryWorldState
	if err := json.Unmarshal([]byte(view.Payload), &state); err != nil {
		return nil, currentBoardHistoryFailure(
			recordPath,
			sessionID,
			"CORRUPT_HISTORY: decode world state failed; preserve the artifact for investigation and repair or replace it from a trusted backup before retrying",
			err,
		)
	}
	return &currentBoardHistory{
		state:  &state,
		events: factoryEventsFromCanonical(result.Events),
	}, nil
}

func factoryEventsFromCanonical(events []recordings.CanonicalEvent) []factorydefinitions.FactoryEvent {
	if len(events) == 0 {
		return nil
	}
	converted := make([]factorydefinitions.FactoryEvent, len(events))
	for index, event := range events {
		context := factorydefinitions.FactoryEventContext{
			EventTime: event.RecordedAt,
			Sequence:  int(event.Sequence),
			Tick:      event.FactoryTick,
		}
		if json.Valid([]byte(event.SourceContext)) {
			_ = json.Unmarshal([]byte(event.SourceContext), &context)
		}
		if event.Scope.FactorySessionID != "" {
			sessionID := event.Scope.FactorySessionID
			context.SessionID = &sessionID
		}
		converted[index] = factorydefinitions.FactoryEvent{
			Context:       context,
			Id:            string(event.ID),
			Payload:       json.RawMessage(event.Payload),
			SchemaVersion: factorydefinitions.FactoryEventSchemaVersionV1,
			Type:          factorydefinitions.FactoryEventType(event.Kind),
		}
	}
	return converted
}

func currentBoardHistoryFailure(
	recordPath string,
	sessionID string,
	diagnostic string,
	cause error,
) error {
	message := fmt.Sprintf(
		"restore current Factory Session board from %q (session %q): %s",
		recordPath,
		sessionID,
		diagnostic,
	)
	code := currentBoardHistoryFailureCode(diagnostic)
	return &currentBoardHistoryRestoreError{
		code:    code,
		message: message,
		cause:   cause,
	}
}

func currentBoardHistoryFailureCode(diagnostic string) string {
	colon := strings.IndexByte(diagnostic, ':')
	if colon < 0 {
		return currentBoardRecordingFailureCode
	}
	switch strings.TrimSpace(diagnostic[:colon]) {
	case "MISSING_HISTORY":
		return currentBoardRecordingMissingCode
	case "CORRUPT_HISTORY":
		return currentBoardRecordingCorruptCode
	case "UNREADABLE_RECORDING":
		return currentBoardRecordingUnreadableCode
	default:
		return currentBoardRecordingFailureCode
	}
}
