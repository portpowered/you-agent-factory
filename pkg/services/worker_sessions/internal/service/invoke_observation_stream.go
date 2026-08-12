package service

import (
	"context"
	"errors"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

func (r *registry) StreamObservations(ctx context.Context, req workersessions.StreamObservationsRequest) (workersessions.ObservationSubscription, error) {
	if err := req.Validate(); err != nil {
		r.logger.Info("worker session observation stream rejected", "outcome", "invalid")
		return workersessions.ObservationSubscription{}, err
	}
	if err := observationContextError(ctx); err != nil {
		return workersessions.ObservationSubscription{}, err
	}
	workerSessionID, alreadyTerminal, workerSessionState, err := r.observationStreamSession(req.ProviderSession)
	if err != nil {
		return workersessions.ObservationSubscription{}, err
	}
	return r.streamObservationTopic(ctx, workerSessionID, workerSessionState, alreadyTerminal, req.Limit, req.ReplayOnly, req.Cursor)
}

func (r *registry) StreamObservationsByWorkerSessionID(ctx context.Context, req workersessions.StreamObservationsByWorkerSessionIDRequest) (workersessions.ObservationSubscription, error) {
	if err := req.Validate(); err != nil {
		r.logger.Info("worker session observation stream by Worker Session rejected", "outcome", "invalid")
		return workersessions.ObservationSubscription{}, err
	}
	req.WorkerSessionID = strings.TrimSpace(req.WorkerSessionID)
	if err := observationContextError(ctx); err != nil {
		return workersessions.ObservationSubscription{}, err
	}
	workerSessionID, alreadyTerminal, workerSessionState, err := r.observationStreamSessionByID(req.WorkerSessionID)
	if err != nil {
		return workersessions.ObservationSubscription{}, err
	}
	return r.streamObservationTopic(ctx, workerSessionID, workerSessionState, alreadyTerminal, req.Limit, req.ReplayOnly, req.Cursor)
}

func (r *registry) streamObservationTopic(
	ctx context.Context,
	workerSessionID string,
	workerSessionState workersessions.State,
	alreadyTerminal bool,
	limit int,
	replayOnly bool,
	cursor *workersessions.ObservationCursor,
) (workersessions.ObservationSubscription, error) {
	if err := validateObservationCursorWorkerSessionID(cursor, workerSessionID); err != nil {
		return workersessions.ObservationSubscription{}, err
	}
	if !replayOnly && r.eventReader == nil {
		r.logger.Info("worker session observation stream", "outcome", "source_unavailable")
		return workersessions.ObservationSubscription{}, workersessions.ErrObservationSourceUnavailable
	}
	if limit == 0 {
		limit = workersessions.DefaultObservationStreamLimit
	}
	topic := workersessions.Topic(workerSessionID)
	if replayOnly {
		return r.replayObservationStream(ctx, topic, workerSessionState, limit, cursor)
	}
	return r.liveObservationStream(ctx, topic, limit, alreadyTerminal, cursor)
}

func validateObservationCursorWorkerSessionID(
	cursor *workersessions.ObservationCursor,
	workerSessionID string,
) error {
	if cursor == nil || strings.TrimSpace(cursor.WorkerSessionID) == "" {
		return nil
	}
	if strings.TrimSpace(cursor.WorkerSessionID) != strings.TrimSpace(workerSessionID) {
		return workersessions.ErrObservationCursorForeign
	}
	return nil
}

func (r *registry) observationStreamSession(ref providers.SessionRef) (string, bool, workersessions.State, error) {
	r.mu.RLock()
	workerSessionID := ""
	alreadyTerminal := false
	workerSessionState := workersessions.StateReserved
	for id, session := range r.sessions {
		if session.ProviderSessionAssociation != nil &&
			session.ProviderSessionAssociation.Reference == ref {
			workerSessionID = id
			alreadyTerminal = session.Terminal()
			workerSessionState = session.State
			break
		}
	}
	r.mu.RUnlock()
	if workerSessionID == "" {
		r.logger.Info("worker session observation stream", "outcome", "not_found")
		return "", false, workersessions.StateReserved, workersessions.ErrObservationSessionNotFound
	}
	return workerSessionID, alreadyTerminal, workerSessionState, nil
}

func (r *registry) observationStreamSessionByID(id string) (string, bool, workersessions.State, error) {
	r.mu.RLock()
	session, exists := r.sessions[id]
	r.mu.RUnlock()
	if !exists {
		r.logger.Info("worker session observation stream by Worker Session", "workerSessionID", id, "outcome", "not_found")
		return "", false, workersessions.StateReserved, workersessions.ErrObservationSessionNotFound
	}
	return id, session.Terminal(), session.State, nil
}

func (r *registry) replayObservationStream(
	ctx context.Context,
	topic events.Topic,
	state workersessions.State,
	limit int,
	cursor *workersessions.ObservationCursor,
) (workersessions.ObservationSubscription, error) {
	replay, err := newReplayObservationSubscription(ctx, r.retainedReader, topic, state, limit, cursor)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, workersessions.ErrObservationCanceled) {
			return workersessions.ObservationSubscription{}, workersessions.ErrObservationCanceled
		}
		return workersessions.ObservationSubscription{}, err
	}
	wrapped := &observationSubscription{replay: replay, workerSessionID: observationWorkerSessionIDFromTopic(topic)}
	return workersessions.ObservationSubscription{NextFunc: wrapped.Next, CloseFunc: wrapped.Close}, nil
}

func (r *registry) liveObservationStream(
	ctx context.Context,
	topic events.Topic,
	limit int,
	terminalReplay bool,
	cursor *workersessions.ObservationCursor,
) (workersessions.ObservationSubscription, error) {
	from := events.Cursor{Topic: topic}
	if cursor != nil {
		from.Position = events.AggregateSequence(cursor.Position)
		if cursor.StreamGenerationID != "" {
			return workersessions.ObservationSubscription{}, workersessions.ErrObservationCursorUnavailable
		}
	}
	subscription, err := r.eventReader.Subscribe(ctx, events.SubscribeRequest{
		Topic: topic,
		From:  from,
		Limit: limit,
	})
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return workersessions.ObservationSubscription{}, workersessions.ErrObservationCanceled
		}
		if errors.Is(err, events.ErrUnresolvableCursor) {
			return workersessions.ObservationSubscription{}, workersessions.ErrObservationCursorFuture
		}
		return workersessions.ObservationSubscription{}, workersessions.ErrObservationSourceUnavailable
	}
	wrapped := &observationSubscription{
		source: subscription, workerSessionID: observationWorkerSessionIDFromTopic(topic), terminalReplay: terminalReplay,
		cursorProvided: cursor != nil,
	}
	return workersessions.ObservationSubscription{NextFunc: wrapped.Next, CloseFunc: wrapped.Close}, nil
}
