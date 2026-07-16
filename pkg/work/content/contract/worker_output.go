package contentcontract

import (
	"encoding/json"
	"strings"

	"github.com/portpowered/infinite-you/pkg/work"
)

// ContentFromWorkerOutput maps one workstation response body onto canonical work
// content. JSON WorkContent, content envelopes, and part arrays are parsed when
// present; otherwise the raw body becomes one text part.
func ContentFromWorkerOutput(raw string) ([]work.WorkContentPart, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}

	var content []work.WorkContentPart
	if err := json.Unmarshal([]byte(trimmed), &content); err == nil && len(content) > 0 {
		return SupportedParts(content), nil
	}

	var envelope struct {
		Content []work.WorkContentPart `json:"content"`
	}
	if err := json.Unmarshal([]byte(trimmed), &envelope); err == nil && len(envelope.Content) > 0 {
		return SupportedParts(envelope.Content), nil
	}

	return []work.WorkContentPart{{
		Type: work.WorkContentPartTypeText,
		Text: raw,
	}}, nil
}

// SupportedParts returns content with public aliases normalized and unknown
// part kinds omitted, preserving the established public-contract behavior.
func SupportedParts(parts []work.WorkContentPart) []work.WorkContentPart {
	supported := make([]work.WorkContentPart, 0, len(parts))
	for _, part := range parts {
		part.Type = part.Type.Normalized()
		switch part.Type {
		case work.WorkContentPartTypeText,
			work.WorkContentPartTypeImage,
			work.WorkContentPartTypeAudio,
			work.WorkContentPartTypeJSON,
			work.WorkContentPartTypeBinary:
			supported = append(supported, part)
		}
	}
	return supported
}
