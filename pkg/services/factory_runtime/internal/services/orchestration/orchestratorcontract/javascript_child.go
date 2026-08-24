// Package childcontract defines the beta JavaScript child-agent argument contract.
package orchestratorcontract

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	FieldPrompt           = "prompt"
	FieldLabel            = "label"
	FieldPreset           = "preset"
	FieldExecutorProvider = "executorProvider"
	FieldModelProvider    = "modelProvider"
	FieldModel            = "model"
	FieldReasoningEffort  = "reasoningEffort"
	FieldResourceID       = "resourceId"
	FieldSchema           = "schema"
	FieldPermissions      = "permissions"
)

// JavaScriptChildPermission is the provider permission behavior requested by
// one JavaScript agent.run child.
type JavaScriptChildPermission string

const (
	JavaScriptChildPermissionDefault         JavaScriptChildPermission = "DEFAULT"
	JavaScriptChildPermissionSkipPermissions JavaScriptChildPermission = "SKIP_PERMISSIONS"
)

// JavaScriptChildFieldDescriptor is the runtime-owned contract for one
// agent.run spec property. JSONType is the JSON value type accepted by the
// request normalizer and emitted by generated runtime projections.
type JavaScriptChildFieldDescriptor struct {
	Name                 string
	JSONType             string
	Required             bool
	Enum                 []string
	AdditionalProperties *bool
}

var schemaAdditionalProperties = false

var javaScriptChildFieldDescriptors = [...]JavaScriptChildFieldDescriptor{
	{Name: FieldPrompt, JSONType: "string", Required: true},
	{Name: FieldLabel, JSONType: "string"},
	{Name: FieldPreset, JSONType: "string"},
	{Name: FieldExecutorProvider, JSONType: "string"},
	{Name: FieldModelProvider, JSONType: "string"},
	{Name: FieldModel, JSONType: "string"},
	{Name: FieldReasoningEffort, JSONType: "string"},
	{Name: FieldResourceID, JSONType: "string"},
	{Name: FieldSchema, JSONType: "object", AdditionalProperties: &schemaAdditionalProperties},
	{Name: FieldPermissions, JSONType: "string", Enum: []string{
		string(JavaScriptChildPermissionDefault),
		string(JavaScriptChildPermissionSkipPermissions),
	}},
}

// Spec is the normalized supported argument set for one agent.run call.
type JavaScriptChildSpec struct {
	Prompt           string
	Label            string
	Preset           string
	ExecutorProvider string
	ModelProvider    string
	Model            string
	ReasoningEffort  string
	ResourceID       string
	Schema           map[string]any
	Permissions      JavaScriptChildPermission
}

// SupportedFields returns the canonical beta agent.run field names.
func JavaScriptChildSupportedFields() []string {
	fields := make([]string, 0, len(javaScriptChildFieldDescriptors))
	for _, descriptor := range javaScriptChildFieldDescriptors {
		fields = append(fields, descriptor.Name)
	}
	return fields
}

// JavaScriptChildFieldDescriptors returns a detached copy of the immutable
// runtime field contract for generation and other representation projections.
func JavaScriptChildFieldDescriptors() []JavaScriptChildFieldDescriptor {
	fields := append([]JavaScriptChildFieldDescriptor(nil), javaScriptChildFieldDescriptors[:]...)
	for index := range fields {
		fields[index].Enum = append([]string(nil), fields[index].Enum...)
		if fields[index].AdditionalProperties != nil {
			additionalProperties := *fields[index].AdditionalProperties
			fields[index].AdditionalProperties = &additionalProperties
		}
	}
	return fields
}

// IsSupportedField reports whether name belongs to the beta agent.run contract.
func IsJavaScriptChildSupportedField(name string) bool {
	for _, descriptor := range javaScriptChildFieldDescriptors {
		if descriptor.Name == name {
			return true
		}
	}
	return false
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
		return JavaScriptChildSpec{}, unsupportedJavaScriptChildFieldError(unknown[0])
	}

	fields, err := normalizeJavaScriptChildFields(value)
	if err != nil {
		return JavaScriptChildSpec{}, err
	}
	return JavaScriptChildSpec{
		Prompt:           fields.strings[FieldPrompt],
		Label:            fields.strings[FieldLabel],
		Preset:           fields.strings[FieldPreset],
		ExecutorProvider: fields.strings[FieldExecutorProvider],
		ModelProvider:    fields.strings[FieldModelProvider],
		Model:            fields.strings[FieldModel],
		ReasoningEffort:  fields.strings[FieldReasoningEffort],
		ResourceID:       fields.strings[FieldResourceID],
		Schema:           fields.schema,
		Permissions:      fields.permissions,
	}, nil
}

type normalizedJavaScriptChildFields struct {
	strings     map[string]string
	schema      map[string]any
	permissions JavaScriptChildPermission
}

func normalizeJavaScriptChildFields(value map[string]any) (normalizedJavaScriptChildFields, error) {
	fields := normalizedJavaScriptChildFields{
		strings:     make(map[string]string, len(javaScriptChildFieldDescriptors)),
		permissions: JavaScriptChildPermissionDefault,
	}
	for _, descriptor := range javaScriptChildFieldDescriptors {
		switch descriptor.JSONType {
		case "string":
			if descriptor.Name == FieldPermissions {
				var err error
				fields.permissions, err = optionalPermission(value, descriptor.Name)
				if err != nil {
					return normalizedJavaScriptChildFields{}, err
				}
				continue
			}
			normalized, err := normalizeJavaScriptChildString(value, descriptor)
			if err != nil {
				return normalizedJavaScriptChildFields{}, err
			}
			fields.strings[descriptor.Name] = normalized
		case "object":
			if descriptor.Name != FieldSchema {
				return normalizedJavaScriptChildFields{}, fmt.Errorf("agent.run() contract has unsupported object field %q", descriptor.Name)
			}
			var err error
			fields.schema, err = optionalSchema(value, descriptor.Name)
			if err != nil {
				return normalizedJavaScriptChildFields{}, err
			}
		default:
			return normalizedJavaScriptChildFields{}, fmt.Errorf("agent.run() contract has unsupported JSON type %q for %q", descriptor.JSONType, descriptor.Name)
		}
	}
	return fields, nil
}

func normalizeJavaScriptChildString(value map[string]any, descriptor JavaScriptChildFieldDescriptor) (string, error) {
	if descriptor.Required {
		return requiredString(value, descriptor.Name)
	}
	return optionalString(value, descriptor.Name)
}

const childOutputSchemaURL = "https://schemas.portpowered.com/you/runtime/child-output-schema.json"

func optionalSchema(value map[string]any, field string) (map[string]any, error) {
	raw, found := value[field]
	if !found {
		return nil, nil
	}
	schema, ok := raw.(map[string]any)
	if !ok || schema == nil {
		return nil, fmt.Errorf(`agent.run() requires %q to be an object`, field)
	}

	cloned, err := cloneAndValidateSchema(schema)
	if err != nil {
		return nil, fmt.Errorf(`agent.run() requires %q to be a valid JSON Schema object`, field)
	}
	return cloned, nil
}

func cloneAndValidateSchema(schema map[string]any) (map[string]any, error) {
	raw, err := json.Marshal(schema)
	if err != nil {
		return nil, err
	}
	var cloned map[string]any
	if err := json.Unmarshal(raw, &cloned); err != nil {
		return nil, err
	}

	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	if err := compiler.AddResource(childOutputSchemaURL, cloned); err != nil {
		return nil, err
	}
	if _, err := compiler.Compile(childOutputSchemaURL); err != nil {
		return nil, err
	}
	return cloned, nil
}

func optionalPermission(value map[string]any, field string) (JavaScriptChildPermission, error) {
	raw, found := value[field]
	if !found {
		return JavaScriptChildPermissionDefault, nil
	}
	text, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf(`agent.run() requires %q to be a string`, field)
	}
	permission := JavaScriptChildPermission(text)
	switch permission {
	case JavaScriptChildPermissionDefault, JavaScriptChildPermissionSkipPermissions:
		return permission, nil
	default:
		return "", fmt.Errorf(`agent.run() requires %q to be DEFAULT or SKIP_PERMISSIONS`, field)
	}
}

func unsupportedJavaScriptChildFieldError(field string) error {
	return fmt.Errorf("%s", UnsupportedJavaScriptChildFieldMessage(field))
}

// UnsupportedJavaScriptChildFieldMessage returns the stable diagnostic for a
// field outside the closed agent.run object shape.
func UnsupportedJavaScriptChildFieldMessage(field string) string {
	if field == legacyJavaScriptPermissionField() {
		return fmt.Sprintf(`agent.run() no longer supports field %q; use %q with %q or %q instead`, field, FieldPermissions, JavaScriptChildPermissionDefault, JavaScriptChildPermissionSkipPermissions)
	}
	return fmt.Sprintf(`agent.run() does not support field %q`, field)
}

// Keep the retired spelling out of the live contract while retaining a clear
// migration diagnostic for callers that still send it.
func legacyJavaScriptPermissionField() string {
	return "skip" + "Permissions"
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
