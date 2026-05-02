package config

import (
	"fmt"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

const (
	publicFactoryInputKindDefault                  = "DEFAULT"
	publicFactoryWorkerTypeModel                   = "MODEL_WORKER"
	publicFactoryWorkerTypeScript                  = "SCRIPT_WORKER"
	publicFactoryWorkerModelProviderClaude         = "claude"
	publicFactoryWorkerModelProviderCodex          = "codex"
	publicFactoryWorkerProviderScriptWrap          = "script_wrap"
	publicFactoryWorkstationKindStandard           = "STANDARD"
	publicFactoryWorkstationKindRepeater           = "REPEATER"
	publicFactoryWorkstationKindCron               = "CRON"
	publicFactoryWorkstationTypeModel              = "MODEL_WORKSTATION"
	publicFactoryWorkstationTypeLogical            = "LOGICAL_MOVE"
	publicFactoryWorkstationGuardTypeVisitCount    = "VISIT_COUNT"
	publicFactoryWorkstationGuardTypeMatchesFields = "MATCHES_FIELDS"
	publicFactoryInputGuardTypeAllChildrenComplete = "ALL_CHILDREN_COMPLETE"
	publicFactoryInputGuardTypeAnyChildFailed      = "ANY_CHILD_FAILED"
	publicFactoryInputGuardTypeSameName            = "SAME_NAME"
)

var publicFactoryInputKindAliases = map[string]string{
	"DEFAULT": publicFactoryInputKindDefault,
	"default": publicFactoryInputKindDefault,
}

var publicFactoryWorkstationKindAliases = map[string]string{
	publicFactoryWorkstationKindCron:     publicFactoryWorkstationKindCron,
	publicFactoryWorkstationKindRepeater: publicFactoryWorkstationKindRepeater,
	publicFactoryWorkstationKindStandard: publicFactoryWorkstationKindStandard,
	"cron":                               publicFactoryWorkstationKindCron,
	"repeater":                           publicFactoryWorkstationKindRepeater,
	"standard":                           publicFactoryWorkstationKindStandard,
}

var publicFactoryWorkstationGuardTypeAliases = map[string]string{
	publicFactoryWorkstationGuardTypeVisitCount:    publicFactoryWorkstationGuardTypeVisitCount,
	publicFactoryWorkstationGuardTypeMatchesFields: publicFactoryWorkstationGuardTypeMatchesFields,
	"visit_count":    publicFactoryWorkstationGuardTypeVisitCount,
	"matches_fields": publicFactoryWorkstationGuardTypeMatchesFields,
}

var publicFactoryInputGuardTypeAliases = map[string]string{
	publicFactoryInputGuardTypeAllChildrenComplete: publicFactoryInputGuardTypeAllChildrenComplete,
	publicFactoryInputGuardTypeAnyChildFailed:      publicFactoryInputGuardTypeAnyChildFailed,
	publicFactoryInputGuardTypeSameName:            publicFactoryInputGuardTypeSameName,
	"all_children_complete":                        publicFactoryInputGuardTypeAllChildrenComplete,
	"any_child_failed":                             publicFactoryInputGuardTypeAnyChildFailed,
	"same_name":                                    publicFactoryInputGuardTypeSameName,
}

func canonicalPublicFactoryEnumValue(value string, aliases map[string]string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	if canonical, ok := aliases[trimmed]; ok {
		return canonical
	}
	return ""
}

func normalizePublicFactoryEnumValueInObject(container map[string]any, key string, aliases map[string]string) error {
	raw, ok := container[key]
	if !ok {
		return nil
	}
	value, ok := raw.(string)
	if !ok {
		return nil
	}
	if canonical := canonicalPublicFactoryEnumValue(value, aliases); canonical != "" {
		container[key] = canonical
		return nil
	}
	return fmt.Errorf("unsupported value %q", value)
}

func normalizePublicFactoryEnumValueInObjectWith(container map[string]any, key string, normalize func(string) string) error {
	raw, ok := container[key]
	if !ok {
		return nil
	}
	value, ok := raw.(string)
	if !ok {
		return nil
	}
	if canonical := normalize(value); canonical != "" {
		container[key] = canonical
		return nil
	}
	return fmt.Errorf("unsupported value %q", value)
}

func publicFactoryInputKindFromInternal(kind interfaces.InputKind) factoryapi.InputKind {
	return factoryapi.InputKind(publicFactoryInputKindStringFromInternal(kind))
}

func publicFactoryInputKindStringFromInternal(kind interfaces.InputKind) string {
	if canonical := canonicalPublicFactoryEnumValue(string(kind), publicFactoryInputKindAliases); canonical != "" {
		return canonical
	}
	return strings.TrimSpace(string(kind))
}

func internalFactoryInputKindFromPublic(kind factoryapi.InputKind) interfaces.InputKind {
	switch canonicalPublicFactoryEnumValue(string(kind), publicFactoryInputKindAliases) {
	case publicFactoryInputKindDefault:
		return interfaces.InputKindDefault
	default:
		return interfaces.InputKind(strings.TrimSpace(string(kind)))
	}
}

func publicFactoryWorkerTypeFromInternal(value string) factoryapi.WorkerType {
	return interfaces.GeneratedPublicFactoryWorkerType(value)
}

func internalFactoryWorkerTypeFromPublic(value factoryapi.WorkerType) string {
	if canonical := interfaces.PermissivePublicFactoryWorkerType(string(value)); canonical != "" {
		return canonical
	}
	return strings.TrimSpace(string(value))
}

func publicFactoryWorkerModelProviderFromInternal(value string) factoryapi.WorkerModelProvider {
	return interfaces.GeneratedPublicFactoryWorkerModelProvider(value)
}

func internalFactoryWorkerModelProviderFromPublic(value *factoryapi.WorkerModelProvider) string {
	if value == nil {
		return ""
	}
	switch interfaces.PermissivePublicFactoryWorkerModelProvider(string(*value)) {
	case publicFactoryWorkerModelProviderClaude:
		return string(factoryapiToInternalModelProviderClaude())
	case publicFactoryWorkerModelProviderCodex:
		return string(factoryapiToInternalModelProviderCodex())
	default:
		return strings.TrimSpace(string(*value))
	}
}

func publicFactoryWorkerProviderFromInternal(value string) factoryapi.WorkerProvider {
	return interfaces.GeneratedPublicFactoryWorkerProvider(value)
}

func internalFactoryWorkerProviderFromPublic(value *factoryapi.WorkerProvider) string {
	if value == nil {
		return ""
	}
	if canonical := interfaces.PermissivePublicFactoryWorkerProvider(string(*value)); canonical != "" {
		return canonical
	}
	return strings.TrimSpace(string(*value))
}

func publicFactoryWorkstationKindFromInternal(kind interfaces.WorkstationKind) factoryapi.WorkstationKind {
	return interfaces.GeneratedPublicWorkstationKind(kind)
}

func internalFactoryWorkstationKindFromPublic(kind *factoryapi.WorkstationKind) interfaces.WorkstationKind {
	if kind == nil {
		return ""
	}
	switch canonicalPublicFactoryEnumValue(string(*kind), publicFactoryWorkstationKindAliases) {
	case publicFactoryWorkstationKindStandard:
		return interfaces.WorkstationKindStandard
	case publicFactoryWorkstationKindRepeater:
		return interfaces.WorkstationKindRepeater
	case publicFactoryWorkstationKindCron:
		return interfaces.WorkstationKindCron
	default:
		return interfaces.WorkstationKind(strings.TrimSpace(string(*kind)))
	}
}

func publicFactoryWorkstationTypeFromInternal(value string) factoryapi.WorkstationType {
	return interfaces.GeneratedPublicFactoryWorkstationType(value)
}

func internalFactoryWorkstationTypeFromPublic(value *factoryapi.WorkstationType) string {
	if value == nil {
		return ""
	}
	if canonical := interfaces.PermissivePublicFactoryWorkstationType(string(*value)); canonical != "" {
		return canonical
	}
	return strings.TrimSpace(string(*value))
}

func publicFactoryWorkstationGuardTypeFromInternal(value interfaces.GuardType) factoryapi.WorkstationGuardType {
	return factoryapi.WorkstationGuardType(publicFactoryWorkstationGuardTypeStringFromInternal(value))
}

func publicFactoryWorkstationGuardTypeStringFromInternal(value interfaces.GuardType) string {
	if canonical := canonicalPublicFactoryEnumValue(string(value), publicFactoryWorkstationGuardTypeAliases); canonical != "" {
		return canonical
	}
	return strings.TrimSpace(string(value))
}

func internalFactoryWorkstationGuardTypeFromPublic(value factoryapi.WorkstationGuardType) interfaces.GuardType {
	switch canonicalPublicFactoryEnumValue(string(value), publicFactoryWorkstationGuardTypeAliases) {
	case publicFactoryWorkstationGuardTypeVisitCount:
		return interfaces.GuardTypeVisitCount
	case publicFactoryWorkstationGuardTypeMatchesFields:
		return interfaces.GuardTypeMatchesFields
	default:
		return interfaces.GuardType(strings.TrimSpace(string(value)))
	}
}

func publicFactoryInputGuardTypeFromInternal(value interfaces.GuardType) factoryapi.InputGuardType {
	return factoryapi.InputGuardType(publicFactoryInputGuardTypeStringFromInternal(value))
}

func publicFactoryInputGuardTypeStringFromInternal(value interfaces.GuardType) string {
	if canonical := canonicalPublicFactoryEnumValue(string(value), publicFactoryInputGuardTypeAliases); canonical != "" {
		return canonical
	}
	return strings.TrimSpace(string(value))
}

func internalFactoryInputGuardTypeFromPublic(value factoryapi.InputGuardType) interfaces.GuardType {
	switch canonicalPublicFactoryEnumValue(string(value), publicFactoryInputGuardTypeAliases) {
	case publicFactoryInputGuardTypeAllChildrenComplete:
		return interfaces.GuardTypeAllChildrenComplete
	case publicFactoryInputGuardTypeAnyChildFailed:
		return interfaces.GuardTypeAnyChildFailed
	case publicFactoryInputGuardTypeSameName:
		return interfaces.GuardTypeSameName
	default:
		return interfaces.GuardType(strings.TrimSpace(string(value)))
	}
}

func factoryapiToInternalModelProviderClaude() string {
	return "claude"
}

func factoryapiToInternalModelProviderCodex() string {
	return "codex"
}
