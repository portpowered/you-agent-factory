package stream

import (
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responsestream"
	"go.uber.org/zap"
)

type SessionResolver interface {
	RequireSession(sessionID string) (*factorysessions.LiveSession, error)
	GetLiveSession(sessionID string) *factorysessions.LiveSession
}

type Observer interface {
	ObserveResponseStreamPublished(session *factorysessions.LiveSession, sessionID string, event responsestream.Event)
	ObserveResponseStreamCompaction(session *factorysessions.LiveSession, sessionID string, dispatchID string, summary responsestream.CompactionSummary)
	ObserveResponseStreamDegraded(session *factorysessions.LiveSession, sessionID string, dispatchID string, reason string, fallbackLogger *zap.Logger, err error)
}

// Host is the compatibility union accepted by legacy gateway constructors.
type Host interface {
	SessionResolver
	Observer
}
