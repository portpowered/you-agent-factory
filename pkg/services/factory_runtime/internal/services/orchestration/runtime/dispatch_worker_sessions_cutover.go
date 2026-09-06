package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factory_context "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/context"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

// startThroughWorkerSessions reserves identity and preserves the dispatch shape.
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
	recordDispatchWorkerSessionAssociation(
		eventHistory,
		request.Execution.Dispatch.Execution.DispatchCreatedTick,
		dispatchID,
		sessionID,
		request.Execution.Dispatch.Execution.RequestID,
		recordings.DispatchWorkerSessionExecutionFacts{
			Model:           request.Execution.Model,
			ReasoningEffort: request.Execution.ReasoningEffort,
		},
		cfg.clock.Now(),
	)
	execute := func() {
		// Petri dispatch remains one attempt; retryability is classified outward.
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
	return newRecordedWorkerSessionObservationWithRestoredState(
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
}

// recordedWorkerSessionObservation adapts the runtime ledger and projector to
// the detached Worker Session observation vocabulary.
type recordedWorkerSessionObservation struct {
	workersessions.Service
	ledger              recordings.RuntimeLedger
	durability          recordings.CompletedFlushWatermarkReader
	projector           factory.WorldStateProjector
	clock               factory.Clock
	providerSessions    providersessions.Service
	replayEvents        []interfaces.FactoryEvent
	restoredWorldState  *interfaces.FactoryWorldState
	restoredEventPrefix []interfaces.FactoryEvent
	recordingID         string
	recordingReader     recordings.WorkerRecordingReader
	factorySessionID    string
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
	world, err := s.projectRecordedWorldState(ctx, events, ordered, selectedTick)
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
		fact := s.annotateRecordedFact(recordedDispatchFact(dispatchID, association, requests, completed, world.ProviderSessions, world.ActiveDispatches, ordered))
		if workID != "" && !containsRecordedWorkID(fact.workIDs, workID) {
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
	var events []interfaces.FactoryEvent
	if len(s.replayEvents) > 0 {
		events = cloneAndSortFactoryEvents(s.replayEvents)
	} else {
		if s.ledger == nil {
			return nil
		}
		events = cloneAndSortFactoryEvents(s.ledger.CanonicalEvents())
	}
	return filterFactorySessionEvents(events, s.factorySessionID)
}

func filterFactorySessionEvents(events []interfaces.FactoryEvent, factorySessionID string) []interfaces.FactoryEvent {
	factorySessionID = strings.TrimSpace(factorySessionID)
	if factorySessionID == "" || len(events) == 0 {
		return events
	}
	scoped := make([]interfaces.FactoryEvent, 0, len(events))
	for _, event := range events {
		if event.Context.SessionID == nil {
			if factorySessionID == factory_context.DefaultSessionID {
				scoped = append(scoped, event)
			}
			continue
		}
		if strings.TrimSpace(*event.Context.SessionID) != factorySessionID {
			continue
		}
		scoped = append(scoped, event)
	}
	return scoped
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
	if s == nil || s.ledger == nil || (s.projector == nil && s.restoredWorldState == nil) {
		result, err := s.listLive(ctx, req)
		if err == nil {
			s.applyConfirmation(result.Observations, s.sampleCompletedFlushWatermark())
		}
		return result, err
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
	sample := completedFlushWatermarkSample{}
	if len(recorded) > 0 || len(live.Observations) > 0 {
		sample = s.sampleCompletedFlushWatermark()
	}
	s.applyConfirmation(recorded, sample)
	s.applyConfirmation(live.Observations, sample)
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
	result, err := s.Service.ListWorkerSessionObservations(ctx, req)
	if err != nil {
		return result, err
	}
	// The process-local fleet projection can contain the live identity while
	// the canonical Factory ledger already has the authoritative lifecycle,
	// timing, and provider facts. Overlay those facts onto the page returned by
	// the service so pagination and scope filtering remain service-owned while
	// fleet reads retain the same durable semantics as Work-scoped reads.
	recorded, _, projectionErr := s.projectRecorded(ctx, s.canonicalEvents(), "")
	if s.factorySessionID != "" {
		result.Observations = filterObservationPageForFactorySession(result.Observations, s.factorySessionID, recorded)
	}
	if projectionErr == nil {
		result.Observations = overlayRecordedObservationPage(result.Observations, recorded)
	}
	if err := s.applyRecordingHealth(ctx, result.Observations); err != nil {
		return workersessions.ListWorkerSessionObservationsResult{}, err
	}
	s.applyConfirmation(result.Observations, s.sampleCompletedFlushWatermark())
	return result, err
}

func filterObservationPageForFactorySession(
	page []workersessions.Observation,
	factorySessionID string,
	recorded []workersessions.Observation,
) []workersessions.Observation {
	factorySessionID = strings.TrimSpace(factorySessionID)
	if factorySessionID == "" || len(page) == 0 {
		return page
	}
	recordedIDs := make(map[string]struct{}, len(recorded))
	for _, observation := range recorded {
		recordedIDs[observation.WorkerSessionID] = struct{}{}
	}
	filtered := make([]workersessions.Observation, 0, len(page))
	for _, observation := range page {
		if strings.TrimSpace(observation.FactorySessionID) == factorySessionID {
			filtered = append(filtered, observation)
			continue
		}
		if strings.TrimSpace(observation.FactorySessionID) == "" && factorySessionID == factory_context.DefaultSessionID {
			// Direct Worker Session admissions have no Factory Session identity.
			// Keep them in the process default view so the fleet endpoint does
			// not hide provider-neutral/direct execution history.
			filtered = append(filtered, observation)
			continue
		}
		if _, recorded := recordedIDs[observation.WorkerSessionID]; recorded {
			filtered = append(filtered, observation)
		}
	}
	return filtered
}

func overlayRecordedObservationPage(
	page []workersessions.Observation,
	recorded []workersessions.Observation,
) []workersessions.Observation {
	if len(page) == 0 || len(recorded) == 0 {
		return page
	}
	recordedByID := make(map[string]workersessions.Observation, len(recorded))
	for _, observation := range recorded {
		recordedByID[observation.WorkerSessionID] = observation
	}
	overlaid := make([]workersessions.Observation, len(page))
	for index, live := range page {
		recordedObservation, ok := recordedByID[live.WorkerSessionID]
		if !ok {
			overlaid[index] = live.Clone()
			continue
		}
		overlaid[index] = recordedObservation.Clone()
		mergeLiveObservation(&overlaid[index], live)
	}
	return overlaid
}

// hasDurableWorkerHistory reports whether the embedded per-runtime Worker
// Sessions service exposes the Recordings-owned Worker-ID history seam. The
// runtime ledger remains the compatibility fallback for test doubles and
// older callers that do not provide the new capability.
func (s *recordedWorkerSessionObservation) hasDurableWorkerHistory() bool {
	if s == nil {
		return false
	}
	if reader, ok := s.Service.(recordings.WorkerRecordingHistoryReader); ok && reader != nil {
		return true
	}
	if reader, ok := s.recordingReader.(recordings.WorkerRecordingHistoryReader); ok && reader != nil {
		return true
	}
	return false
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
	observation, err := s.Service.GetObservation(ctx, req)
	if err != nil {
		return workersessions.Observation{}, err
	}
	observation, err = s.withRecordingHealth(ctx, observation)
	if err != nil {
		return workersessions.Observation{}, err
	}
	return s.confirmedObservation(observation), nil
}

// GetObservationByWorkerSessionID resolves the Worker Session against this
// Factory Session's durable history before consulting the process-local registry.
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
	if observation, handled, err := s.readWorkerSessionHistory(ctx, req); handled {
		return observation, err
	}
	if s == nil || s.Service == nil {
		return workersessions.Observation{}, workersessions.ErrObservationProjectionUnavailable
	}
	return s.readLiveWorkerSessionByID(ctx, req)
}

func (s *recordedWorkerSessionObservation) readWorkerSessionHistory(
	ctx context.Context,
	req workersessions.GetObservationByWorkerSessionIDRequest,
) (workersessions.Observation, bool, error) {
	if s.hasDurableWorkerHistory() && s.Service != nil {
		observation, err := s.Service.GetObservationByWorkerSessionID(ctx, req)
		if err == nil {
			if recorded, handled, recordedErr := s.recordedWorkerObservationIfAvailable(ctx, observation, req.WorkerSessionID); handled {
				return recorded, true, recordedErr
			}
			return observation, true, nil
		}
		if !errors.Is(err, workersessions.ErrObservationSessionNotFound) {
			return workersessions.Observation{}, true, err
		}
	}
	if observation, handled, err := s.readRecordedWorkerHistory(ctx, req.WorkerSessionID); handled {
		return observation, true, err
	}
	return workersessions.Observation{}, false, nil
}

func (s *recordedWorkerSessionObservation) readRecordedWorkerHistory(
	ctx context.Context,
	workerSessionID string,
) (workersessions.Observation, bool, error) {
	if s == nil || s.ledger == nil || s.projector == nil {
		return workersessions.Observation{}, false, nil
	}
	observation, found, err := s.readRecordedWorkerSessionByID(ctx, workerSessionID)
	if err != nil || found {
		return observation, true, err
	}
	if s.Service == nil {
		return workersessions.Observation{}, true, workersessions.ErrObservationSessionNotFound
	}
	return workersessions.Observation{}, false, nil
}

func (s *recordedWorkerSessionObservation) recordedWorkerObservationIfAvailable(
	ctx context.Context,
	observation workersessions.Observation,
	workerSessionID string,
) (workersessions.Observation, bool, error) {
	if !observation.ProviderSessionAvailable || s.ledger == nil || s.projector == nil {
		return workersessions.Observation{}, false, nil
	}
	recorded, found, err := s.readRecordedWorkerSessionByID(ctx, workerSessionID)
	if err != nil || found {
		return recorded, true, err
	}
	return workersessions.Observation{}, false, nil
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
	if fact.provider != nil {
		observation, err = s.enrichRecordedObservation(ctx, observation, providerSessionRef(*fact.provider))
		if err != nil {
			return workersessions.Observation{}, false, err
		}
	}
	observation, err = s.withRecordingHealth(ctx, observation)
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
	observation, err = s.withRecordingHealth(ctx, observation)
	if err != nil {
		return workersessions.Observation{}, err
	}
	return s.confirmedObservation(observation), nil
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
	if result, handled, err := s.readTranscriptHistory(ctx, req); handled {
		return result, err
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

func (s *recordedWorkerSessionObservation) readTranscriptHistory(
	ctx context.Context,
	req workersessions.ReadTranscriptRequest,
) (workersessions.ReadTranscriptResult, bool, error) {
	if req.WorkerSessionID != "" && s.hasDurableWorkerHistory() && s.Service != nil {
		result, err := s.Service.ReadTranscript(ctx, req)
		if err == nil {
			if recorded, handled, recordedErr := s.recordedTranscriptIfAvailable(ctx, req, result); handled {
				return recorded, true, recordedErr
			}
			return result, true, nil
		}
		if !errors.Is(err, workersessions.ErrObservationSessionNotFound) {
			return workersessions.ReadTranscriptResult{}, true, err
		}
	}
	if result, handled, err := s.readRecordedTranscriptHistory(ctx, req); handled {
		return result, true, err
	}
	return workersessions.ReadTranscriptResult{}, false, nil
}

func (s *recordedWorkerSessionObservation) readRecordedTranscriptHistory(
	ctx context.Context,
	req workersessions.ReadTranscriptRequest,
) (workersessions.ReadTranscriptResult, bool, error) {
	if s == nil || s.ledger == nil || s.projector == nil {
		return workersessions.ReadTranscriptResult{}, false, nil
	}
	result, handled, err := s.readRecordedTranscriptForRequest(ctx, req)
	if handled || err != nil {
		return result, true, err
	}
	return workersessions.ReadTranscriptResult{}, false, nil
}

func (s *recordedWorkerSessionObservation) recordedTranscriptIfAvailable(
	ctx context.Context,
	req workersessions.ReadTranscriptRequest,
	result workersessions.ReadTranscriptResult,
) (workersessions.ReadTranscriptResult, bool, error) {
	if !transcriptProviderSessionAvailable(result) || s.ledger == nil || s.projector == nil {
		return workersessions.ReadTranscriptResult{}, false, nil
	}
	recorded, handled, err := s.readRecordedTranscriptForRequest(ctx, req)
	if err != nil || handled {
		return recorded, true, err
	}
	return workersessions.ReadTranscriptResult{}, false, nil
}

func transcriptProviderSessionAvailable(result workersessions.ReadTranscriptResult) bool {
	return strings.TrimSpace(string(result.ProviderSession.Provider)) != "" ||
		strings.TrimSpace(result.ProviderSession.Kind) != "" ||
		strings.TrimSpace(result.ProviderSession.ID) != ""
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
