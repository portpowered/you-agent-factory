package http

import (
	"encoding/json"
	"errors"
	"fmt"
)

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
