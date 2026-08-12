package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
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
// every resolved dispatch it: boots the underlying Workers pool if needed,
// reserves one stable, control-addressable Worker Session identity, commits
// that association to canonical Factory Events before Start can publish Worker
// Session lifecycle records, then hands the resolved request to
// worker_sessions.Service.Start (which drives the existing Workers
// workstation-pool boundary underneath).
// The Worker Sessions terminal outcome is translated back into the exact
// workers.WorkstationDispatchResult shape the pre-cutover accept callback
// expects, so existing Work materialization and Factory result behavior is
// unchanged.
func startThroughWorkerSessions(
	ctx context.Context,
	cfg *runtimeConfig,
	eventHistory recordings.RuntimeLedger,
	workersBoundary workers.WorkstationPoolBoundary,
	request workers.WorkstationDispatchRequest,
	accept workers.WorkstationDispatchAcceptFunc,
) error {
	if strings.TrimSpace(request.Execution.RecordingID) == "" && cfg != nil {
		request.Execution.RecordingID = strings.TrimSpace(cfg.recordingID)
	}
	if err := workersBoundary.Start(ctx); err != nil {
		return err
	}
	dispatchID := request.Execution.Dispatch.DispatchID
	sessionID := dispatchID
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
	return newRecordedWorkerSessionObservation(
		f.cfg.workerSessions,
		f.eventHistory,
		f.cfg.worldStateProjector,
		f.clock,
		f.cfg.providerSessions,
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
}

var _ workersessions.Service = (*recordedWorkerSessionObservation)(nil)

func newRecordedWorkerSessionObservation(
	live workersessions.Service,
	ledger recordings.RuntimeLedger,
	projector factory.WorldStateProjector,
	clock factory.Clock,
	providerSessions providersessions.Service,
) workersessions.Service {
	return &recordedWorkerSessionObservation{
		Service: live, ledger: ledger, projector: projector, clock: clock,
		providerSessions: providerSessions,
	}
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

	events := s.ledger.CanonicalEvents()
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
			return s.enrichRecordedObservation(ctx, recordedObservationFromFact(fact, s.clock), req.ProviderSession)
		}
		if s.Service == nil {
			return workersessions.Observation{}, workersessions.ErrObservationSessionNotFound
		}
	}
	if s == nil || s.Service == nil {
		return workersessions.Observation{}, workersessions.ErrObservationProjectionUnavailable
	}
	return s.Service.GetObservation(ctx, req)
}

func (s *recordedWorkerSessionObservation) ReadTranscript(
	ctx context.Context,
	req workersessions.ReadTranscriptRequest,
) (workersessions.ReadTranscriptResult, error) {
	if err := req.Validate(); err != nil {
		return workersessions.ReadTranscriptResult{}, err
	}
	if err := observationContextError(ctx); err != nil {
		return workersessions.ReadTranscriptResult{}, err
	}
	if s != nil && s.ledger != nil && s.projector != nil {
		fact, found, err := s.recordedObservationForProvider(ctx, req.ProviderSession)
		if err != nil {
			return workersessions.ReadTranscriptResult{}, err
		}
		if found {
			if !fact.state.Terminal() {
				return workersessions.ReadTranscriptResult{}, workersessions.ErrObservationTranscriptActive
			}
			return s.readRecordedTranscript(ctx, req, fact)
		}
		if s.Service == nil {
			return workersessions.ReadTranscriptResult{}, workersessions.ErrObservationSessionNotFound
		}
	}
	if s == nil || s.Service == nil {
		return workersessions.ReadTranscriptResult{}, workersessions.ErrObservationProjectionUnavailable
	}
	return s.Service.ReadTranscript(ctx, req)
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

func (s *recordedWorkerSessionObservation) StreamObservations(
	ctx context.Context,
	req workersessions.StreamObservationsRequest,
) (workersessions.ObservationSubscription, error) {
	if s == nil || (s.ledger == nil && s.projector == nil && s.Service == nil) {
		return workersessions.ObservationSubscription{}, workersessions.ErrObservationProjectionUnavailable
	}
	if err := req.Validate(); err != nil {
		return workersessions.ObservationSubscription{}, err
	}
	if err := observationContextError(ctx); err != nil {
		return workersessions.ObservationSubscription{}, err
	}
	if subscription, handled, err := s.streamRecorded(ctx, req); handled {
		return subscription, err
	}
	if s.Service == nil {
		return workersessions.ObservationSubscription{}, workersessions.ErrObservationProjectionUnavailable
	}
	return s.Service.StreamObservations(ctx, req)
}

func (s *recordedWorkerSessionObservation) streamRecorded(
	ctx context.Context,
	req workersessions.StreamObservationsRequest,
) (workersessions.ObservationSubscription, bool, error) {
	if s.ledger == nil || s.projector == nil {
		return workersessions.ObservationSubscription{}, false, nil
	}
	fact, found, err := s.recordedObservationForProvider(ctx, req.ProviderSession)
	if err != nil {
		return workersessions.ObservationSubscription{}, true, err
	}
	if !found {
		if s.Service == nil {
			return workersessions.ObservationSubscription{}, true, workersessions.ErrObservationSessionNotFound
		}
		return workersessions.ObservationSubscription{}, false, nil
	}
	streamContext := ctx
	if streamContext == nil {
		streamContext = context.Background()
	}
	streamContext, cancel := context.WithCancel(streamContext)
	limit := observationStreamLimit(req.Limit)
	source, subscribeErr := s.ledger.Subscribe(streamContext, nil, interfaces.FactoryEventReconnectScope{
		DispatchID: fact.dispatchID,
		Limit:      limit,
	})
	if subscribeErr != nil {
		cancel()
		if errors.Is(subscribeErr, context.Canceled) {
			return workersessions.ObservationSubscription{}, true, workersessions.ErrObservationCanceled
		}
		return workersessions.ObservationSubscription{}, true, workersessions.ErrObservationSourceUnavailable
	}
	source.History = boundedRecordedObservationHistory(source.History, fact.dispatchID, limit)
	terminalReplay := recordedObservationHistoryHasTerminal(source.History, fact.dispatchID)
	return newRecordedObservationSubscription(source, fact.dispatchID, terminalReplay, cancel, streamContext), true, nil
}

func observationStreamLimit(limit int) int {
	if limit <= 0 {
		return workersessions.DefaultObservationStreamLimit
	}
	return limit
}

func boundedRecordedObservationHistory(
	events []interfaces.FactoryEvent,
	dispatchID string,
	limit int,
) []interfaces.FactoryEvent {
	limit = observationStreamLimit(limit)
	ordered := make([]interfaces.FactoryEvent, 0, len(events))
	for _, event := range cloneAndSortFactoryEvents(events) {
		if stringPointerValue(event.Context.DispatchID) == dispatchID {
			ordered = append(ordered, event)
		}
	}
	if len(ordered) <= limit {
		return ordered
	}
	return ordered[len(ordered)-limit:]
}

func recordedObservationHistoryHasTerminal(events []interfaces.FactoryEvent, dispatchID string) bool {
	for _, event := range events {
		if stringPointerValue(event.Context.DispatchID) == dispatchID && recordedWorkerSessionTerminalEvent(event) {
			return true
		}
	}
	return false
}

func (s *recordedWorkerSessionObservation) recordedObservationForProvider(
	ctx context.Context,
	ref providers.SessionRef,
) (recordedDispatchObservation, bool, error) {
	if err := observationContextError(ctx); err != nil {
		return recordedDispatchObservation{}, false, err
	}
	ordered := cloneAndSortFactoryEvents(s.ledger.CanonicalEvents())
	world, err := s.projector(ordered, latestFactoryEventTick(ordered))
	if err != nil {
		return recordedDispatchObservation{}, false, workersessions.ErrObservationProjectionUnavailable
	}
	associations, requests := recordedDispatchFacts(ordered)
	completed := recordedDispatchStateMaps(world)
	for dispatchID, association := range associations {
		fact := recordedDispatchFact(dispatchID, association, requests, completed, world.ProviderSessions, world.ActiveDispatches, ordered)
		if fact.provider != nil && providerSessionRef(*fact.provider) == ref {
			return fact, true, nil
		}
	}
	return recordedDispatchObservation{}, false, nil
}

type recordedObservationSubscription struct {
	stream         interfaces.FactoryEventStream
	dispatchID     string
	terminalReplay bool
	cancel         context.CancelFunc
	sourceContext  context.Context

	mu          sync.Mutex
	closed      bool
	history     []interfaces.FactoryEvent
	historyRead int
}

func newRecordedObservationSubscription(
	stream interfaces.FactoryEventStream,
	dispatchID string,
	terminalReplay bool,
	cancel context.CancelFunc,
	sourceContext context.Context,
) workersessions.ObservationSubscription {
	subscription := &recordedObservationSubscription{
		stream:         stream,
		dispatchID:     dispatchID,
		terminalReplay: terminalReplay,
		cancel:         cancel,
		sourceContext:  sourceContext,
		history:        cloneAndSortFactoryEvents(stream.History),
	}
	return workersessions.ObservationSubscription{
		NextFunc:  subscription.next,
		CloseFunc: subscription.close,
	}
}

func (s *recordedObservationSubscription) next(ctx context.Context) workersessions.ObservationDelivery {
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		if ctx.Err() != nil {
			s.close()
			return workersessions.ObservationDelivery{Kind: workersessions.ObservationDeliveryCanceled, Err: workersessions.ErrObservationCanceled}
		}
		if event, ok := s.nextHistoryEvent(); ok {
			if delivery, terminal := s.project(event); delivery != nil {
				if terminal {
					s.close()
				}
				return *delivery
			}
			continue
		}

		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			return workersessions.ObservationDelivery{Kind: workersessions.ObservationDeliveryClosed}
		}
		streamContext := s.streamContext()
		events := s.stream.Events
		s.mu.Unlock()
		if events == nil {
			s.close()
			return workersessions.ObservationDelivery{Kind: workersessions.ObservationDeliverySourceFailure, Err: workersessions.ErrObservationSourceClosed}
		}
		select {
		case <-ctx.Done():
			s.close()
			return workersessions.ObservationDelivery{Kind: workersessions.ObservationDeliveryCanceled, Err: workersessions.ErrObservationCanceled}
		case <-streamContext.Done():
			return workersessions.ObservationDelivery{Kind: workersessions.ObservationDeliveryClosed}
		case event, ok := <-events:
			if !ok {
				s.close()
				return workersessions.ObservationDelivery{Kind: workersessions.ObservationDeliverySourceFailure, Err: workersessions.ErrObservationSourceClosed}
			}
			if delivery, terminal := s.project(event); delivery != nil {
				if terminal {
					s.close()
				}
				return *delivery
			}
		}
	}
}

func (s *recordedObservationSubscription) nextHistoryEvent() (interfaces.FactoryEvent, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.historyRead >= len(s.history) {
		return interfaces.FactoryEvent{}, false
	}
	event := s.history[s.historyRead]
	s.historyRead++
	return event, true
}

func (s *recordedObservationSubscription) project(event interfaces.FactoryEvent) (*workersessions.ObservationDelivery, bool) {
	if stringPointerValue(event.Context.DispatchID) != s.dispatchID {
		return nil, false
	}
	terminal := recordedWorkerSessionTerminalEvent(event)
	delivery := workersessions.ObservationDelivery{Kind: workersessions.ObservationDeliveryRecord, Event: recordedObservationEvent(event)}
	if terminal {
		delivery.Kind = workersessions.ObservationDeliveryTerminal
		if s.terminalReplay {
			delivery.Kind = workersessions.ObservationDeliveryTerminalReplay
		}
	}
	return &delivery, terminal
}

func (s *recordedObservationSubscription) streamContext() context.Context {
	if s.sourceContext == nil {
		return context.Background()
	}
	return s.sourceContext
}

func (s *recordedObservationSubscription) close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	cancel := s.cancel
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func recordedWorkerSessionTerminalEvent(event interfaces.FactoryEvent) bool {
	switch event.Type {
	case interfaces.FactoryEventTypeDispatchResponse,
		interfaces.FactoryEventTypeDispatchInterrupted,
		interfaces.FactoryEventTypeDispatchReconciled:
		return true
	default:
		return false
	}
}

func recordedObservationEvent(event interfaces.FactoryEvent) workersessions.ObservationEvent {
	position := event.Context.Sequence
	if position < 0 {
		position = 0
	}
	return workersessions.ObservationEvent{
		Position:       uint64(position),
		SourceType:     "factory_event",
		SourceID:       event.Id,
		SourceSequence: uint64(position),
		SourceEventID:  event.Id,
		SchemaID:       string(event.Type),
		Payload:        append([]byte(nil), event.Payload...),
	}
}

func applyRecordedProviderDetail(observation *workersessions.Observation, detail providersessions.Detail) {
	if observation == nil {
		return
	}
	observation.Transcript = workersessions.TranscriptAvailabilityAvailable
	observation.TokenUsage = recordedTokenUsage(detail.Parse.TokenUsage)
	observation.Parse = recordedParseDiagnostics(detail.Parse)
}

func recordedTokenUsage(source *providersessions.TokenUsage) *workersessions.TokenUsage {
	if source == nil {
		return nil
	}
	return &workersessions.TokenUsage{
		CacheWriteTokens:      cloneRecordedInt(source.CacheWriteTokens),
		CachedInputTokens:     cloneRecordedInt(source.CachedInputTokens),
		InputTokens:           cloneRecordedInt(source.InputTokens),
		OutputTokens:          cloneRecordedInt(source.OutputTokens),
		ReasoningOutputTokens: cloneRecordedInt(source.ReasoningOutputTokens),
		TotalTokens:           cloneRecordedInt(source.TotalTokens),
	}
}

func recordedParseDiagnostics(source providersessions.ParseSummary) workersessions.ParseDiagnostics {
	result := workersessions.ParseDiagnostics{
		EventCount:         source.EventCount,
		MalformedLineCount: source.MalformedLineCount,
		UnknownEventCount:  source.UnknownEventCount,
		Errors:             make([]workersessions.ParseDiagnostic, 0, len(source.ParseErrors)),
	}
	for _, item := range source.ParseErrors {
		result.Errors = append(result.Errors, workersessions.ParseDiagnostic{
			Code:       "provider_session_parse_error",
			LineNumber: item.LineNumber,
			Message:    recordedDiagnosticMessage(item.Message),
		})
	}
	return result
}

func recordedDiagnosticMessage(message string) string {
	message = strings.Join(strings.Fields(message), " ")
	if message == "" || strings.ContainsAny(message, `/\`) {
		return "provider session parse error"
	}
	lower := strings.ToLower(message)
	for _, sensitive := range []string{"password", "authorization", "bearer ", "secret", "prompt"} {
		if strings.Contains(lower, sensitive) {
			return "provider session parse error"
		}
	}
	if len(message) > 256 {
		return message[:256]
	}
	return message
}

func recordedTranscriptEntries(values []providersessions.TranscriptEntry) []workersessions.TranscriptEntry {
	entries := make([]workersessions.TranscriptEntry, len(values))
	for index, value := range values {
		entries[index] = workersessions.TranscriptEntry{
			Arguments:        cloneRecordedString(value.Arguments),
			CallID:           cloneRecordedString(value.CallID),
			Encrypted:        cloneRecordedBool(value.Encrypted),
			EncryptedContent: cloneRecordedString(value.EncryptedContent),
			LineNumber:       cloneRecordedInt(value.LineNumber),
			Name:             cloneRecordedString(value.Name),
			Order:            value.Order,
			Output:           cloneRecordedString(value.Output),
			SourceType:       cloneRecordedString(value.SourceType),
			Status:           cloneRecordedString(value.Status),
			Summary:          cloneRecordedString(value.Summary),
			Text:             cloneRecordedString(value.Text),
			Timestamp:        cloneRecordedTime(value.Timestamp),
			TurnIndex:        cloneRecordedInt(value.TurnIndex),
			Type:             workersessions.TranscriptEntryType(value.Type),
		}
	}
	return entries
}

func recordedTranscriptSourceUnavailable(err error) bool {
	return errors.Is(err, providersessions.ErrSessionNotFound) ||
		errors.Is(err, providersessions.ErrAmbiguousSessionFile) ||
		errors.Is(err, providersessions.ErrSessionSourceNotRegularFile) ||
		errors.Is(err, providersessions.ErrSessionStorageUnavailable) ||
		errors.Is(err, providersessions.ErrSessionOutsideRoot)
}

func cloneRecordedBool(value *bool) *bool {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func cloneRecordedInt(value *int) *int {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func cloneRecordedString(value *string) *string {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func cloneRecordedTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

type recordedDispatchObservation struct {
	workerSessionID string
	dispatchID      string
	turnID          string
	workIDs         []string
	startedAt       time.Time
	endedAt         *time.Time
	state           workersessions.State
	failure         *workersessions.FailureCause
	provider        *workers.ProviderSessionMetadata
}

type recordedDispatchAssociation struct {
	workerSessionID string
	turnID          string
	eventTime       time.Time
}

type recordedDispatchRequest struct {
	workIDs   []string
	startedAt time.Time
}

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
		result = append(result, observation)
	}
	return result, knownWork, nil
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
