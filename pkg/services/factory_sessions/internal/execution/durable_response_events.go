package factorysessionexecution

import (
	"context"
	"errors"
	"fmt"
	"strings"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responseevents"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responsestream/fragmentmap"
	responsestreamservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/response_stream"
	responsestream "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/stream"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// LiveChildInvocationFactory rebuilds one live child invocation executor with
// session-scoped provider progress publishing.
type LiveChildInvocationFactory func(workers.ProgressPublisher) (workers.InvocationExecutor, error)

func (s *JavaScriptRuntimeService) ensureSessionResponseEventsIfNeeded(state *runtimeSessionState) error {
	if s == nil || s.liveChildInvocation == nil {
		return nil
	}
	if state == nil {
		return errors.New("durable response-event store is unavailable")
	}
	return s.ensureSessionResponseEvents(state.session.SessionID, state)
}

func (s *JavaScriptRuntimeService) ensureSessionResponseEvents(sessionID string, state *runtimeSessionState) error {
	if s == nil || state == nil {
		return errors.New("durable response-event store is unavailable")
	}
	if state.responseEvents != nil {
		return nil
	}
	if s.generateResponseEventID == nil {
		return errors.New("durable response-event ID generator is required")
	}
	if s.responseStreams == nil {
		return errors.New("durable response-event stream service is required")
	}
	store, err := s.responseStreams.NewEventStore(sessionID, s.clock)
	if err != nil {
		return fmt.Errorf("create durable response-event store: %w", err)
	}
	state.responseEvents = store
	return nil
}

func (s *JavaScriptRuntimeService) sessionProgressPublisher(sessionID string, state *runtimeSessionState) workers.ProgressPublisher {
	return func(fragment workers.ProgressFragment) {
		if err := s.ensureSessionResponseEvents(sessionID, state); err != nil {
			return
		}
		if fragment.CanonicalDraft != nil {
			draft, ok := fragment.CanonicalDraft.(responseevents.Draft)
			if !ok {
				return
			}
			if err := responseevents.ValidateDraft(draft); err != nil {
				return
			}
			_, _ = state.responseEvents.Publish(responseevents.FactoryResponseEvent{
				RunID:              draft.RunID,
				Kind:               draft.Kind,
				Phase:              draft.Phase,
				Provenance:         draft.Provenance,
				Payload:            append([]byte(nil), draft.Payload...),
				DispatchID:         draft.DispatchID,
				TurnID:             draft.TurnID,
				ItemID:             draft.ItemID,
				ParentItemID:       draft.ParentItemID,
				ProviderSessionRef: draft.ProviderSessionRef,
			})
			return
		}
		mapped, err := fragmentmap.MapFragment(
			fragmentmap.Context{FactorySessionID: sessionID, RunID: fragment.DispatchID},
			responsestream.MapProgressFragment(fragment),
		)
		if err != nil {
			return
		}
		for _, event := range mapped {
			_, _ = state.responseEvents.Publish(event)
		}
	}
}

func (s *JavaScriptRuntimeService) liveChildExecutor(
	sessionID string,
	state *runtimeSessionState,
) workers.InvocationExecutor {
	if s.liveChildInvocation != nil {
		var publisher workers.ProgressPublisher
		if state != nil {
			publisher = s.sessionProgressPublisher(sessionID, state)
		}
		executor, err := s.liveChildInvocation(publisher)
		if err == nil && executor != nil {
			return executor
		}
	}
	return s.providerExecutor
}

// SubscribeResponseEvents opens one durable-session response-event cursor.
func (s *JavaScriptRuntimeService) SubscribeResponseEvents(
	ctx context.Context,
	sessionID string,
	request factorysessions.ResponseEventSubscriptionRequest,
) (*factorysessions.ResponseEventCursor, error) {
	if s == nil || s.responseStreams == nil {
		return nil, factorysessions.ErrRuntimeNotAvailable
	}
	state, err := s.snapshotSessionState(strings.TrimSpace(sessionID))
	if err != nil {
		return nil, err
	}
	if state.responseEvents == nil {
		return nil, factorysessions.ErrSessionNotFound
	}
	kinds := make([]responseevents.Kind, 0, len(request.Kinds))
	for _, kind := range request.Kinds {
		kinds = append(kinds, responseevents.Kind(kind))
	}
	cursor, err := s.responseStreams.Subscribe(ctx, state.responseEvents, responsestreamservice.SubscriptionRequest{
		AfterSequence: request.AfterSequence,
		DispatchID:    request.DispatchID,
		Kinds:         kinds,
	})
	if err != nil {
		switch {
		case errors.Is(err, responsestreamservice.ErrInvalidCursor):
			return nil, factorysessions.ErrInvalidResponseEventCursor
		case errors.Is(err, responsestreamservice.ErrInvalidFilter):
			return nil, factorysessions.ErrInvalidResponseEventFilter
		default:
			return nil, err
		}
	}
	return cursor, nil
}

func completeSessionResponseEvents(state *runtimeSessionState) {
	if state == nil || state.responseEvents == nil {
		return
	}
	state.responseEvents.Complete()
}
