package factorysessionexecution

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responseevents"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responsestream/fragmentmap"
	responsestreamservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/response_stream"
	responsestream "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/stream"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func (s *JavaScriptRuntimeService) ensureSessionResponseEventsIfNeeded(state *runtimeSessionState) error {
	// Only a runtime-backed session publishes provider progress through this
	// store: its children are Workers, invoked through the narrow Execute
	// capability bound after the Runtime has opened. A fake session, a replay,
	// or the standalone `you run script.js` composition has no such capability
	// and needs no store.
	if s == nil || !s.workerExecutionBound() {
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
	s.mu.Lock()
	defer s.mu.Unlock()
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
			_, _ = s.publishSessionResponseEvent(state, responseevents.FactoryResponseEvent{
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
			_, _ = s.publishSessionResponseEvent(state, event)
		}
	}
}

// publishSessionResponseEvent publishes event into state's durable
// response-event store, routing through the owner-private response-stream
// service when available so the accepted record also mirrors into the
// injected Events root, matching the live-session Manager's publish path.
func (s *JavaScriptRuntimeService) publishSessionResponseEvent(
	state *runtimeSessionState,
	event responseevents.FactoryResponseEvent,
) (responseevents.FactoryResponseEvent, error) {
	if s != nil && s.responseStreams != nil {
		return s.responseStreams.Publish(state.responseEvents, event)
	}
	return state.responseEvents.Publish(event)
}

// SubscribeResponseEvents opens one durable-session response-event cursor.
func (s *JavaScriptRuntimeService) SubscribeResponseEvents(
	ctx context.Context,
	sessionID string,
	request factorysessions.ResponseEventSubscriptionRequest,
) (*factorysessions.ResponseEventCursor, error) {
	return s.subscribeResponseEvents(ctx, sessionID, request)
}

func (s *JavaScriptRuntimeService) SubscribeResponsesCanonical(
	ctx context.Context,
	request factorysessions.ResponseEventSubscriptionRequest,
) (*factorysessions.ResponseEventCursor, error) {
	return s.subscribeResponseEvents(ctx, request.SessionID, request)
}

func (s *JavaScriptRuntimeService) subscribeResponseEvents(
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

// PublishWorkerProgress routes one Worker's progress fragment to the durable
// session that owns that Worker.
//
// A JavaScript child is a Worker, so its output reaches this service through
// the request-scoped Workers progress publisher, addressed by dispatch.
// Workers knows the dispatch and nothing about durable sessions, which is why
// the session that started the Worker registers the mapping before it invokes
// and drops it afterwards. A fragment for a dispatch this service does not own
// is not its business and is ignored.
func (s *JavaScriptRuntimeService) PublishWorkerProgress(fragment workers.ProgressFragment) {
	if s == nil {
		return
	}
	sessionID := s.workerProgressSession(fragment.DispatchID)
	if sessionID == "" {
		return
	}
	state := s.liveSessionState(sessionID)
	if state == nil {
		return
	}
	s.sessionProgressPublisher(sessionID, state)(fragment)
}

// observeWorkerDispatch claims workerDispatchID for sessionID and returns the
// release its caller must run once the Worker is terminal. Releasing is what
// keeps the index the size of the Workers actually running rather than of
// every Worker the process has ever run.
func (s *JavaScriptRuntimeService) observeWorkerDispatch(workerDispatchID, sessionID string) func() {
	if s == nil || strings.TrimSpace(workerDispatchID) == "" {
		return func() {}
	}
	s.workerSessionsMu.Lock()
	if s.workerSessions == nil {
		s.workerSessions = make(map[string]string)
	}
	s.workerSessions[workerDispatchID] = sessionID
	s.workerSessionsMu.Unlock()
	return sync.OnceFunc(func() {
		s.workerSessionsMu.Lock()
		delete(s.workerSessions, workerDispatchID)
		s.workerSessionsMu.Unlock()
	})
}

func (s *JavaScriptRuntimeService) workerProgressSession(workerDispatchID string) string {
	s.workerSessionsMu.RLock()
	defer s.workerSessionsMu.RUnlock()
	return s.workerSessions[workerDispatchID]
}

// liveSessionState returns the running session's own state rather than a
// snapshot, because publishing provisions the response-event store on first
// use and a copy would provision one nobody reads.
func (s *JavaScriptRuntimeService) liveSessionState(sessionID string) *runtimeSessionState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sessions[sessionID]
}
