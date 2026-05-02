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
	publicFactoryWorkerModelProviderClaude         = "CLAUDE"
	publicFactoryWorkerModelProviderCodex          = "CODEX"
	publicFactoryWorkerProviderScriptWrap          = "SCRIPT_WRAP"
	publicFactoryWorkstationKindStandard           = "STANDARD"
	publicFactoryWorkstationKindRepeater           = "REPEATER"
	publicFactoryWorkstationKindCron               = "CRON"
	publicFactoryWorkstationTypeModel              = "MODEL_WORKSTATION"
	publicFactoryWorkstationTypeLogical            = "LOGICAL_MOVE"
	publicFactoryGuardTypeVisitCount          = "VISIT_COUNT"
	publicFactoryGuardTypeMatchesFields       = "MATCHES_FIELDS"
	publicFactoryGuardTypeAllChildrenComplete = "ALL_CHILDREN_COMPLETE"
	publicFactoryGuardTypeAnyChildFailed      = "ANY_CHILD_FAILED"
	publicFactoryGuardTypeSameName            = "SAME_NAME"
	publicFactoryGuardTypeInferenceThrottle   = "INFERENCE_THROTTLE_GUARD"
)

var publicFactoryInputKindAliases = map[string]string{
	"DEFAULT": publicFactoryInputKindDefault,
}

var publicFactoryWorkstationKindAliases = map[string]string{
	publicFactoryWorkstationKindCron:     publicFactoryWorkstationKindCron,
	publicFactoryWorkstationKindRepeater: publicFactoryWorkstationKindRepeater,
	publicFactoryWorkstationKindStandard: publicFactoryWorkstationKindStandard,
}

var publicFactoryGuardTypeAliases = map[string]string{
	publicFactoryGuardTypeVisitCount:          publicFactoryGuardTypeVisitCount,
	publicFactoryGuardTypeMatchesFields:       publicFactoryGuardTypeMatchesFields,
	publicFactoryGuardTypeAllChildrenComplete: publicFactoryGuardTypeAllChildrenComplete,
	publicFactoryGuardTypeAnyChildFailed:      publicFactoryGuardTypeAnyChildFailed,
	publicFactoryGuardTypeSameName:            publicFactoryGuardTypeSameName,
	publicFactoryGuardTypeInferenceThrottle:   publicFactoryGuardTypeInferenceThrottle,
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
	switch strings.TrimSpace(string(kind)) {
	case string(interfaces.InputKindDefault), publicFactoryInputKindDefault:
		return publicFactoryInputKindDefault
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
	switch interfaces.StrictPublicFactoryWorkerModelProvider(string(*value)) {
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
	if canonical := interfaces.StrictPublicFactoryWorkerProvider(string(*value)); canonical != "" {
		return strings.ToLower(canonical)
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

func publicFactoryGuardTypeFromInternal(value interfaces.GuardType) factoryapi.GuardType {
	return factoryapi.GuardType(publicFactoryGuardTypeStringFromInternal(value))
}

func publicFactoryGuardTypeStringFromInternal(value interfaces.GuardType) string {
	switch strings.TrimSpace(string(value)) {
	case string(interfaces.GuardTypeVisitCount), publicFactoryGuardTypeVisitCount:
		return publicFactoryGuardTypeVisitCount
	case string(interfaces.GuardTypeMatchesFields), publicFactoryGuardTypeMatchesFields:
		return publicFactoryGuardTypeMatchesFields
	case string(interfaces.GuardTypeAllChildrenComplete), publicFactoryGuardTypeAllChildrenComplete:
		return publicFactoryGuardTypeAllChildrenComplete
	case string(interfaces.GuardTypeAnyChildFailed), publicFactoryGuardTypeAnyChildFailed:
		return publicFactoryGuardTypeAnyChildFailed
	case string(interfaces.GuardTypeSameName), publicFactoryGuardTypeSameName:
		return publicFactoryGuardTypeSameName
	case string(interfaces.GuardTypeInferenceThrottle), publicFactoryGuardTypeInferenceThrottle:
		return publicFactoryGuardTypeInferenceThrottle
	}
	return strings.TrimSpace(string(value))
}

func internalFactoryGuardTypeFromPublic(value factoryapi.GuardType) interfaces.GuardType {
	switch canonicalPublicFactoryEnumValue(string(value), publicFactoryGuardTypeAliases) {
	case publicFactoryGuardTypeVisitCount:
		return interfaces.GuardTypeVisitCount
	case publicFactoryGuardTypeMatchesFields:
		return interfaces.GuardTypeMatchesFields
	case publicFactoryGuardTypeAllChildrenComplete:
		return interfaces.GuardTypeAllChildrenComplete
	case publicFactoryGuardTypeAnyChildFailed:
		return interfaces.GuardTypeAnyChildFailed
	case publicFactoryGuardTypeSameName:
		return interfaces.GuardTypeSameName
	case publicFactoryGuardTypeInferenceThrottle:
		return interfaces.GuardTypeInferenceThrottle
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
