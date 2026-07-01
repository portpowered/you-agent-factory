package service

import (
	"fmt"

	"github.com/portpowered/infinite-you/pkg/factorysessions"
	workerprovider "github.com/portpowered/infinite-you/pkg/workers/provider"
	"go.uber.org/zap"
)

// SubscribeSessionResponseStream opens one dispatch-scoped response-stream subscription.
func (s *Service) SubscribeSessionResponseStream(
	sessionID string,
	dispatchID string,
	afterSequence int64,
) (*factorysessions.SessionResponseStreamSubscription, error) {
	if s == nil || s.streams == nil {
		return nil, errSessionGatewayRequired()
	}
	return s.streams.Subscribe(sessionID, dispatchID, afterSequence)
}

// SessionResponseStreamDispatchIDs returns active dispatch stream identifiers.
func (s *Service) SessionResponseStreamDispatchIDs(sessionID string) ([]string, error) {
	if s == nil || s.streams == nil {
		return nil, errSessionGatewayRequired()
	}
	return s.streams.DispatchIDs(sessionID)
}

// CloseSessionResponseStreams releases all response streams for one live session.
func (s *Service) CloseSessionResponseStreams(session *factorysessions.LiveSession) {
	if s == nil || s.streams == nil {
		return
	}
	s.streams.CloseAll(session)
}

// JavaScriptCheckpointStore returns the session-owned JavaScript checkpoint store.
func (s *Service) JavaScriptCheckpointStore(session *factorysessions.LiveSession) *factorysessions.JavaScriptCheckpointStore {
	if s == nil || s.streams == nil {
		return nil
	}
	return s.streams.JavaScriptCheckpointStore(session)
}

// InferenceProgressPublisherFactory returns worker-provider progress publishers.
func (s *Service) InferenceProgressPublisherFactory(logger *zap.Logger) func(sessionID string) workerprovider.InferenceProgressPublisher {
	if s == nil || s.streams == nil {
		return nil
	}
	return s.streams.InferenceProgressPublisherFactory(logger)
}

// DispatchCompletionObserverFactory returns dispatch-completion stream cleanup observers.
func (s *Service) DispatchCompletionObserverFactory() func(sessionID string) func(string) {
	if s == nil || s.streams == nil {
		return nil
	}
	return s.streams.DispatchCompletionObserverFactory()
}

func errSessionGatewayRequired() error {
	return fmt.Errorf("factory session gateway is required")
}
