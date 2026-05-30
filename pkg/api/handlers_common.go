package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface/optional"
	factoryrequests "github.com/portpowered/infinite-you/pkg/factory/requests"
	"github.com/portpowered/infinite-you/pkg/interfaces"
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
	if err := factoryrequests.ValidateCanonicalWorkRequestJSON(data); err != nil {
		return translateCanonicalWorkRequestValidationError(err)
	}
	return nil
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

func validatedRawWorkContentPart(fields map[string]json.RawMessage, prefix string) (interfaces.WorkContentPart, error) {
	partType, err := requiredWorkContentPartType(fields, prefix)
	if err != nil {
		return interfaces.WorkContentPart{}, err
	}

	switch partType {
	case interfaces.WorkContentPartTypeText:
		return validatedRawTextContentPart(fields, prefix)
	case interfaces.WorkContentPartTypeImage:
		return validatedRawFileContentPart(fields, prefix, interfaces.WorkContentPartTypeImage, "image content parts")
	case interfaces.WorkContentPartTypeAudio:
		return validatedRawFileContentPart(fields, prefix, interfaces.WorkContentPartTypeAudio, "audio content parts")
	case interfaces.WorkContentPartTypeJSON:
		return validatedRawJSONContentPart(fields, prefix)
	case interfaces.WorkContentPartTypeBinary:
		return validatedRawFileContentPart(fields, prefix, interfaces.WorkContentPartTypeBinary, "binary content parts")
	default:
		return interfaces.WorkContentPart{}, requestFieldValidationError{message: fmt.Sprintf("%stype must be one of text, image, TEXT, IMAGE, AUDIO, JSON, or BINARY", prefix)}
	}
}

func requiredWorkContentPartType(fields map[string]json.RawMessage, prefix string) (interfaces.WorkContentPartType, error) {
	typeRaw, ok := fields["type"]
	if !ok {
		return "", requestFieldValidationError{message: fmt.Sprintf("%stype is required", prefix)}
	}

	var partType string
	if err := json.Unmarshal(typeRaw, &partType); err != nil || partType == "" {
		return "", requestFieldValidationError{message: fmt.Sprintf("%stype must be a non-empty string", prefix)}
	}

	return interfaces.WorkContentPartType(partType).Normalized(), nil
}

func validatedRawTextContentPart(fields map[string]json.RawMessage, prefix string) (interfaces.WorkContentPart, error) {
	if err := requireOnlyFields(fields, prefix, "type", "text", "label", "role", "contentType", "artifactId", "metadata"); err != nil {
		return interfaces.WorkContentPart{}, err
	}
	text, err := requiredStringField(fields, prefix, "text", "text content parts")
	if err != nil {
		return interfaces.WorkContentPart{}, err
	}
	shared, err := validateSharedWorkContentFields(fields, prefix)
	if err != nil {
		return interfaces.WorkContentPart{}, err
	}
	shared.Type = interfaces.WorkContentPartTypeText
	shared.Text = text
	return shared, nil
}

func validatedRawFileContentPart(fields map[string]json.RawMessage, prefix string, partType interfaces.WorkContentPartType, usage string) (interfaces.WorkContentPart, error) {
	if err := requireOnlyFields(fields, prefix, "type", "file", "label", "role", "contentType", "artifactId", "metadata"); err != nil {
		return interfaces.WorkContentPart{}, err
	}
	file, err := requiredNonEmptyStringField(fields, prefix, "file", usage)
	if err != nil {
		return interfaces.WorkContentPart{}, err
	}
	shared, err := validateSharedWorkContentFields(fields, prefix)
	if err != nil {
		return interfaces.WorkContentPart{}, err
	}
	shared.Type = partType
	shared.File = file
	return shared, nil
}

func validatedRawJSONContentPart(fields map[string]json.RawMessage, prefix string) (interfaces.WorkContentPart, error) {
	if err := requireOnlyFields(fields, prefix, "type", "json", "label", "role", "contentType", "artifactId", "metadata"); err != nil {
		return interfaces.WorkContentPart{}, err
	}
	jsonRaw, ok := fields["json"]
	if !ok {
		return interfaces.WorkContentPart{}, requestFieldValidationError{message: fmt.Sprintf("%sjson is required for JSON content parts", prefix)}
	}
	var value any
	if err := json.Unmarshal(jsonRaw, &value); err != nil {
		return interfaces.WorkContentPart{}, requestFieldValidationError{message: fmt.Sprintf("%sjson must be valid JSON", prefix)}
	}
	shared, err := validateSharedWorkContentFields(fields, prefix)
	if err != nil {
		return interfaces.WorkContentPart{}, err
	}
	shared.Type = interfaces.WorkContentPartTypeJSON
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

func validateSharedWorkContentFields(fields map[string]json.RawMessage, prefix string) (interfaces.WorkContentPart, error) {
	part := interfaces.WorkContentPart{}

	label, err := optionalStringField(fields, prefix, "label")
	if err != nil {
		return interfaces.WorkContentPart{}, err
	}
	role, err := optionalStringField(fields, prefix, "role")
	if err != nil {
		return interfaces.WorkContentPart{}, err
	}
	contentType, err := optionalStringField(fields, prefix, "contentType")
	if err != nil {
		return interfaces.WorkContentPart{}, err
	}
	artifactID, err := optionalStringField(fields, prefix, "artifactId")
	if err != nil {
		return interfaces.WorkContentPart{}, err
	}
	part.Label = label
	part.Role = role
	part.ContentType = contentType
	part.ArtifactID = artifactID

	if metadataRaw, ok := fields["metadata"]; ok {
		var metadata map[string]any
		if err := json.Unmarshal(metadataRaw, &metadata); err != nil || metadata == nil {
			return interfaces.WorkContentPart{}, requestFieldValidationError{message: fmt.Sprintf("%smetadata must be an object", prefix)}
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

func translateCanonicalWorkRequestValidationError(err error) error {
	if err == nil {
		return nil
	}

	message := err.Error()
	message = strings.TrimPrefix(message, "work request batch ")
	message = strings.ReplaceAll(message, " uses retired work_type_id field; use workTypeName", ".work_type_id is not supported; use workTypeName")
	message = strings.ReplaceAll(message, " uses retired target_state field; use state", ".target_state is not supported; use state")
	if strings.HasPrefix(message, "works[") && strings.Contains(message, "] ") {
		message = strings.Replace(message, "] ", "].", 1)
	}
	if strings.HasSuffix(message, ".work_type_id is not supported; use workTypeName") ||
		strings.HasSuffix(message, ".target_state is not supported; use state") {
		return requestFieldValidationError{message: message}
	}
	switch message {
	case "uses retired work_type_id field; use workTypeName":
		return requestFieldValidationError{message: "work_type_id is not supported; use workTypeName"}
	case "uses retired target_state field; use state":
		return requestFieldValidationError{message: "target_state is not supported; use state"}
	default:
		return requestFieldValidationError{message: message}
	}
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
