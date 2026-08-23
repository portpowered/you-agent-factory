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
	FieldResourceID       = "resourceId"
	FieldPermissions      = "permissions"
	FieldSkipPermissions  = "skipPermissions"
)

// JavaScriptChildPermission is the provider permission behavior requested by
// one JavaScript agent.run child.
type JavaScriptChildPermission string

const (
	JavaScriptChildPermissionDefault         JavaScriptChildPermission = "DEFAULT"
	JavaScriptChildPermissionSkipPermissions JavaScriptChildPermission = "SKIP_PERMISSIONS"
)

var supportedFields = []string{
	FieldPrompt,
	FieldLabel,
	FieldPreset,
	FieldExecutorProvider,
	FieldModelProvider,
	FieldModel,
	FieldReasoningEffort,
	FieldResourceID,
	FieldPermissions,
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
	Prompt                       string
	Label                        string
	Preset                       string
	ExecutorProvider             string
	ModelProvider                string
	Model                        string
	ReasoningEffort              string
	ResourceID                   string
	Permissions                  JavaScriptChildPermission
	SkipPermissions              bool
	LegacySkipPermissionsPresent bool
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
	optional := make(map[string]string, len(supportedFields)-3)
	for _, field := range supportedFields {
		switch field {
		case FieldPrompt, FieldPermissions, FieldSkipPermissions:
			continue
		default:
			optional[field], err = optionalString(value, field)
			if err != nil {
				return JavaScriptChildSpec{}, err
			}
		}
	}
	permissions, permissionsPresent, err := optionalPermission(value, FieldPermissions)
	if err != nil {
		return JavaScriptChildSpec{}, err
	}
	skipPermissions, skipPermissionsPresent, err := optionalBoolWithPresence(value, FieldSkipPermissions)
	if err != nil {
		return JavaScriptChildSpec{}, err
	}
	if permissionsPresent {
		skipPermissions = permissions == JavaScriptChildPermissionSkipPermissions
	} else if skipPermissions {
		permissions = JavaScriptChildPermissionSkipPermissions
	}
	return JavaScriptChildSpec{
		Prompt:                       prompt,
		Label:                        optional[FieldLabel],
		Preset:                       optional[FieldPreset],
		ExecutorProvider:             optional[FieldExecutorProvider],
		ModelProvider:                optional[FieldModelProvider],
		Model:                        optional[FieldModel],
		ReasoningEffort:              optional[FieldReasoningEffort],
		ResourceID:                   optional[FieldResourceID],
		Permissions:                  permissions,
		SkipPermissions:              skipPermissions,
		LegacySkipPermissionsPresent: skipPermissionsPresent,
	}, nil
}

func optionalBoolWithPresence(value map[string]any, field string) (bool, bool, error) {
	raw, found := value[field]
	if !found {
		return false, false, nil
	}
	normalized, ok := raw.(bool)
	if !ok {
		return false, true, fmt.Errorf(`agent.run() requires %q to be a boolean`, field)
	}
	return normalized, true, nil
}

func optionalPermission(value map[string]any, field string) (JavaScriptChildPermission, bool, error) {
	raw, found := value[field]
	if !found {
		return JavaScriptChildPermissionDefault, false, nil
	}
	text, ok := raw.(string)
	if !ok {
		return "", true, fmt.Errorf(`agent.run() requires %q to be a string`, field)
	}
	permission := JavaScriptChildPermission(text)
	switch permission {
	case JavaScriptChildPermissionDefault, JavaScriptChildPermissionSkipPermissions:
		return permission, true, nil
	default:
		return "", true, fmt.Errorf(`agent.run() requires %q to be DEFAULT or SKIP_PERMISSIONS`, field)
	}
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
