package http

import (
	"context"
	"io"
	"net/http"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
	"go.uber.org/zap"
)

func decodeOpenFactorySessionRequest(body io.Reader) (factoryapi.OpenFactorySessionRequest, error) {
	return decodeOpenFactorySessionBody(body)
}

func decodeStartFactorySessionRequest(body io.Reader, prepare RequestPreparation) (factorysessions.StartRequest, error) {
	req, err := decodeStrictJSON[factoryapi.FactorySessionExecutionRequest](body)
	if err != nil {
		return factorysessions.StartRequest{}, err
	}
	raw, err := factorysession.StartRequestFromAPI(req)
	if err != nil {
		return factorysessions.StartRequest{}, err
	}
	if prepare == nil {
		prepare = noopRequestPreparation{}
	}
	return prepare.PrepareStart(raw)
}

func decodeLifecycleControlRequest(body io.Reader, prepare RequestPreparation) (factorysessions.ControlRequest, error) {
	req, err := decodeOptionalLifecycleControlRequest(body)
	if err != nil {
		return factorysessions.ControlRequest{}, err
	}
	control, err := factorysession.ControlRequestFromAPI(req)
	if err != nil {
		return factorysessions.ControlRequest{}, err
	}
	if prepare == nil {
		prepare = noopRequestPreparation{}
	}
	return prepare.PrepareControl(control)
}

func decodeApproveFactorySessionRequest(body io.Reader, prepare RequestPreparation) (factorysessions.ApproveRequest, error) {
	req, err := decodeOptionalApproveRequest(body)
	if err != nil {
		return factorysessions.ApproveRequest{}, err
	}
	approve, err := factorysession.ApproveRequestFromAPI(req)
	if err != nil {
		return factorysessions.ApproveRequest{}, err
	}
	if prepare == nil {
		prepare = noopRequestPreparation{}
	}
	return prepare.PrepareApprove(approve)
}

func decodeRetryDispatchRequest(body io.Reader, prepare RequestPreparation) (factorysessions.RetryDispatchRequest, error) {
	req, err := decodeOptionalRetryDispatchRequest(body)
	if err != nil {
		return factorysessions.RetryDispatchRequest{}, err
	}
	retry, err := factorysession.RetryDispatchRequestFromAPI(req)
	if err != nil {
		return factorysessions.RetryDispatchRequest{}, err
	}
	if prepare == nil {
		prepare = noopRequestPreparation{}
	}
	return prepare.PrepareRetryDispatch(retry)
}

func decodeInterruptDispatchRequest(body io.Reader, prepare RequestPreparation) (factorysessions.InterruptDispatchRequest, error) {
	req, err := decodeOptionalInterruptDispatchRequest(body)
	if err != nil {
		return factorysessions.InterruptDispatchRequest{}, err
	}
	interrupt, err := factorysession.InterruptDispatchRequestFromAPI(req)
	if err != nil {
		return factorysessions.InterruptDispatchRequest{}, err
	}
	if prepare == nil {
		prepare = noopRequestPreparation{}
	}
	return prepare.PrepareInterruptDispatch(interrupt)
}

func (s *Server) finishRootLifecycleControl(
	w http.ResponseWriter,
	sessionID string,
	operation string,
	result factorysessions.LifecycleControlResult,
	err error,
) {
	if err != nil {
		if s.writeSessionsRootError(w, sessionID, err) {
			return
		}
		s.logger.Error("factory session lifecycle control failed",
			zap.Error(err),
			zap.String("session_id", sessionID),
			zap.String("operation", operation),
		)
		s.writeSessionsRootErrorOrInternal(w, sessionID, err, "factory session lifecycle control failed")
		return
	}
	s.writeLifecycleControlSuccess(w, factorysession.LifecycleControlResponseToAPI(result))
}

func (s *Server) invokeRootDurableLifecycleControl(
	w http.ResponseWriter,
	ctx context.Context,
	sessionID factoryapi.SessionID,
	operation string,
	control factorysessions.ControlRequest,
) {
	switch operation {
	case "pause":
		result, err := s.sessionsRoot.PauseDurableFactorySession(ctx, string(sessionID), control)
		s.finishRootLifecycleControl(w, string(sessionID), operation, result, err)
	case "resume":
		result, err := s.sessionsRoot.ResumeDurableFactorySession(ctx, string(sessionID), control)
		s.finishRootLifecycleControl(w, string(sessionID), operation, result, err)
	case "cancel":
		result, err := s.sessionsRoot.CancelDurableFactorySession(ctx, string(sessionID), control)
		s.finishRootLifecycleControl(w, string(sessionID), operation, result, err)
	case "terminate":
		result, err := s.sessionsRoot.TerminateDurableFactorySession(ctx, string(sessionID), control)
		s.finishRootLifecycleControl(w, string(sessionID), operation, result, err)
	default:
		s.writeError(w, http.StatusInternalServerError, "durable factory session lifecycle control failed", "INTERNAL_ERROR")
	}
}

func (s *Server) invokeRootLiveLifecycleControl(
	w http.ResponseWriter,
	ctx context.Context,
	sessionID factoryapi.SessionID,
	operation string,
	control factorysessions.ControlRequest,
) {
	switch operation {
	case "pause":
		result, err := s.sessionsRoot.PauseLiveFactorySession(ctx, string(sessionID), control)
		s.finishRootLifecycleControl(w, string(sessionID), operation, result, err)
	case "resume":
		result, err := s.sessionsRoot.ResumeLiveFactorySession(ctx, string(sessionID), control)
		s.finishRootLifecycleControl(w, string(sessionID), operation, result, err)
	default:
		s.writeError(w, http.StatusInternalServerError, "live factory session lifecycle control failed", "INTERNAL_ERROR")
	}
}
