package stream

import (
	"fmt"
	"strings"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responseevents"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responsestream"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responsestream/fragmentmap"
	responsestreamservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/response_stream"
	"go.uber.org/zap"
)

// Manager owns transient response-stream subscription, publication, cleanup, and
// checkpoint access for live Factory Sessions.
type Manager struct {
	sessions  SessionResolver
	observer  Observer
	registry  *responsestream.Registry
	responses responsestreamservice.Service
}

// NewManager constructs a stream manager with explicit host and registry dependencies.
func NewManager(host Host, registry *responsestream.Registry) *Manager {
	return NewManagerWithRegistry(host, registry)
}

// NewManagerWithRegistry constructs a stream manager using the injected
// response-stream owner.
func NewManagerWithRegistry(host Host, registry *responsestream.Registry) *Manager {
	return NewManagerWithDependencies(host, host, registry)
}

func NewManagerWithDependencies(sessions SessionResolver, observer Observer, registry *responsestream.Registry) *Manager {
	return NewManagerWithResponseService(sessions, observer, registry, nil)
}

// NewManagerWithResponseService routes publication, lifecycle, and diagnostics
// through the owner-private response-stream capability.
func NewManagerWithResponseService(sessions SessionResolver, observer Observer, registry *responsestream.Registry, responses responsestreamservice.Service) *Manager {
	if registry == nil {
		return nil
	}
	return &Manager{sessions: sessions, observer: observer, registry: registry, responses: responses}
}

// Subscribe opens one dispatch-scoped response-stream subscription.
func (m *Manager) Subscribe(
	sessionID string,
	dispatchID string,
	afterSequence int64,
) (*factorysessions.SessionResponseStreamSubscription, error) {
	if m == nil || m.sessions == nil {
		return nil, fmt.Errorf("session stream manager is required")
	}
	session, err := m.sessions.RequireSession(sessionID)
	if err != nil {
		return nil, err
	}
	streams := m.registry.Streams(responseStreamSessionKey(session))
	if streams == nil {
		return nil, responsestream.ErrSubscriptionClosed
	}
	return streams.Subscribe(dispatchID, afterSequence)
}

// DispatchIDs returns active dispatch stream identifiers for one live session.
func (m *Manager) DispatchIDs(sessionID string) ([]string, error) {
	if m == nil || m.sessions == nil {
		return nil, fmt.Errorf("session stream manager is required")
	}
	session, err := m.sessions.RequireSession(sessionID)
	if err != nil {
		return nil, err
	}
	streams := m.registry.Existing(responseStreamSessionKey(session))
	if streams == nil {
		return nil, nil
	}
	return streams.DispatchIDs(), nil
}

// ResponseStreams returns the canonical registry-owned stream set for a live
// session. Compatibility shells use this while their direct stream helpers are
// retired.
func (m *Manager) ResponseStreams(session *factorysessions.LiveSession) *factorysessions.SessionResponseStreamSet {
	if m == nil || m.registry == nil || session == nil {
		return nil
	}
	return m.registry.Streams(responseStreamSessionKey(session))
}

// CloseAll releases all response streams owned by one live session.
func (m *Manager) CloseAll(session *factorysessions.LiveSession) {
	if m == nil || session == nil {
		return
	}
	if m.responses != nil {
		m.responses.Close(session.ResponseEvents)
	} else {
		session.CloseResponseEvents()
	}
	m.registry.Close(responseStreamSessionKey(session))
}

// CloseDispatch releases one dispatch-scoped response stream.
func (m *Manager) CloseDispatch(session *factorysessions.LiveSession, dispatchID string) bool {
	if m == nil || session == nil {
		return false
	}
	return m.registry.CloseDispatch(responseStreamSessionKey(session), dispatchID)
}

// InferenceProgressPublisherFactory returns one publisher factory for worker
// provider progress routed into session-owned response streams.
func (m *Manager) InferenceProgressPublisherFactory(logger *zap.Logger) func(sessionID string) factorysessions.ProgressPublisher {
	if m == nil || m.sessions == nil {
		return nil
	}
	return func(sessionID string) factorysessions.ProgressPublisher {
		return m.inferenceProgressPublisher(sessionID, logger)
	}
}

// DispatchCompletionObserverFactory returns observers that close dispatch streams
// after terminal workstation completion.
func (m *Manager) DispatchCompletionObserverFactory() func(sessionID string) func(string) {
	if m == nil || m.sessions == nil {
		return nil
	}
	return func(sessionID string) func(string) {
		normalizedSessionID := normalizeSessionID(sessionID)
		return func(dispatchID string) {
			session := m.sessions.GetLiveSession(normalizedSessionID)
			if session == nil && normalizedSessionID == factorysessions.DefaultSessionID {
				session = m.sessions.GetLiveSession(factorysessions.DefaultSessionID)
			}
			m.CloseDispatch(session, dispatchID)
		}
	}
}

func (m *Manager) inferenceProgressPublisher(
	sessionID string,
	logger *zap.Logger,
) factorysessions.ProgressPublisher {
	if m == nil || m.sessions == nil {
		return nil
	}
	normalizedSessionID := normalizeSessionID(sessionID)
	return func(fragment factorysessions.ProgressFragment) {
		dispatchID := strings.TrimSpace(fragment.DispatchID)
		var session *factorysessions.LiveSession
		defer func() {
			if recovered := recover(); recovered != nil {
				m.observeDegraded(
					session,
					normalizedSessionID,
					dispatchID,
					"PUBLISH_PANIC",
					logger,
					fmt.Errorf("panic during internal provider progress publication: %v", recovered),
				)
			}
		}()
		session = m.sessions.GetLiveSession(normalizedSessionID)
		if session == nil && normalizedSessionID == factorysessions.DefaultSessionID {
			session = m.sessions.GetLiveSession(factorysessions.DefaultSessionID)
		}
		if fragment.CanonicalDraft != nil {
			draft, ok := fragment.CanonicalDraft.(responseevents.Draft)
			if !ok {
				m.observeDegraded(session, normalizedSessionID, dispatchID, "CANONICAL_EVENT_PUBLISH_FAILED", logger, fmt.Errorf("canonical response draft has type %T", fragment.CanonicalDraft))
				return
			}
			if err := m.publishCanonicalDraft(session, draft); err != nil {
				m.observeDegraded(session, normalizedSessionID, dispatchID, "CANONICAL_EVENT_PUBLISH_FAILED", logger, err)
			}
			return
		}
		streams := m.registry.Streams(responseStreamSessionKey(session))
		if streams == nil {
			m.observeDegraded(session, normalizedSessionID, dispatchID, "STREAM_UNAVAILABLE", logger, nil)
			return
		}
		stream := streams.Stream(dispatchID)
		if stream == nil {
			m.observeDegraded(session, normalizedSessionID, dispatchID, "STREAM_UNAVAILABLE", logger, nil)
			return
		}
		publisher := m.newPublisher(stream, func(summary responsestream.CompactionSummary) {
			if m.observer != nil {
				m.observer.ObserveResponseStreamCompaction(session, normalizedSessionID, dispatchID, summary)
			}
		})
		event := mapInferenceProgressFragment(fragment)
		stored := publisher.Publish(event)
		if err := m.publishCanonicalResponseEvents(session, stored, fragment.CanonicalEventAlreadyPublished); err != nil {
			m.observeDegraded(
				session,
				normalizedSessionID,
				dispatchID,
				"CANONICAL_EVENT_PUBLISH_FAILED",
				logger,
				err,
			)
		}
		if m.observer != nil {
			m.observer.ObserveResponseStreamPublished(session, normalizedSessionID, stored)
		}
	}
}

func (m *Manager) observeDegraded(session *factorysessions.LiveSession, sessionID, dispatchID, reason string, logger *zap.Logger, err error) {
	if m != nil && m.observer != nil {
		m.observer.ObserveResponseStreamDegraded(session, sessionID, dispatchID, reason, logger, err)
	}
}

func (m *Manager) publishCanonicalDraft(session *factorysessions.LiveSession, draft responseevents.Draft) error {
	if session == nil || session.ResponseEvents == nil {
		return fmt.Errorf("session response-event store is unavailable")
	}
	if err := responseevents.ValidateDraft(draft); err != nil {
		return fmt.Errorf("validate canonical response draft: %w", err)
	}
	_, err := m.publishResponseEvent(session.ResponseEvents, responseevents.FactoryResponseEvent{
		RunID: draft.RunID, Kind: draft.Kind, Phase: draft.Phase, Provenance: draft.Provenance,
		Payload: append([]byte(nil), draft.Payload...), DispatchID: draft.DispatchID, TurnID: draft.TurnID,
		ItemID: draft.ItemID, ParentItemID: draft.ParentItemID, ProviderSessionRef: draft.ProviderSessionRef,
	})
	if err != nil {
		return fmt.Errorf("publish canonical response draft: %w", err)
	}
	return nil
}

func (m *Manager) publishCanonicalResponseEvents(session *factorysessions.LiveSession, fragment responsestream.Event, skipCanonical bool) error {
	if session == nil || session.ResponseEvents == nil {
		return fmt.Errorf("session response-event store is unavailable")
	}
	if skipCanonical {
		return nil
	}
	events, err := fragmentmap.MapFragment(fragmentmap.Context{
		FactorySessionID: livesession.CanonicalID(session),
	}, fragment)
	if err != nil {
		return fmt.Errorf("map canonical response event: %w", err)
	}
	for _, event := range events {
		if _, err := m.publishResponseEvent(session.ResponseEvents, event); err != nil {
			return fmt.Errorf("publish canonical response event: %w", err)
		}
	}
	return nil
}

func (m *Manager) publishResponseEvent(store *factorysessions.SessionResponseEventStore, event responseevents.FactoryResponseEvent) (responseevents.FactoryResponseEvent, error) {
	if m != nil && m.responses != nil {
		return m.responses.Publish(store, event)
	}
	return store.Publish(event)
}

func (m *Manager) newPublisher(stream *responsestream.SessionResponseStream, observer responsestream.DiagnosticsObserver) interface {
	Publish(responsestream.Event) responsestream.Event
} {
	if m != nil && m.responses != nil {
		return m.responses.NewPublisher(stream, observer)
	}
	return responsestream.NewPublisher(stream, observer)
}

func normalizeSessionID(sessionID string) string {
	normalized := strings.TrimSpace(sessionID)
	if normalized == "" {
		return factorysessions.DefaultSessionID
	}
	return normalized
}

func responseStreamSessionKey(session *factorysessions.LiveSession) string {
	if session == nil {
		return ""
	}
	return livesession.CanonicalID(session)
}
