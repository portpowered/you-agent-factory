package http

import (
	"encoding/json"
	"fmt"
)

func validateWorkContentField(fields map[string]json.RawMessage, prefix string) error {
	contentRaw, ok := fields["content"]
	if !ok {
		return nil
	}
	var payloads []json.RawMessage
	if err := json.Unmarshal(contentRaw, &payloads); err != nil {
		return requestValidationError{message: fmt.Sprintf("%scontent must be an array", prefix)}
	}
	for i, payload := range payloads {
		var partFields map[string]json.RawMessage
		if err := json.Unmarshal(payload, &partFields); err != nil {
			return requestValidationError{message: fmt.Sprintf("%scontent[%d] must be an object", prefix, i)}
		}
		if err := validateWorkContentPart(partFields, fmt.Sprintf("%scontent[%d].", prefix, i)); err != nil {
			return err
		}
	}
	return nil
}

func validateWorkContentPart(fields map[string]json.RawMessage, prefix string) error {
	partType, err := requiredPartType(fields, prefix)
	if err != nil {
		return err
	}
	switch partType {
	case "text", "TEXT":
		return validateTextPart(fields, prefix)
	case "image", "IMAGE":
		return validateURLPart(fields, prefix, "image content parts")
	case "AUDIO":
		return validateURLPart(fields, prefix, "audio content parts")
	case "JSON":
		return validateJSONPart(fields, prefix)
	case "BINARY":
		return validateURLPart(fields, prefix, "binary content parts")
	default:
		return requestValidationError{message: fmt.Sprintf("%stype must be one of text, image, TEXT, IMAGE, AUDIO, JSON, or BINARY", prefix)}
	}
}

func requiredPartType(fields map[string]json.RawMessage, prefix string) (string, error) {
	raw, ok := fields["type"]
	if !ok {
		return "", requestValidationError{message: fmt.Sprintf("%stype is required", prefix)}
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil || value == "" {
		return "", requestValidationError{message: fmt.Sprintf("%stype must be a non-empty string", prefix)}
	}
	return value, nil
}

func validateTextPart(fields map[string]json.RawMessage, prefix string) error {
	if err := requireOnlyPartFields(fields, prefix, "type", "text", "label", "role", "contentType", "artifactId", "metadata"); err != nil {
		return err
	}
	if _, err := requiredPartString(fields, prefix, "text", "text content parts"); err != nil {
		return err
	}
	return validateSharedPartFields(fields, prefix)
}

func validateURLPart(fields map[string]json.RawMessage, prefix, usage string) error {
	if err := requireOnlyPartFields(fields, prefix, "type", "url", "file", "label", "role", "contentType", "artifactId", "metadata"); err != nil {
		return err
	}
	if err := validateSharedPartFields(fields, prefix); err != nil {
		return err
	}
	hasFile, err := hasNonEmptyPartString(fields, prefix, "file")
	if err != nil {
		return err
	}
	hasURL, err := hasNonEmptyPartString(fields, prefix, "url")
	if err != nil {
		return err
	}
	if !hasURL && !hasFile {
		return requestValidationError{message: fmt.Sprintf("%surl is required for %s", prefix, usage)}
	}
	return nil
}

func hasNonEmptyPartString(fields map[string]json.RawMessage, prefix, field string) (bool, error) {
	raw, ok := fields[field]
	if !ok {
		return false, nil
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil || value == "" {
		return false, requestValidationError{message: fmt.Sprintf("%s%s must be a non-empty string when provided", prefix, field)}
	}
	return true, nil
}

func validateJSONPart(fields map[string]json.RawMessage, prefix string) error {
	if err := requireOnlyPartFields(fields, prefix, "type", "json", "label", "role", "contentType", "artifactId", "metadata"); err != nil {
		return err
	}
	raw, ok := fields["json"]
	if !ok {
		return requestValidationError{message: fmt.Sprintf("%sjson is required for JSON content parts", prefix)}
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return requestValidationError{message: fmt.Sprintf("%sjson must be valid JSON", prefix)}
	}
	return validateSharedPartFields(fields, prefix)
}

func requiredPartString(fields map[string]json.RawMessage, prefix, field, usage string) (string, error) {
	raw, ok := fields[field]
	if !ok {
		return "", requestValidationError{message: fmt.Sprintf("%s%s is required for %s", prefix, field, usage)}
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", requestValidationError{message: fmt.Sprintf("%s%s must be a string", prefix, field)}
	}
	return value, nil
}

func validateSharedPartFields(fields map[string]json.RawMessage, prefix string) error {
	for _, field := range []string{"label", "role", "contentType", "artifactId"} {
		if raw, ok := fields[field]; ok {
			var value string
			if err := json.Unmarshal(raw, &value); err != nil {
				return requestValidationError{message: fmt.Sprintf("%s%s must be a string", prefix, field)}
			}
		}
	}
	if raw, ok := fields["metadata"]; ok {
		var metadata map[string]any
		if err := json.Unmarshal(raw, &metadata); err != nil || metadata == nil {
			return requestValidationError{message: fmt.Sprintf("%smetadata must be an object", prefix)}
		}
	}
	return nil
}

func requireOnlyPartFields(fields map[string]json.RawMessage, prefix string, allowed ...string) error {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, field := range allowed {
		allowedSet[field] = struct{}{}
	}
	for field := range fields {
		if _, ok := allowedSet[field]; !ok {
			return requestValidationError{message: fmt.Sprintf("%s%s is not supported", prefix, field)}
		}
	}
	return nil
}
