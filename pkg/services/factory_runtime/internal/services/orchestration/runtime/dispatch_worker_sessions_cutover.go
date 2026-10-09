package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func runtimeAttemptPreparation(
	cfg *runtimeConfig,
	request workers.WorkstationDispatchRequest,
	executeRequest workers.ExecuteRequest,
	allowRetry bool,
) attemptPreparation {
	if cfg == nil || cfg.workerAttempts == nil {
		return nil
	}
	if resolver, ok := cfg.completionDeliveryPlanner.(factory.ReplayWorkerSessionIDResolver); ok {
		if _, recorded := resolver.WorkerSessionIDForDispatch(request.Execution.Dispatch); recorded {
			// Recorded dispatches derive their Worker observations from Factory
			// history. Replaying a result must not admit the original Worker ID
			// again or reacquire execution/control authority over its live owner.
			return nil
		}
	}
	recorder := cfg.workerAttempts
	lifecycle := cfg.attempts
	clock := cfg.clock
	execution := cfg.workerExecution
	scheduler := cfg.workerAttemptScheduler
	dispatchID := strings.TrimSpace(executeRequest.Correlation.DispatchID)
	return func(ctx context.Context, executing *workers.ExecuteRequest) (attemptCompletionFunc, error) {
		sessionID := runtimeWorkerSessionID(cfg, request, executeRequest, allowRetry)
		admissionRequest := request
		if strings.TrimSpace(request.WorkstationName) != workers.ProviderInvocationRoute {
			admissionRequest = runtimeAttemptAdmissionRequest(request, executeRequest)
		}
		if strings.TrimSpace(admissionRequest.Execution.RuntimeID) == "" {
			admissionRequest.Execution.RuntimeID = strings.TrimSpace(executeRequest.Correlation.RuntimeID)
		}
		if executing != nil {
			admissionRequest.Execution.AttemptControlObserver = executing.Input.AttemptControlObserver
		}
		attempt, err := recorder.BeginRuntimeAttempt(
			context.WithoutCancel(ctx),
			workersessions.RuntimeAttemptRequest{
				Key:                         workersessions.RuntimeAttemptKey{RuntimeID: executeRequest.Correlation.RuntimeID, DispatchID: executeRequest.Correlation.DispatchID},
				ObservationRuntimeID:        cfg.runtimeID,
				ObservationFactorySessionID: sessionIDFromFactoryConfig(cfg),
				ID:                          sessionID,
				AttemptID:                   executeRequest.Correlation.AttemptID,
				Execution:                   admissionRequest,
				BindAttemptControl:          bindRuntimeAttemptControl(executing, runtimeForceDispositionAvailable(cfg, request, executeRequest)),
			},
			execution,
			clock,
			scheduler,
			func(cancelCtx context.Context) (workers.WorkstationDispatchCancelOutcome, error) {
				if lifecycle == nil {
					return "", ErrAttemptLifecycleUnavailable
				}
				return lifecycle.cancel(cancelCtx, dispatchID)
			},
		)
		if err != nil {
			return nil, err
		}
		return func(callbackCtx context.Context, _ workers.ExecuteRequest, result workers.ExecuteResult, executeErr error) (workers.ExecuteResult, error) {
			result = normalizeAttemptResult(
				executeRequest,
				result,
				executeErr,
				platformprocess.CancellationReasonFromError(executeErr),
			)
			result = normalizeDetachedExecutionResult(cfg, executeRequest, result)
			dispatchResult, dispatchErr := workstationDispatchResultFromExecute(
				workstationDispatchRequestForResult(request, executeRequest),
				result,
				executeErr,
			)
			_, forced, completionErr := attempt.Resolve(callbackCtx, dispatchResult, dispatchErr)
			if forced {
				lifecycle.recordConfirmedForce(request.Execution.Dispatch.DispatchID)
				result.Failure = nil
				result.ProposedOutputPresent = false
				result = canceledAttemptResult(executeRequest, result, workers.DispatchCancellationReasonCanceled)
				return result, completionErr
			}
			return result, errors.Join(executeErr, completionErr)
		}, nil
	}
}

// Factory force must have an authored terminal destination for every input
// Work. Withholding the capability makes the existing force path UNSUPPORTED
// before any process effect; ordinary cancellation keeps its original owner.
func runtimeForceDispositionAvailable(cfg *runtimeConfig, request workers.WorkstationDispatchRequest, execution workers.ExecuteRequest) bool {
	if request.WorkstationName == workers.ProviderInvocationRoute {
		return true
	}
	if cfg == nil || cfg.net == nil {
		return false
	}
	workFound := false
	for _, input := range execution.Input.Work {
		if input.Kind == string(workers.DataTypeResource) {
			continue
		}
		workFound = true
		placeID := restoredFailedPlaceID(cfg.net, input.WorkTypeID)
		if placeID == "" || cfg.net.Places[placeID] == nil {
			return false
		}
	}
	return workFound
}

func bindRuntimeAttemptControl(executing *workers.ExecuteRequest, forceAvailable bool) func(providers.AttemptControlObserver) {
	return func(observe providers.AttemptControlObserver) {
		if executing == nil || observe == nil {
			return
		}
		executing.Input.AttemptControlObserver = func(control providers.AttemptControl) {
			if forceAvailable {
				observe(control)
			}
		}
	}
}

func runtimeAttemptAdmissionRequest(
	request workers.WorkstationDispatchRequest,
	executeRequest workers.ExecuteRequest,
) workers.WorkstationDispatchRequest {
	request.Execution = workers.CloneWorkstationExecutionRequest(request.Execution)
	request.Execution.Model = strings.TrimSpace(executeRequest.Target.Model.Name)
	request.Execution.ModelProvider = strings.TrimSpace(executeRequest.Target.Model.Provider)
	request.Execution.ReasoningEffort = strings.TrimSpace(executeRequest.Target.Model.ReasoningEffort)
	return request
}

func runtimeWorkerSessionID(
	cfg *runtimeConfig,
	request workers.WorkstationDispatchRequest,
	executeRequest workers.ExecuteRequest,
	allowRetry bool,
) string {
	if allowRetry && strings.TrimSpace(executeRequest.Correlation.AttemptID) != "" {
		return strings.TrimSpace(executeRequest.Correlation.AttemptID)
	}
	sessionID := strings.TrimSpace(executeRequest.Correlation.DispatchID)
	if resolver, ok := cfg.completionDeliveryPlanner.(factory.ReplayWorkerSessionIDResolver); ok {
		if recordedSessionID, found := resolver.WorkerSessionIDForDispatch(request.Execution.Dispatch); found {
			sessionID = recordedSessionID
		}
	}
	return sessionID
}

// workerSessionDispatchOutcome preserves handoff results and rejects admission failures.
func workerSessionDispatchOutcome(
	request workers.WorkstationDispatchRequest,
	startResult workersessions.InvokeSessionResult,
	startErr error,
) (workers.WorkstationDispatchResult, error) {
	dispatchID := request.Execution.Dispatch.DispatchID
	transitionID := request.Execution.Dispatch.TransitionID
	if startErr != nil {
		return workers.WorkstationDispatchResult{
			DispatchID:      dispatchID,
			WorkstationName: request.WorkstationName,
			TerminalOutcome: workers.WorkstationDispatchTerminalOutcomeFailed,
			Result: workerexecution.WorkResult{
				DispatchID:   dispatchID,
				TransitionID: transitionID,
				Outcome:      workerexecution.OutcomeFailed,
				Error:        startErr.Error(),
			},
		}, startErr
	}
	if handedOffToWorkers(startResult) {
		return startResult.Dispatch, startResult.DispatchErr
	}
	errText := "worker session start failed before Workers handoff"
	if startResult.Session.Result != nil && startResult.Session.Result.Cause != nil {
		errText = string(startResult.Session.Result.Cause.Kind)
	}
	return workers.WorkstationDispatchResult{
		DispatchID:      dispatchID,
		WorkstationName: request.WorkstationName,
		TerminalOutcome: workers.WorkstationDispatchTerminalOutcomeFailed,
		Result: workerexecution.WorkResult{
			DispatchID:   dispatchID,
			TransitionID: transitionID,
			Outcome:      workerexecution.OutcomeFailed,
			Error:        errText,
		},
	}, nil
}

// handedOffToWorkers reports whether Start reached DispatchWorkstation.
func handedOffToWorkers(startResult workersessions.InvokeSessionResult) bool {
	result := startResult.Session.Result
	if result == nil || result.Cause == nil {
		return true
	}
	return result.Cause.Kind != workersessions.FailureCauseEventPublicationFailure
}

// WorkerSessionsObservation returns detached Worker Session observations.
func (f *factoryImpl) WorkerSessionsObservation() workersessions.ObservationService {
	if f == nil || f.cfg == nil {
		return nil
	}
	return f.WorkerSessionsObservationForSession(sessionIDFromFactoryConfig(f.cfg))
}

// WorkerSessionsObservationForSession binds reads to the effective Factory Session.
func (f *factoryImpl) WorkerSessionsObservationForSession(factorySessionID string) workersessions.ObservationService {
	if f == nil || f.cfg == nil {
		return nil
	}
	factorySessionID = strings.TrimSpace(factorySessionID)
	if factorySessionID == "" {
		factorySessionID = sessionIDFromFactoryConfig(f.cfg)
	}
	var workerRecordingReader recordings.WorkerRecordingReader
	if reader, ok := f.cfg.workerSessions.(recordings.WorkerRecordingReader); ok {
		workerRecordingReader = reader
	}
	view := newRecordedWorkerSessionObservationWithRestoredState(
		f.cfg.workerSessions,
		f.eventHistory,
		f.cfg.worldStateProjector,
		f.clock,
		f.cfg.providerSessions,
		f.cfg.replayEvents,
		f.cfg.recordingID,
		workerRecordingReader,
		f.cfg.restoredWorldState,
		f.cfg.restoredEventPrefix,
		factorySessionID,
	)
	view.runtimeID = strings.TrimSpace(f.cfg.runtimeID)
	view.executionFactorySessionID = canonicalSessionIDFromFactoryConfig(f.cfg)
	return view
}

// recordedWorkerSessionObservation adapts the runtime ledger and projector to
// the detached Worker Session observation vocabulary.
type recordedWorkerSessionObservation struct {
	workersessions.Service
	ledger                    recordings.RuntimeLedger
	durability                recordings.CompletedFlushWatermarkReader
	projector                 factory.WorldStateProjector
	clock                     factory.Clock
	providerSessions          providersessions.Service
	replayEvents              []interfaces.FactoryEvent
	restoredWorldState        *interfaces.FactoryWorldState
	restoredEventPrefix       []interfaces.FactoryEvent
	recordingID               string
	recordingReader           recordings.WorkerRecordingReader
	factorySessionID          string
	executionFactorySessionID string
	runtimeID                 string
}

var _ workersessions.Service = (*recordedWorkerSessionObservation)(nil)

func (s *recordedWorkerSessionObservation) canonicalEvents() []interfaces.FactoryEvent {
	if s == nil {
		return nil
	}
	if len(s.replayEvents) > 0 {
		return cloneAndSortFactoryEvents(s.replayEvents)
	}
	if s.ledger == nil {
		return nil
	}
	return s.ledger.CanonicalEvents()
}

func latestFactoryEventTick(events []interfaces.FactoryEvent) int {
	selectedTick := 0
	for _, event := range events {
		if event.Context.Tick > selectedTick {
			selectedTick = event.Context.Tick
		}
	}
	return selectedTick
}

func recordedDispatchStateMaps(
	world interfaces.FactoryWorldState,
) map[string]interfaces.FactoryWorldDispatchCompletion {
	completed := make(map[string]interfaces.FactoryWorldDispatchCompletion, len(world.CompletedDispatches))
	for _, dispatch := range world.CompletedDispatches {
		completed[dispatch.DispatchID] = dispatch
	}
	for _, dispatch := range world.FailedDispatches {
		completed[dispatch.DispatchID] = dispatch
	}
	return completed
}

func recordedDispatchEnd(
	dispatch interfaces.FactoryWorldDispatchCompletion,
	index recordedDispatchEventIndex,
	dispatchID string,
) *time.Time {
	ended := dispatch.CompletedAt
	if ended.IsZero() {
		ended = index.responseTimes[dispatchID]
	}
	if ended.IsZero() {
		return nil
	}
	ended = ended.UTC()
	return &ended
}

func recordedDispatchFacts(events []interfaces.FactoryEvent) (map[string]recordedDispatchAssociation, map[string]recordedDispatchRequest) {
	associations := make(map[string]recordedDispatchAssociation)
	requests := make(map[string]recordedDispatchRequest)
	for _, event := range events {
		dispatchID := stringPointerValue(event.Context.DispatchID)
		switch event.Type {
		case interfaces.FactoryEventTypeDispatchWorkerSessionAssoc:
			if dispatchID == "" {
				continue
			}
			var payload struct {
				WorkerSessionID string `json:"workerSessionId"`
				Model           string `json:"model"`
				ReasoningEffort string `json:"reasoningEffort"`
			}
			if json.Unmarshal(event.Payload, &payload) != nil || payload.WorkerSessionID == "" {
				continue
			}
			associations[dispatchID] = recordedDispatchAssociation{
				workerSessionID: payload.WorkerSessionID,
				model:           recordedOptionalString(payload.Model),
				reasoningEffort: recordedOptionalString(payload.ReasoningEffort),
				turnID:          stringPointerValue(event.Context.RequestID),
				eventTime:       event.Context.EventTime.UTC(),
			}
		case interfaces.FactoryEventTypeDispatchRequest:
			if dispatchID == "" {
				continue
			}
			var payload interfaces.DispatchRequestEventPayload
			if json.Unmarshal(event.Payload, &payload) != nil {
				continue
			}
			workIDs := append([]string(nil), pointerStringSlice(event.Context.WorkIDs)...)
			if len(workIDs) == 0 {
				for _, input := range payload.Inputs {
					if input.WorkID != "" {
						workIDs = appendUniqueRecordedString(workIDs, input.WorkID)
					}
				}
			}
			requests[dispatchID] = recordedDispatchRequest{
				workIDs:   workIDs,
				startedAt: event.Context.EventTime.UTC(),
			}
		}
	}
	return associations, requests
}

func newRecordedWorkerSessionObservation(
	live workersessions.Service,
	ledger recordings.RuntimeLedger,
	projector factory.WorldStateProjector,
	clock factory.Clock,
	providerSessions providersessions.Service,
) workersessions.Service {
	return newRecordedWorkerSessionObservationWithRestoredState(
		live, ledger, projector, clock, providerSessions, nil, "", nil, nil, nil,
	)
}

func (s *recordedWorkerSessionObservation) ListObservations(
	ctx context.Context,
	req workersessions.ListObservationsRequest,
) (workersessions.ListObservationsResult, error) {
	if err := req.Validate(); err != nil {
		return workersessions.ListObservationsResult{}, err
	}
	if err := observationContextError(ctx); err != nil {
		return workersessions.ListObservationsResult{}, err
	}
	if s == nil || s.ledger == nil {
		result, err := s.listLive(ctx, req)
		if err == nil {
			s.applyConfirmation(result.Observations, s.sampleCompletedFlushWatermark())
		}
		return result, err
	}

	live, liveErr := s.listLive(ctx, req)
	if liveErr != nil && !errors.Is(liveErr, workersessions.ErrObservationWorkNotFound) && s.Service != nil {
		return workersessions.ListObservationsResult{}, liveErr
	}
	recorded, knownWork, facts, err := s.projectListedWorkSnapshot(ctx, req.WorkID, listedObservationIndex(live.Observations))
	if err != nil {
		return workersessions.ListObservationsResult{}, err
	}
	health, err := s.recordingHealth(ctx)
	if err != nil {
		return workersessions.ListObservationsResult{}, err
	}
	s.decorateRecordingHealth(recorded, health)
	if liveErr == nil {
		s.decorateLiveRecordingHealth(live.Observations, health)
		recorded = mergeRecordedObservations(recorded, live.Observations)
	}
	sample := completedFlushWatermarkSample{}
	if len(recorded) > 0 || len(live.Observations) > 0 {
		sample = s.sampleCompletedFlushWatermark()
	}
	if err := applySelectedWorkConfirmation(ctx, recorded, *facts, sample); err != nil {
		return workersessions.ListObservationsResult{}, err
	}
	if err := applySelectedWorkConfirmation(ctx, live.Observations, *facts, sample); err != nil {
		return workersessions.ListObservationsResult{}, err
	}
	if err := observationContextError(ctx); err != nil {
		return workersessions.ListObservationsResult{}, err
	}
	return recordedObservationListResult(recorded, knownWork, live, liveErr)
}

// One request retains one detached selected snapshot for both row projection
// and terminal confirmation. A concurrent append is visible on the next read.
func (s *recordedWorkerSessionObservation) projectListedWorkSnapshot(ctx context.Context, workID string, live map[string]workersessions.Observation) ([]workersessions.Observation, bool, *recordings.WorkerSessionWorkFacts, error) {
	reader, ok := s.ledger.(recordings.WorkerSessionWorkProjectionReader)
	if !ok {
		return nil, false, nil, workersessions.ErrObservationProjectionUnavailable
	}
	facts, err := reader.CurrentWorkerSessionWorkFacts(ctx, workID)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, false, nil, workersessions.ErrObservationCanceled
		}
		if canceled := observationContextError(ctx); canceled != nil {
			return nil, false, nil, canceled
		}
		return nil, false, nil, workersessions.ErrObservationProjectionUnavailable
	}
	requests := make(map[string]recordedDispatchRequest, len(facts.Requests))
	for id, request := range facts.Requests {
		requests[id] = recordedDispatchRequest{workIDs: request.WorkItemIDs, startedAt: request.StartedAt}
	}
	index := recordedDispatchEventIndex{cursors: make(map[string]int64, len(facts.StateCursors)), responseTimes: facts.ResponseTimes, interruptions: make(map[string]recordedDispatchInterruptionFact, len(facts.Interruptions))}
	for id, cursor := range facts.StateCursors {
		index.cursors[id] = int64(cursor.Sequence)
	}
	for id, interruption := range facts.Interruptions {
		index.interruptions[id] = recordedDispatchInterruptionFact{workIDs: facts.Requests[id].WorkItemIDs, interruptedAt: interruption.InterruptedAt, reason: interruption.Reason}
	}
	completed := recordedDispatchStateMaps(facts.World)
	providers := make(map[string]interfaces.FactoryWorldProviderSessionRecord, len(facts.World.ProviderSessions))
	for _, provider := range facts.World.ProviderSessions {
		if _, exists := providers[provider.DispatchID]; !exists {
			providers[provider.DispatchID] = provider
		}
	}
	result := make([]workersessions.Observation, 0, len(facts.Associations))
	for id, association := range facts.Associations {
		var selectedProvider []interfaces.FactoryWorldProviderSessionRecord
		if provider, ok := providers[id]; ok {
			selectedProvider = []interfaces.FactoryWorldProviderSessionRecord{provider}
		}
		fact := s.annotateRecordedFact(recordedDispatchFact(id, recordedDispatchAssociation{workerSessionID: association.WorkerSessionID, turnID: association.TurnID, model: association.Model, reasoningEffort: association.ReasoningEffort, eventTime: association.AssociatedAt}, requests, completed, selectedProvider, facts.World.ActiveDispatches, index))
		fact.streamGenerationID = facts.StreamGenerationID
		observation := recordedObservationFromFact(fact, s.clock)
		if observation.State == workersessions.StateCanceled {
			observation, err = s.withCapturedWorkerIdentity(ctx, observation)
			if err != nil {
				return nil, false, nil, err
			}
		}
		if fact.provider != nil && !listedObservationMatches(observation, providerSessionRef(*fact.provider), live) {
			observation, err = s.enrichRecordedObservation(ctx, observation, providerSessionRef(*fact.provider))
			if err != nil {
				return nil, false, nil, err
			}
		}
		result = append(result, observation)
	}
	return result, facts.KnownWork, &facts, nil
}

func recordedObservationListResult(
	recorded []workersessions.Observation,
	knownWork bool,
	live workersessions.ListObservationsResult,
	liveErr error,
) (workersessions.ListObservationsResult, error) {
	if !knownWork && len(recorded) == 0 {
		if liveErr == nil && len(live.Observations) > 0 {
			return live, nil
		}
		return workersessions.ListObservationsResult{}, workersessions.ErrObservationWorkNotFound
	}
	if len(recorded) == 0 && liveErr == nil && len(live.Observations) > 0 {
		return live, nil
	}
	sortObservationAttempts(recorded)
	return workersessions.ListObservationsResult{Observations: recorded}, nil
}

func (s *recordedWorkerSessionObservation) listLive(
	ctx context.Context,
	req workersessions.ListObservationsRequest,
) (workersessions.ListObservationsResult, error) {
	if s == nil || s.Service == nil {
		return workersessions.ListObservationsResult{}, workersessions.ErrObservationProjectionUnavailable
	}
	if factorySessionID := strings.TrimSpace(s.factorySessionID); factorySessionID != "" {
		req.FactorySessionID = factorySessionID
	}
	return s.Service.ListObservations(ctx, req)
}

// ListWorkerSessionObservations decorates the process-local top-level list so
// direct Worker Session reads use the same explicit default as session-scoped
// and replay-backed observations.
func (s *recordedWorkerSessionObservation) ListWorkerSessionObservations(
	ctx context.Context,
	req workersessions.ListWorkerSessionObservationsRequest,
) (workersessions.ListWorkerSessionObservationsResult, error) {
	if s == nil || s.Service == nil {
		return workersessions.ListWorkerSessionObservationsResult{}, workersessions.ErrObservationProjectionUnavailable
	}
	req.RuntimeID = s.runtimeID
	result, err := s.Service.ListWorkerSessionObservations(ctx, req)
	if err == nil || errors.Is(err, workersessions.ErrObservationProjectionUnavailable) {
		if healthErr := s.applyLiveRecordingHealth(ctx, result.Observations); healthErr != nil {
			return workersessions.ListWorkerSessionObservationsResult{}, healthErr
		}
		s.applyConfirmation(result.Observations, s.sampleCompletedFlushWatermark())
	}
	return result, err
}

// Start carries the runtime-owned recording identity into direct admission.
func (s *recordedWorkerSessionObservation) Start(
	ctx context.Context,
	req workersessions.StartRequest,
) (workersessions.StartResult, error) {
	if s == nil || s.Service == nil {
		return workersessions.StartResult{}, workersessions.ErrObservationProjectionUnavailable
	}
	if strings.TrimSpace(req.Execution.Execution.RecordingID) == "" {
		req.Execution.Execution.RecordingID = strings.TrimSpace(s.recordingID)
	}
	return s.Service.Start(ctx, req)
}

type workerRecordingHealth struct {
	status    recordings.WorkerRecordingStatus
	reason    string
	startedAt *time.Time
}

func (s *recordedWorkerSessionObservation) recordingHealth(
	ctx context.Context,
) (map[string]workerRecordingHealth, error) {
	if s == nil || s.recordingReader == nil || s.recordingID == "" {
		return nil, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := observationContextError(ctx); err != nil {
		return nil, err
	}
	snapshot, err := s.recordingReader.LoadWorkerRecording(ctx, s.recordingID)
	if err != nil {
		return nil, recordingHealthLoadError(err)
	}
	return workerRecordingHealthMap(snapshot, s.recordingID)
}

func recordingHealthLoadError(err error) error {
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return workersessions.ErrObservationCanceled
	case errors.Is(err, os.ErrNotExist):
		return nil
	case errors.Is(err, recordings.ErrWorkerRecordingIncomplete):
		// A live recording is a valid partial source while its snapshot persists.
		return nil
	case isCorruptWorkerRecordingError(err):
		return fmt.Errorf("%w: %v", workersessions.ErrObservationRecordingCorrupt, err)
	default:
		return fmt.Errorf("%w: %v", workersessions.ErrObservationRecordingUnavailable, err)
	}
}

func workerRecordingHealthMap(
	snapshot recordings.WorkerRecordingSnapshot,
	recordingID string,
) (map[string]workerRecordingHealth, error) {
	if snapshot.RecordingID != "" && snapshot.RecordingID != recordingID {
		return nil, fmt.Errorf("%w: recording identity %q does not match %q", workersessions.ErrObservationRecordingCorrupt, snapshot.RecordingID, recordingID)
	}
	if len(snapshot.Sessions) == 0 {
		// The capture may be visible before its first Worker Session is persisted.
		return map[string]workerRecordingHealth{}, nil
	}
	health := make(map[string]workerRecordingHealth, len(snapshot.Sessions))
	for _, session := range snapshot.Sessions {
		workerSessionID := strings.TrimSpace(session.WorkerSessionID)
		if workerSessionID == "" {
			return nil, fmt.Errorf("%w: recording contains an empty Worker Session identity", workersessions.ErrObservationRecordingCorrupt)
		}
		if _, exists := health[workerSessionID]; exists {
			return nil, fmt.Errorf("%w: recording contains duplicate Worker Session %q", workersessions.ErrObservationRecordingCorrupt, workerSessionID)
		}
		if !validWorkerRecordingHealth(session.Status) {
			return nil, fmt.Errorf("%w: Worker Session %q has invalid health %q", workersessions.ErrObservationRecordingCorrupt, workerSessionID, session.Status)
		}
		startedAt, err := workerRecordingSessionStartedAt(session)
		if err != nil {
			return nil, fmt.Errorf("%w: Worker Session %q has an invalid opening record: %v", workersessions.ErrObservationRecordingCorrupt, workerSessionID, err)
		}
		health[workerSessionID] = workerRecordingHealth{
			status:    session.Status,
			reason:    recordingHealthReason(session.Status, session.Failure, session.InterruptionReason),
			startedAt: startedAt,
		}
	}
	return health, nil
}

func isCorruptWorkerRecordingError(err error) bool {
	return errors.Is(err, recordings.ErrWorkerRecordingReplay) ||
		errors.Is(err, recordings.ErrWorkerRecordingCompatibility) ||
		errors.Is(err, recordings.ErrWorkerRecordingOrder) ||
		errors.Is(err, recordings.ErrWorkerRecordingDuplicate) ||
		errors.Is(err, recordings.ErrWorkerRecordingTerminal) ||
		errors.Is(err, recordings.ErrWorkerRecordingOpening) ||
		errors.Is(err, recordings.ErrWorkerRecordingDelivery)
}

func (s *recordedWorkerSessionObservation) withRecordingHealth(
	ctx context.Context,
	observation workersessions.Observation,
) (workersessions.Observation, error) {
	if s != nil && s.factorySessionID != "" {
		observation.FactorySessionID = s.factorySessionID
	}
	health, err := s.recordingHealth(ctx)
	if err != nil {
		return workersessions.Observation{}, err
	}
	if current, ok := health[observation.WorkerSessionID]; ok {
		observation.RecordingHealth = current.status
		observation.RecordingHealthReason = current.reason
		if current.startedAt != nil {
			startedAt := *current.startedAt
			observation.StartedAt = &startedAt
		}
	}
	return observation, nil
}

func (s *recordedWorkerSessionObservation) decorateRecordingHealth(observations []workersessions.Observation, health map[string]workerRecordingHealth) {
	for index := range observations {
		if s != nil && s.factorySessionID != "" {
			observations[index].FactorySessionID = s.factorySessionID
		}
		if current, ok := health[observations[index].WorkerSessionID]; ok {
			observations[index].RecordingHealth = current.status
			observations[index].RecordingHealthReason = current.reason
			if current.startedAt != nil {
				startedAt := *current.startedAt
				observations[index].StartedAt = &startedAt
			}
		}
	}
}

func (s *recordedWorkerSessionObservation) validateRecordingHealth(ctx context.Context) error {
	_, err := s.recordingHealth(ctx)
	return err
}

func (s *recordedWorkerSessionObservation) GetObservation(
	ctx context.Context,
	req workersessions.GetObservationRequest,
) (workersessions.Observation, error) {
	if err := req.Validate(); err != nil {
		return workersessions.Observation{}, err
	}
	if err := observationContextError(ctx); err != nil {
		return workersessions.Observation{}, err
	}
	scope, err := s.observationReadScope(req.FactorySessionID)
	if err != nil {
		return workersessions.Observation{}, err
	}
	req.FactorySessionID = scope
	if s != nil && s.Service != nil {
		observation, err := s.Service.GetObservation(ctx, req)
		if err == nil {
			observation, err = s.withLiveRecordingHealth(ctx, observation)
			if err != nil {
				return workersessions.Observation{}, err
			}
			return s.confirmedObservation(observation), nil
		}
		if !errors.Is(err, workersessions.ErrObservationSessionNotFound) {
			return workersessions.Observation{}, err
		}
	}
	if s != nil && s.ledger != nil && s.projector != nil {
		fact, found, err := s.recordedObservationForProvider(ctx, req.ProviderSession)
		if err != nil {
			return workersessions.Observation{}, err
		}
		if found {
			observation, enrichErr := s.enrichRecordedObservation(ctx, recordedObservationFromFact(fact, s.clock), req.ProviderSession)
			if enrichErr != nil {
				return workersessions.Observation{}, enrichErr
			}
			observation, healthErr := s.withRecordingHealth(ctx, observation)
			if healthErr != nil {
				return workersessions.Observation{}, healthErr
			}
			return s.confirmedObservation(observation), nil
		}
		if s.Service == nil {
			return workersessions.Observation{}, workersessions.ErrObservationSessionNotFound
		}
	}
	if s == nil || s.Service == nil {
		return workersessions.Observation{}, workersessions.ErrObservationProjectionUnavailable
	}
	return workersessions.Observation{}, workersessions.ErrObservationSessionNotFound
}

// GetObservationByWorkerSessionID resolves the Worker Session against the
// live registry before falling back to this Factory Session's durable history.
func (s *recordedWorkerSessionObservation) GetObservationByWorkerSessionID(
	ctx context.Context,
	req workersessions.GetObservationByWorkerSessionIDRequest,
) (workersessions.Observation, error) {
	if err := req.Validate(); err != nil {
		return workersessions.Observation{}, err
	}
	scope, err := s.observationReadScopeForWorker(ctx, req.WorkerSessionID, req.FactorySessionID)
	if err != nil {
		return workersessions.Observation{}, err
	}
	req.FactorySessionID = scope
	req.WorkerSessionID = strings.TrimSpace(req.WorkerSessionID)
	if err := observationContextError(ctx); err != nil {
		return workersessions.Observation{}, err
	}
	if s != nil && s.Service != nil {
		observation, err := s.readLiveWorkerSessionByID(ctx, req)
		if err == nil || !errors.Is(err, workersessions.ErrObservationSessionNotFound) {
			return observation, err
		}
	}
	if s != nil && s.ledger != nil && s.projector != nil {
		observation, found, err := s.readRecordedWorkerSessionByID(ctx, req.WorkerSessionID)
		if err != nil {
			return workersessions.Observation{}, err
		}
		if found {
			return observation, nil
		}
		if s.Service == nil {
			return workersessions.Observation{}, workersessions.ErrObservationSessionNotFound
		}
	}
	if s == nil || s.Service == nil {
		return workersessions.Observation{}, workersessions.ErrObservationProjectionUnavailable
	}
	return workersessions.Observation{}, workersessions.ErrObservationSessionNotFound
}

func (s *recordedWorkerSessionObservation) readRecordedWorkerSessionByID(
	ctx context.Context,
	workerSessionID string,
) (workersessions.Observation, bool, error) {
	fact, found, err := s.recordedObservationForWorkerSessionID(ctx, workerSessionID)
	if err != nil || !found {
		return workersessions.Observation{}, found, err
	}
	observation := recordedObservationFromFact(fact, s.clock)
	observation, err = s.withCapturedWorkerIdentity(ctx, observation)
	if err != nil {
		return workersessions.Observation{}, false, err
	}
	return s.confirmedObservation(observation), true, nil
}

func (s *recordedWorkerSessionObservation) readLiveWorkerSessionByID(
	ctx context.Context,
	req workersessions.GetObservationByWorkerSessionIDRequest,
) (workersessions.Observation, error) {
	observation, err := s.Service.GetObservationByWorkerSessionID(ctx, req)
	if err != nil {
		return workersessions.Observation{}, err
	}
	observation, err = s.withLiveRecordingHealth(ctx, observation)
	if err != nil {
		return workersessions.Observation{}, err
	}
	return s.confirmedObservation(observation), nil
}

func (s *recordedWorkerSessionObservation) withLiveRecordingHealth(
	ctx context.Context,
	observation workersessions.Observation,
) (workersessions.Observation, error) {
	observations := []workersessions.Observation{observation}
	if err := s.applyLiveRecordingHealth(ctx, observations); err != nil {
		return workersessions.Observation{}, err
	}
	return observations[0], nil
}

func (s *recordedWorkerSessionObservation) applyLiveRecordingHealth(
	ctx context.Context,
	observations []workersessions.Observation,
) error {
	if len(observations) == 0 {
		return nil
	}
	health, err := s.recordingHealth(ctx)
	if err != nil {
		return err
	}
	s.decorateLiveRecordingHealth(observations, health)
	return nil
}

func (s *recordedWorkerSessionObservation) decorateLiveRecordingHealth(observations []workersessions.Observation, health map[string]workerRecordingHealth) {
	for index := range observations {
		observation := &observations[index]
		// Only restored lineage can be rebound; shared registries can contain
		// another Factory Session's attempts whose attribution must survive.
		restored := s.liveObservationBelongsToRestoredPrefix(*observation)
		if restored {
			observation.FactorySessionID = s.factorySessionID
		}
		if current, ok := health[observation.WorkerSessionID]; ok {
			observation.RecordingHealth = current.status
			observation.RecordingHealthReason = current.reason
			if restored && current.startedAt != nil {
				startedAt := *current.startedAt
				observation.StartedAt = &startedAt
			}
		}
		*observation = liveRecordingHealth(*observation)
	}
}

func liveRecordingHealth(observation workersessions.Observation) workersessions.Observation {
	// Loading a still-open file through replay can classify its missing terminal
	// record as process interruption. Registry presence proves the attempt is
	// still supervised; incomplete capture is truthful, inferred interruption is
	// not. Preserve explicit live process-loss failures and real capture faults.
	if observation.RecordingHealth == recordings.WorkerRecordingStatusIncomplete &&
		observation.RecordingHealthReason == recordings.WorkerRecordingInterruptionProcessStopped &&
		(observation.Failure == nil || observation.Failure.Kind != workersessions.FailureCauseProcessGone) {
		observation.RecordingHealthReason = ""
	}
	return observation
}

func (s *recordedWorkerSessionObservation) ReadTranscript(
	ctx context.Context,
	req workersessions.ReadTranscriptRequest,
) (workersessions.ReadTranscriptResult, error) {
	if err := req.Validate(); err != nil {
		return workersessions.ReadTranscriptResult{}, err
	}
	scope, err := s.observationReadScopeForWorker(ctx, req.WorkerSessionID, req.FactorySessionID)
	if err != nil {
		return workersessions.ReadTranscriptResult{}, err
	}
	req.FactorySessionID = scope
	req.WorkerSessionID = strings.TrimSpace(req.WorkerSessionID)
	if err := observationContextError(ctx); err != nil {
		return workersessions.ReadTranscriptResult{}, err
	}
	if s != nil && s.ledger != nil && s.projector != nil {
		result, handled, err := s.readRecordedTranscriptForRequest(ctx, req)
		if handled || err != nil {
			return result, err
		}
	}
	if s == nil || s.Service == nil {
		return workersessions.ReadTranscriptResult{}, workersessions.ErrObservationProjectionUnavailable
	}
	result, err := s.Service.ReadTranscript(ctx, req)
	if err != nil {
		return workersessions.ReadTranscriptResult{}, err
	}
	if err := s.validateRecordingHealth(ctx); err != nil {
		return workersessions.ReadTranscriptResult{}, err
	}
	return result, nil
}

func (s *recordedWorkerSessionObservation) readRecordedTranscriptForRequest(
	ctx context.Context,
	req workersessions.ReadTranscriptRequest,
) (workersessions.ReadTranscriptResult, bool, error) {
	var fact recordedDispatchObservation
	var found bool
	var err error
	if req.WorkerSessionID != "" {
		fact, found, err = s.recordedObservationForWorkerSessionID(ctx, req.WorkerSessionID)
	} else {
		fact, found, err = s.recordedObservationForProvider(ctx, req.ProviderSession)
	}
	if err != nil {
		return workersessions.ReadTranscriptResult{}, true, err
	}
	if !found {
		if s.Service == nil {
			return workersessions.ReadTranscriptResult{}, true, workersessions.ErrObservationSessionNotFound
		}
		return workersessions.ReadTranscriptResult{}, false, nil
	}
	if err := s.validateRecordingHealth(ctx); err != nil {
		return workersessions.ReadTranscriptResult{}, true, err
	}
	if !fact.state.Terminal() {
		return workersessions.ReadTranscriptResult{}, true, workersessions.ErrObservationTranscriptActive
	}
	if fact.provider == nil {
		return workersessions.ReadTranscriptResult{}, true, workersessions.ErrObservationTranscriptUnavailable
	}
	readRequest := req
	if readRequest.WorkerSessionID != "" {
		readRequest = workersessions.ReadTranscriptRequest{ProviderSession: providerSessionRef(*fact.provider)}
	}
	result, err := s.readRecordedTranscript(ctx, readRequest, fact)
	return result, true, err
}

func (s *recordedWorkerSessionObservation) enrichRecordedObservation(
	ctx context.Context,
	observation workersessions.Observation,
	ref providers.SessionRef,
) (workersessions.Observation, error) {
	if s.Service != nil {
		live, err := s.Service.GetObservation(ctx, workersessions.GetObservationRequest{ProviderSession: ref})
		if err == nil {
			merged := mergeRecordedObservations([]workersessions.Observation{observation}, []workersessions.Observation{live})
			if len(merged) == 1 {
				return merged[0], nil
			}
		}
		if errors.Is(err, workersessions.ErrObservationCanceled) {
			return workersessions.Observation{}, err
		}
	}
	if s.providerSessions == nil || !observation.ProviderSessionAvailable {
		return observation, nil
	}
	projected, err := s.providerSessions.Project(providersessions.ProjectRequest{
		Session: ref.Clone(),
		Context: ctx,
	})
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, providersessions.ErrOperationCanceled) {
			return workersessions.Observation{}, workersessions.ErrObservationCanceled
		}
		return observation, nil
	}
	applyRecordedProviderDetail(&observation, projected.Detail)
	return observation, nil
}

func (s *recordedWorkerSessionObservation) readRecordedTranscript(
	ctx context.Context,
	req workersessions.ReadTranscriptRequest,
	fact recordedDispatchObservation,
) (workersessions.ReadTranscriptResult, error) {
	if fact.provider == nil {
		return workersessions.ReadTranscriptResult{}, workersessions.ErrObservationTranscriptUnavailable
	}
	if s.Service != nil {
		live, err := s.Service.ReadTranscript(ctx, req)
		if err == nil {
			return historicalTranscriptResult(fact, live.Entries, req.ProviderSession)
		}
		if errors.Is(err, workersessions.ErrObservationCanceled) {
			return workersessions.ReadTranscriptResult{}, err
		}
	}
	if s.providerSessions == nil {
		return workersessions.ReadTranscriptResult{}, workersessions.ErrObservationTranscriptProjectionUnavailable
	}
	projected, err := s.providerSessions.Project(providersessions.ProjectRequest{
		Session: req.ProviderSession.Clone(),
		Context: ctx,
	})
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, providersessions.ErrOperationCanceled) {
			return workersessions.ReadTranscriptResult{}, workersessions.ErrObservationCanceled
		}
		if recordedTranscriptSourceUnavailable(err) {
			return workersessions.ReadTranscriptResult{}, workersessions.ErrObservationTranscriptUnavailable
		}
		return workersessions.ReadTranscriptResult{}, fmt.Errorf("%w: %v", workersessions.ErrObservationTranscriptProjectionUnavailable, err)
	}
	return historicalTranscriptResult(fact, recordedTranscriptEntries(projected.Detail.Transcript), req.ProviderSession)
}

func historicalTranscriptResult(
	fact recordedDispatchObservation,
	entries []workersessions.TranscriptEntry,
	ref providers.SessionRef,
) (workersessions.ReadTranscriptResult, error) {
	result := workersessions.ReadTranscriptResult{
		WorkerSessionID: fact.workerSessionID,
		ProviderSession: ref.Clone(),
		WorkIDs:         append([]string(nil), fact.workIDs...),
		TurnID:          fact.turnID,
		AttemptID:       fact.dispatchID,
		State:           fact.state,
		Entries:         entries,
	}
	if err := result.Validate(); err != nil {
		return workersessions.ReadTranscriptResult{}, fmt.Errorf("validate historical Worker Session transcript: %w", err)
	}
	return result, nil
}

// observationReadScopeForWorker preserves caller-supplied direct correlation.
// Runtime aliases translate Factory Workers only; a direct Worker remains at
// its process-owned address, with explicit foreign selectors rejected first.
func (s *recordedWorkerSessionObservation) observationReadScopeForWorker(ctx context.Context, workerSessionID, requested string) (string, error) {
	scope, err := s.observationReadScope(requested)
	if err != nil || s == nil || s.Service == nil || strings.TrimSpace(workerSessionID) == "" {
		return scope, err
	}
	observation, lookupErr := s.Service.GetObservationByWorkerSessionID(ctx, workersessions.GetObservationByWorkerSessionIDRequest{WorkerSessionID: workerSessionID})
	if lookupErr != nil {
		if errors.Is(lookupErr, context.Canceled) || errors.Is(lookupErr, context.DeadlineExceeded) {
			return "", lookupErr
		}
		// Live identity is optional for retained history and unavailable storage.
		return scope, nil
	}
	if !observation.Direct {
		return scope, nil
	}
	// Retained history belongs to this adapter's owner. A shared direct ID
	// must not redirect that owner's read to another Factory Session before
	// the fleet can resolve candidate cardinality.
	if s.ledger != nil && s.projector != nil {
		_, found, historyErr := s.recordedObservationForWorkerSessionID(ctx, strings.TrimSpace(workerSessionID))
		if historyErr != nil {
			return "", historyErr
		}
		if found {
			return scope, nil
		}
	}
	actual := strings.TrimSpace(observation.FactorySessionID)
	requested = strings.TrimSpace(requested)
	if !s.directObservationScopeMatches(observation, requested, scope) {
		return "", workersessions.ErrObservationSessionNotFound
	}
	return actual, nil
}

func (s *recordedWorkerSessionObservation) directObservationScopeMatches(observation workersessions.Observation, requested, scope string) bool {
	actual := strings.TrimSpace(observation.FactorySessionID)
	if actual != "" {
		return requested == "" || actual == requested || actual == scope || actual == strings.TrimSpace(s.factorySessionID)
	}
	return requested == "" || requested == "~default" || scope == "~default" ||
		(s.runtimeID != "" && observation.RuntimeID == s.runtimeID)
}
