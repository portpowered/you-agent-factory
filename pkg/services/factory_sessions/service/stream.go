package service

import (
	"fmt"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
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

// ResponseStreams returns the registry-owned stream set for one live session.
func (s *Service) ResponseStreams(session *factorysessions.LiveSession) *factorysessions.SessionResponseStreamSet {
	if s == nil || s.streams == nil {
		return nil
	}
	return s.streams.ResponseStreams(session)
}

// CloseSessionResponseStreams releases all response streams for one live session.
func (s *Service) CloseSessionResponseStreams(session *factorysessions.LiveSession) {
	if s == nil || s.streams == nil {
		return
	}
	s.streams.CloseAll(session)
}

// JavaScriptCheckpointStore returns the session-owned JavaScript checkpoint store.
func (s *Service) JavaScriptCheckpointStore(session *factorysessions.LiveSession) factoryruntime.JavaScriptCheckpointStore {
	if s == nil || s.host == nil {
		return nil
	}
	return s.host.JavaScriptCheckpointStore(session)
}

// InferenceProgressPublisherFactory returns worker-provider progress publishers.
func (s *Service) InferenceProgressPublisherFactory(logger *zap.Logger) func(sessionID string) factorysessions.ProgressPublisher {
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
