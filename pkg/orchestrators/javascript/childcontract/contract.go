// Package childcontract defines the beta JavaScript child-agent argument contract.
package childcontract

import (
	"fmt"
	"sort"
	"strings"
)

const (
	FieldPrompt          = "prompt"
	FieldLabel           = "label"
	FieldPreset          = "preset"
	FieldModelProvider   = "modelProvider"
	FieldModel           = "model"
	FieldReasoningEffort = "reasoningEffort"
)

var supportedFields = []string{
	FieldPrompt,
	FieldLabel,
	FieldPreset,
	FieldModelProvider,
	FieldModel,
	FieldReasoningEffort,
}

var supportedFieldSet = func() map[string]struct{} {
	set := make(map[string]struct{}, len(supportedFields))
	for _, field := range supportedFields {
		set[field] = struct{}{}
	}
	return set
}()

// Spec is the normalized supported argument set for one agent.run call.
type Spec struct {
	Prompt          string
	Label           string
	Preset          string
	ModelProvider   string
	Model           string
	ReasoningEffort string
}

// SupportedFields returns the canonical beta agent.run field names.
func SupportedFields() []string {
	return append([]string(nil), supportedFields...)
}

// IsSupportedField reports whether name belongs to the beta agent.run contract.
func IsSupportedField(name string) bool {
	_, ok := supportedFieldSet[name]
	return ok
}

// Normalize validates and trims one dynamically constructed agent.run object.
func Normalize(value map[string]any) (Spec, error) {
	unknown := make([]string, 0)
	for field := range value {
		if !IsSupportedField(field) {
			unknown = append(unknown, field)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return Spec{}, fmt.Errorf(`agent.run() does not support field %q`, unknown[0])
	}

	prompt, err := requiredString(value, FieldPrompt)
	if err != nil {
		return Spec{}, err
	}
	optional := make(map[string]string, len(supportedFields)-1)
	for _, field := range supportedFields[1:] {
		optional[field], err = optionalString(value, field)
		if err != nil {
			return Spec{}, err
		}
	}
	return Spec{
		Prompt:          prompt,
		Label:           optional[FieldLabel],
		Preset:          optional[FieldPreset],
		ModelProvider:   optional[FieldModelProvider],
		Model:           optional[FieldModel],
		ReasoningEffort: optional[FieldReasoningEffort],
	}, nil
}

func requiredString(value map[string]any, field string) (string, error) {
	normalized, err := optionalString(value, field)
	if err != nil || normalized != "" {
		return normalized, err
	}
	return "", fmt.Errorf(`agent.run() requires a non-empty string %q property`, field)
}

func optionalString(value map[string]any, field string) (string, error) {
	raw, found := value[field]
	if !found {
		return "", nil
	}
	text, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf(`agent.run() requires %q to be a string`, field)
	}
	return strings.TrimSpace(text), nil
}
