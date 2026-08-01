package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"

	factorysessionexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/optional"

	"go.uber.org/zap"
)

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

func (s *Server) writeSSEDataJSON(w http.ResponseWriter, v any) error {
	payload, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "data: %s\n\n", payload)
	return err
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

type requestFieldValidationError struct {
	message string
}

func (e requestFieldValidationError) Error() string {
	return e.message
}

func requestFieldValidationMessage(err error) (string, bool) {
	var validationErr requestFieldValidationError
	if errors.As(err, &validationErr) {
		return validationErr.message, true
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

func ensureSingleJSONObject(dec *json.Decoder) error {
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return requestFieldValidationError{message: "request payload must contain one JSON object"}
		}
		return err
	}
	return nil
}

func decodeStrictJSON[T any](body io.Reader) (T, error) {
	var zero T
	data, err := io.ReadAll(body)
	if err != nil {
		return zero, err
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	var req T
	if err := decoder.Decode(&req); err != nil {
		return zero, err
	}
	if err := ensureSingleJSONObject(decoder); err != nil {
		return zero, err
	}
	return req, nil
}

// DecodeStrictJSON decodes exactly one JSON object and rejects unknown fields.
func DecodeStrictJSON[T any](body io.Reader) (T, error) {
	return decodeStrictJSON[T](body)
}

func stringValue(value *string) string {
	return optional.StringValue(value)
}

func intValue(value *int) int {
	return optional.IntValue(value)
}

func stringSliceValue(values *[]string) []string {
	return optional.StringsValue(values)
}

func stringSlicePtrCopy(values []string) *[]string {
	return optional.CopiedStringsPtr(values)
}

func stringPtrIfNotEmpty(value string) *string {
	return optional.NonEmptyStringPtr(value)
}

func stringMapPtr(values map[string]string) *factoryapi.StringMap {
	return optional.CopiedStringMapPtr(values)
}

func intPtrIfPositive(value int) *int {
	return optional.PositiveIntPtr(value)
}

func generatedStringMap(values *factoryapi.StringMap) map[string]string {
	return optional.StringMapValue(values)
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

func (s *Server) writeLifecycleControlSuccess(
	w http.ResponseWriter,
	response factoryapi.FactorySessionLifecycleControlResponse,
) {
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

	control, err := decodeLifecycleControlRequest(r.Body, s.sessionRequests)
	if err != nil {
		if message, ok := requestFieldValidationMessage(err); ok {
			s.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
			return
		}
		s.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}

	if s.sessionsRoot != nil {
		s.invokeRootDurableLifecycleControl(w, r.Context(), sessionID, operation, control)
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

	s.writeLifecycleControlSuccess(w, response)
}

func (s *Adapter) InvokeFactorySessionBySessionId(
	w http.ResponseWriter,
	r *http.Request,
	sessionID factoryapi.SessionID,
) {
	req, err := decodeInvocationRequestBody(r.Body)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}
	if s.invocation == nil {
		s.writeError(w, http.StatusInternalServerError, "session invocation API is unavailable", "INTERNAL_ERROR")
		return
	}

	result, err := s.invocation.InvokeFactorySession(r.Context(), string(sessionID), req)
	if err != nil {
		switch typed := err.(type) {
		case *work.InputError:
			s.writeError(w, http.StatusBadRequest, typed.Message, string(typed.Code))
		case *work.ArgumentError:
			s.writeError(w, http.StatusBadRequest, typed.Message, string(typed.Code))
		case *apisurface.RequestValidationError:
			s.writeError(w, http.StatusBadRequest, typed.Message, "BAD_REQUEST")
		default:
			if errors.Is(err, apisurface.ErrFactorySessionNotFound) {
				s.writeError(w, http.StatusNotFound, "factory session not found", "NOT_FOUND")
				return
			}
			s.logger.Error("invoke factory session failed", zap.Error(err), zap.String("session_id", string(sessionID)))
			s.writeError(w, http.StatusInternalServerError, "failed to invoke factory session", "INTERNAL_ERROR")
		}
		return
	}

	response := apisurface.InvocationResponseFromResult(result)
	s.writeJSON(w, http.StatusOK, response)
}

func decodeInvocationRequestBody(
	body io.Reader,
) (factoryapi.InvokeFactorySessionBySessionIdJSONRequestBody, error) {
	return decodeStrictJSON[factoryapi.InvokeFactorySessionBySessionIdJSONRequestBody](body)
}

// StageSubmitWorkFileBySessionId retains the session existence guard that is
// specific to the Factory Sessions surface, then delegates decoding, staging,
// response projection, and Work error mapping to Work HTTP.
func (s *Adapter) StageSubmitWorkFileBySessionId(
	w http.ResponseWriter,
	r *http.Request,
	sessionID factoryapi.SessionID,
) {
	definitions, ok := s.requireFactoryDefinitionAPI(w)
	if !ok {
		return
	}
	if _, err := definitions.GetCurrentFactoryForSession(r.Context(), string(sessionID)); err != nil {
		if errors.Is(err, apisurface.ErrFactorySessionNotFound) {
			s.writeError(w, http.StatusNotFound, "factory session not found", "NOT_FOUND")
			return
		}
		s.logger.Error("stage submit-work file failed", zap.Error(err))
		s.writeError(w, http.StatusInternalServerError, "failed to stage submit-work file", "INTERNAL_ERROR")
		return
	}
	if s.Adapter == nil {
		s.writeError(w, http.StatusInternalServerError, "Work HTTP adapter is unavailable", "INTERNAL_ERROR")
		return
	}
	s.Adapter.StageSubmitWorkFileBySessionId(w, r, sessionID)
}

func (s *Server) handleLiveLifecycleControl(
	w http.ResponseWriter,
	r *http.Request,
	sessionID factoryapi.SessionID,
	operation string,
	invoke func(apisurface.LiveSessionAPI, factorysessionexecution.ControlRequest) (factoryapi.FactorySessionLifecycleControlResponse, error),
) {
	control, err := decodeLifecycleControlRequest(r.Body, s.sessionRequests)
	if err != nil {
		if message, ok := requestFieldValidationMessage(err); ok {
			s.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
			return
		}
		s.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}

	if s.sessionsRoot != nil {
		s.invokeRootLiveLifecycleControl(w, r.Context(), sessionID, operation, control)
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

	s.writeLifecycleControlSuccess(w, response)
}

func decodeOptionalLifecycleControlRequest(body io.Reader) (factoryapi.FactorySessionLifecycleControlRequest, error) {
	return decodeOptionalJSONRequest(body, func() factoryapi.FactorySessionLifecycleControlRequest {
		return factoryapi.FactorySessionLifecycleControlRequest{}
	})
}

func decodeOptionalApproveRequest(body io.Reader) (factoryapi.FactorySessionApproveRequest, error) {
	return decodeOptionalJSONRequest(body, func() factoryapi.FactorySessionApproveRequest {
		return factoryapi.FactorySessionApproveRequest{}
	})
}

func decodeOptionalRetryDispatchRequest(body io.Reader) (factoryapi.FactorySessionRetryDispatchRequest, error) {
	return decodeOptionalJSONRequest(body, func() factoryapi.FactorySessionRetryDispatchRequest {
		return factoryapi.FactorySessionRetryDispatchRequest{}
	})
}

func decodeOptionalInterruptDispatchRequest(body io.Reader) (factoryapi.FactorySessionInterruptDispatchRequest, error) {
	return decodeOptionalJSONRequest(body, func() factoryapi.FactorySessionInterruptDispatchRequest {
		return factoryapi.FactorySessionInterruptDispatchRequest{}
	})
}

func decodeOptionalJSONRequest[T any](body io.Reader, zero func() T) (T, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return zero(), err
	}
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return zero(), nil
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.DisallowUnknownFields()
	var req T
	if err := decoder.Decode(&req); err != nil {
		return zero(), err
	}
	if err := ensureSingleJSONObject(decoder); err != nil {
		return zero(), err
	}
	return req, nil
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

	approve, err := decodeApproveFactorySessionRequest(r.Body, s.sessionRequests)
	if err != nil {
		if message, ok := requestFieldValidationMessage(err); ok {
			s.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
			return
		}
		s.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}

	if s.sessionsRoot != nil {
		result, invokeErr := s.sessionsRoot.ApproveDurableFactorySession(r.Context(), string(sessionID), approve)
		s.finishRootLifecycleControl(w, string(sessionID), "approve", result, invokeErr)
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

	s.writeLifecycleControlSuccess(w, response)
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

	retry, err := decodeRetryDispatchRequest(r.Body, s.sessionRequests)
	if err != nil {
		if message, ok := requestFieldValidationMessage(err); ok {
			s.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
			return
		}
		s.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}

	if s.sessionsRoot != nil {
		result, invokeErr := s.sessionsRoot.RetryDurableFactorySessionDispatch(r.Context(), string(sessionID), retry)
		s.finishRootLifecycleControl(w, string(sessionID), "retry-dispatch", result, invokeErr)
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

	s.writeLifecycleControlSuccess(w, response)
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
	s.handleDurableLifecycleControl(w, r, sessionID, "cancel", func(
		lifecycle apisurface.DurableSessionLifecycleAPI,
		req factorysessionexecution.ControlRequest,
	) (factoryapi.FactorySessionLifecycleControlResponse, error) {
		return lifecycle.CancelDurableFactorySession(r.Context(), string(sessionID), req)
	})
}

func (s *Server) TerminateFactorySession(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID) {
	s.handleDurableLifecycleControl(w, r, sessionID, "terminate", func(
		lifecycle apisurface.DurableSessionLifecycleAPI,
		req factorysessionexecution.ControlRequest,
	) (factoryapi.FactorySessionLifecycleControlResponse, error) {
		return lifecycle.TerminateDurableFactorySession(r.Context(), string(sessionID), req)
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

	interrupt, err := decodeInterruptDispatchRequest(r.Body, s.sessionRequests)
	if err != nil {
		if message, ok := requestFieldValidationMessage(err); ok {
			s.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
			return
		}
		s.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}

	if s.sessionsRoot != nil {
		result, invokeErr := s.sessionsRoot.InterruptDurableFactorySessionDispatch(r.Context(), string(sessionID), interrupt)
		s.finishRootLifecycleControl(w, string(sessionID), "interrupt-dispatch", result, invokeErr)
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

	s.writeLifecycleControlSuccess(w, response)
}
