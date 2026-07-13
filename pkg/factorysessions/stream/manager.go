package stream

import (
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/factorysessions"
	"github.com/portpowered/infinite-you/pkg/factorysessions/responseevents"
	"github.com/portpowered/infinite-you/pkg/factorysessions/responsestream"
	"github.com/portpowered/infinite-you/pkg/factorysessions/responsestream/compat"
	workerprovider "github.com/portpowered/infinite-you/pkg/workers/provider"
	"go.uber.org/zap"
)

// Manager owns transient response-stream subscription, publication, cleanup, and
// checkpoint access for live Factory Sessions.
type Manager struct {
	host Host
}

// NewManager constructs a stream manager with explicit host dependencies.
func NewManager(host Host) *Manager {
	return &Manager{host: host}
}

// Subscribe opens one dispatch-scoped response-stream subscription.
func (m *Manager) Subscribe(
	sessionID string,
	dispatchID string,
	afterSequence int64,
) (*factorysessions.SessionResponseStreamSubscription, error) {
	if m == nil || m.host == nil {
		return nil, fmt.Errorf("session stream manager is required")
	}
	session, err := m.host.RequireSession(sessionID)
	if err != nil {
		return nil, err
	}
	streams := m.host.ResponseStreams(session)
	if streams == nil {
		return nil, responsestream.ErrSubscriptionClosed
	}
	return streams.Subscribe(dispatchID, afterSequence)
}

// DispatchIDs returns active dispatch stream identifiers for one live session.
func (m *Manager) DispatchIDs(sessionID string) ([]string, error) {
	if m == nil || m.host == nil {
		return nil, fmt.Errorf("session stream manager is required")
	}
	session, err := m.host.RequireSession(sessionID)
	if err != nil {
		return nil, err
	}
	streams := m.host.ResponseStreams(session)
	if streams == nil {
		return nil, nil
	}
	return streams.DispatchIDs(), nil
}

// CloseAll releases all response streams owned by one live session.
func (m *Manager) CloseAll(session *factorysessions.LiveSession) {
	if m == nil || m.host == nil || session == nil {
		return
	}
	m.host.CloseResponseStreams(session)
}

// CloseDispatch releases one dispatch-scoped response stream.
func (m *Manager) CloseDispatch(session *factorysessions.LiveSession, dispatchID string) bool {
	if m == nil || m.host == nil || session == nil {
		return false
	}
	return m.host.CloseResponseStreamDispatch(session, dispatchID)
}

// JavaScriptCheckpointStore returns the session-owned JavaScript checkpoint store.
func (m *Manager) JavaScriptCheckpointStore(session *factorysessions.LiveSession) *factorysessions.JavaScriptCheckpointStore {
	if m == nil || m.host == nil || session == nil {
		return nil
	}
	return m.host.JavaScriptCheckpointStore(session)
}

// InferenceProgressPublisherFactory returns one publisher factory for worker
// provider progress routed into session-owned response streams.
func (m *Manager) InferenceProgressPublisherFactory(logger *zap.Logger) func(sessionID string) workerprovider.InferenceProgressPublisher {
	if m == nil || m.host == nil {
		return nil
	}
	return func(sessionID string) workerprovider.InferenceProgressPublisher {
		return m.inferenceProgressPublisher(sessionID, logger)
	}
}

// DispatchCompletionObserverFactory returns observers that close dispatch streams
// after terminal workstation completion.
func (m *Manager) DispatchCompletionObserverFactory() func(sessionID string) func(string) {
	if m == nil || m.host == nil {
		return nil
	}
	return func(sessionID string) func(string) {
		normalizedSessionID := normalizeSessionID(sessionID)
		return func(dispatchID string) {
			session := m.host.GetLiveSession(normalizedSessionID)
			if session == nil && normalizedSessionID == factorysessions.DefaultSessionID {
				session = m.host.GetLiveSession(factorysessions.DefaultSessionID)
			}
			m.CloseDispatch(session, dispatchID)
		}
	}
}

func (m *Manager) inferenceProgressPublisher(
	sessionID string,
	logger *zap.Logger,
) workerprovider.InferenceProgressPublisher {
	if m == nil || m.host == nil {
		return nil
	}
	normalizedSessionID := normalizeSessionID(sessionID)
	return func(fragment workerprovider.InferenceProgressFragment) {
		dispatchID := strings.TrimSpace(fragment.DispatchID)
		var session *factorysessions.LiveSession
		defer func() {
			if recovered := recover(); recovered != nil {
				m.host.ObserveResponseStreamDegraded(
					session,
					normalizedSessionID,
					dispatchID,
					"PUBLISH_PANIC",
					logger,
					fmt.Errorf("panic during internal provider progress publication: %v", recovered),
				)
			}
		}()
		session = m.host.GetLiveSession(normalizedSessionID)
		if session == nil && normalizedSessionID == factorysessions.DefaultSessionID {
			session = m.host.GetLiveSession(factorysessions.DefaultSessionID)
		}
		streams := m.host.ResponseStreams(session)
		if streams == nil {
			m.host.ObserveResponseStreamDegraded(session, normalizedSessionID, dispatchID, "STREAM_UNAVAILABLE", logger, nil)
			return
		}
		stream := streams.Stream(dispatchID)
		if stream == nil {
			m.host.ObserveResponseStreamDegraded(session, normalizedSessionID, dispatchID, "STREAM_UNAVAILABLE", logger, nil)
			return
		}
		publisher := responsestream.NewPublisher(stream, func(summary responsestream.CompactionSummary) {
			m.host.ObserveResponseStreamCompaction(session, normalizedSessionID, dispatchID, summary)
		})
		event := mapInferenceProgressFragment(fragment)
		stored := publisher.Publish(event)
		var canonicalDraft *responseevents.Draft
		if fragment.CanonicalDraft != nil {
			canonicalDraft, _ = fragment.CanonicalDraft.(*responseevents.Draft)
		}
		if err := publishCanonicalResponseEvents(session, stored, canonicalDraft); err != nil {
			m.host.ObserveResponseStreamDegraded(
				session,
				normalizedSessionID,
				dispatchID,
				"CANONICAL_EVENT_PUBLISH_FAILED",
				logger,
				err,
			)
		}
		m.host.ObserveResponseStreamPublished(session, normalizedSessionID, stored)
	}
}

func publishCanonicalResponseEvents(session *factorysessions.LiveSession, fragment responsestream.Event, draft *responseevents.Draft) error {
	if session == nil || session.ResponseEvents == nil {
		return fmt.Errorf("session response-event store is unavailable")
	}
	if draft != nil {
		event := responseevents.FactoryResponseEvent{
			RunID: draft.RunID, Kind: draft.Kind, Phase: draft.Phase, Provenance: draft.Provenance,
			Payload: draft.Payload, DispatchID: draft.DispatchID, TurnID: draft.TurnID,
			ItemID: draft.ItemID, ParentItemID: draft.ParentItemID, ProviderSessionRef: draft.ProviderSessionRef,
		}
		if _, err := session.ResponseEvents.Publish(event); err != nil {
			return fmt.Errorf("publish canonical adapter event: %w", err)
		}
		return nil
	}
	events, err := compat.MapFragment(compat.Context{
		FactorySessionID: factorysessions.CanonicalFactorySessionID(session),
	}, fragment)
	if err != nil {
		return fmt.Errorf("map canonical response event: %w", err)
	}
	for _, event := range events {
		if _, err := session.ResponseEvents.Publish(event); err != nil {
			return fmt.Errorf("publish canonical response event: %w", err)
		}
	}
	return nil
}

func normalizeSessionID(sessionID string) string {
	normalized := strings.TrimSpace(sessionID)
	if normalized == "" {
		return factorysessions.DefaultSessionID
	}
	return normalized
}
