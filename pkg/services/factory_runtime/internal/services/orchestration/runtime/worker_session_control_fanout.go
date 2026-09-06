package runtime

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

// workerSessionControlKey identifies the one immutable target set owned by a
// committed Factory turn control. ControlID is the upstream control-intent
// identity; a retry must return the original evidence, never reselect the
// current ledger or reach a replacement turn.
type workerSessionControlKey struct {
	turnID    string
	controlID string
	action    factory.WorkerSessionControlAction
}

// controlAssociatedWorkerSessions applies one committed Factory turn control
// to the captured turn's canonical Worker Session target set. It always
// attempts every target in the selector's stable order. Individual Worker
// Sessions failures remain per-child evidence instead of becoming a fail-fast
// Factory error, leaving Factory Runtime's existing dispatch-result authority
// intact.
func (f *factoryImpl) controlAssociatedWorkerSessions(
	ctx context.Context,
	turnID string,
	controlID string,
	action factory.WorkerSessionControlAction,
	lifecycleOutcome factory.ControlOutcome,
) factory.WorkerSessionControlResult {
	turnID = strings.TrimSpace(turnID)
	controlID = strings.TrimSpace(controlID)
	key := workerSessionControlKey{turnID: turnID, controlID: controlID, action: action}
	if controlID != "" {
		if prior, ok := f.workerSessionControlResults[key]; ok {
			return cloneWorkerSessionControlResult(prior)
		}
	}
	if lifecycleOutcome == factory.ControlOutcomeNoOp {
		if prior, ok := f.workerSessionControlResults[key]; ok {
			return cloneWorkerSessionControlResult(prior)
		}
		return newWorkerSessionControlNoOp(turnID, action)
	}

	captured := selectAssociatedWorkerSessionTargets(f.canonicalWorkerSessionControlEvents(), turnID)
	result := fanOutWorkerSessionControl(ctx, f.cfg.workerSessions, captured, action, controlID)
	f.workerSessionControlResults[key] = cloneWorkerSessionControlResult(result)
	f.logWorkerSessionControlFanout(result)
	return result
}

func newWorkerSessionControlNoOp(
	turnID string,
	action factory.WorkerSessionControlAction,
) factory.WorkerSessionControlResult {
	return factory.WorkerSessionControlResult{
		TurnID:   turnID,
		Action:   action,
		Outcome:  factory.WorkerSessionControlAggregateOutcomeNoOp,
		Children: []factory.WorkerSessionControlChildResult{},
	}
}

func fanOutWorkerSessionControl(
	ctx context.Context,
	service workersessions.Service,
	captured capturedWorkerSessionControlTargets,
	action factory.WorkerSessionControlAction,
	controlIDs ...string,
) factory.WorkerSessionControlResult {
	result := newWorkerSessionControlNoOp(captured.turnID, action)
	targets := captured.workerSessionIDsSnapshot()
	if len(targets) == 0 {
		return result
	}

	// The parent control has already committed before fan-out. A client
	// disconnect must not abandon later siblings; Worker Sessions remains the
	// owner of any accepted child lifecycle and its own explicit effects.
	if ctx == nil {
		ctx = context.Background()
	}
	controlCtx := context.WithoutCancel(ctx)
	controlID := ""
	if len(controlIDs) > 0 {
		controlID = strings.TrimSpace(controlIDs[0])
	}
	result.Children = make([]factory.WorkerSessionControlChildResult, 0, len(targets))
	for _, workerSessionID := range targets {
		child := factory.WorkerSessionControlChildResult{WorkerSessionID: workerSessionID}
		if service == nil {
			child.Outcome = factory.WorkerSessionControlChildOutcomeFailed
			result.Children = append(result.Children, child)
			continue
		}
		controlResult, err := callWorkerSessionControl(
			controlCtx, service, action, workersessions.ControlRequest{ID: workerSessionID, RequestID: controlID},
		)
		child.DispatchID = controlResult.DispatchID
		child.Outcome = workerSessionControlChildOutcome(controlResult.Outcome, err)
		result.Children = append(result.Children, child)
	}
	result.Outcome = aggregateWorkerSessionControlOutcome(result.Children)
	return result
}

func callWorkerSessionControl(
	ctx context.Context,
	service workersessions.Service,
	action factory.WorkerSessionControlAction,
	req workersessions.ControlRequest,
) (workersessions.ControlResult, error) {
	switch action {
	case factory.WorkerSessionControlActionPause:
		return service.Pause(ctx, req)
	case factory.WorkerSessionControlActionResume:
		return service.Resume(ctx, req)
	case factory.WorkerSessionControlActionCancel:
		return service.Cancel(ctx, req)
	case factory.WorkerSessionControlActionTerminate:
		return service.Terminate(ctx, req)
	default:
		return workersessions.ControlResult{}, workersessions.ErrInvalidSessionID
	}
}

func workerSessionControlChildOutcome(
	outcome workersessions.ControlOutcome,
	err error,
) factory.WorkerSessionControlChildOutcome {
	if err != nil {
		return factory.WorkerSessionControlChildOutcomeFailed
	}
	switch outcome {
	case workersessions.ControlOutcomeApplied:
		return factory.WorkerSessionControlChildOutcomeApplied
	case workersessions.ControlOutcomeNoop:
		return factory.WorkerSessionControlChildOutcomeNoOp
	case workersessions.ControlOutcomeUnsupported:
		return factory.WorkerSessionControlChildOutcomeUnsupported
	default:
		return factory.WorkerSessionControlChildOutcomeFailed
	}
}

func aggregateWorkerSessionControlOutcome(
	children []factory.WorkerSessionControlChildResult,
) factory.WorkerSessionControlAggregateOutcome {
	if len(children) == 0 {
		return factory.WorkerSessionControlAggregateOutcomeNoOp
	}
	first := children[0].Outcome
	mixed := false
	for _, child := range children {
		if child.Outcome == factory.WorkerSessionControlChildOutcomeFailed {
			return factory.WorkerSessionControlAggregateOutcomeFailed
		}
		if child.Outcome != first {
			mixed = true
		}
	}
	if mixed {
		return factory.WorkerSessionControlAggregateOutcomePartial
	}
	switch first {
	case factory.WorkerSessionControlChildOutcomeApplied:
		return factory.WorkerSessionControlAggregateOutcomeApplied
	case factory.WorkerSessionControlChildOutcomeNoOp:
		return factory.WorkerSessionControlAggregateOutcomeNoOp
	case factory.WorkerSessionControlChildOutcomeUnsupported:
		return factory.WorkerSessionControlAggregateOutcomeUnsupported
	default:
		return factory.WorkerSessionControlAggregateOutcomeFailed
	}
}

func cloneWorkerSessionControlResult(
	value factory.WorkerSessionControlResult,
) factory.WorkerSessionControlResult {
	value.Children = append([]factory.WorkerSessionControlChildResult(nil), value.Children...)
	return value
}

func (f *factoryImpl) logWorkerSessionControlFanout(result factory.WorkerSessionControlResult) {
	if f == nil || f.logger == nil {
		return
	}
	f.logger.Info(
		"factory runtime worker session control fanout",
		"session_id", sessionIDFromFactoryConfig(f.cfg),
		"turn_id", result.TurnID,
		"action", string(result.Action),
		"outcome", string(result.Outcome),
		"target_count", len(result.Children),
	)
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

// StreamObservationsByWorkerSessionID gives the canonical Worker-ID route the
// same durable Factory Session history as the legacy provider-reference route.
// A live registry is only the fallback for a session that is not present in
// the durable projection.
func (s *recordedWorkerSessionObservation) StreamObservationsByWorkerSessionID(
	ctx context.Context,
	req workersessions.StreamObservationsByWorkerSessionIDRequest,
) (workersessions.ObservationSubscription, error) {
	if s == nil || (s.ledger == nil && s.projector == nil && s.Service == nil) {
		return workersessions.ObservationSubscription{}, workersessions.ErrObservationProjectionUnavailable
	}
	if err := req.Validate(); err != nil {
		return workersessions.ObservationSubscription{}, err
	}
	req.WorkerSessionID = strings.TrimSpace(req.WorkerSessionID)
	if err := observationContextError(ctx); err != nil {
		return workersessions.ObservationSubscription{}, err
	}
	if subscription, handled, err := s.streamDurableWorkerSession(ctx, req); handled {
		return subscription, err
	}
	if subscription, handled, err := s.streamRecordedByWorkerSessionID(ctx, req); handled {
		return subscription, err
	}
	if s.Service == nil {
		return workersessions.ObservationSubscription{}, workersessions.ErrObservationProjectionUnavailable
	}
	subscription, err := s.Service.StreamObservationsByWorkerSessionID(ctx, req)
	if err != nil {
		return workersessions.ObservationSubscription{}, err
	}
	if s.hasDurableWorkerHistory() {
		return withRecordedWorkerStreamGeneration(subscription, req.WorkerSessionID), nil
	}
	return subscription, nil
}

func (s *recordedWorkerSessionObservation) streamDurableWorkerSession(
	ctx context.Context,
	req workersessions.StreamObservationsByWorkerSessionIDRequest,
) (workersessions.ObservationSubscription, bool, error) {
	if !s.hasDurableWorkerHistory() || s.Service == nil {
		return workersessions.ObservationSubscription{}, false, nil
	}
	observation, err := s.Service.GetObservationByWorkerSessionID(ctx, workersessions.GetObservationByWorkerSessionIDRequest{
		WorkerSessionID: req.WorkerSessionID,
	})
	if err == nil && observation.ProviderSessionAvailable && !durableWorkerCursor(req.Cursor) {
		if subscription, handled, streamErr := s.streamRecordedByWorkerSessionID(ctx, req); handled {
			return subscription, true, streamErr
		}
	}
	subscription, err := s.Service.StreamObservationsByWorkerSessionID(ctx, req)
	if err == nil {
		return withRecordedWorkerStreamGeneration(subscription, req.WorkerSessionID), true, nil
	}
	if !errors.Is(err, workersessions.ErrObservationSessionNotFound) {
		return workersessions.ObservationSubscription{}, true, err
	}
	return workersessions.ObservationSubscription{}, false, nil
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
	if err := s.validateRecordingHealth(ctx); err != nil {
		return workersessions.ObservationSubscription{}, true, err
	}
	return s.streamRecordedFact(ctx, fact, req.Limit, req.ReplayOnly, req.Cursor)
}

func (s *recordedWorkerSessionObservation) streamRecordedByWorkerSessionID(
	ctx context.Context,
	req workersessions.StreamObservationsByWorkerSessionIDRequest,
) (workersessions.ObservationSubscription, bool, error) {
	if s.ledger == nil || s.projector == nil {
		return workersessions.ObservationSubscription{}, false, nil
	}
	fact, found, err := s.recordedObservationForWorkerSessionID(ctx, req.WorkerSessionID)
	if err != nil {
		return workersessions.ObservationSubscription{}, true, err
	}
	if !found {
		if s.Service == nil {
			return workersessions.ObservationSubscription{}, true, workersessions.ErrObservationSessionNotFound
		}
		return workersessions.ObservationSubscription{}, false, nil
	}
	if err := s.validateRecordingHealth(ctx); err != nil {
		return workersessions.ObservationSubscription{}, true, err
	}
	return s.streamRecordedFact(ctx, fact, req.Limit, req.ReplayOnly, req.Cursor)
}

func (s *recordedWorkerSessionObservation) streamRecordedFact(
	ctx context.Context,
	fact recordedDispatchObservation,
	limit int,
	replayOnly bool,
	cursor *workersessions.ObservationCursor,
) (workersessions.ObservationSubscription, bool, error) {
	streamContext := ctx
	if streamContext == nil {
		streamContext = context.Background()
	}
	streamContext, cancel := context.WithCancel(streamContext)
	limit = observationStreamLimit(limit)
	canonicalEvents := s.canonicalEvents()
	if err := validateRecordedObservationCursor(s.ledger, canonicalEvents, fact, cursor, s.hasDurableWorkerHistory()); err != nil {
		cancel()
		return workersessions.ObservationSubscription{}, true, err
	}
	var reconnect *interfaces.FactoryEventReconnectCursor
	if cursor != nil {
		sequence := int(cursor.Position)
		reconnect = &interfaces.FactoryEventReconnectCursor{AfterSequence: &sequence}
	}
	var source interfaces.FactoryEventStream
	var subscribeErr error
	if len(s.replayEvents) > 0 {
		source, subscribeErr = s.subscribeReplayObservationEvents(
			streamContext, canonicalEvents, fact.dispatchID, limit, replayOnly,
		)
	} else {
		source, subscribeErr = s.ledger.Subscribe(streamContext, reconnect, interfaces.FactoryEventReconnectScope{
			DispatchID: fact.dispatchID,
			Limit:      limit,
		})
	}
	if subscribeErr != nil {
		cancel()
		if errors.Is(subscribeErr, context.Canceled) {
			return workersessions.ObservationSubscription{}, true, workersessions.ErrObservationCanceled
		}
		return workersessions.ObservationSubscription{}, true, workersessions.ErrObservationSourceUnavailable
	}
	source.History = recordedObservationHistory(source.History, fact.dispatchID, cursor)
	if s.hasDurableWorkerHistory() {
		source.StreamGenerationID = recordedWorkerStreamGenerationForIdentity(fact.workerSessionID)
	}
	terminalReplay := recordedObservationHistoryHasTerminal(source.History, fact.dispatchID)
	finite := replayOnly || fact.state.Terminal() || terminalReplay
	var summary *workersessions.ReplaySummary
	if finite {
		health := workerRecordingHealth{}
		if statuses, healthErr := s.recordingHealth(ctx); healthErr != nil {
			cancel()
			return workersessions.ObservationSubscription{}, true, healthErr
		} else if statuses != nil {
			health = statuses[fact.workerSessionID]
		}
		summary = recordedObservationReplaySummary(fact, health)
	}
	return newRecordedObservationSubscriptionWithSummary(
		source, fact.dispatchID, terminalReplay, cancel, streamContext, finite, summary, fact.workerSessionID,
	), true, nil
}

func (s *recordedWorkerSessionObservation) subscribeReplayObservationEvents(
	ctx context.Context,
	events []interfaces.FactoryEvent,
	dispatchID string,
	limit int,
	replayOnly bool,
) (interfaces.FactoryEventStream, error) {
	if replayOnly {
		closed := make(chan interfaces.FactoryEvent)
		close(closed)
		return interfaces.FactoryEventStream{
			StreamGenerationID: s.ledger.StreamGenerationID(),
			History:            cloneAndSortFactoryEvents(events),
			Events:             closed,
		}, nil
	}
	source, err := s.ledger.Subscribe(ctx, nil, interfaces.FactoryEventReconnectScope{
		DispatchID: dispatchID,
		Limit:      limit,
	})
	if err != nil {
		return interfaces.FactoryEventStream{}, err
	}
	source.History = cloneAndSortFactoryEvents(events)
	return source, nil
}

func validateRecordedObservationCursor(
	ledger recordings.RuntimeLedger,
	events []interfaces.FactoryEvent,
	fact recordedDispatchObservation,
	cursor *workersessions.ObservationCursor,
	durableHistory bool,
) error {
	if cursor == nil {
		return nil
	}
	if err := cursor.Validate(); err != nil {
		return err
	}
	if err := validateRecordedCursorIdentity(ledger, fact, cursor, durableHistory); err != nil {
		return err
	}
	return validateRecordedCursorPosition(events, fact, cursor)
}

func validateRecordedCursorIdentity(
	ledger recordings.RuntimeLedger,
	fact recordedDispatchObservation,
	cursor *workersessions.ObservationCursor,
	durableHistory bool,
) error {
	if workerSessionID := strings.TrimSpace(cursor.WorkerSessionID); workerSessionID != "" &&
		workerSessionID != strings.TrimSpace(fact.workerSessionID) {
		return workersessions.ErrObservationCursorForeign
	}
	generationID := strings.TrimSpace(cursor.StreamGenerationID)
	if generationID == "" || generationID == recordedObservationGeneration(ledger, fact.workerSessionID, durableHistory) {
		return nil
	}
	if strings.HasPrefix(generationID, "worker-recording/") &&
		strings.TrimPrefix(generationID, "worker-recording/") != strings.TrimSpace(fact.workerSessionID) {
		return workersessions.ErrObservationCursorForeign
	}
	return workersessions.ErrObservationCursorUnavailable
}

func validateRecordedCursorPosition(
	events []interfaces.FactoryEvent,
	fact recordedDispatchObservation,
	cursor *workersessions.ObservationCursor,
) error {
	var highest uint64
	var acknowledged *interfaces.FactoryEvent
	for _, event := range events {
		sequence := event.Context.Sequence
		if sequence > 0 && uint64(sequence) > highest {
			highest = uint64(sequence)
		}
		if sequence > 0 && uint64(sequence) == cursor.Position {
			candidate := event
			acknowledged = &candidate
		}
	}
	if cursor.Position > highest {
		return workersessions.ErrObservationCursorFuture
	}
	if acknowledged == nil {
		return workersessions.ErrObservationCursorStale
	}
	if stringPointerValue(acknowledged.Context.DispatchID) != fact.dispatchID {
		return workersessions.ErrObservationCursorForeign
	}
	return nil
}

func (s *recordedWorkerSessionObservation) recordedObservationForProvider(
	ctx context.Context,
	ref providers.SessionRef,
) (recordedDispatchObservation, bool, error) {
	facts, err := s.recordedObservationFacts(ctx)
	if err != nil {
		return recordedDispatchObservation{}, false, err
	}
	for _, fact := range facts {
		if fact.provider != nil && providerSessionRef(*fact.provider) == ref {
			return fact, true, nil
		}
	}
	return recordedDispatchObservation{}, false, nil
}

func (s *recordedWorkerSessionObservation) recordedObservationForWorkerSessionID(
	ctx context.Context,
	workerSessionID string,
) (recordedDispatchObservation, bool, error) {
	facts, err := s.recordedObservationFacts(ctx)
	if err != nil {
		return recordedDispatchObservation{}, false, err
	}
	for _, fact := range facts {
		if fact.workerSessionID == workerSessionID {
			return fact, true, nil
		}
	}
	return recordedDispatchObservation{}, false, nil
}

func (s *recordedWorkerSessionObservation) recordedObservationFacts(
	ctx context.Context,
) ([]recordedDispatchObservation, error) {
	if err := observationContextError(ctx); err != nil {
		return nil, err
	}
	if s == nil || s.ledger == nil || s.projector == nil {
		return nil, workersessions.ErrObservationProjectionUnavailable
	}
	ordered := s.canonicalEvents()
	world, err := s.projector(ordered, latestFactoryEventTick(ordered))
	if err != nil {
		return nil, workersessions.ErrObservationProjectionUnavailable
	}
	associations, requests := recordedDispatchFacts(ordered)
	completed := recordedDispatchStateMaps(world)
	dispatchIDs := make([]string, 0, len(associations))
	for dispatchID := range associations {
		dispatchIDs = append(dispatchIDs, dispatchID)
	}
	sort.Strings(dispatchIDs)
	facts := make([]recordedDispatchObservation, 0, len(dispatchIDs))
	for _, dispatchID := range dispatchIDs {
		facts = append(facts, s.annotateRecordedFact(recordedDispatchFact(
			dispatchID, associations[dispatchID], requests, completed,
			world.ProviderSessions, world.ActiveDispatches, ordered,
		)))
	}
	return facts, nil
}

type recordedObservationSubscription struct {
	stream          interfaces.FactoryEventStream
	dispatchID      string
	generationID    string
	workerSessionID string
	terminalReplay  bool
	finite          bool
	summary         *workersessions.ReplaySummary
	cancel          context.CancelFunc
	sourceContext   context.Context

	mu            sync.Mutex
	closed        bool
	summarySent   bool
	eventsEmitted int
	seen          map[uint64]struct{}
	history       []interfaces.FactoryEvent
	historyRead   int
}

func newRecordedObservationSubscription(
	stream interfaces.FactoryEventStream,
	dispatchID string,
	terminalReplay bool,
	cancel context.CancelFunc,
	sourceContext context.Context,
	workerSessionIDArgs ...string,
) workersessions.ObservationSubscription {
	return newRecordedObservationSubscriptionWithSummary(
		stream, dispatchID, terminalReplay, cancel, sourceContext, false, nil, workerSessionIDArgs...,
	)
}

func newRecordedObservationSubscriptionWithSummary(
	stream interfaces.FactoryEventStream,
	dispatchID string,
	terminalReplay bool,
	cancel context.CancelFunc,
	sourceContext context.Context,
	finite bool,
	summary *workersessions.ReplaySummary,
	workerSessionIDArgs ...string,
) workersessions.ObservationSubscription {
	workerSessionID := ""
	if len(workerSessionIDArgs) > 0 {
		workerSessionID = strings.TrimSpace(workerSessionIDArgs[0])
	}
	subscription := &recordedObservationSubscription{
		stream:          stream,
		dispatchID:      dispatchID,
		terminalReplay:  terminalReplay,
		finite:          finite,
		summary:         cloneReplaySummary(summary),
		generationID:    strings.TrimSpace(stream.StreamGenerationID),
		workerSessionID: workerSessionID,
		cancel:          cancel,
		sourceContext:   sourceContext,
		seen:            make(map[uint64]struct{}),
		history:         cloneAndSortFactoryEvents(stream.History),
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
		if delivery, ok := s.nextHistoryDelivery(); ok {
			return delivery
		}
		if delivery, ok := s.nextFiniteSummary(); ok {
			return delivery
		}
		if delivery, ok := s.nextLiveDelivery(ctx); ok {
			return delivery
		}
	}
}

func (s *recordedObservationSubscription) nextHistoryDelivery() (workersessions.ObservationDelivery, bool) {
	event, ok := s.nextHistoryEvent()
	if !ok {
		return workersessions.ObservationDelivery{}, false
	}
	delivery, terminal := s.project(event)
	if delivery == nil {
		return workersessions.ObservationDelivery{}, false
	}
	if terminal {
		s.close()
	}
	return *delivery, true
}

func (s *recordedObservationSubscription) nextLiveDelivery(ctx context.Context) (workersessions.ObservationDelivery, bool) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return workersessions.ObservationDelivery{Kind: workersessions.ObservationDeliveryClosed}, true
	}
	streamContext := s.streamContext()
	events := s.stream.Events
	s.mu.Unlock()
	if events == nil {
		s.close()
		return workersessions.ObservationDelivery{Kind: workersessions.ObservationDeliverySourceFailure, Err: workersessions.ErrObservationSourceClosed}, true
	}
	select {
	case <-ctx.Done():
		s.close()
		return workersessions.ObservationDelivery{Kind: workersessions.ObservationDeliveryCanceled, Err: workersessions.ErrObservationCanceled}, true
	case <-streamContext.Done():
		return workersessions.ObservationDelivery{Kind: workersessions.ObservationDeliveryClosed}, true
	case event, ok := <-events:
		if !ok {
			s.close()
			return workersessions.ObservationDelivery{Kind: workersessions.ObservationDeliverySourceFailure, Err: workersessions.ErrObservationSourceClosed}, true
		}
		if delivery, terminal := s.project(event); delivery != nil {
			if terminal {
				s.close()
			}
			return *delivery, true
		}
		return workersessions.ObservationDelivery{}, false
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
	position := event.Context.Sequence
	if position > 0 {
		s.mu.Lock()
		if s.seen == nil {
			s.seen = make(map[uint64]struct{})
		}
		if _, exists := s.seen[uint64(position)]; exists {
			s.mu.Unlock()
			return nil, false
		}
		s.seen[uint64(position)] = struct{}{}
		s.eventsEmitted++
		s.mu.Unlock()
	}
	terminal := recordedWorkerSessionTerminalEvent(event)
	delivery := workersessions.ObservationDelivery{Kind: workersessions.ObservationDeliveryRecord, Event: recordedObservationEvent(event, s.generationID, s.workerSessionID)}
	if terminal {
		delivery.Kind = workersessions.ObservationDeliveryTerminal
		if s.terminalReplay {
			delivery.Kind = workersessions.ObservationDeliveryTerminalReplay
		}
		if s.finite && s.summary != nil {
			summary := *s.summary
			summary.EventsEmitted = s.eventsEmitted
			delivery.Summary = &summary
			s.mu.Lock()
			s.summarySent = true
			s.mu.Unlock()
		}
	}
	return &delivery, terminal
}

func (s *recordedObservationSubscription) nextFiniteSummary() (workersessions.ObservationDelivery, bool) {
	s.mu.Lock()
	if !s.finite || s.summary == nil || s.summarySent || s.closed {
		s.mu.Unlock()
		return workersessions.ObservationDelivery{}, false
	}
	summary := *s.summary
	summary.EventsEmitted = s.eventsEmitted
	s.summarySent = true
	s.closed = true
	cancel := s.cancel
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return workersessions.ObservationDelivery{
		Kind:    workersessions.ObservationDeliveryReplaySummary,
		Summary: &summary,
	}, true
}

func cloneReplaySummary(summary *workersessions.ReplaySummary) *workersessions.ReplaySummary {
	if summary == nil {
		return nil
	}
	clone := *summary
	return &clone
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

func recordedObservationEvent(event interfaces.FactoryEvent, identityArgs ...string) workersessions.ObservationEvent {
	generationID := ""
	workerSessionID := ""
	if len(identityArgs) > 0 {
		generationID = identityArgs[0]
	}
	if len(identityArgs) > 1 {
		workerSessionID = strings.TrimSpace(identityArgs[1])
	}
	position := event.Context.Sequence
	if position < 0 {
		position = 0
	}
	return workersessions.ObservationEvent{
		Position: uint64(position),
		Cursor: workersessions.ObservationCursor{
			StreamGenerationID: strings.TrimSpace(generationID),
			WorkerSessionID:    workerSessionID,
			Position:           uint64(position),
		},
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

func recordedOptionalString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func cloneRecordedTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

type recordedDispatchObservation struct {
	workerSessionID    string
	dispatchID         string
	streamGenerationID string
	stateSequence      int64
	stateSequenceKnown bool
	turnID             string
	workIDs            []string
	model              *string
	reasoningEffort    *string
	startedAt          time.Time
	endedAt            *time.Time
	state              workersessions.State
	failure            *workersessions.FailureCause
	provider           *providers.SessionMetadata
	tokenUsage         *workersessions.TokenUsage
}

type recordedDispatchAssociation struct {
	workerSessionID string
	turnID          string
	model           *string
	reasoningEffort *string
	eventTime       time.Time
}

type recordedDispatchRequest struct {
	workIDs   []string
	startedAt time.Time
}
