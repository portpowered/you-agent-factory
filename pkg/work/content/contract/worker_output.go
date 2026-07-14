package contentcontract

import (
	"encoding/json"
	"strings"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// ContentFromWorkerOutput maps one workstation response body onto canonical work
// content. JSON WorkContent, content envelopes, and part arrays are parsed when
// present; otherwise the raw body becomes one text part.
func ContentFromWorkerOutput(raw string) ([]interfaces.WorkContentPart, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}

	var content factoryapi.WorkContent
	if err := json.Unmarshal([]byte(trimmed), &content); err == nil && len(content) > 0 {
		return PartsFromGenerated(&content), nil
	}

	var envelope struct {
		Content factoryapi.WorkContent `json:"content"`
	}
	if err := json.Unmarshal([]byte(trimmed), &envelope); err == nil && len(envelope.Content) > 0 {
		return PartsFromGenerated(&envelope.Content), nil
	}

	var parts []interfaces.WorkContentPart
	if err := json.Unmarshal([]byte(trimmed), &parts); err == nil && len(parts) > 0 {
		return parts, nil
	}

	return []interfaces.WorkContentPart{{
		Type: interfaces.WorkContentPartTypeText,
		Text: raw,
	}}, nil
}
