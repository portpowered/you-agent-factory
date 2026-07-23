package stream

import (
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responsestream"
	"go.uber.org/zap"
)

type SessionResolver interface {
	RequireSession(sessionID string) (*livesession.LiveSession, error)
	GetLiveSession(sessionID string) *livesession.LiveSession
}

type Observer interface {
	ObserveResponseStreamPublished(session *livesession.LiveSession, sessionID string, event responsestream.Event)
	ObserveResponseStreamCompaction(session *livesession.LiveSession, sessionID string, dispatchID string, summary responsestream.CompactionSummary)
	ObserveResponseStreamDegraded(session *livesession.LiveSession, sessionID string, dispatchID string, reason string, fallbackLogger *zap.Logger, err error)
}

// Host is the compatibility union accepted by legacy gateway constructors.
type Host interface {
	SessionResolver
	Observer
}
