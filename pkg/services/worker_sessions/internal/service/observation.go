package service

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/services/events"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

// observation is the registry-owned timing and Work correlation captured at
// the Worker Sessions lifecycle boundary. Provider identity is deliberately
// read from the Session association rather than duplicated here.
type observation struct {
	workIDs   []string
	turnID    string
	attemptID string
	startedAt time.Time
	endedAt   *time.Time
}

func (r *registry) ensureObservation(id, attemptID, turnID string, workIDs []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.observations[id]; exists {
		return
	}
	r.observations[id] = &observation{
		workIDs:   append([]string(nil), workIDs...),
		turnID:    turnID,
		attemptID: attemptID,
		startedAt: r.clock.Now(),
	}
}

func (r *registry) finishObservationLocked(id string, endedAt time.Time) {
	current := r.observations[id]
	if current == nil || current.endedAt != nil {
		return
	}
	endedAt = endedAt.UTC()
	current.endedAt = &endedAt
}

func (r *registry) ListObservations(ctx context.Context, req workersessions.ListObservationsRequest) (workersessions.ListObservationsResult, error) {
	if err := req.Validate(); err != nil {
		r.logger.Info("worker session observation list rejected", "outcome", "invalid")
		return workersessions.ListObservationsResult{}, err
	}
	if err := observationContextError(ctx); err != nil {
		return workersessions.ListObservationsResult{}, err
	}

	r.mu.RLock()
	ids := make([]string, 0)
	for id, current := range r.observations {
		if containsString(current.workIDs, req.WorkID) {
			ids = append(ids, id)
		}
	}
	r.mu.RUnlock()
	if len(ids) == 0 {
		r.logger.Info("worker session observation list", "workID", req.WorkID, "outcome", "not_found")
		return workersessions.ListObservationsResult{}, workersessions.ErrObservationWorkNotFound
	}

	observations := make([]workersessions.Observation, 0, len(ids))
	for _, id := range ids {
		projected, err := r.projectObservation(ctx, id)
		if err != nil {
			return workersessions.ListObservationsResult{}, err
		}
		observations = append(observations, projected)
	}
	sortObservationAttempts(observations)
	r.logger.Info("worker session observation list", "workID", req.WorkID, "outcome", "success", "result_count", len(observations))
	return workersessions.ListObservationsResult{Observations: observations}, nil
}

func (r *registry) GetObservation(ctx context.Context, req workersessions.GetObservationRequest) (workersessions.Observation, error) {
	if err := req.Validate(); err != nil {
		r.logger.Info("worker session observation get rejected", "outcome", "invalid")
		return workersessions.Observation{}, err
	}
	if err := observationContextError(ctx); err != nil {
		return workersessions.Observation{}, err
	}

	r.mu.RLock()
	ids := make([]string, 0, 1)
	for id, session := range r.sessions {
		if session.ProviderSessionAssociation != nil &&
			session.ProviderSessionAssociation.Reference == req.ProviderSession {
			ids = append(ids, id)
		}
	}
	r.mu.RUnlock()
	if len(ids) == 0 {
		r.logger.Info("worker session observation get", "outcome", "not_found")
		return workersessions.Observation{}, workersessions.ErrObservationSessionNotFound
	}
	// An exact Provider Session identity must be unique. If corrupted or
	// legacy state ever contains two matches, deterministic identity order
	// still makes the result stable without exposing both as one observation.
	sortStrings(ids)
	projected, err := r.projectObservation(ctx, ids[0])
	if err != nil {
		return workersessions.Observation{}, err
	}
	r.logger.Info("worker session observation get", "workerSessionID", projected.WorkerSessionID, "outcome", "success")
	return projected, nil
}

func (r *registry) projectObservation(ctx context.Context, id string) (workersessions.Observation, error) {
	if err := observationContextError(ctx); err != nil {
		return workersessions.Observation{}, err
	}

	session, metadata, ok := r.loadObservationState(id)
	if !ok {
		return workersessions.Observation{}, workersessions.ErrObservationSessionNotFound
	}

	projected := baseObservation(id, session, metadata)
	applyObservationTiming(&projected, session, metadata, r.clock)
	if session.Result != nil && session.Result.Cause != nil {
		failure := *session.Result.Cause
		projected.Failure = &failure
	}

	if !projected.ProviderSessionAvailable {
		return projected, nil
	}
	return r.enrichWithProviderSessionsProjection(ctx, projected)
}

// loadObservationState returns detached snapshots of the registered session
// and observation metadata for id. ok is false when either is missing.
func (r *registry) loadObservationState(id string) (workersessions.Session, *observation, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	session, exists := r.sessions[id]
	metadata := r.observations[id]
	if exists {
		session = cloneSession(session)
	}
	if metadata != nil {
		metadata = cloneObservation(metadata)
	}
	if !exists || metadata == nil {
		return workersessions.Session{}, nil, false
	}
	return session, metadata, true
}

// baseObservation projects the registry-owned identity, correlation, and
// lifecycle facts that never require the Provider Sessions root.
func baseObservation(id string, session workersessions.Session, metadata *observation) workersessions.Observation {
	projected := workersessions.Observation{
		WorkerSessionID: id,
		WorkIDs:         append([]string(nil), metadata.workIDs...),
		TurnID:          metadata.turnID,
		AttemptID:       metadata.attemptID,
		State:           session.State,
		DurationBasis:   workersessions.DurationBasisUnavailable,
		Transcript:      workersessions.TranscriptAvailabilityUnavailable,
	}
	if session.ProviderSessionAssociation != nil {
		projected.ProviderSession = session.ProviderSessionAssociation.Reference.Clone()
		projected.ProviderSessionAvailable = true
		projected.TurnID = session.ProviderSessionAssociation.TurnID
		projected.AttemptID = session.ProviderSessionAssociation.AttemptID
	}
	return projected
}

// applyObservationTiming fills projected's start/end/duration fields from
// metadata, using clock for an active (non-terminal) session's elapsed time.
func applyObservationTiming(projected *workersessions.Observation, session workersessions.Session, metadata *observation, clock platformclock.Source) {
	if metadata.startedAt.IsZero() {
		return
	}
	started := metadata.startedAt
	projected.StartedAt = &started
	switch {
	case metadata.endedAt != nil:
		ended := *metadata.endedAt
		projected.EndedAt = &ended
		projected.Duration = nonNegativeDuration(ended.Sub(started))
		projected.DurationBasis = workersessions.DurationBasisRecordedTimestamps
	case !session.Terminal():
		projected.Duration = nonNegativeDuration(clock.Now().Sub(started))
		projected.DurationBasis = workersessions.DurationBasisActiveClock
	}
}

func nonNegativeDuration(duration time.Duration) *time.Duration {
	if duration < 0 {
		duration = 0
	}
	return &duration
}

// enrichWithProviderSessionsProjection adds transcript availability, token
// usage, and parse diagnostics from the Provider Sessions root. It is only
// called when projected already carries an available Provider Session
// reference.
func (r *registry) enrichWithProviderSessionsProjection(ctx context.Context, projected workersessions.Observation) (workersessions.Observation, error) {
	if r.providerSessions == nil {
		return workersessions.Observation{}, workersessions.ErrObservationProjectionUnavailable
	}
	result, err := r.providerSessions.Project(providersessions.ProjectRequest{
		Session: projected.ProviderSession.Clone(),
		Context: ctx,
	})
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, providersessions.ErrOperationCanceled) {
			return workersessions.Observation{}, workersessions.ErrObservationCanceled
		}
		return workersessions.Observation{}, workersessions.ErrObservationProjectionUnavailable
	}
	projected.Transcript = workersessions.TranscriptAvailabilityAvailable
	projected.TokenUsage = observationTokenUsage(result.Detail.Parse.TokenUsage)
	projected.Parse = observationParseDiagnostics(result.Detail.Parse)
	return projected, nil
}

func observationContextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return workersessions.ErrObservationCanceled
	}
	return nil
}

func cloneObservation(value *observation) *observation {
	if value == nil {
		return nil
	}
	clone := *value
	clone.workIDs = append([]string(nil), value.workIDs...)
	if value.endedAt != nil {
		ended := *value.endedAt
		clone.endedAt = &ended
	}
	return &clone
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}

func sortObservationAttempts(observations []workersessions.Observation) {
	sort.SliceStable(observations, func(i, j int) bool {
		left, right := observations[i], observations[j]
		switch {
		case left.StartedAt != nil && right.StartedAt != nil && !left.StartedAt.Equal(*right.StartedAt):
			return left.StartedAt.Before(*right.StartedAt)
		case left.StartedAt != nil && right.StartedAt == nil:
			return true
		case left.StartedAt == nil && right.StartedAt != nil:
			return false
		case left.AttemptID != right.AttemptID:
			return left.AttemptID < right.AttemptID
		default:
			return left.WorkerSessionID < right.WorkerSessionID
		}
	})
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func observationTokenUsage(source *providersessions.TokenUsage) *workersessions.TokenUsage {
	if source == nil {
		return nil
	}
	return &workersessions.TokenUsage{
		CacheWriteTokens:      cloneInt(source.CacheWriteTokens),
		CachedInputTokens:     cloneInt(source.CachedInputTokens),
		InputTokens:           cloneInt(source.InputTokens),
		OutputTokens:          cloneInt(source.OutputTokens),
		ReasoningOutputTokens: cloneInt(source.ReasoningOutputTokens),
		TotalTokens:           cloneInt(source.TotalTokens),
	}
}

func observationParseDiagnostics(source providersessions.ParseSummary) workersessions.ParseDiagnostics {
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
			Message:    safeDiagnosticMessage(item.Message),
		})
	}
	return result
}

func safeDiagnosticMessage(message string) string {
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
		message = message[:256]
	}
	return message
}

// observationSubscription adapts the canonical Events subscription to the
// Worker Sessions outcome vocabulary and closes itself immediately after the
// lifecycle terminal record.
type observationSubscription struct {
	source events.Subscription

	mu             sync.Mutex
	closed         bool
	terminalReplay bool
	activeCancel   context.CancelFunc
}

func (s *observationSubscription) Next(ctx context.Context) workersessions.ObservationDelivery {
	if ctx == nil {
		ctx = context.Background()
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return workersessions.ObservationDelivery{Kind: workersessions.ObservationDeliveryClosed}
	}
	nextContext, cancel := context.WithCancel(ctx)
	s.activeCancel = cancel
	s.mu.Unlock()
	delivery := s.source.Next(nextContext)
	cancel()

	s.mu.Lock()
	s.activeCancel = nil
	closed := s.closed
	s.mu.Unlock()
	if closed && delivery.Kind != events.DeliveryCanceled {
		return workersessions.ObservationDelivery{Kind: workersessions.ObservationDeliveryClosed}
	}

	switch delivery.Kind {
	case events.DeliveryRecord:
		event := projectObservationEvent(delivery.Record)
		if delivery.Record.SourceType == lifecycleSourceType && delivery.Record.SourceSequence >= terminalSourceSequence {
			s.closeSource()
			s.mu.Lock()
			terminalReplay := s.terminalReplay
			s.mu.Unlock()
			kind := workersessions.ObservationDeliveryTerminal
			if terminalReplay {
				kind = workersessions.ObservationDeliveryTerminalReplay
			}
			return workersessions.ObservationDelivery{Kind: kind, Event: event}
		}
		return workersessions.ObservationDelivery{Kind: workersessions.ObservationDeliveryRecord, Event: event}
	case events.DeliveryCanceled:
		s.closeSource()
		return workersessions.ObservationDelivery{Kind: workersessions.ObservationDeliveryCanceled, Err: workersessions.ErrObservationCanceled}
	case events.DeliveryGap:
		s.closeSource()
		return workersessions.ObservationDelivery{Kind: workersessions.ObservationDeliverySourceFailure, Err: workersessions.ErrObservationSourceGap}
	case events.DeliveryBackpressure:
		s.closeSource()
		return workersessions.ObservationDelivery{Kind: workersessions.ObservationDeliverySourceFailure, Err: workersessions.ErrObservationSourceUnavailable}
	case events.DeliveryClosed:
		s.closeSource()
		return workersessions.ObservationDelivery{Kind: workersessions.ObservationDeliverySourceFailure, Err: workersessions.ErrObservationSourceClosed}
	default:
		s.closeSource()
		return workersessions.ObservationDelivery{Kind: workersessions.ObservationDeliverySourceFailure, Err: workersessions.ErrObservationSourceUnavailable}
	}
}

func (s *observationSubscription) Close() {
	s.closeSource()
}

func (s *observationSubscription) closeSource() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	activeCancel := s.activeCancel
	s.mu.Unlock()
	if activeCancel != nil {
		activeCancel()
		return
	}
	// Events has no separate Close method. A canceled Next is its explicit
	// unregister operation, and it is non-blocking because the context is
	// already canceled.
	if s.source != nil {
		cancelled, cancel := context.WithCancel(context.Background())
		cancel()
		s.source.Next(cancelled)
	}
}

func projectObservationEvent(record events.Record) workersessions.ObservationEvent {
	return workersessions.ObservationEvent{
		Position:       uint64(record.ID.Position),
		SourceType:     string(record.SourceType),
		SourceID:       string(record.SourceID),
		SourceSequence: uint64(record.SourceSequence),
		SourceEventID:  string(record.SourceEventID),
		SchemaID:       string(record.SchemaID),
		Payload:        append([]byte(nil), record.Payload...),
	}
}

func (r *registry) StreamObservations(ctx context.Context, req workersessions.StreamObservationsRequest) (workersessions.ObservationSubscription, error) {
	if err := req.Validate(); err != nil {
		r.logger.Info("worker session observation stream rejected", "outcome", "invalid")
		return nil, err
	}
	if err := observationContextError(ctx); err != nil {
		return nil, err
	}
	if r.eventReader == nil {
		r.logger.Info("worker session observation stream", "outcome", "source_unavailable")
		return nil, workersessions.ErrObservationSourceUnavailable
	}

	r.mu.RLock()
	workerSessionID := ""
	alreadyTerminal := false
	for id, session := range r.sessions {
		if session.ProviderSessionAssociation != nil &&
			session.ProviderSessionAssociation.Reference == req.ProviderSession {
			workerSessionID = id
			alreadyTerminal = session.Terminal()
			break
		}
	}
	r.mu.RUnlock()
	if workerSessionID == "" {
		r.logger.Info("worker session observation stream", "outcome", "not_found")
		return nil, workersessions.ErrObservationSessionNotFound
	}

	limit := req.Limit
	if limit == 0 {
		limit = workersessions.DefaultObservationStreamLimit
	}
	topic := workersessions.Topic(workerSessionID)
	subscription, err := r.eventReader.Subscribe(ctx, events.SubscribeRequest{
		Topic: topic,
		From:  events.Cursor{Topic: topic},
		Limit: limit,
	})
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil, workersessions.ErrObservationCanceled
		}
		return nil, workersessions.ErrObservationSourceUnavailable
	}
	return &observationSubscription{source: subscription, terminalReplay: alreadyTerminal}, nil
}
