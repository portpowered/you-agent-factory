package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
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
	mu           sync.Mutex
	owner        interface{ Close() error }
	closeRuntime func(context.Context) error
	releaseScope func()
	closed       bool
	closeErr     error
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

func (cleanup *portableReplayRuntimeCleanup) Set(opening *factoryruntime.RuntimeInitialOpening) {
	if cleanup == nil || opening == nil || opening.Activation == nil || opening.Activation.Close == nil {
		return
	}
	cleanup.mu.Lock()
	defer cleanup.mu.Unlock()
	if cleanup.closed {
		cleanup.closeErr = errors.Join(cleanup.closeErr, opening.Activation.Close(context.Background()))
		return
	}
	cleanup.closeRuntime = opening.Activation.Close
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
	if cleanup.releaseScope != nil {
		cleanup.releaseScope()
		cleanup.releaseScope = nil
	}
	if cleanup.closeRuntime != nil {
		cleanup.closeErr = errors.Join(cleanup.closeErr, cleanup.closeRuntime(context.Background()))
		cleanup.closeRuntime = nil
	}
	return cleanup.closeErr
}

// openPortableReplayDurableOwner constructs the existing durable execution
// owner for a checkpoint-bearing replay. Runtime assembly is deferred until
// HasRestorableState confirms that the durable owner can actually resume; a
// public checkpoint summary alone must remain inspection-only.
func (r *Root) openPortableReplayDurableOwner(
	configured preparedRuntime,
	root RuntimeRoot,
) (durableexecution.Service, func() error, error) {
	if r.durableOpening == nil {
		return nil, nil, fmt.Errorf("construct portable replay runtime: durable execution operation is required")
	}
	clock, err := clockForReplay(r.clock, nil, nil, r.resolveClock)
	if err != nil {
		return nil, nil, err
	}
	durable, err := r.durableOpening.Open(
		configured.Definition,
		configured.Session.Persistence,
		runtimeSelectionForStart(configured.Session).SystemConfigHome,
		runtimeSelectionForStart(configured.Session).SystemConfigPath,
		configured.OperatorDefaults,
		root,
		clock,
		r.providerOverride,
		configured.Workers.MockWorkers,
	)
	if err != nil {
		// Acquisition may have opened resources before failing. Return only
		// their owned release; the failed candidate must never become usable.
		cleanup := newPortableReplayRuntimeCleanup()
		cleanup.SetOwner(&portableReplayDurableOwner{Service: durable.Service})
		return nil, cleanup.Close, err
	}
	if durable.Service == nil {
		return nil, nil, fmt.Errorf("construct portable replay runtime: durable execution owner is required")
	}
	cleanup := newPortableReplayRuntimeCleanup()
	owner := &portableReplayDurableOwner{
		Service: durable.Service,
		prepare: func(probeContext context.Context) error {
			runtime, err := r.preparePortableReplayRuntime(
				probeContext,
				configured,
				durable.Service,
				cleanup,
			)
			// A failed opening can still own artifacts. Register them before
			// forwarding the error; the durable owner must stop before release.
			cleanup.Set(runtime)
			return err
		},
	}
	cleanup.SetOwner(owner)
	return owner, cleanup.Close, nil
}

func (r *Root) preparePortableReplayRuntime(
	ctx context.Context,
	configured preparedRuntime,
	durableOwner durableexecution.Service,
	cleanup *portableReplayRuntimeCleanup,
) (*factoryruntime.RuntimeInitialOpening, error) {
	opening, err := r.assemblePortableReplayRuntime(
		ctx,
		configured,
		durableOwner,
	)
	if err != nil {
		return opening, err
	}
	runtime := opening.Record
	runtimeService := runtime.RuntimeService()
	var resourceLeaseAdmission factoryruntime.ResourceCapacityLeaseAdmission
	if admission, ok := runtimeService.(factoryruntime.ResourceCapacityLeaseAdmission); ok {
		resourceLeaseAdmission = admission
	}
	releaseScope, err := bindDurableExecutionCapabilities(
		configured.Session.SessionID,
		durableOwner,
		runtimeService,
		resourceLeaseAdmission,
		configured.Runtime.RuntimeInstanceID,
		runtime.StreamGeneration(),
		runtime.RecordingLedger(),
		r.providerOverride,
		configured.Workers.MockWorkers,
		r.providerCommandRunner,
		runtimeProgressPublisher(runtime),
		runtimeWorkerAttemptStarter(runtime),
	)
	if err != nil {
		return opening, err
	}
	cleanup.mu.Lock()
	if cleanup.closed {
		releaseScope()
	} else {
		cleanup.releaseScope = releaseScope
	}
	cleanup.mu.Unlock()
	return opening, nil
}

func (r *Root) assemblePortableReplayRuntime(
	ctx context.Context,
	configured preparedRuntime,
	durableOwner durableexecution.Service,
) (*factoryruntime.RuntimeInitialOpening, error) {
	mutationOwner, ok := durableOwner.(interface {
		RecordPetriTokenMutations(string, []factorydefinitions.TokenMutationRecord) error
	})
	if !ok {
		return nil, fmt.Errorf("construct portable replay runtime: durable execution owner does not record Petri mutations")
	}
	// Preserve the replay compatibility path's optional progress observation.
	var observe workers.ProgressPublisher
	if owner, ok := durableOwner.(interface {
		PublishWorkerProgress(workers.ProgressFragment)
	}); ok {
		observe = owner.PublishWorkerProgress
	}
	observations := replaySessionObservations{mutations: mutationOwner.RecordPetriTokenMutations, progress: observe}
	// Select the authored live definition only after the durable owner confirms
	// restorable state. Inspection and unsuccessful probes remain nonexecuting.
	resolved, err := r.resolveActivationSnapshot(ctx, configured.Definition, recordings.RuntimeSelection{}, nil, nil, configured.Session.SessionID)
	if err != nil {
		return nil, err
	}
	inputs := runtimeActivationInputs(configured.Definition, configured.Session,
		configured.CanonicalSessionIDGenerated, configured.Workers, configured.Recordings,
		configured.ModelCacheDirectory, configured.OperatorDefaults, nil)
	inputs.Session.CanonicalSessionID = configured.Session.SessionID
	inputs.RecoveryInput.CheckpointContinuation = true
	opening, err := r.initialActivation(ctx, factoryruntime.RuntimeActivationRequest{
		RuntimeID: configured.Runtime.RuntimeInstanceID, FactorySessionID: configured.Session.SessionID,
		Snapshot: resolved.snapshot, Runtime: configured.Runtime, Inputs: inputs,
	}, observations)
	if err != nil {
		return opening, fmt.Errorf("construct portable replay runtime: %w", err)
	}
	if opening == nil || opening.Record == nil {
		return opening, fmt.Errorf("construct portable replay runtime: runtime instance is required")
	}
	return opening, nil
}

// replaySessionObservations preserves the legacy replay boundary's optional
// progress capability while keeping its required mutation owner scoped.
type replaySessionObservations struct {
	mutations factoryruntime.PetriMutationRecorder
	progress  workers.ProgressPublisher
}

func (observations replaySessionObservations) RecordPetriTokenMutations(sessionID string, records []factorydefinitions.TokenMutationRecord) error {
	return observations.mutations(sessionID, records)
}

func (observations replaySessionObservations) PublishWorkerProgress(fragment workers.ProgressFragment) {
	if observations.progress != nil {
		observations.progress(fragment)
	}
}

func bindDurableExecutionCapabilities(
	sessionID string,
	execution durableexecution.Service,
	invoker factoryruntime.Service,
	admission factoryruntime.ResourceCapacityLeaseAdmission,
	runtimeID string,
	generationID string,
	recordingLedger recordings.Ledger,
	providerOverride providers.Service,
	mockWorkers *workers.MockWorkersConfig,
	commandRunner platformprocess.CommandRunner,
	progressPublisher workers.ProgressPublisher,
	attemptStarter func(context.Context, *workers.ExecuteRequest) (func(context.Context, workers.ExecuteResult, error) (workers.ExecuteResult, error), error),
) (func(), error) {
	release, err := bindWorkerScope(
		sessionID,
		execution,
		admission,
		runtimeID,
		generationID,
		providerOverride,
		mockWorkers,
		commandRunner,
		progressPublisher,
		attemptStarter,
	)
	if err != nil {
		return nil, err
	}
	releaseLiveChange := bindLiveChangeScope(sessionID, execution, invoker)
	releaseDurability := bindDispatchDurability(sessionID, execution, recordingLedger, generationID)
	return func() {
		release()
		releaseDurability()
		releaseLiveChange()
	}, nil
}

func bindLiveChangeScope(sessionID string, execution durableexecution.Service, runtime factoryruntime.Service) func() {
	binder, ok := execution.(interface {
		BindLiveChangeScope(string, factorysessions.LiveChangeApplication, factorysessions.LiveChangeAdmission, bool, func(int)) func()
	})
	if !ok || runtime == nil {
		return func() {}
	}
	_, required := runtime.(factoryruntime.AdmittedResourceCapacityService)
	var setRevision func(int)
	if revision, ok := runtime.(factoryruntime.ResourceCapacityRevisionService); ok {
		setRevision = revision.SetFactoryRevision
	}
	return binder.BindLiveChangeScope(sessionID, runtimebinding.NewLiveChangeApplication(runtime),
		runtimebinding.NewLiveChangeAdmission(runtime), required, setRevision)
}

func bindDispatchDurability(
	sessionID string,
	execution durableexecution.Service,
	ledger recordings.Ledger,
	generationID string,
) func() {
	reader, _ := ledger.(recordings.CompletedFlushWatermarkReader)
	binder, ok := execution.(interface {
		BindDispatchDurability(string, recordings.CompletedFlushWatermarkReader, string) func()
	})
	if !ok {
		return func() {}
	}
	return binder.BindDispatchDurability(sessionID, reader, generationID)
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
