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

func validateWorkContentField(fields map[string]json.RawMessage, prefix string) error {
	contentRaw, ok := fields["content"]
	if !ok {
		return nil
	}

	var partPayloads []json.RawMessage
	if err := json.Unmarshal(contentRaw, &partPayloads); err != nil {
		return requestFieldValidationError{message: fmt.Sprintf("%scontent must be an array", prefix)}
	}
	for i, payload := range partPayloads {
		var partFields map[string]json.RawMessage
		if err := json.Unmarshal(payload, &partFields); err != nil {
			return requestFieldValidationError{message: fmt.Sprintf("%scontent[%d] must be an object", prefix, i)}
		}
		if err := validateRawWorkContentPart(partFields, fmt.Sprintf("%scontent[%d].", prefix, i)); err != nil {
			return err
		}
	}
	return nil
}

func validateRawWorkContentPart(fields map[string]json.RawMessage, prefix string) error {
	partType, err := requiredWorkContentPartType(fields, prefix)
	if err != nil {
		return err
	}

	switch partType {
	case "text", "TEXT":
		return validateRawTextContentPart(fields, prefix)
	case "image", "IMAGE":
		return validateRawURLContentPart(fields, prefix, "image content parts")
	case "AUDIO":
		return validateRawURLContentPart(fields, prefix, "audio content parts")
	case "JSON":
		return validateRawJSONContentPart(fields, prefix)
	case "BINARY":
		return validateRawURLContentPart(fields, prefix, "binary content parts")
	default:
		return requestFieldValidationError{message: fmt.Sprintf("%stype must be one of text, image, TEXT, IMAGE, AUDIO, JSON, or BINARY", prefix)}
	}
}

func requiredWorkContentPartType(fields map[string]json.RawMessage, prefix string) (string, error) {
	typeRaw, ok := fields["type"]
	if !ok {
		return "", requestFieldValidationError{message: fmt.Sprintf("%stype is required", prefix)}
	}

	var partType string
	if err := json.Unmarshal(typeRaw, &partType); err != nil || partType == "" {
		return "", requestFieldValidationError{message: fmt.Sprintf("%stype must be a non-empty string", prefix)}
	}

	return partType, nil
}

func validateRawTextContentPart(fields map[string]json.RawMessage, prefix string) error {
	if err := requireOnlyFields(fields, prefix, "type", "text", "label", "role", "contentType", "artifactId", "metadata"); err != nil {
		return err
	}
	if _, err := requiredStringField(fields, prefix, "text", "text content parts"); err != nil {
		return err
	}
	return validateSharedWorkContentFields(fields, prefix)
}

func validateRawURLContentPart(fields map[string]json.RawMessage, prefix string, usage string) error {
	if err := requireOnlyFields(fields, prefix, "type", "url", "file", "label", "role", "contentType", "artifactId", "metadata"); err != nil {
		return err
	}
	if err := validateSharedWorkContentFields(fields, prefix); err != nil {
		return err
	}
	hasFile := false
	if fileRaw, ok := fields["file"]; ok {
		var file string
		if err := json.Unmarshal(fileRaw, &file); err != nil || file == "" {
			return requestFieldValidationError{message: fmt.Sprintf("%sfile must be a non-empty string when provided", prefix)}
		}
		hasFile = true
	}
	hasURL := false
	if urlRaw, ok := fields["url"]; ok {
		var contentURL string
		if err := json.Unmarshal(urlRaw, &contentURL); err != nil || contentURL == "" {
			return requestFieldValidationError{message: fmt.Sprintf("%surl must be a non-empty string", prefix)}
		}
		hasURL = true
	}
	if !hasURL && !hasFile {
		return requestFieldValidationError{
			message: fmt.Sprintf("%surl is required for %s", prefix, usage),
		}
	}
	return nil
}

func validateRawJSONContentPart(fields map[string]json.RawMessage, prefix string) error {
	if err := requireOnlyFields(fields, prefix, "type", "json", "label", "role", "contentType", "artifactId", "metadata"); err != nil {
		return err
	}
	jsonRaw, ok := fields["json"]
	if !ok {
		return requestFieldValidationError{message: fmt.Sprintf("%sjson is required for JSON content parts", prefix)}
	}
	var value any
	if err := json.Unmarshal(jsonRaw, &value); err != nil {
		return requestFieldValidationError{message: fmt.Sprintf("%sjson must be valid JSON", prefix)}
	}
	return validateSharedWorkContentFields(fields, prefix)
}

func requiredStringField(fields map[string]json.RawMessage, prefix string, fieldName string, usage string) (string, error) {
	raw, ok := fields[fieldName]
	if !ok {
		return "", requestFieldValidationError{message: fmt.Sprintf("%s%s is required for %s", prefix, fieldName, usage)}
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", requestFieldValidationError{message: fmt.Sprintf("%s%s must be a string", prefix, fieldName)}
	}
	return value, nil
}

func validateSharedWorkContentFields(fields map[string]json.RawMessage, prefix string) error {
	for _, field := range []string{"label", "role", "contentType", "artifactId"} {
		if _, err := optionalStringField(fields, prefix, field); err != nil {
			return err
		}
	}

	if metadataRaw, ok := fields["metadata"]; ok {
		var metadata map[string]any
		if err := json.Unmarshal(metadataRaw, &metadata); err != nil || metadata == nil {
			return requestFieldValidationError{message: fmt.Sprintf("%smetadata must be an object", prefix)}
		}
	}

	return nil
}

func requiredNonEmptyStringField(fields map[string]json.RawMessage, prefix string, field string, partLabel string) (string, error) {
	fieldRaw, ok := fields[field]
	if !ok {
		return "", requestFieldValidationError{message: fmt.Sprintf("%s%s is required for %s", prefix, field, partLabel)}
	}
	var value string
	if err := json.Unmarshal(fieldRaw, &value); err != nil || value == "" {
		return "", requestFieldValidationError{message: fmt.Sprintf("%s%s must be a non-empty string", prefix, field)}
	}
	return value, nil
}

func optionalStringField(fields map[string]json.RawMessage, prefix string, field string) (string, error) {
	fieldRaw, ok := fields[field]
	if !ok {
		return "", nil
	}
	var value string
	if err := json.Unmarshal(fieldRaw, &value); err != nil {
		return "", requestFieldValidationError{message: fmt.Sprintf("%s%s must be a string", prefix, field)}
	}
	return value, nil
}

func requireOnlyFields(fields map[string]json.RawMessage, prefix string, allowed ...string) error {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, field := range allowed {
		allowedSet[field] = struct{}{}
	}
	for field := range fields {
		if _, ok := allowedSet[field]; ok {
			continue
		}
		return requestFieldValidationError{message: fmt.Sprintf("%s%s is not supported", prefix, field)}
	}
	return nil
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

	lifecycle, ok := s.requireDurableSessionLifecycleAPI(w)
	if !ok {
		return
	}

	req, err := decodeOptionalLifecycleControlRequest(r.Body)
	if err != nil {
		if message, ok := requestFieldValidationMessage(err); ok {
			s.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
			return
		}
		s.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}
	control, err := factorysession.ControlRequestFromAPI(req)
	if err == nil {
		control, err = s.sessionRequests.PrepareControl(control)
	}
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error(), "BAD_REQUEST")
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

func (s *Server) handleLiveLifecycleControl(
	w http.ResponseWriter,
	r *http.Request,
	sessionID factoryapi.SessionID,
	operation string,
	invoke func(apisurface.LiveSessionAPI, factorysessionexecution.ControlRequest) (factoryapi.FactorySessionLifecycleControlResponse, error),
) {
	sessionRuntime, ok := s.requireSessionRuntime(w)
	if !ok {
		return
	}

	req, err := decodeOptionalLifecycleControlRequest(r.Body)
	if err != nil {
		if message, ok := requestFieldValidationMessage(err); ok {
			s.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
			return
		}
		s.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}
	control, err := factorysession.ControlRequestFromAPI(req)
	if err == nil {
		control, err = s.sessionRequests.PrepareControl(control)
	}
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error(), "BAD_REQUEST")
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

	lifecycle, ok := s.requireDurableSessionLifecycleAPI(w)
	if !ok {
		return
	}

	req, err := decodeOptionalApproveRequest(r.Body)
	if err != nil {
		if message, ok := requestFieldValidationMessage(err); ok {
			s.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
			return
		}
		s.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}
	approve, err := factorysession.ApproveRequestFromAPI(req)
	if err == nil {
		approve, err = s.sessionRequests.PrepareApprove(approve)
	}
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error(), "BAD_REQUEST")
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

	lifecycle, ok := s.requireDurableSessionLifecycleAPI(w)
	if !ok {
		return
	}

	req, err := decodeOptionalRetryDispatchRequest(r.Body)
	if err != nil {
		if message, ok := requestFieldValidationMessage(err); ok {
			s.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
			return
		}
		s.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}
	retry, err := factorysession.RetryDispatchRequestFromAPI(req)
	if err == nil {
		retry, err = s.sessionRequests.PrepareRetryDispatch(retry)
	}
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error(), "BAD_REQUEST")
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

	lifecycle, ok := s.requireDurableSessionLifecycleAPI(w)
	if !ok {
		return
	}

	req, err := decodeOptionalInterruptDispatchRequest(r.Body)
	if err != nil {
		if message, ok := requestFieldValidationMessage(err); ok {
			s.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
			return
		}
		s.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}
	interrupt, err := factorysession.InterruptDispatchRequestFromAPI(req)
	if err == nil {
		interrupt, err = s.sessionRequests.PrepareInterruptDispatch(interrupt)
	}
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error(), "BAD_REQUEST")
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
