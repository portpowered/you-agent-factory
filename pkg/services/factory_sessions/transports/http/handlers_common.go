package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"

	factorysessionexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions"

	httpcompat "github.com/portpowered/infinite-you/pkg/transports/http/compat"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/optional"

	"go.uber.org/zap"
)

const factorySessionsHTTPBoundary = "factory_sessions.http"

func (s *responseWriter) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		s.logger.Error("encode response failed", zap.Error(err))
	}
}

func (s *responseWriter) writeError(w http.ResponseWriter, status int, message, code string) {
	s.writeErrorWithTargets(w, status, message, code, nil)
}

func (s *responseWriter) writeErrorWithTargets(w http.ResponseWriter, status int, message, code string, targets []factoryapi.FactoryValidationTarget) {
	var targetPtr *[]factoryapi.FactoryValidationTarget
	if len(targets) > 0 {
		targetPtr = &targets
	}
	s.writeJSON(w, status, factoryapi.ErrorResponse{
		Message: message,
		Family:  errorFamilyForStatus(status),
		Code:    factoryapi.ErrorResponseCode(code),
		Targets: targetPtr,
	})
}

func errorFamilyForStatus(status int) factoryapi.ErrorFamily {
	switch status {
	case http.StatusBadRequest:
		return factoryapi.ErrorFamilyBadRequest
	case http.StatusConflict:
		return factoryapi.ErrorFamilyConflict
	case http.StatusNotFound:
		return factoryapi.ErrorFamilyNotFound
	case http.StatusGone:
		return factoryapi.ErrorFamilyGone
	case http.StatusMethodNotAllowed:
		return factoryapi.ErrorFamilyBadRequest
	case http.StatusUnsupportedMediaType:
		return factoryapi.ErrorFamilyBadRequest
	default:
		return factoryapi.ErrorFamilyInternalServerError
	}
}

func requestFieldValidationMessage(err error) (string, bool) {
	var validationErr httpcompat.RequestFieldValidationError
	if errors.As(err, &validationErr) {
		return validationErr.Message, true
	}
	return "", false
}

// RequestFieldValidationMessage returns the public message carried by a
// request-field validation failure.
func RequestFieldValidationMessage(err error) (string, bool) {
	return requestFieldValidationMessage(err)
}

func requestAcceptsJSONContentType(contentTypeHeader string) bool {
	contentTypeHeader = strings.TrimSpace(contentTypeHeader)
	if contentTypeHeader == "" {
		return true
	}
	mediaType, _, err := mime.ParseMediaType(contentTypeHeader)
	if err != nil {
		return false
	}
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))
	return mediaType == "application/json" || strings.HasSuffix(mediaType, "+json")
}

func (s *responseWriter) writeUnsupportedMediaTypeError(w http.ResponseWriter) {
	s.writeError(w, http.StatusUnsupportedMediaType, "unsupported media type", "UNSUPPORTED_MEDIA_TYPE")
}

func decodeStrictJSON[T any](body io.Reader) (T, error) {
	result, err := decodeJSONWithDiagnostics[T](body)
	return result.Value, err
}

func decodeJSONWithDiagnostics[T any](body io.Reader) (httpcompat.DecodeResult[T], error) {
	return httpcompat.Decode[T](body)
}

// DecodeStrictJSON decodes exactly one JSON object; compatibility-aware
// handlers also retain diagnostics for ignored fields.
func DecodeStrictJSON[T any](body io.Reader) (T, error) {
	return decodeStrictJSON[T](body)
}

func (s *responseWriter) writeCompatibilityWarning(w http.ResponseWriter, operation string, paths []string) {
	httpcompat.ApplyWarning(w, s.logger, factorySessionsHTTPBoundary, operation, paths)
}

func stringValue(value *string) string {
	return optional.StringValue(value)
}

func (s *LifecycleHandler) writeLifecycleControlSuccessWithDiagnostics(
	w http.ResponseWriter,
	response factoryapi.FactorySessionLifecycleControlResponse,
	paths []string,
) {
	s.writeCompatibilityWarning(w, "factory_session_lifecycle_control", paths)
	s.writeJSON(
		w,
		factorysession.LifecycleControlSuccessStatus(factorysession.LifecycleControlResultFromAPI(response)),
		response,
	)
}

func (s *LifecycleHandler) handleDurableLifecycleControl(
	w http.ResponseWriter,
	r *http.Request,
	sessionID factoryapi.SessionID,
	operation string,
) {
	if !isDurableExecutionSessionID(string(sessionID)) {
		s.writeError(w, http.StatusNotImplemented, "durable factory session "+operation+" is not implemented", "INTERNAL_ERROR")
		return
	}

	control, diagnostics, err := decodeLifecycleControlRequestWithDiagnostics(r.Body, s.sessionRequests)
	if err != nil {
		if message, ok := requestFieldValidationMessage(err); ok {
			s.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
			return
		}
		s.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}
	if s.sessionsRoot == nil {
		s.writeError(w, http.StatusServiceUnavailable, "factory session service is unavailable", "SERVICE_UNAVAILABLE")
		return
	}
	result, err := s.sessionsRoot.Control(r.Context(), factorysessionexecution.SessionControlRequest{
		SessionID: string(sessionID), Mode: factorysessionexecution.SessionOperationModeDurable,
		Operation:   factorysessionexecution.SessionControlOperation(strings.ToUpper(operation)),
		Correlation: factorysessionexecution.SessionOperationCorrelation{RequestID: control.RequestID}, Control: control,
	})
	s.finishRootLifecycleControl(w, string(sessionID), operation, canonicalLifecycleResult(result), diagnostics.Paths(), err)
}

// usesDurableLifecycleControl selects the durable control owner for a durable
// identity that is not also registered as a live Factory Session. A live
// session remains authoritative when an injected or restored live identity
// happens to use the durable prefix; otherwise its stop request would be
// applied to a separate durable owner and leave the live runtime active.
func (s *LifecycleHandler) usesDurableLifecycleControl(ctx context.Context, sessionID string) bool {
	if !isDurableExecutionSessionID(sessionID) {
		return false
	}
	if s.liveControl != nil {
		if _, err := s.liveControl.ReadSessionDetail(ctx, sessionID); err == nil {
			return false
		}
	}
	return true
}

func (s *LifecycleHandler) handleLiveLifecycleControl(
	w http.ResponseWriter,
	r *http.Request,
	sessionID factoryapi.SessionID,
	operation string,
) {
	if s.liveControl == nil {
		s.writeError(w, http.StatusNotImplemented, "live factory session "+operation+" is not implemented", "INTERNAL_ERROR")
		return
	}

	control, diagnostics, err := decodeLifecycleControlRequestWithDiagnostics(r.Body, s.sessionRequests)
	if err != nil {
		if message, ok := requestFieldValidationMessage(err); ok {
			s.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
			return
		}
		s.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}

	s.invokeRootLiveLifecycleControl(w, r.Context(), sessionID, operation, control, diagnostics.Paths())
}

func decodeOptionalLifecycleControlRequestWithDiagnostics(body io.Reader) (httpcompat.DecodeResult[factoryapi.FactorySessionLifecycleControlRequest], error) {
	return decodeOptionalJSONWithDiagnostics(body, func() factoryapi.FactorySessionLifecycleControlRequest {
		return factoryapi.FactorySessionLifecycleControlRequest{}
	})
}

func decodeOptionalApproveRequestWithDiagnostics(body io.Reader) (httpcompat.DecodeResult[factoryapi.FactorySessionApproveRequest], error) {
	return decodeOptionalJSONWithDiagnostics(body, func() factoryapi.FactorySessionApproveRequest {
		return factoryapi.FactorySessionApproveRequest{}
	})
}

func decodeOptionalRetryDispatchRequestWithDiagnostics(body io.Reader) (httpcompat.DecodeResult[factoryapi.FactorySessionRetryDispatchRequest], error) {
	return decodeOptionalJSONWithDiagnostics(body, func() factoryapi.FactorySessionRetryDispatchRequest {
		return factoryapi.FactorySessionRetryDispatchRequest{}
	})
}

func decodeOptionalInterruptDispatchRequestWithDiagnostics(body io.Reader) (httpcompat.DecodeResult[factoryapi.FactorySessionInterruptDispatchRequest], error) {
	return decodeOptionalJSONWithDiagnostics(body, func() factoryapi.FactorySessionInterruptDispatchRequest {
		return factoryapi.FactorySessionInterruptDispatchRequest{}
	})
}

func decodeOptionalJSONWithDiagnostics[T any](body io.Reader, zero func() T) (httpcompat.DecodeResult[T], error) {
	return httpcompat.DecodeOptional(body, zero)
}

func (s *LifecycleHandler) handleDurableApproveControl(
	w http.ResponseWriter,
	r *http.Request,
	sessionID factoryapi.SessionID,
) {
	if !isDurableExecutionSessionID(string(sessionID)) {
		s.writeError(w, http.StatusNotImplemented, "durable factory session approve is not implemented", "INTERNAL_ERROR")
		return
	}

	approve, diagnostics, err := decodeApproveFactorySessionRequestWithDiagnostics(r.Body, s.sessionRequests)
	if err != nil {
		if message, ok := requestFieldValidationMessage(err); ok {
			s.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
			return
		}
		s.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}

	if s.sessionsRoot == nil {
		s.writeError(w, http.StatusServiceUnavailable, "factory session service is unavailable", "SERVICE_UNAVAILABLE")
		return
	}
	result, err := s.sessionsRoot.Control(r.Context(), factorysessionexecution.SessionControlRequest{
		SessionID: string(sessionID), Mode: factorysessionexecution.SessionOperationModeDurable,
		Operation:   factorysessionexecution.SessionControlApprove,
		Correlation: factorysessionexecution.SessionOperationCorrelation{RequestID: approve.RequestID}, Approve: &approve,
	})
	s.finishRootLifecycleControl(w, string(sessionID), "approve", canonicalLifecycleResult(result), diagnostics.Paths(), err)
}

func (s *LifecycleHandler) handleDurableRetryDispatchControl(
	w http.ResponseWriter,
	r *http.Request,
	sessionID factoryapi.SessionID,
) {
	if !isDurableExecutionSessionID(string(sessionID)) {
		s.writeError(w, http.StatusNotImplemented, "durable factory session retry-dispatch is not implemented", "INTERNAL_ERROR")
		return
	}

	retry, diagnostics, err := decodeRetryDispatchRequestWithDiagnostics(r.Body, s.sessionRequests)
	if err != nil {
		if message, ok := requestFieldValidationMessage(err); ok {
			s.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
			return
		}
		s.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}

	if s.sessionsRoot == nil {
		s.writeError(w, http.StatusServiceUnavailable, "factory session service is unavailable", "SERVICE_UNAVAILABLE")
		return
	}
	result, err := s.sessionsRoot.Control(r.Context(), factorysessionexecution.SessionControlRequest{
		SessionID: string(sessionID), Mode: factorysessionexecution.SessionOperationModeDurable,
		Operation:   factorysessionexecution.SessionControlRetryDispatch,
		Correlation: factorysessionexecution.SessionOperationCorrelation{RequestID: retry.RequestID}, Retry: &retry,
	})
	s.finishRootLifecycleControl(w, string(sessionID), "retry-dispatch", canonicalLifecycleResult(result), diagnostics.Paths(), err)
}

func (s *LifecycleHandler) ApproveFactorySession(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID) {
	s.handleDurableApproveControl(w, r, sessionID)
}

func (s *LifecycleHandler) PauseFactorySession(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID) {
	if s.usesDurableLifecycleControl(r.Context(), string(sessionID)) {
		s.handleDurableLifecycleControl(w, r, sessionID, "pause")
		return
	}
	s.handleLiveLifecycleControl(w, r, sessionID, "pause")
}

func (s *LifecycleHandler) ResumeFactorySession(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID) {
	if s.usesDurableLifecycleControl(r.Context(), string(sessionID)) {
		s.handleDurableLifecycleControl(w, r, sessionID, "resume")
		return
	}
	s.handleLiveLifecycleControl(w, r, sessionID, "resume")
}

func (s *LifecycleHandler) CancelFactorySession(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID) {
	if s.usesDurableLifecycleControl(r.Context(), string(sessionID)) {
		s.handleDurableLifecycleControl(w, r, sessionID, "cancel")
		return
	}
	s.handleLiveLifecycleControl(w, r, sessionID, "cancel")
}

func (s *LifecycleHandler) TerminateFactorySession(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID) {
	if s.usesDurableLifecycleControl(r.Context(), string(sessionID)) {
		s.handleDurableLifecycleControl(w, r, sessionID, "terminate")
		return
	}
	s.handleLiveLifecycleControl(w, r, sessionID, "terminate")
}

func (s *LifecycleHandler) RetryFactorySessionDispatch(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID) {
	s.handleDurableRetryDispatchControl(w, r, sessionID)
}

func (s *LifecycleHandler) handleDurableInterruptDispatchControl(
	w http.ResponseWriter,
	r *http.Request,
	sessionID factoryapi.SessionID,
) {
	if !isDurableExecutionSessionID(string(sessionID)) {
		s.writeError(w, http.StatusNotImplemented, "durable factory session interrupt-dispatch is not implemented", "INTERNAL_ERROR")
		return
	}

	interrupt, diagnostics, err := decodeInterruptDispatchRequestWithDiagnostics(r.Body, s.sessionRequests)
	if err != nil {
		if message, ok := requestFieldValidationMessage(err); ok {
			s.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
			return
		}
		s.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}

	if s.sessionsRoot == nil {
		s.writeError(w, http.StatusServiceUnavailable, "factory session service is unavailable", "SERVICE_UNAVAILABLE")
		return
	}
	result, err := s.sessionsRoot.Control(r.Context(), factorysessionexecution.SessionControlRequest{
		SessionID: string(sessionID), Mode: factorysessionexecution.SessionOperationModeDurable,
		Operation:   factorysessionexecution.SessionControlInterruptDispatch,
		Correlation: factorysessionexecution.SessionOperationCorrelation{RequestID: interrupt.RequestID}, Interrupt: &interrupt,
	})
	s.finishRootLifecycleControl(w, string(sessionID), "interrupt-dispatch", canonicalLifecycleResult(result), diagnostics.Paths(), err)
}
