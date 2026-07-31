// Package childcontract defines the beta JavaScript child-agent argument contract.
package orchestratorcontract

import (
	"fmt"
	"sort"
	"strings"
)

const (
	FieldPrompt           = "prompt"
	FieldLabel            = "label"
	FieldPreset           = "preset"
	FieldExecutorProvider = "executorProvider"
	FieldModelProvider    = "modelProvider"
	FieldModel            = "model"
	FieldReasoningEffort  = "reasoningEffort"
	FieldSkipPermissions  = "skipPermissions"
)

var supportedFields = []string{
	FieldPrompt,
	FieldLabel,
	FieldPreset,
	FieldExecutorProvider,
	FieldModelProvider,
	FieldModel,
	FieldReasoningEffort,
	FieldSkipPermissions,
}

var supportedFieldSet = func() map[string]struct{} {
	set := make(map[string]struct{}, len(supportedFields))
	for _, field := range supportedFields {
		set[field] = struct{}{}
	}
	return set
}()

// Spec is the normalized supported argument set for one agent.run call.
type JavaScriptChildSpec struct {
	Prompt           string
	Label            string
	Preset           string
	ExecutorProvider string
	ModelProvider    string
	Model            string
	ReasoningEffort  string
	SkipPermissions  bool
}

// SupportedFields returns the canonical beta agent.run field names.
func JavaScriptChildSupportedFields() []string {
	return append([]string(nil), supportedFields...)
}

// IsSupportedField reports whether name belongs to the beta agent.run contract.
func IsJavaScriptChildSupportedField(name string) bool {
	_, ok := supportedFieldSet[name]
	return ok
}

// Normalize validates and trims one dynamically constructed agent.run object.
func NormalizeJavaScriptChild(value map[string]any) (JavaScriptChildSpec, error) {
	unknown := make([]string, 0)
	for field := range value {
		if !IsJavaScriptChildSupportedField(field) {
			unknown = append(unknown, field)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return JavaScriptChildSpec{}, fmt.Errorf(`agent.run() does not support field %q`, unknown[0])
	}

	prompt, err := requiredString(value, FieldPrompt)
	if err != nil {
		return JavaScriptChildSpec{}, err
	}
	optional := make(map[string]string, len(supportedFields)-2)
	for _, field := range supportedFields[1 : len(supportedFields)-1] {
		optional[field], err = optionalString(value, field)
		if err != nil {
			return JavaScriptChildSpec{}, err
		}
	}
	skipPermissions, err := optionalBool(value, FieldSkipPermissions)
	if err != nil {
		return JavaScriptChildSpec{}, err
	}
	return JavaScriptChildSpec{
		Prompt:           prompt,
		Label:            optional[FieldLabel],
		Preset:           optional[FieldPreset],
		ExecutorProvider: optional[FieldExecutorProvider],
		ModelProvider:    optional[FieldModelProvider],
		Model:            optional[FieldModel],
		ReasoningEffort:  optional[FieldReasoningEffort],
		SkipPermissions:  skipPermissions,
	}, nil
}

func optionalBool(value map[string]any, field string) (bool, error) {
	raw, found := value[field]
	if !found {
		return false, nil
	}
	normalized, ok := raw.(bool)
	if !ok {
		return false, fmt.Errorf(`agent.run() requires %q to be a boolean`, field)
	}
	return normalized, nil
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
