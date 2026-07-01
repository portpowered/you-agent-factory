package stream

import (
	"github.com/portpowered/infinite-you/pkg/factorysessions"
	"github.com/portpowered/infinite-you/pkg/factorysessions/responsestream"
	"go.uber.org/zap"
)

// Host exposes composition-root seams for transient session response streams and
// JavaScript checkpoint storage without owning live runtime host internals.
type Host interface {
	RequireSession(sessionID string) (*factorysessions.LiveSession, error)
	GetLiveSession(sessionID string) *factorysessions.LiveSession
	ResponseStreams(session *factorysessions.LiveSession) *factorysessions.SessionResponseStreamSet
	NewResponseStream() *factorysessions.SessionResponseStream
	CloseResponseStreams(session *factorysessions.LiveSession)
	CloseResponseStreamDispatch(session *factorysessions.LiveSession, dispatchID string) bool
	JavaScriptCheckpointStore(session *factorysessions.LiveSession) *factorysessions.JavaScriptCheckpointStore
	ObserveResponseStreamPublished(session *factorysessions.LiveSession, sessionID string, event responsestream.Event)
	ObserveResponseStreamCompaction(session *factorysessions.LiveSession, sessionID string, dispatchID string, summary responsestream.CompactionSummary)
	ObserveResponseStreamDegraded(session *factorysessions.LiveSession, sessionID string, dispatchID string, reason string, fallbackLogger *zap.Logger, err error)
}
