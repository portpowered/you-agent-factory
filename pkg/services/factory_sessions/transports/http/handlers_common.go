package http

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"

	factorysessionexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"

	httpcompat "github.com/portpowered/infinite-you/pkg/transports/http/compat"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/optional"

	"go.uber.org/zap"
)

const factorySessionsHTTPBoundary = "factory_sessions.http"

func (s *Server) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		s.logger.Error("encode response failed", zap.Error(err))
	}
}

func (s *Server) writeError(w http.ResponseWriter, status int, message, code string) {
	s.writeErrorWithTargets(w, status, message, code, nil)
}

func (s *Server) writeErrorWithTargets(w http.ResponseWriter, status int, message, code string, targets []factoryapi.FactoryValidationTarget) {
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

func (s *Server) writeUnsupportedMediaTypeError(w http.ResponseWriter) {
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

func (s *Adapter) writeCompatibilityWarning(w http.ResponseWriter, operation string, paths []string) {
	httpcompat.ApplyWarning(w, s.logger, factorySessionsHTTPBoundary, operation, paths)
}

func stringValue(value *string) string {
	return optional.StringValue(value)
}

func (s *Server) requireDurableSessionLifecycleAPI(w http.ResponseWriter) (apisurface.DurableSessionLifecycleAPI, bool) {
	if s.durableLifecycle == nil {
		s.writeError(w, http.StatusInternalServerError, "durable factory session lifecycle control is unavailable", "INTERNAL_ERROR")
		return nil, false
	}
	return s.durableLifecycle, true
}

func (s *Server) writeDurableLifecycleControlError(w http.ResponseWriter, sessionID string, err error) bool {
	if status, response, ok := factorysession.LifecycleControlErrorResponse(sessionID, err); ok {
		s.writeJSON(w, status, response)
		return true
	}
	return false
}

func (s *Server) writeLifecycleControlSuccessWithDiagnostics(
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

func (s *Server) handleDurableLifecycleControl(
	w http.ResponseWriter,
	r *http.Request,
	sessionID factoryapi.SessionID,
	operation string,
	invoke func(apisurface.DurableSessionLifecycleAPI, factorysessionexecution.ControlRequest) (factoryapi.FactorySessionLifecycleControlResponse, error),
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

	lifecycle, ok := s.requireDurableSessionLifecycleAPI(w)
	if !ok {
		return
	}

	response, err := invoke(lifecycle, control)
	if err != nil {
		if s.writeDurableLifecycleControlError(w, string(sessionID), err) {
			return
		}
		s.logger.Error("durable factory session lifecycle control failed",
			zap.Error(err),
			zap.String("session_id", string(sessionID)),
			zap.String("operation", operation),
		)
		s.writeError(w, http.StatusInternalServerError, "durable factory session lifecycle control failed", "INTERNAL_ERROR")
		return
	}

	s.writeLifecycleControlSuccessWithDiagnostics(w, response, diagnostics.Paths())
}

func (s *Server) handleLiveLifecycleControl(
	w http.ResponseWriter,
	r *http.Request,
	sessionID factoryapi.SessionID,
	operation string,
	invoke func(apisurface.LiveSessionAPI, factorysessionexecution.ControlRequest) (factoryapi.FactorySessionLifecycleControlResponse, error),
) {
	control, diagnostics, err := decodeLifecycleControlRequestWithDiagnostics(r.Body, s.sessionRequests)
	if err != nil {
		if message, ok := requestFieldValidationMessage(err); ok {
			s.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
			return
		}
		s.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}

	if s.liveControl != nil {
		s.invokeRootLiveLifecycleControl(w, r.Context(), sessionID, operation, control, diagnostics.Paths())
		return
	}

	sessionRuntime, ok := s.requireSessionRuntime(w)
	if !ok {
		return
	}

	response, err := invoke(sessionRuntime, control)
	if err != nil {
		if s.writeDurableLifecycleControlError(w, string(sessionID), err) {
			return
		}
		s.logger.Error("live factory session lifecycle control failed",
			zap.Error(err),
			zap.String("session_id", string(sessionID)),
			zap.String("operation", operation),
		)
		s.writeError(w, http.StatusInternalServerError, "live factory session lifecycle control failed", "INTERNAL_ERROR")
		return
	}

	s.writeLifecycleControlSuccessWithDiagnostics(w, response, diagnostics.Paths())
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

func (s *Server) handleDurableApproveControl(
	w http.ResponseWriter,
	r *http.Request,
	sessionID factoryapi.SessionID,
	invoke func(apisurface.DurableSessionLifecycleAPI, factorysessionexecution.ApproveRequest) (factoryapi.FactorySessionLifecycleControlResponse, error),
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

	lifecycle, ok := s.requireDurableSessionLifecycleAPI(w)
	if !ok {
		return
	}

	response, err := invoke(lifecycle, approve)
	if err != nil {
		if s.writeDurableLifecycleControlError(w, string(sessionID), err) {
			return
		}
		s.logger.Error("durable factory session approve failed",
			zap.Error(err),
			zap.String("session_id", string(sessionID)),
		)
		s.writeError(w, http.StatusInternalServerError, "durable factory session approve failed", "INTERNAL_ERROR")
		return
	}

	s.writeLifecycleControlSuccessWithDiagnostics(w, response, diagnostics.Paths())
}

func (s *Server) handleDurableRetryDispatchControl(
	w http.ResponseWriter,
	r *http.Request,
	sessionID factoryapi.SessionID,
	invoke func(apisurface.DurableSessionLifecycleAPI, factorysessionexecution.RetryDispatchRequest) (factoryapi.FactorySessionLifecycleControlResponse, error),
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

	lifecycle, ok := s.requireDurableSessionLifecycleAPI(w)
	if !ok {
		return
	}

	response, err := invoke(lifecycle, retry)
	if err != nil {
		if s.writeDurableLifecycleControlError(w, string(sessionID), err) {
			return
		}
		s.logger.Error("durable factory session retry-dispatch failed",
			zap.Error(err),
			zap.String("session_id", string(sessionID)),
		)
		s.writeError(w, http.StatusInternalServerError, "durable factory session retry-dispatch failed", "INTERNAL_ERROR")
		return
	}

	s.writeLifecycleControlSuccessWithDiagnostics(w, response, diagnostics.Paths())
}

func (s *Server) ApproveFactorySession(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID) {
	s.handleDurableApproveControl(w, r, sessionID, func(
		lifecycle apisurface.DurableSessionLifecycleAPI,
		req factorysessionexecution.ApproveRequest,
	) (factoryapi.FactorySessionLifecycleControlResponse, error) {
		return lifecycle.ApproveDurableFactorySession(r.Context(), string(sessionID), req)
	})
}

func (s *Server) PauseFactorySession(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID) {
	if isDurableExecutionSessionID(string(sessionID)) {
		s.handleDurableLifecycleControl(w, r, sessionID, "pause", func(
			lifecycle apisurface.DurableSessionLifecycleAPI,
			req factorysessionexecution.ControlRequest,
		) (factoryapi.FactorySessionLifecycleControlResponse, error) {
			return lifecycle.PauseDurableFactorySession(r.Context(), string(sessionID), req)
		})
		return
	}
	s.handleLiveLifecycleControl(w, r, sessionID, "pause", func(
		sessionRuntime apisurface.LiveSessionAPI,
		req factorysessionexecution.ControlRequest,
	) (factoryapi.FactorySessionLifecycleControlResponse, error) {
		return sessionRuntime.PauseLiveFactorySession(r.Context(), string(sessionID), req)
	})
}

func (s *Server) ResumeFactorySession(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID) {
	if isDurableExecutionSessionID(string(sessionID)) {
		s.handleDurableLifecycleControl(w, r, sessionID, "resume", func(
			lifecycle apisurface.DurableSessionLifecycleAPI,
			req factorysessionexecution.ControlRequest,
		) (factoryapi.FactorySessionLifecycleControlResponse, error) {
			return lifecycle.ResumeDurableFactorySession(r.Context(), string(sessionID), req)
		})
		return
	}
	s.handleLiveLifecycleControl(w, r, sessionID, "resume", func(
		sessionRuntime apisurface.LiveSessionAPI,
		req factorysessionexecution.ControlRequest,
	) (factoryapi.FactorySessionLifecycleControlResponse, error) {
		return sessionRuntime.ResumeLiveFactorySession(r.Context(), string(sessionID), req)
	})
}

func (s *Server) CancelFactorySession(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID) {
	if isDurableExecutionSessionID(string(sessionID)) {
		s.handleDurableLifecycleControl(w, r, sessionID, "cancel", func(
			lifecycle apisurface.DurableSessionLifecycleAPI,
			req factorysessionexecution.ControlRequest,
		) (factoryapi.FactorySessionLifecycleControlResponse, error) {
			return lifecycle.CancelDurableFactorySession(r.Context(), string(sessionID), req)
		})
		return
	}
	s.handleLiveLifecycleControl(w, r, sessionID, "cancel", func(
		sessionRuntime apisurface.LiveSessionAPI,
		req factorysessionexecution.ControlRequest,
	) (factoryapi.FactorySessionLifecycleControlResponse, error) {
		return sessionRuntime.CancelLiveFactorySession(r.Context(), string(sessionID), req)
	})
}

func (s *Server) TerminateFactorySession(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID) {
	if isDurableExecutionSessionID(string(sessionID)) {
		s.handleDurableLifecycleControl(w, r, sessionID, "terminate", func(
			lifecycle apisurface.DurableSessionLifecycleAPI,
			req factorysessionexecution.ControlRequest,
		) (factoryapi.FactorySessionLifecycleControlResponse, error) {
			return lifecycle.TerminateDurableFactorySession(r.Context(), string(sessionID), req)
		})
		return
	}
	s.handleLiveLifecycleControl(w, r, sessionID, "terminate", func(
		sessionRuntime apisurface.LiveSessionAPI,
		req factorysessionexecution.ControlRequest,
	) (factoryapi.FactorySessionLifecycleControlResponse, error) {
		return sessionRuntime.TerminateLiveFactorySession(r.Context(), string(sessionID), req)
	})
}

func (s *Server) RetryFactorySessionDispatch(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID) {
	s.handleDurableRetryDispatchControl(w, r, sessionID, func(
		lifecycle apisurface.DurableSessionLifecycleAPI,
		req factorysessionexecution.RetryDispatchRequest,
	) (factoryapi.FactorySessionLifecycleControlResponse, error) {
		return lifecycle.RetryDurableFactorySessionDispatch(r.Context(), string(sessionID), req)
	})
}

func (s *Server) handleDurableInterruptDispatchControl(
	w http.ResponseWriter,
	r *http.Request,
	sessionID factoryapi.SessionID,
	invoke func(apisurface.DurableSessionLifecycleAPI, factorysessionexecution.InterruptDispatchRequest) (factoryapi.FactorySessionLifecycleControlResponse, error),
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

	lifecycle, ok := s.requireDurableSessionLifecycleAPI(w)
	if !ok {
		return
	}

	response, err := invoke(lifecycle, interrupt)
	if err != nil {
		if s.writeDurableLifecycleControlError(w, string(sessionID), err) {
			return
		}
		s.logger.Error("durable factory session interrupt-dispatch failed",
			zap.Error(err),
			zap.String("session_id", string(sessionID)),
		)
		s.writeError(w, http.StatusInternalServerError, "durable factory session interrupt-dispatch failed", "INTERNAL_ERROR")
		return
	}

	s.writeLifecycleControlSuccessWithDiagnostics(w, response, diagnostics.Paths())
}
