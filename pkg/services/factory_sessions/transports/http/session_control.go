package http

import (
	"context"
	"io"
	"net/http"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	httpcompat "github.com/portpowered/infinite-you/pkg/transports/http/compat"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
	"go.uber.org/zap"
)

func decodeStartFactorySessionRequestWithDiagnostics(
	body io.Reader,
	prepare RequestPreparation,
) (factorysessions.StartRequest, httpcompat.Diagnostics, error) {
	decoded, err := decodeJSONWithDiagnostics[factoryapi.FactorySessionExecutionRequest](body)
	if err != nil {
		return factorysessions.StartRequest{}, decoded.Diagnostics, err
	}
	raw, err := factorysession.StartRequestFromAPI(decoded.Value)
	if err != nil {
		return factorysessions.StartRequest{}, decoded.Diagnostics, err
	}
	if prepare == nil {
		prepare = noopRequestPreparation{}
	}
	prepared, err := prepare.PrepareStart(raw)
	return prepared, decoded.Diagnostics, err
}

func decodeLifecycleControlRequestWithDiagnostics(
	body io.Reader,
	prepare RequestPreparation,
) (factorysessions.ControlRequest, httpcompat.Diagnostics, error) {
	decoded, err := decodeOptionalLifecycleControlRequestWithDiagnostics(body)
	if err != nil {
		return factorysessions.ControlRequest{}, decoded.Diagnostics, err
	}
	control, err := factorysession.ControlRequestFromAPI(decoded.Value)
	if err != nil {
		return factorysessions.ControlRequest{}, decoded.Diagnostics, err
	}
	if prepare == nil {
		prepare = noopRequestPreparation{}
	}
	prepared, err := prepare.PrepareControl(control)
	return prepared, decoded.Diagnostics, err
}

func decodeApproveFactorySessionRequestWithDiagnostics(
	body io.Reader,
	prepare RequestPreparation,
) (factorysessions.ApproveRequest, httpcompat.Diagnostics, error) {
	decoded, err := decodeOptionalApproveRequestWithDiagnostics(body)
	if err != nil {
		return factorysessions.ApproveRequest{}, decoded.Diagnostics, err
	}
	approve, err := factorysession.ApproveRequestFromAPI(decoded.Value)
	if err != nil {
		return factorysessions.ApproveRequest{}, decoded.Diagnostics, err
	}
	if prepare == nil {
		prepare = noopRequestPreparation{}
	}
	prepared, err := prepare.PrepareApprove(approve)
	return prepared, decoded.Diagnostics, err
}

func decodeRetryDispatchRequestWithDiagnostics(
	body io.Reader,
	prepare RequestPreparation,
) (factorysessions.RetryDispatchRequest, httpcompat.Diagnostics, error) {
	decoded, err := decodeOptionalRetryDispatchRequestWithDiagnostics(body)
	if err != nil {
		return factorysessions.RetryDispatchRequest{}, decoded.Diagnostics, err
	}
	retry, err := factorysession.RetryDispatchRequestFromAPI(decoded.Value)
	if err != nil {
		return factorysessions.RetryDispatchRequest{}, decoded.Diagnostics, err
	}
	if prepare == nil {
		prepare = noopRequestPreparation{}
	}
	prepared, err := prepare.PrepareRetryDispatch(retry)
	return prepared, decoded.Diagnostics, err
}

func decodeInterruptDispatchRequestWithDiagnostics(
	body io.Reader,
	prepare RequestPreparation,
) (factorysessions.InterruptDispatchRequest, httpcompat.Diagnostics, error) {
	decoded, err := decodeOptionalInterruptDispatchRequestWithDiagnostics(body)
	if err != nil {
		return factorysessions.InterruptDispatchRequest{}, decoded.Diagnostics, err
	}
	interrupt, err := factorysession.InterruptDispatchRequestFromAPI(decoded.Value)
	if err != nil {
		return factorysessions.InterruptDispatchRequest{}, decoded.Diagnostics, err
	}
	if prepare == nil {
		prepare = noopRequestPreparation{}
	}
	prepared, err := prepare.PrepareInterruptDispatch(interrupt)
	return prepared, decoded.Diagnostics, err
}

func (s *Server) finishRootLifecycleControl(
	w http.ResponseWriter,
	sessionID string,
	operation string,
	result factorysessions.LifecycleControlResult,
	paths []string,
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
	s.writeLifecycleControlSuccessWithDiagnostics(w, factorysession.LifecycleControlResponseToAPI(result), paths)
}

func (s *Server) invokeRootLiveLifecycleControl(
	w http.ResponseWriter,
	ctx context.Context,
	sessionID factoryapi.SessionID,
	operation string,
	control factorysessions.ControlRequest,
	paths []string,
) {
	switch operation {
	case "pause":
		result, err := s.liveControl.PauseLiveFactorySession(ctx, string(sessionID), control)
		s.finishRootLifecycleControl(w, string(sessionID), operation, result, paths, err)
	case "resume":
		result, err := s.liveControl.ResumeLiveFactorySession(ctx, string(sessionID), control)
		s.finishRootLifecycleControl(w, string(sessionID), operation, result, paths, err)
	case "cancel", "terminate":
		lifecycle, ok := s.liveControl.(factorysessions.LiveLifecycleControlService)
		if !ok {
			s.writeError(w, http.StatusInternalServerError, "live factory session lifecycle control is unavailable", "INTERNAL_ERROR")
			return
		}
		var result factorysessions.LifecycleControlResult
		var err error
		if operation == "cancel" {
			result, err = lifecycle.CancelLiveFactorySession(ctx, string(sessionID), control)
		} else {
			result, err = lifecycle.TerminateLiveFactorySession(ctx, string(sessionID), control)
		}
		s.finishRootLifecycleControl(w, string(sessionID), operation, result, paths, err)
	default:
		s.writeError(w, http.StatusInternalServerError, "live factory session lifecycle control failed", "INTERNAL_ERROR")
	}
}
