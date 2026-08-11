// Package service implements the Worker Sessions W1+W2 registry:
// reservation, immutable Get, deterministic filtered List, and supervised
// Start with exactly-once terminal classification, over a synchronized
// process-local map.
package service

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"sort"
	"strings"
	"sync"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/services/events"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// ErrMissingExecution reports that New was constructed without the one
// required directly injected workers.WorkstationExecutionService.
var ErrMissingExecution = errors.New("worker sessions: execution service is required")

// ErrMissingEventsAppender reports that New was constructed without the one
// required directly injected EventsAppender.
var ErrMissingEventsAppender = errors.New("worker sessions: events appender is required")

// ErrMissingClock reports that New was constructed without the required
// runtime time source used for observation timing.
var ErrMissingClock = errors.New("worker sessions: clock is required")

// ErrMissingProviderSessions reports that New was constructed without the
// Provider Sessions read-side contract used to enrich worker observations.
var ErrMissingProviderSessions = errors.New("worker sessions: provider sessions service is required")

// EventsAppender is the narrow Events dependency Start's before-handoff
// publication barrier needs: commit one source-native record into a topic's
// aggregate ordering. registry depends on this port rather than the full
// events.Service so a caller wiring only this package's own tests never has
// to satisfy Read/Subscribe/AttachSource too. Any events.Service value
// already satisfies this interface structurally.
type EventsAppender interface {
	Append(context.Context, events.AppendRequest) (events.AppendResult, error)
}

// EventsReader is the narrow canonical source needed by the observation
// stream. An events.Service satisfies it structurally; tests and alternate
// construction paths may provide only the read/subscribe capabilities.
type EventsReader interface {
	Subscribe(context.Context, events.SubscribeRequest) (events.Subscription, error)
}

// EventsRetainedReader is the narrow finite-replay dependency. Keeping Read
// separate from EventsReader makes the live subscription path impossible to
// enter accidentally when a replay-only request is handled.
type EventsRetainedReader interface {
	Read(context.Context, events.ReadRequest) (events.ReadResult, error)
}

type registry struct {
	mu           sync.RWMutex
	sessions     map[string]workersessions.Session
	publications map[string]*publication
	supervisions map[string]*supervision
	observations map[string]*observation
	startReplays map[string]*startReplay
	// dispatchOwners is the Worker Sessions-owned reverse lookup from the
	// currently supervised Workers dispatch to its stable session identity.
	// Provider progress names dispatches, never Worker Sessions, so this map is
	// the only accepted route from a provider observation to its owner.
	dispatchOwners   map[string]string
	boundary         workers.WorkstationPoolBoundary
	events           EventsAppender
	eventReader      EventsReader
	retainedReader   EventsRetainedReader
	providerSessions providersessions.Service
	clock            platformclock.Source
	logger           logging.Logger

	// lifecycleCtx is owned by the process composition boundary. Request
	// contexts are never used as the lifetime of an admitted Start. Stop
	// keeps this context alive until active callbacks have published their
	// terminal records, then cancels it to release any remaining lifecycle
	// work.
	lifecycleCtx    context.Context
	lifecycleCancel context.CancelFunc
	stopping        bool
	activeStarts    int
	startsDone      chan struct{}
	stopOnce        sync.Once
	stopDone        chan struct{}
	stopErr         error
}

// Compile-time proof that production registry seals the W1+W2 root contract
// (Reserve + Get + List + Start) without exposing the mutable map or a
// broader API.
var _ workersessions.Service = (*registry)(nil)

// New constructs the process-local Worker Session registry from its required
// lifecycle, time, and Provider Sessions collaborators. A nil logger falls
// back to logging.NoopLogger. A nil execution, Events appender, clock, or
// Provider Sessions service is rejected: the registry cannot truthfully
// supervise, time, or enrich an observation without each of them.
func New(
	boundary workers.WorkstationPoolBoundary,
	eventsAppender EventsAppender,
	logger logging.Logger,
	clock platformclock.Source,
	providerSessions providersessions.Service,
) (workersessions.Service, error) {
	if boundary == nil {
		return nil, ErrMissingExecution
	}
	if eventsAppender == nil {
		return nil, ErrMissingEventsAppender
	}
	if clock == nil {
		return nil, ErrMissingClock
	}
	if providerSessions == nil {
		return nil, ErrMissingProviderSessions
	}
	lifecycleCtx, lifecycleCancel := context.WithCancel(context.Background())
	startsDone := make(chan struct{})
	close(startsDone)
	registry := &registry{
		sessions:         make(map[string]workersessions.Session),
		publications:     make(map[string]*publication),
		supervisions:     make(map[string]*supervision),
		observations:     make(map[string]*observation),
		startReplays:     make(map[string]*startReplay),
		dispatchOwners:   make(map[string]string),
		boundary:         boundary,
		events:           eventsAppender,
		clock:            clock,
		providerSessions: providerSessions,
		logger:           logging.EnsureLogger(logger),
		lifecycleCtx:     lifecycleCtx,
		lifecycleCancel:  lifecycleCancel,
		startsDone:       startsDone,
		stopDone:         make(chan struct{}),
	}
	if reader, ok := eventsAppender.(EventsReader); ok {
		registry.eventReader = reader
	}
	if reader, ok := eventsAppender.(EventsRetainedReader); ok {
		registry.retainedReader = reader
	}
	return registry, nil
}

// Start establishes or replays one Worker Session and returns at the exact
// Workers admission barrier. The request ID is claimed together with the
// stable session identity before any opening event or Workers handoff. The
// owner drives the state machine once; concurrent and later replays wait for
// and return the owner's stored acceptance or pre-admission failure.
func (r *registry) Start(ctx context.Context, req workersessions.StartRequest) (workersessions.StartResult, error) {
	callerCtx := ctx
	if callerCtx == nil {
		callerCtx = context.Background()
	}
	attemptID := req.Execution.Execution.Dispatch.DispatchID
	if err := req.Validate(); err != nil {
		r.logger.Info("worker session start rejected", "sessionID", req.ID, "attemptID", attemptID, "outcome", "invalid")
		return workersessions.StartResult{}, err
	}
	req = normalizeStartRequest(req)
	replay, owner, err := r.reserveStart(req)
	if err != nil {
		r.logger.Info("worker session start rejected", "sessionID", req.ID, "attemptID", attemptID, "requestID", req.RequestID, "outcome", startReservationOutcome(err))
		return workersessions.StartResult{}, err
	}
	if !owner {
		result, replayErr := awaitStartReplay(callerCtx, replay)
		r.logger.Info("worker session start replay", "sessionID", replay.sessionID, "attemptID", attemptID, "requestID", req.RequestID, "outcome", startReplayOutcome(replayErr))
		return result, replayErr
	}

	outcomes := make(chan asyncStartCompletion, 1)
	go func() {
		result, startErr := r.startReserved(callerCtx, req)
		r.finishStartReplay(replay, result, startErr)
		r.finishStart()
		outcomes <- asyncStartCompletion{result: result, err: startErr}
	}()
	select {
	case outcome := <-outcomes:
		return outcome.result, outcome.err
	case <-callerCtx.Done():
		select {
		case outcome := <-outcomes:
			return outcome.result, outcome.err
		default:
		}
		r.logger.Info("worker session start wait canceled", "sessionID", req.ID, "attemptID", attemptID, "requestID", req.RequestID, "outcome", "caller_canceled")
		return workersessions.StartResult{}, callerCtx.Err()
	}
}

// reserveStart atomically claims the caller request ID and stable session
// identity. No downstream effect is possible until this method has installed
// both registry records and returned ownership to the first caller.
func (r *registry) reserveStart(req workersessions.StartRequest) (*startReplay, bool, error) {
	tuple := startTupleFor(req)
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.startReplays == nil {
		r.startReplays = make(map[string]*startReplay)
	}
	if existing, ok := r.startReplays[req.RequestID]; ok {
		if !reflect.DeepEqual(existing.tuple, tuple) {
			return nil, false, workersessions.ErrStartRequestIDConflict
		}
		return existing, false, nil
	}
	if r.stopping {
		return nil, false, workersessions.ErrStartServerStopping
	}
	if _, exists := r.sessions[req.ID]; exists {
		return nil, false, workersessions.ErrSessionNotStartable
	}

	replay := &startReplay{
		tuple:     tuple,
		sessionID: req.ID,
		done:      make(chan struct{}),
	}
	if r.startsDone == nil {
		r.startsDone = make(chan struct{})
		close(r.startsDone)
	}
	if r.activeStarts == 0 {
		r.startsDone = make(chan struct{})
	}
	r.activeStarts++
	r.sessions[req.ID] = workersessions.Session{ID: req.ID, State: workersessions.StateReserved}
	r.publications[req.ID] = &publication{}
	r.startReplays[req.RequestID] = replay
	r.logger.Info("worker session start", "sessionID", req.ID, "attemptID", req.Execution.Execution.Dispatch.DispatchID, "requestID", req.RequestID, "outcome", "reserved", "state", string(workersessions.StateReserved))
	return replay, true, nil
}

// startReserved runs the original Start state machine after reserveStart has
// atomically installed the request replay and RESERVED session records.
func (r *registry) startReserved(ctx context.Context, req workersessions.StartRequest) (workersessions.StartResult, error) {
	serverCtx := r.serverOwnedContext()
	prepared, err := r.prepareInvocation(
		serverCtx,
		workersessions.InvokeSessionRequest{ID: req.ID, Execution: req.Execution, Retry: req.Retry},
		invocationPreparationOptions{
			serverOwned:      true,
			requestID:        req.RequestID,
			verifyTopicReady: true,
		},
	)
	if err != nil {
		return workersessions.StartResult{}, err
	}
	if prepared.terminal {
		return workersessions.StartResult{Session: prepared.session}, startNotAccepted(prepared.failure)
	}
	invokeReq := workersessions.InvokeSessionRequest{
		ID:        req.ID,
		Execution: req.Execution,
		Retry:     req.Retry,
	}
	go func() {
		r.driveRegisteredInvocation(serverCtx, invokeReq, prepared.supervision)
	}()

	select {
	case <-prepared.supervision.admitted:
		return r.startResult(req.ID), nil
	case <-prepared.supervision.done:
		select {
		case <-prepared.supervision.admitted:
			return r.startResult(req.ID), nil
		default:
			return r.startResult(req.ID), startNotAccepted(r.startAdmissionCause(prepared.supervision))
		}
	}
}

func (r *registry) startResult(id string) workersessions.StartResult {
	session, _ := r.Get(context.Background(), workersessions.GetRequest{ID: id})
	return workersessions.StartResult{Session: session}
}

func (r *registry) startAdmissionCause(supervision *supervision) error {
	supervision.mu.Lock()
	defer supervision.mu.Unlock()
	if supervision.err != nil {
		return errors.Join(workersessions.ErrStartAdmissionFailed, supervision.err)
	}
	return workersessions.ErrStartAdmissionFailed
}

func (r *registry) rejectStartBeforeAdmission(ctx context.Context, id, attemptID string, cause error) (workersessions.StartResult, error) {
	final := r.terminalizeInvocationBeforeAdmission(ctx, id, attemptID)
	return workersessions.StartResult{Session: final}, startNotAccepted(cause)
}

func (r *registry) publishTerminalSnapshot(ctx context.Context, id, attemptID string, session workersessions.Session) {
	if !session.Terminal() {
		return
	}
	result := workersessions.TerminalResult{}
	if session.Result != nil {
		result = *session.Result
	}
	r.publishTerminalRecordOrLog(ctx, id, attemptID, session.State, result)
}

// ensureOpeningTopicReady proves both retained replay and live subscription
// can observe the opening record before Start hands the execution to Workers.
// The canceled cleanup call unregisters the short-lived readiness subscriber
// without relying on timing or a store-specific Close operation.
func (r *registry) ensureOpeningTopicReady(ctx context.Context, id string) error {
	if r.retainedReader == nil || r.eventReader == nil {
		return workersessions.ErrEventTopicUnavailable
	}
	topic := workersessions.Topic(id)
	readResult, err := r.retainedReader.Read(ctx, events.ReadRequest{
		Topic: topic,
		From:  events.Cursor{Topic: topic},
		Limit: 1,
	})
	if err != nil || readResult.Validate() != nil || readResult.Outcome != events.ReadOutcomeProgress || len(readResult.Records) == 0 || !openingRecordMatches(readResult.Records[0], id) {
		return workersessions.ErrEventTopicUnavailable
	}

	subscription, err := r.eventReader.Subscribe(ctx, events.SubscribeRequest{
		Topic: topic,
		From:  events.Cursor{Topic: topic},
		Limit: 1,
	})
	if err != nil || subscription == nil {
		return workersessions.ErrEventTopicUnavailable
	}
	delivery := subscription.Next(ctx)
	cleanupCtx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = subscription.Next(cleanupCtx)
	if delivery.Validate() != nil || delivery.Kind != events.DeliveryRecord || !openingRecordMatches(delivery.Record, id) {
		return workersessions.ErrEventTopicUnavailable
	}
	return nil
}

func openingRecordMatches(record events.Record, id string) bool {
	return record.ID.Topic == workersessions.Topic(id) &&
		record.SourceType == lifecycleSourceType &&
		record.SourceID == events.SourceID(id) &&
		record.SourceSequence == openingSourceSequence &&
		record.SourceEventID == openingSourceEventID &&
		record.SchemaID == workerDraftSchemaID
}

func (r *registry) Reserve(_ context.Context, req workersessions.ReserveRequest) (workersessions.Session, error) {
	if err := req.Validate(); err != nil {
		r.logger.Info("worker session reserve rejected", "sessionID", req.ID, "outcome", "invalid")
		return workersessions.Session{}, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.sessions[req.ID]; exists {
		r.logger.Info("worker session reserve", "sessionID", req.ID, "outcome", "duplicate")
		return workersessions.Session{}, workersessions.ErrSessionAlreadyExists
	}
	session := workersessions.Session{ID: req.ID, State: workersessions.StateReserved}
	r.sessions[req.ID] = session
	r.publications[req.ID] = &publication{}
	r.logger.Info("worker session reserve", "sessionID", req.ID, "outcome", "reserved")
	return session, nil
}

func (r *registry) Get(_ context.Context, req workersessions.GetRequest) (workersessions.Session, error) {
	if err := req.Validate(); err != nil {
		r.logger.Info("worker session get rejected", "sessionID", req.ID, "outcome", "invalid")
		return workersessions.Session{}, err
	}

	r.mu.RLock()
	session, exists := r.sessions[req.ID]
	r.mu.RUnlock()

	if !exists {
		r.logger.Info("worker session get", "sessionID", req.ID, "outcome", "not_found")
		return workersessions.Session{}, workersessions.ErrSessionNotFound
	}
	r.logger.Info("worker session get", "sessionID", req.ID, "outcome", "found", "state", string(session.State))
	return cloneSession(session), nil
}

func (r *registry) List(_ context.Context, req workersessions.ListRequest) (workersessions.ListResult, error) {
	if err := req.Validate(); err != nil {
		r.logger.Info("worker session list rejected", "outcome", "invalid")
		return workersessions.ListResult{}, err
	}

	r.mu.RLock()
	matched := make([]workersessions.Session, 0, len(r.sessions))
	for _, session := range r.sessions {
		if matchesFilter(session, req.Filter) {
			matched = append(matched, cloneSession(session))
		}
	}
	r.mu.RUnlock()

	sort.Slice(matched, func(i, j int) bool { return matched[i].ID < matched[j].ID })
	r.logger.Info("worker session list", "outcome", "success", "filter_state_count", len(req.Filter.States), "result_count", len(matched))
	return workersessions.ListResult{Sessions: matched}, nil
}

func matchesFilter(session workersessions.Session, filter workersessions.Filter) bool {
	if len(filter.States) == 0 {
		return true
	}
	return slices.Contains(filter.States, session.State)
}

// reserveIfAbsent stores id as a new StateReserved session when it is not
// already registered, in its own locked critical section distinct from
// transitionToStarting. This makes a brand-new identity's RESERVED state a
// genuine, observable map write (visible to a concurrent Get/List) before
// Start ever transitions it to StateStarting or calls Workers. An identity
// already registered, in any state, is left untouched here; conflicts are
// reported by the following transitionToStarting call.
func (r *registry) reserveIfAbsent(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.sessions[id]; exists {
		return
	}
	r.sessions[id] = workersessions.Session{ID: id, State: workersessions.StateReserved}
	r.publications[id] = &publication{}
}

// transitionToStarting atomically moves id from StateReserved to
// StateStarting. Only one caller can win this transition for a given id: a
// concurrent Start racing to claim the same newly reserved or already
// reserved identity, or an identity in any other state, sees
// ErrSessionNotStartable and makes no mutation and no Workers call.
func (r *registry) transitionToStarting(id string) (workersessions.Session, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	session, exists := r.sessions[id]
	if !exists || session.State != workersessions.StateReserved {
		return workersessions.Session{}, workersessions.ErrSessionNotStartable
	}

	session.State = workersessions.StateStarting
	session.Result = nil
	r.sessions[id] = session
	return cloneSession(session), nil
}

// commitTerminal stores the exactly-once terminal outcome for id and reports
// whether this call is the one that committed it. The commit requires the
// one allowed W2 predecessor state, StateStarting: a missing identity, an
// already-terminal identity (for example because a duplicate or racing
// callback reaches commitTerminal for the same identity), or any other
// nonterminal state (StateReserved, StatePaused) is left completely
// unchanged and returned as-is, and committed reports false. This makes the
// terminal write itself absorbing regardless of how many callers reach
// commitTerminal for one identity. STARTING handles a synchronous boundary
// callback; RUNNING handles an asynchronously accepted attempt. Reserved
// attempts remain ineligible so a terminal outcome is never fabricated for
// an identity that never reached supervision.
// Only the caller for which committed is true may emit the terminal
// effect/log for this identity.
func (r *registry) commitTerminal(id string, state workersessions.State, result workersessions.TerminalResult) (workersessions.Session, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	existing, exists := r.sessions[id]
	if !exists || (existing.State != workersessions.StateStarting && existing.State != workersessions.StateRunning) {
		return cloneSession(existing), false
	}
	result = normalizeCommittedTerminal(state, result)

	session := existing
	session.State = state
	session.Result = cloneTerminalResult(&result)
	r.sessions[id] = session
	r.finishObservationLocked(id, r.clock.Now())
	return cloneSession(session), true
}

// normalizeCommittedTerminal is the last in-process guard before a terminal
// snapshot becomes durable. Normal classification already supplies a valid
// result, but this boundary also protects against an adapter or future caller
// constructing an empty/overlong cause: a FAILED session and its event must
// never be committed with a blank diagnostic.
func normalizeCommittedTerminal(state workersessions.State, result workersessions.TerminalResult) workersessions.TerminalResult {
	switch state {
	case workersessions.StateCompleted:
		return workersessions.TerminalResult{Outcome: workersessions.TerminalOutcomeCompleted}
	case workersessions.StateFailed:
		if result.Outcome != workersessions.TerminalOutcomeFailed || result.Cause == nil || !result.Cause.Kind.Valid() {
			return failedTerminal(
				workersessions.FailureCauseWorkersExecutionFailure,
				"the Worker Session failed without a reported cause",
			)
		}
		cause := *result.Cause
		cause.Detail = boundedFailureDetail(cause.Kind, cause.Detail)
		cause.ProviderFailureKind,
			cause.ProviderContinuationFailureKind,
			cause.ProviderContinuationOutcome = workersessions.SanitizeProviderFailureClassification(
			cause.ProviderFailureKind,
			cause.ProviderContinuationFailureKind,
			cause.ProviderContinuationOutcome,
		)
		return workersessions.TerminalResult{
			Outcome: workersessions.TerminalOutcomeFailed,
			Cause:   &cause,
		}
	default:
		return result
	}
}

// commitControlTerminal terminalizes an unstarted or explicitly canceled
// session without inventing a completed/failed result. A control can win
// before a boundary publish begins (RESERVED/STARTING) or after a boundary
// cancellation is observed (RUNNING); terminal states remain absorbing.
func (r *registry) commitControlTerminal(id string, state workersessions.State) (workersessions.Session, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	existing, exists := r.sessions[id]
	if !exists || existing.Terminal() {
		return cloneSession(existing), false
	}
	if state != workersessions.StateCanceled && state != workersessions.StateTerminated {
		return cloneSession(existing), false
	}
	existing.State = state
	existing.Result = nil
	r.sessions[id] = existing
	r.finishObservationLocked(id, r.clock.Now())
	return cloneSession(existing), true
}

// cloneSession returns a detached copy of session: mutating the returned
// value, or its Result, never affects registry-owned state.
func cloneSession(session workersessions.Session) workersessions.Session {
	session.Result = cloneTerminalResult(session.Result)
	if session.ProviderSessionAssociation != nil {
		association := session.ProviderSessionAssociation.Clone()
		session.ProviderSessionAssociation = &association
	}
	return session
}

func cloneTerminalResult(result *workersessions.TerminalResult) *workersessions.TerminalResult {
	if result == nil {
		return nil
	}
	clone := *result
	if result.Cause != nil {
		cause := *result.Cause
		clone.Cause = &cause
	}
	return &clone
}

func causeKindString(cause *workersessions.FailureCause) string {
	if cause == nil {
		return ""
	}
	return string(cause.Kind)
}

// AssociateProviderSession records the exact reference one Worker attempt
// observed. The caller provides only source-native facts; this registry owns
// every correlation value and refuses a reference for another dispatch.
func (r *registry) AssociateProviderSession(
	_ context.Context,
	req workersessions.ProviderSessionAssociationRequest,
) (workersessions.ProviderSessionAssociationResult, error) {
	if err := req.Validate(); err != nil {
		r.logger.Info("worker session provider session association rejected", "sessionID", req.WorkerSessionID, "attemptID", req.DispatchID, "outcome", "invalid")
		return workersessions.ProviderSessionAssociationResult{}, err
	}

	result, err := r.associateProviderSession(req)
	if err != nil {
		r.logger.Info("worker session provider session association rejected", "sessionID", req.WorkerSessionID, "attemptID", req.DispatchID, "outcome", "rejected")
		return workersessions.ProviderSessionAssociationResult{}, err
	}
	r.logger.Info("worker session provider session association", "sessionID", req.WorkerSessionID, "attemptID", req.DispatchID, "outcome", string(result.Outcome))
	return result, nil
}

// ObserveProviderSession commits one typed reference reported by the Workers
// progress path. The source identifies only its dispatch; resolving that
// dispatch under the registry lock keeps one child observation from being
// attached to another Worker Session.
func (r *registry) ObserveProviderSession(
	_ context.Context,
	req workersessions.ProviderSessionObservationRequest,
) (workersessions.ProviderSessionAssociationResult, error) {
	if err := req.Validate(); err != nil {
		r.logger.Info("worker session provider session observation rejected", "attemptID", req.DispatchID, "outcome", "invalid")
		return workersessions.ProviderSessionAssociationResult{}, err
	}

	r.mu.Lock()
	ownerID, exists := r.dispatchOwners[req.DispatchID]
	if !exists {
		r.mu.Unlock()
		r.logger.Info("worker session provider session observation rejected", "attemptID", req.DispatchID, "outcome", "unknown_dispatch")
		return workersessions.ProviderSessionAssociationResult{}, workersessions.ErrProviderSessionAssociationAttemptMismatch
	}
	result, err := r.associateProviderSessionLocked(workersessions.ProviderSessionAssociationRequest{
		WorkerSessionID: ownerID,
		DispatchID:      req.DispatchID,
		Reference:       req.Reference.Clone(),
	})
	r.mu.Unlock()
	if err != nil {
		r.logger.Info("worker session provider session observation rejected", "sessionID", ownerID, "attemptID", req.DispatchID, "outcome", "rejected")
		return workersessions.ProviderSessionAssociationResult{}, err
	}
	r.logger.Info("worker session provider session observation", "sessionID", ownerID, "attemptID", req.DispatchID, "outcome", string(result.Outcome))
	return result, nil
}

func (r *registry) associateProviderSession(
	req workersessions.ProviderSessionAssociationRequest,
) (workersessions.ProviderSessionAssociationResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.associateProviderSessionLocked(req)
}

func (r *registry) associateProviderSessionLocked(
	req workersessions.ProviderSessionAssociationRequest,
) (workersessions.ProviderSessionAssociationResult, error) {
	session, exists := r.sessions[req.WorkerSessionID]
	if !exists {
		return workersessions.ProviderSessionAssociationResult{}, workersessions.ErrSessionNotFound
	}
	supervision := r.supervisions[req.WorkerSessionID]
	if supervision == nil || r.dispatchOwners[req.DispatchID] != req.WorkerSessionID {
		return workersessions.ProviderSessionAssociationResult{}, workersessions.ErrProviderSessionAssociationAttemptMismatch
	}

	association := workersessions.ProviderSessionAssociation{
		WorkerSessionID: req.WorkerSessionID,
		TurnID:          supervision.turnID,
		DispatchID:      supervision.dispatchID,
		AttemptID:       supervision.dispatchID,
		Reference:       req.Reference.Clone(),
	}
	if existing := session.ProviderSessionAssociation; existing != nil {
		if existing.Reference == association.Reference {
			return workersessions.ProviderSessionAssociationResult{
				Association: existing.Clone(),
				Outcome:     workersessions.ProviderSessionAssociationOutcomeDuplicate,
			}, nil
		}
		return workersessions.ProviderSessionAssociationResult{}, workersessions.ErrProviderSessionAssociationConflict
	}
	if session.Terminal() {
		return workersessions.ProviderSessionAssociationResult{}, workersessions.ErrProviderSessionAssociationNotAvailable
	}

	session.ProviderSessionAssociation = &association
	r.sessions[req.WorkerSessionID] = session
	return workersessions.ProviderSessionAssociationResult{
		Association: association.Clone(),
		Outcome:     workersessions.ProviderSessionAssociationOutcomeAccepted,
	}, nil
}

// replayObservationSubscription drains one retained Events snapshot. It never
// calls Subscribe: once the first Read captures the retained head, later reads
// stop at that position even if the topic advances while the drain is in
// progress. Completeness is derived from a terminal lifecycle record inside
// that captured range, not from the separately synchronized session state.
type replayObservationSubscription struct {
	reader             EventsRetainedReader
	topic              events.Topic
	limit              int
	snapshotHead       events.AggregateSequence
	next               events.Cursor
	pending            []events.Record
	sessionState       workersessions.State
	terminalReplay     bool
	terminalRecordSeen bool
	reason             string
	eventsEmitted      int
	summarySent        bool
	closed             bool
	mu                 sync.Mutex
}

func newReplayObservationSubscription(
	ctx context.Context,
	reader EventsRetainedReader,
	topic events.Topic,
	state workersessions.State,
	limit int,
) (*replayObservationSubscription, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if reader == nil {
		return nil, workersessions.ErrObservationSourceUnavailable
	}
	if limit <= 0 {
		limit = workersessions.DefaultObservationStreamLimit
	}
	replay := &replayObservationSubscription{
		reader:         reader,
		topic:          topic,
		limit:          limit,
		next:           events.Cursor{Topic: topic},
		sessionState:   state,
		terminalReplay: state.Terminal(),
		reason:         replayReason(state),
	}
	result, err := reader.Read(ctx, events.ReadRequest{Topic: topic, From: replay.next, Limit: limit})
	if err != nil {
		return nil, replayReadError(err)
	}
	if err := replay.acceptInitial(result); err != nil {
		return nil, err
	}
	return replay, nil
}

func (s *replayObservationSubscription) acceptInitial(result events.ReadResult) error {
	if err := result.Validate(); err != nil {
		return replayReadError(err)
	}
	if result.Outcome == events.ReadOutcomeGap {
		return workersessions.ErrObservationSourceGap
	}
	if result.Next.Topic != s.topic || result.Retained.Topic != s.topic {
		return workersessions.ErrObservationSourceUnavailable
	}
	s.snapshotHead = result.Retained.Head
	s.next = result.Next
	switch result.Outcome {
	case events.ReadOutcomeProgress:
		s.pending = cloneEventRecords(result.Records)
		s.noteTerminalRecord(result.Records)
	case events.ReadOutcomeAtHead:
		return nil
	default:
		return workersessions.ErrObservationSourceUnavailable
	}
	return nil
}

func (s *replayObservationSubscription) Next(ctx context.Context) workersessions.ObservationDelivery {
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		if err := ctx.Err(); err != nil {
			s.mu.Lock()
			s.closed = true
			s.mu.Unlock()
			return workersessions.ObservationDelivery{Kind: workersessions.ObservationDeliveryCanceled, Err: workersessions.ErrObservationCanceled}
		}

		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			return workersessions.ObservationDelivery{Kind: workersessions.ObservationDeliveryClosed}
		}
		if len(s.pending) > 0 {
			record := s.pending[0]
			s.pending = s.pending[1:]
			s.eventsEmitted++
			terminalReplay := s.terminalReplay
			s.mu.Unlock()
			return observationRecordDelivery(record, terminalReplay)
		}
		if s.next.Position >= s.snapshotHead {
			if !s.summarySent {
				s.summarySent = true
				summary := &workersessions.ReplaySummary{
					Complete:      s.terminalRecordSeen,
					Reason:        s.replaySummaryReason(),
					EventsEmitted: s.eventsEmitted,
				}
				s.mu.Unlock()
				return workersessions.ObservationDelivery{Kind: workersessions.ObservationDeliveryReplaySummary, Summary: summary}
			}
			s.closed = true
			s.mu.Unlock()
			return workersessions.ObservationDelivery{Kind: workersessions.ObservationDeliveryClosed}
		}
		reader := s.reader
		request := events.ReadRequest{Topic: s.topic, From: s.next, Limit: s.limit}
		s.mu.Unlock()

		result, err := reader.Read(ctx, request)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
				s.mu.Lock()
				s.closed = true
				s.mu.Unlock()
				return workersessions.ObservationDelivery{Kind: workersessions.ObservationDeliveryCanceled, Err: workersessions.ErrObservationCanceled}
			}
			s.mu.Lock()
			s.closed = true
			s.mu.Unlock()
			return workersessions.ObservationDelivery{Kind: workersessions.ObservationDeliverySourceFailure, Err: replayReadError(err)}
		}

		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			return workersessions.ObservationDelivery{Kind: workersessions.ObservationDeliveryClosed}
		}
		err = s.appendPage(result)
		s.mu.Unlock()
		if err != nil {
			s.mu.Lock()
			s.closed = true
			s.mu.Unlock()
			return workersessions.ObservationDelivery{Kind: workersessions.ObservationDeliverySourceFailure, Err: err}
		}
	}
}

func (s *replayObservationSubscription) appendPage(result events.ReadResult) error {
	if err := result.Validate(); err != nil {
		return replayReadError(err)
	}
	if result.Outcome == events.ReadOutcomeGap {
		return workersessions.ErrObservationSourceGap
	}
	if result.Next.Topic != s.topic || result.Retained.Topic != s.topic {
		return workersessions.ErrObservationSourceUnavailable
	}
	switch result.Outcome {
	case events.ReadOutcomeInvalidCursor:
		return workersessions.ErrObservationSourceUnavailable
	case events.ReadOutcomeAtHead:
		if result.Next.Position < s.snapshotHead {
			return workersessions.ErrObservationSourceUnavailable
		}
		s.next = result.Next
		return nil
	case events.ReadOutcomeProgress:
		if len(result.Records) == 0 || result.Records[0].ID.Position != s.next.Position+1 {
			return workersessions.ErrObservationSourceUnavailable
		}
		lastIncluded := s.next.Position
		for _, record := range result.Records {
			if record.ID.Position > s.snapshotHead {
				break
			}
			detached := record.Detached()
			s.pending = append(s.pending, detached)
			s.noteTerminalRecord([]events.Record{detached})
			lastIncluded = record.ID.Position
		}
		if lastIncluded == s.next.Position {
			return workersessions.ErrObservationSourceUnavailable
		}
		s.next = events.Cursor{Topic: s.topic, Position: lastIncluded}
		return nil
	default:
		return workersessions.ErrObservationSourceUnavailable
	}
}

func (s *replayObservationSubscription) noteTerminalRecord(records []events.Record) {
	if s.terminalRecordSeen {
		return
	}
	for _, record := range records {
		if record.SourceType == lifecycleSourceType && record.SourceSequence >= terminalSourceSequence {
			s.terminalRecordSeen = true
			return
		}
	}
}

func (s *replayObservationSubscription) replaySummaryReason() string {
	if s.terminalRecordSeen && !s.sessionState.Terminal() {
		return "session-terminal-record"
	}
	if !s.terminalRecordSeen && s.sessionState.Terminal() {
		return "session-terminal-record-missing"
	}
	return s.reason
}

func (s *replayObservationSubscription) Close() {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
}

func cloneEventRecords(records []events.Record) []events.Record {
	clone := make([]events.Record, len(records))
	for index, record := range records {
		clone[index] = record.Detached()
	}
	return clone
}

func observationRecordDelivery(record events.Record, terminalReplay bool) workersessions.ObservationDelivery {
	event := projectObservationEvent(record)
	if record.SourceType == lifecycleSourceType && record.SourceSequence >= terminalSourceSequence {
		kind := workersessions.ObservationDeliveryTerminal
		if terminalReplay {
			kind = workersessions.ObservationDeliveryTerminalReplay
		}
		return workersessions.ObservationDelivery{Kind: kind, Event: event}
	}
	return workersessions.ObservationDelivery{Kind: workersessions.ObservationDeliveryRecord, Event: event}
}

func replayReadError(err error) error {
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, workersessions.ErrObservationCanceled):
		return workersessions.ErrObservationCanceled
	case errors.Is(err, workersessions.ErrObservationSourceGap):
		return workersessions.ErrObservationSourceGap
	default:
		return workersessions.ErrObservationSourceUnavailable
	}
}

func replayReason(state workersessions.State) string {
	if !state.Terminal() {
		return "session-active"
	}
	return "session-" + strings.ToLower(string(state))
}
