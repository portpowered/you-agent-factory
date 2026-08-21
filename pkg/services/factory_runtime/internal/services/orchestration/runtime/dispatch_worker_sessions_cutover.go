package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

// startThroughWorkerSessions is the W4 Runtime dispatch cutover seam. For
// every resolved dispatch it reserves one stable, control-addressable Worker
// Session identity, commits
// that association to canonical Factory Events before Start can publish Worker
// Session lifecycle records, then hands the resolved request to
// worker_sessions.Service.Start (which drives the injected Workers execution
// service underneath).
// The Worker Sessions terminal outcome is translated back into the exact
// workers.WorkstationDispatchResult shape the pre-cutover accept callback
// expects, so existing Work materialization and Factory result behavior is
// unchanged.
func startThroughWorkerSessions(
	ctx context.Context,
	cfg *runtimeConfig,
	eventHistory recordings.RuntimeLedger,
	request workers.WorkstationDispatchRequest,
	accept workers.WorkstationDispatchAcceptFunc,
) error {
	if strings.TrimSpace(request.Execution.RecordingID) == "" && cfg != nil {
		request.Execution.RecordingID = strings.TrimSpace(cfg.recordingID)
	}
	dispatchID := request.Execution.Dispatch.DispatchID
	sessionID := dispatchID
	if resolver, ok := cfg.completionDeliveryPlanner.(factory.ReplayWorkerSessionIDResolver); ok {
		recordedSessionID, found := resolver.WorkerSessionIDForDispatch(request.Execution.Dispatch)
		if found {
			sessionID = recordedSessionID
		}
	}
	if _, err := cfg.workerSessions.Reserve(
		context.WithoutCancel(ctx),
		workersessions.ReserveRequest{ID: sessionID},
	); err != nil {
		return err
	}
	eventHistory.RecordDispatchWorkerSessionAssociation(
		request.Execution.Dispatch.Execution.DispatchCreatedTick,
		dispatchID,
		sessionID,
		request.Execution.Dispatch.Execution.RequestID,
		cfg.clock.Now(),
	)
	execute := func() {
		// Retry is left at its zero value on purpose: Petri dispatch has always
		// been one attempt, with retryability classified and handed outward for
		// the graph to act on. Converging JavaScript children onto this call
		// must not quietly give every Petri Worker attempt-level retry it never
		// had.
		startResult, startErr := cfg.workerSessions.InvokeSession(
			context.WithoutCancel(ctx),
			workersessions.InvokeSessionRequest{ID: sessionID, Execution: request},
		)
		result, dispatchErr := workerSessionDispatchOutcome(request, startResult, startErr)
		accept(context.Background(), request, result, dispatchErr)
	}
	async := !cfg.inlineDispatch && cfg.completionDeliveryPlanner == nil
	if async {
		go execute()
		return nil
	}
	execute()
	return nil
}

func runtimeAttemptPreparation(
	cfg *runtimeConfig,
	request workers.WorkstationDispatchRequest,
	executeRequest workers.ExecuteRequest,
	allowRetry bool,
) attemptPreparation {
	if cfg == nil || cfg.workerSessions == nil {
		return nil
	}
	recorder, ok := cfg.workerSessions.(interface {
		BeginRuntimeAttempt(context.Context, workersessions.RuntimeAttemptRequest) (workersessions.RuntimeAttempt, error)
	})
	if !ok || recorder == nil {
		return nil
	}
	return func(ctx context.Context, _ *workers.ExecuteRequest) (attemptTerminalFunc, error) {
		sessionID := runtimeWorkerSessionID(cfg, request, executeRequest, allowRetry)
		admissionRequest := request
		if strings.TrimSpace(request.WorkstationName) != workers.ProviderInvocationRoute {
			admissionRequest = runtimeAttemptAdmissionRequest(request, executeRequest)
		}
		attempt, err := recorder.BeginRuntimeAttempt(
			context.WithoutCancel(ctx),
			workersessions.RuntimeAttemptRequest{
				ID:        sessionID,
				AttemptID: executeRequest.Correlation.AttemptID,
				Execution: admissionRequest,
			},
		)
		if err != nil {
			return nil, err
		}
		return func(callbackCtx context.Context, _ workers.ExecuteRequest, result workers.ExecuteResult, executeErr error) {
			result = normalizeDetachedExecutionResult(cfg, executeRequest, result)
			dispatchResult, dispatchErr := workstationDispatchResultFromExecute(
				workstationDispatchRequestForResult(request, executeRequest),
				result,
				executeErr,
			)
			_ = attempt.Complete(callbackCtx, dispatchResult, dispatchErr)
		}, nil
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

// workerSessionDispatchOutcome translates one Worker Sessions Start result
// into the exact workers.WorkstationDispatchResult/error shape the Runtime
// accept callback expects. When Start handed the attempt off to Workers, the
// raw StartResult.Dispatch/DispatchErr already carry that exact shape and are
// returned unchanged. This includes a canceled terminal result when a control
// won after identity reservation but before Workers admission. When Start
// rejected the request before any Workers call (invalid request, conflicting
// start, or a before-handoff Events publication failure), a synthesized FAILED
// result is returned instead of fabricating a Workers payload that never
// existed.
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

// handedOffToWorkers reports whether Start actually reached the Workers
// DispatchWorkstation call. The only FAILED terminal Start commits without a
// Workers handoff is FailureCauseEventPublicationFailure. A canceled terminal
// Session with no cause also carries a synthesized canceled dispatch when
// control won before admission; all other terminal causes (start failure,
// adapter failure, executor panic, or Workers execution failure) are produced
// from within the handoff itself.
func handedOffToWorkers(startResult workersessions.InvokeSessionResult) bool {
	result := startResult.Session.Result
	if result == nil || result.Cause == nil {
		return true
	}
	return result.Cause.Kind != workersessions.FailureCauseEventPublicationFailure
}

// WorkerSessionsObservation returns the runtime-bound detached Worker Session
// observation projection for the current Factory Session.
func (f *factoryImpl) WorkerSessionsObservation() workersessions.ObservationService {
	if f == nil || f.cfg == nil {
		return nil
	}
	var workerRecordingReader recordings.WorkerRecordingReader
	if reader, ok := f.cfg.workerSessions.(recordings.WorkerRecordingReader); ok {
		workerRecordingReader = reader
	}
	return newRecordedWorkerSessionObservationWithRecording(
		f.cfg.workerSessions,
		f.eventHistory,
		f.cfg.worldStateProjector,
		f.clock,
		f.cfg.providerSessions,
		f.cfg.replayEvents,
		f.cfg.recordingID,
		workerRecordingReader,
		sessionIDFromFactoryConfig(f.cfg),
	)
}

// recordedWorkerSessionObservation is the runtime-facing observation adapter.
// Factory Runtime owns the per-session canonical ledger and Recordings owns
// its world-state projector; Worker Sessions remains the owner of the public
// detached observation vocabulary and Provider Sessions enrichment.
type recordedWorkerSessionObservation struct {
	workersessions.Service
	ledger           recordings.RuntimeLedger
	projector        factory.WorldStateProjector
	clock            factory.Clock
	providerSessions providersessions.Service
	replayEvents     []interfaces.FactoryEvent
	recordingID      string
	recordingReader  recordings.WorkerRecordingReader
	factorySessionID string
}

var _ workersessions.Service = (*recordedWorkerSessionObservation)(nil)

func (s *recordedWorkerSessionObservation) projectRecorded(
	ctx context.Context,
	events []interfaces.FactoryEvent,
	workID string,
) ([]workersessions.Observation, bool, error) {
	if err := observationContextError(ctx); err != nil {
		return nil, false, err
	}
	ordered := cloneAndSortFactoryEvents(events)
	selectedTick := latestFactoryEventTick(ordered)
	world, err := s.projector(ordered, selectedTick)
	if err != nil {
		return nil, false, workersessions.ErrObservationProjectionUnavailable
	}

	knownWork := recordedWorkExists(world, ordered, workID)
	associations, requests := recordedDispatchFacts(ordered)
	if len(associations) == 0 {
		return make([]workersessions.Observation, 0), knownWork, nil
	}

	completed := recordedDispatchStateMaps(world)
	result := make([]workersessions.Observation, 0, len(associations))
	for dispatchID, association := range associations {
		fact := recordedDispatchFact(dispatchID, association, requests, completed, world.ProviderSessions, world.ActiveDispatches, ordered)
		if !containsRecordedWorkID(fact.workIDs, workID) {
			continue
		}
		observation := recordedObservationFromFact(fact, s.clock)
		if fact.provider != nil {
			observation, err = s.enrichRecordedObservation(ctx, observation, providerSessionRef(*fact.provider))
			if err != nil {
				return nil, false, err
			}
		}
		result = append(result, observation)
	}
	return result, knownWork, nil
}

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

func recordedDispatchFact(
	dispatchID string,
	association recordedDispatchAssociation,
	requests map[string]recordedDispatchRequest,
	completed map[string]interfaces.FactoryWorldDispatchCompletion,
	providerSessions []interfaces.FactoryWorldProviderSessionRecord,
	active map[string]interfaces.FactoryWorldDispatch,
	events []interfaces.FactoryEvent,
) recordedDispatchObservation {
	fact := recordedDispatchObservation{
		workerSessionID: association.workerSessionID,
		dispatchID:      dispatchID,
		turnID:          association.turnID,
		startedAt:       association.eventTime,
		state:           workersessions.StateStarting,
	}
	if request, ok := requests[dispatchID]; ok {
		fact.workIDs = append([]string(nil), request.workIDs...)
		fact.startedAt = request.startedAt
	}
	if dispatch, ok := active[dispatchID]; ok {
		fact.state = workersessions.StateRunning
		fact.startedAt = firstRecordedTime(dispatch.StartedAt, fact.startedAt)
		fact.workIDs = firstRecordedWorkIDs(dispatch.WorkItemIDs, fact.workIDs)
	}
	if dispatch, ok := completed[dispatchID]; ok {
		fact.state = recordedObservationState(dispatch.Result.Outcome)
		fact.startedAt = firstRecordedTime(dispatch.StartedAt, fact.startedAt)
		fact.endedAt = recordedDispatchEnd(dispatch, events, dispatchID)
		fact.workIDs = firstRecordedWorkIDs(dispatch.WorkItemIDs, fact.workIDs)
		fact.failure = recordedFailureWithDiagnostics(
			workers.WorkOutcome(dispatch.Result.Outcome),
			dispatch.Result.FailureDetail,
			dispatch.Result.FailureMetadata,
			fact.state,
			dispatch.Diagnostics,
		)
		fact.provider = cloneProviderMetadata(dispatch.ProviderSession)
	}
	for _, provider := range providerSessions {
		if provider.DispatchID != dispatchID {
			continue
		}
		fact.provider = cloneProviderMetadata(&provider.ProviderSession)
		fact.workIDs = firstRecordedWorkIDs(provider.WorkItemIDs, fact.workIDs)
		fact.failure = firstRecordedFailure(fact.failure, recordedFailureWithDiagnostics(
			workers.OutcomeFailed,
			provider.FailureDetail,
			nil,
			fact.state,
			provider.Diagnostics,
		))
		break
	}
	return fact
}

func recordedDispatchEnd(
	dispatch interfaces.FactoryWorldDispatchCompletion,
	events []interfaces.FactoryEvent,
	dispatchID string,
) *time.Time {
	ended := dispatch.CompletedAt
	if ended.IsZero() {
		ended = eventTimeForDispatch(events, dispatchID)
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
			var payload interfaces.DispatchWorkerSessionAssociationEventPayload
			if json.Unmarshal(event.Payload, &payload) != nil || payload.WorkerSessionID == "" {
				continue
			}
			associations[dispatchID] = recordedDispatchAssociation{
				workerSessionID: payload.WorkerSessionID,
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
	return newRecordedWorkerSessionObservationWithRecording(live, ledger, projector, clock, providerSessions, nil, "", nil)
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
	if s == nil || s.ledger == nil || s.projector == nil {
		return s.listLive(ctx, req)
	}

	events := s.canonicalEvents()
	recorded, knownWork, err := s.projectRecorded(ctx, events, req.WorkID)
	if err != nil {
		return workersessions.ListObservationsResult{}, err
	}

	live, liveErr := s.listLive(ctx, req)
	if liveErr == nil {
		recorded = mergeRecordedObservations(recorded, live.Observations)
	}
	if !acceptableLiveObservationError(liveErr) {
		return workersessions.ListObservationsResult{}, liveErr
	}
	if err := s.applyRecordingHealth(ctx, recorded); err != nil {
		return workersessions.ListObservationsResult{}, err
	}
	return recordedObservationListResult(recorded, knownWork, live, liveErr)
}

func acceptableLiveObservationError(err error) bool {
	return err == nil || isObservationNotFound(err) || isObservationProjectionUnavailable(err)
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
	return s.Service.ListObservations(ctx, req)
}

// Start carries the runtime-owned Worker recording identity into direct
// Worker Session admission when the caller did not supply one explicitly.
// Factory dispatch already fills this field at its orchestration boundary;
// this adapter keeps the public HTTP/CLI start path on the same durable
// recording contract.
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
	status recordings.WorkerRecordingStatus
	reason string
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
	if snapshot.RecordingID != recordingID {
		return nil, fmt.Errorf("%w: recording identity %q does not match %q", workersessions.ErrObservationRecordingCorrupt, snapshot.RecordingID, recordingID)
	}
	if len(snapshot.Sessions) == 0 {
		return nil, fmt.Errorf("%w: recording contains no Worker Session history", workersessions.ErrObservationRecordingCorrupt)
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
		health[workerSessionID] = workerRecordingHealth{
			status: session.Status,
			reason: recordingHealthReason(session.Status, session.Failure, session.InterruptionReason),
		}
	}
	return health, nil
}

func validWorkerRecordingHealth(status recordings.WorkerRecordingStatus) bool {
	switch status {
	case recordings.WorkerRecordingStatusComplete,
		recordings.WorkerRecordingStatusDegraded,
		recordings.WorkerRecordingStatusIncomplete:
		return true
	}
	return false
}

func recordingHealthReason(status recordings.WorkerRecordingStatus, failure, interruption string) string {
	if status == recordings.WorkerRecordingStatusDegraded {
		return strings.TrimSpace(failure)
	}
	if status == recordings.WorkerRecordingStatusIncomplete {
		return strings.TrimSpace(interruption)
	}
	return ""
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
	}
	return observation, nil
}

func (s *recordedWorkerSessionObservation) applyRecordingHealth(
	ctx context.Context,
	observations []workersessions.Observation,
) error {
	health, err := s.recordingHealth(ctx)
	if err != nil {
		return err
	}
	for index := range observations {
		if s != nil && s.factorySessionID != "" {
			observations[index].FactorySessionID = s.factorySessionID
		}
		if current, ok := health[observations[index].WorkerSessionID]; ok {
			observations[index].RecordingHealth = current.status
			observations[index].RecordingHealthReason = current.reason
		}
	}
	return nil
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
			return s.withRecordingHealth(ctx, observation)
		}
		if s.Service == nil {
			return workersessions.Observation{}, workersessions.ErrObservationSessionNotFound
		}
	}
	if s == nil || s.Service == nil {
		return workersessions.Observation{}, workersessions.ErrObservationProjectionUnavailable
	}
	observation, err := s.Service.GetObservation(ctx, req)
	if err != nil {
		return workersessions.Observation{}, err
	}
	return s.withRecordingHealth(ctx, observation)
}

// GetObservationByWorkerSessionID resolves the canonical Worker Session
// identity against this Factory Session's durable event history before
// consulting the process-local registry. The wrapper is deliberately
// Factory-Session scoped, so an identical Worker Session ID in another
// session cannot leak into this response.
func (s *recordedWorkerSessionObservation) GetObservationByWorkerSessionID(
	ctx context.Context,
	req workersessions.GetObservationByWorkerSessionIDRequest,
) (workersessions.Observation, error) {
	if err := req.Validate(); err != nil {
		return workersessions.Observation{}, err
	}
	req.WorkerSessionID = strings.TrimSpace(req.WorkerSessionID)
	if err := observationContextError(ctx); err != nil {
		return workersessions.Observation{}, err
	}
	if s != nil && s.ledger != nil && s.projector != nil {
		fact, found, err := s.recordedObservationForWorkerSessionID(ctx, req.WorkerSessionID)
		if err != nil {
			return workersessions.Observation{}, err
		}
		if found {
			observation := recordedObservationFromFact(fact, s.clock)
			if fact.provider == nil {
				return s.withRecordingHealth(ctx, observation)
			}
			observation, enrichErr := s.enrichRecordedObservation(ctx, observation, providerSessionRef(*fact.provider))
			if enrichErr != nil {
				return workersessions.Observation{}, enrichErr
			}
			return s.withRecordingHealth(ctx, observation)
		}
		if s.Service == nil {
			return workersessions.Observation{}, workersessions.ErrObservationSessionNotFound
		}
	}
	if s == nil || s.Service == nil {
		return workersessions.Observation{}, workersessions.ErrObservationProjectionUnavailable
	}
	observation, err := s.Service.GetObservationByWorkerSessionID(ctx, req)
	if err != nil {
		return workersessions.Observation{}, err
	}
	return s.withRecordingHealth(ctx, observation)
}

func (s *recordedWorkerSessionObservation) ReadTranscript(
	ctx context.Context,
	req workersessions.ReadTranscriptRequest,
) (workersessions.ReadTranscriptResult, error) {
	if err := req.Validate(); err != nil {
		return workersessions.ReadTranscriptResult{}, err
	}
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
func newRecordedWorkerSessionObservationWithRecording(
	live workersessions.Service,
	ledger recordings.RuntimeLedger,
	projector factory.WorldStateProjector,
	clock factory.Clock,
	providerSessions providersessions.Service,
	replayEvents []interfaces.FactoryEvent,
	recordingID string,
	recordingReader recordings.WorkerRecordingReader,
	factorySessionIDs ...string,
) workersessions.Service {
	factorySessionID := ""
	if len(factorySessionIDs) > 0 {
		factorySessionID = strings.TrimSpace(factorySessionIDs[0])
	}
	return &recordedWorkerSessionObservation{
		Service: live, ledger: ledger, projector: projector, clock: clock,
		providerSessions: providerSessions, replayEvents: cloneAndSortFactoryEvents(replayEvents), recordingID: strings.TrimSpace(recordingID),
		recordingReader: recordingReader, factorySessionID: factorySessionID,
	}
}
