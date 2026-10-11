package globalconfig

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

// The optional generated pointer collapses explicit null into absence. Inspect
// the authored record first so an invalid route never becomes an omitted route.
func validateSubagentJSON(raw []byte) error {
	var document struct {
		Defaults map[string]json.RawMessage `json:"defaults"`
	}
	if err := json.Unmarshal(raw, &document); err != nil {
		return err
	}
	route, present := document.Defaults["subagent"]
	if !present {
		return nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(route, &fields); err != nil || fields == nil {
		return fmt.Errorf("%w: defaults.subagent must be an object", operatorsettings.ErrDocumentMalformed)
	}
	for _, key := range []string{"provider", "model"} {
		if _, present := fields[key]; !present {
			return fmt.Errorf("%w: defaults.subagent.%s is required", operatorsettings.ErrDocumentMalformed, key)
		}
	}
	for key, value := range fields {
		if key != "provider" && key != "model" && key != "reasoningEffort" {
			return fmt.Errorf("%w: defaults.subagent contains an unknown property", operatorsettings.ErrDocumentMalformed)
		}
		var text string
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) || json.Unmarshal(value, &text) != nil || strings.TrimSpace(text) == "" {
			return fmt.Errorf("%w: defaults.subagent.%s must be a non-empty string", operatorsettings.ErrDocumentMalformed, key)
		}
	}
	return nil
}
