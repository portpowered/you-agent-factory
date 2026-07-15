package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/pkg/work"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/optional"

	factoryrequests "github.com/portpowered/infinite-you/pkg/factory/requests"
	"github.com/portpowered/infinite-you/pkg/work/content"
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

func validateCanonicalWorkRequestJSONForAPI(data []byte) error {
	return factoryrequests.ValidateCanonicalWorkRequestJSON(data)
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
		if _, err := validatedRawWorkContentPart(partFields, fmt.Sprintf("%scontent[%d].", prefix, i)); err != nil {
			return err
		}
	}
	return nil
}

func validatedRawWorkContentPart(fields map[string]json.RawMessage, prefix string) (work.WorkContentPart, error) {
	partType, err := requiredWorkContentPartType(fields, prefix)
	if err != nil {
		return work.WorkContentPart{}, err
	}

	switch partType {
	case work.WorkContentPartTypeText:
		return validatedRawTextContentPart(fields, prefix)
	case work.WorkContentPartTypeImage:
		return validatedRawURLContentPart(fields, prefix, work.WorkContentPartTypeImage, "image content parts")
	case work.WorkContentPartTypeAudio:
		return validatedRawURLContentPart(fields, prefix, work.WorkContentPartTypeAudio, "audio content parts")
	case work.WorkContentPartTypeJSON:
		return validatedRawJSONContentPart(fields, prefix)
	case work.WorkContentPartTypeBinary:
		return validatedRawURLContentPart(fields, prefix, work.WorkContentPartTypeBinary, "binary content parts")
	default:
		return work.WorkContentPart{}, requestFieldValidationError{message: fmt.Sprintf("%stype must be one of text, image, TEXT, IMAGE, AUDIO, JSON, or BINARY", prefix)}
	}
}

func requiredWorkContentPartType(fields map[string]json.RawMessage, prefix string) (work.WorkContentPartType, error) {
	typeRaw, ok := fields["type"]
	if !ok {
		return "", requestFieldValidationError{message: fmt.Sprintf("%stype is required", prefix)}
	}

	var partType string
	if err := json.Unmarshal(typeRaw, &partType); err != nil || partType == "" {
		return "", requestFieldValidationError{message: fmt.Sprintf("%stype must be a non-empty string", prefix)}
	}

	return work.WorkContentPartType(partType).Normalized(), nil
}

func validatedRawTextContentPart(fields map[string]json.RawMessage, prefix string) (work.WorkContentPart, error) {
	if err := requireOnlyFields(fields, prefix, "type", "text", "label", "role", "contentType", "artifactId", "metadata"); err != nil {
		return work.WorkContentPart{}, err
	}
	text, err := requiredStringField(fields, prefix, "text", "text content parts")
	if err != nil {
		return work.WorkContentPart{}, err
	}
	shared, err := validateSharedWorkContentFields(fields, prefix)
	if err != nil {
		return work.WorkContentPart{}, err
	}
	shared.Type = work.WorkContentPartTypeText
	shared.Text = text
	return shared, nil
}

func validatedRawURLContentPart(fields map[string]json.RawMessage, prefix string, partType work.WorkContentPartType, usage string) (work.WorkContentPart, error) {
	if err := requireOnlyFields(fields, prefix, "type", "url", "file", "label", "role", "contentType", "artifactId", "metadata"); err != nil {
		return work.WorkContentPart{}, err
	}
	if _, hasFile := fields["file"]; hasFile {
		if _, hasURL := fields["url"]; hasURL {
			return work.WorkContentPart{}, requestFieldValidationError{message: fmt.Sprintf("%surl and file cannot both be set on the same content part", prefix)}
		}
	}
	shared, err := validateSharedWorkContentFields(fields, prefix)
	if err != nil {
		return work.WorkContentPart{}, err
	}
	shared.Type = partType
	if fileRaw, ok := fields["file"]; ok {
		var file string
		if err := json.Unmarshal(fileRaw, &file); err != nil || file == "" {
			return work.WorkContentPart{}, requestFieldValidationError{message: fmt.Sprintf("%sfile must be a non-empty string when provided", prefix)}
		}
		shared.File = file
	}
	if urlRaw, ok := fields["url"]; ok {
		var contentURL string
		if err := json.Unmarshal(urlRaw, &contentURL); err != nil || contentURL == "" {
			return work.WorkContentPart{}, requestFieldValidationError{message: fmt.Sprintf("%surl must be a non-empty string", prefix)}
		}
		shared.URL = contentURL
	}
	normalized, err := content.NormalizeFileBackedContentPart(shared)
	if err != nil {
		if strings.Contains(err.Error(), "url and file cannot both be set") {
			return work.WorkContentPart{}, requestFieldValidationError{message: fmt.Sprintf("%s%s", prefix, err.Error())}
		}
		if strings.Contains(err.Error(), "url must be a non-empty string") {
			return work.WorkContentPart{}, requestFieldValidationError{message: fmt.Sprintf("%surl is required for %s", prefix, usage)}
		}
		return work.WorkContentPart{}, requestFieldValidationError{message: fmt.Sprintf("%s%s", prefix, err.Error())}
	}
	if err := content.ValidateContentURL(normalized.URL); err != nil {
		return work.WorkContentPart{}, requestFieldValidationError{message: fmt.Sprintf("%surl %s", prefix, err.Error())}
	}
	return normalized, nil
}

func validatedRawJSONContentPart(fields map[string]json.RawMessage, prefix string) (work.WorkContentPart, error) {
	if err := requireOnlyFields(fields, prefix, "type", "json", "label", "role", "contentType", "artifactId", "metadata"); err != nil {
		return work.WorkContentPart{}, err
	}
	jsonRaw, ok := fields["json"]
	if !ok {
		return work.WorkContentPart{}, requestFieldValidationError{message: fmt.Sprintf("%sjson is required for JSON content parts", prefix)}
	}
	var value any
	if err := json.Unmarshal(jsonRaw, &value); err != nil {
		return work.WorkContentPart{}, requestFieldValidationError{message: fmt.Sprintf("%sjson must be valid JSON", prefix)}
	}
	shared, err := validateSharedWorkContentFields(fields, prefix)
	if err != nil {
		return work.WorkContentPart{}, err
	}
	shared.Type = work.WorkContentPartTypeJSON
	shared.JSON = append(json.RawMessage(nil), jsonRaw...)
	return shared, nil
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

func validateSharedWorkContentFields(fields map[string]json.RawMessage, prefix string) (work.WorkContentPart, error) {
	part := work.WorkContentPart{}

	label, err := optionalStringField(fields, prefix, "label")
	if err != nil {
		return work.WorkContentPart{}, err
	}
	role, err := optionalStringField(fields, prefix, "role")
	if err != nil {
		return work.WorkContentPart{}, err
	}
	contentType, err := optionalStringField(fields, prefix, "contentType")
	if err != nil {
		return work.WorkContentPart{}, err
	}
	artifactID, err := optionalStringField(fields, prefix, "artifactId")
	if err != nil {
		return work.WorkContentPart{}, err
	}
	part.Label = label
	part.Role = role
	part.ContentType = contentType
	part.ArtifactID = artifactID

	if metadataRaw, ok := fields["metadata"]; ok {
		var metadata map[string]any
		if err := json.Unmarshal(metadataRaw, &metadata); err != nil || metadata == nil {
			return work.WorkContentPart{}, requestFieldValidationError{message: fmt.Sprintf("%smetadata must be an object", prefix)}
		}
		part.Metadata = metadata
	}

	return part, nil
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

func integerMapPtr(values map[string]int) *factoryapi.IntegerMap {
	if len(values) == 0 {
		return nil
	}
	converted := factoryapi.IntegerMap(values)
	return &converted
}

func stringMapPtr(values map[string]string) *factoryapi.StringMap {
	return optional.CopiedStringMapPtr(values)
}

func intPtrIfPositive(value int) *int {
	return optional.PositiveIntPtr(value)
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func generatedStringMap(values *factoryapi.StringMap) map[string]string {
	return optional.StringMapValue(values)
}
func (s *Server) requireDurableSessionLifecycleAPI(w http.ResponseWriter) (apisurface.DurableSessionLifecycleAPI, bool) {
	if s.runtime == nil {
		s.writeError(w, http.StatusInternalServerError, "durable factory session lifecycle control is unavailable", "INTERNAL_ERROR")
		return nil, false
	}
	lifecycle, ok := s.runtime.(apisurface.DurableSessionLifecycleAPI)
	if !ok {
		s.writeError(w, http.StatusNotImplemented, "durable factory session lifecycle control is not implemented", "INTERNAL_ERROR")
		return nil, false
	}
	return lifecycle, true
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
	invoke func(apisurface.DurableSessionLifecycleAPI, factoryapi.FactorySessionLifecycleControlRequest) (factoryapi.FactorySessionLifecycleControlResponse, error),
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

	response, err := invoke(lifecycle, req)
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
	invoke func(apisurface.SessionAPI, factoryapi.FactorySessionLifecycleControlRequest) (factoryapi.FactorySessionLifecycleControlResponse, error),
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

	response, err := invoke(sessionRuntime, req)
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
	invoke func(apisurface.DurableSessionLifecycleAPI, factoryapi.FactorySessionApproveRequest) (factoryapi.FactorySessionLifecycleControlResponse, error),
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

	response, err := invoke(lifecycle, req)
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
	invoke func(apisurface.DurableSessionLifecycleAPI, factoryapi.FactorySessionRetryDispatchRequest) (factoryapi.FactorySessionLifecycleControlResponse, error),
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

	response, err := invoke(lifecycle, req)
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
		req factoryapi.FactorySessionApproveRequest,
	) (factoryapi.FactorySessionLifecycleControlResponse, error) {
		return lifecycle.ApproveDurableFactorySession(r.Context(), string(sessionID), req)
	})
}

func (s *Server) PauseFactorySession(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID) {
	if isDurableExecutionSessionID(string(sessionID)) {
		s.handleDurableLifecycleControl(w, r, sessionID, "pause", func(
			lifecycle apisurface.DurableSessionLifecycleAPI,
			req factoryapi.FactorySessionLifecycleControlRequest,
		) (factoryapi.FactorySessionLifecycleControlResponse, error) {
			return lifecycle.PauseDurableFactorySession(r.Context(), string(sessionID), req)
		})
		return
	}
	s.handleLiveLifecycleControl(w, r, sessionID, "pause", func(
		sessionRuntime apisurface.SessionAPI,
		req factoryapi.FactorySessionLifecycleControlRequest,
	) (factoryapi.FactorySessionLifecycleControlResponse, error) {
		return sessionRuntime.PauseLiveFactorySession(r.Context(), string(sessionID), req)
	})
}

func (s *Server) ResumeFactorySession(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID) {
	if isDurableExecutionSessionID(string(sessionID)) {
		s.handleDurableLifecycleControl(w, r, sessionID, "resume", func(
			lifecycle apisurface.DurableSessionLifecycleAPI,
			req factoryapi.FactorySessionLifecycleControlRequest,
		) (factoryapi.FactorySessionLifecycleControlResponse, error) {
			return lifecycle.ResumeDurableFactorySession(r.Context(), string(sessionID), req)
		})
		return
	}
	s.handleLiveLifecycleControl(w, r, sessionID, "resume", func(
		sessionRuntime apisurface.SessionAPI,
		req factoryapi.FactorySessionLifecycleControlRequest,
	) (factoryapi.FactorySessionLifecycleControlResponse, error) {
		return sessionRuntime.ResumeLiveFactorySession(r.Context(), string(sessionID), req)
	})
}

func (s *Server) CancelFactorySession(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID) {
	s.handleDurableLifecycleControl(w, r, sessionID, "cancel", func(
		lifecycle apisurface.DurableSessionLifecycleAPI,
		req factoryapi.FactorySessionLifecycleControlRequest,
	) (factoryapi.FactorySessionLifecycleControlResponse, error) {
		return lifecycle.CancelDurableFactorySession(r.Context(), string(sessionID), req)
	})
}

func (s *Server) TerminateFactorySession(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID) {
	s.handleDurableLifecycleControl(w, r, sessionID, "terminate", func(
		lifecycle apisurface.DurableSessionLifecycleAPI,
		req factoryapi.FactorySessionLifecycleControlRequest,
	) (factoryapi.FactorySessionLifecycleControlResponse, error) {
		return lifecycle.TerminateDurableFactorySession(r.Context(), string(sessionID), req)
	})
}

func (s *Server) RetryFactorySessionDispatch(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID) {
	s.handleDurableRetryDispatchControl(w, r, sessionID, func(
		lifecycle apisurface.DurableSessionLifecycleAPI,
		req factoryapi.FactorySessionRetryDispatchRequest,
	) (factoryapi.FactorySessionLifecycleControlResponse, error) {
		return lifecycle.RetryDurableFactorySessionDispatch(r.Context(), string(sessionID), req)
	})
}

func (s *Server) handleDurableInterruptDispatchControl(
	w http.ResponseWriter,
	r *http.Request,
	sessionID factoryapi.SessionID,
	invoke func(apisurface.DurableSessionLifecycleAPI, factoryapi.FactorySessionInterruptDispatchRequest) (factoryapi.FactorySessionLifecycleControlResponse, error),
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

	response, err := invoke(lifecycle, req)
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
