package service

import (
	"context"
	"reflect"
	"strings"

	acpsdk "github.com/coder/acp-go-sdk"
)

func acpClientCapabilities() acpsdk.ClientCapabilities {
	return acpsdk.ClientCapabilities{Elicitation: &acpsdk.ElicitationCapabilities{
		Form: &acpsdk.ElicitationFormCapabilities{},
	}}
}

// UnstableCreateElicitation answers noninteractive form choices. URL mode
// requires an external user interaction and is not advertised by this client.
func (c *client) UnstableCreateElicitation(ctx context.Context, request acpsdk.UnstableCreateElicitationRequest) (acpsdk.UnstableCreateElicitationResponse, error) {
	if ctx.Err() != nil || request.Form == nil {
		return cancelElicitation(), nil
	}
	content := make(map[string]any)
	for name, raw := range request.Form.RequestedSchema.Properties {
		property, ok := raw.(map[string]any)
		if !ok {
			if requiredElicitationProperty(request.Form.RequestedSchema.Required, name) {
				return cancelElicitation(), nil
			}
			continue
		}
		value, ok := affirmativeElicitationValue(property)
		if !ok {
			if requiredElicitationProperty(request.Form.RequestedSchema.Required, name) {
				return cancelElicitation(), nil
			}
			continue
		}
		content[name] = value
	}
	for _, name := range request.Form.RequestedSchema.Required {
		if _, ok := content[name]; !ok {
			return cancelElicitation(), nil
		}
	}
	return acpsdk.UnstableCreateElicitationResponse{Accept: &acpsdk.UnstableCreateElicitationAccept{
		Action: "accept", Content: content,
	}}, nil
}

func cancelElicitation() acpsdk.UnstableCreateElicitationResponse {
	return acpsdk.UnstableCreateElicitationResponse{Cancel: &acpsdk.UnstableCreateElicitationCancel{Action: "cancel"}}
}

func requiredElicitationProperty(required []string, name string) bool {
	for _, value := range required {
		if value == name {
			return true
		}
	}
	return false
}

func affirmativeElicitationValue(property map[string]any) (any, bool) {
	if options, ok := property["oneOf"].([]any); ok {
		choices := make([]any, 0, len(options))
		for _, raw := range options {
			option, ok := raw.(map[string]any)
			if ok {
				if value, exists := option["const"]; exists {
					choices = append(choices, value)
				}
			}
		}
		return chooseElicitationValue(choices, property["default"])
	}
	if options, ok := property["enum"].([]any); ok {
		return chooseElicitationValue(options, property["default"])
	}
	if value, ok := property["const"]; ok {
		return value, true
	}
	if property["type"] == "boolean" {
		return true, true
	}
	if value, ok := property["default"]; ok {
		return value, true
	}
	return nil, false
}

func chooseElicitationValue(choices []any, defaultValue any) (any, bool) {
	for _, choice := range choices {
		if affirmativeElicitationChoice(choice) {
			return choice, true
		}
	}
	for _, choice := range choices {
		if defaultValue != nil && reflect.DeepEqual(choice, defaultValue) {
			return choice, true
		}
	}
	if len(choices) > 0 {
		return choices[0], true
	}
	return nil, false
}

func affirmativeElicitationChoice(value any) bool {
	if value == true {
		return true
	}
	label, ok := value.(string)
	if !ok {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "allow", "allow always", "always", "approve", "approved", "yes", "true", "accept", "accepted", "continue", "proceed", "enable", "enabled":
		return true
	default:
		return false
	}
}
