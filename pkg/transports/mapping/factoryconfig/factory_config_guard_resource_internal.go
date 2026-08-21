// Factory guard/resource reverse mapping and public-enum normalization helpers.
package factoryconfig

import (
	"fmt"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func resourcesInternalFromAPI(resources []factoryapi.Resource) []interfaces.ResourceConfig {
	values := make([]interfaces.ResourceConfig, len(resources))
	for i, resource := range resources {
		values[i] = interfaces.ResourceConfig{
			ID:         stringValue(resource.Id),
			Name:       resource.Name,
			Type:       internalFactoryResourceTypeFromPublic(enumStringValue(resource.Type)),
			Capacity:   resource.Capacity,
			Model:      stringValue(resource.Model),
			Backend:    stringValue(resource.Backend),
			LoadPolicy: stringValue(resource.LoadPolicy),
			Provider:   stringValue(resource.Provider),
		}
	}
	return values
}

func resourceManifestInternalFromAPI(manifest *factoryapi.ResourceManifest) *interfaces.PortableResourceManifestConfig {
	if manifest == nil {
		return nil
	}

	cfg := &interfaces.PortableResourceManifestConfig{
		RequiredTools: requiredToolsInternalFromAPI(manifest.RequiredTools),
		BundledFiles:  bundledFilesInternalFromAPI(manifest.BundledFiles),
	}
	if len(cfg.RequiredTools) == 0 && len(cfg.BundledFiles) == 0 {
		return &interfaces.PortableResourceManifestConfig{}
	}
	return cfg
}

func requiredToolsInternalFromAPI(requiredTools *[]factoryapi.RequiredTool) []interfaces.RequiredToolConfig {
	if requiredTools == nil {
		return nil
	}
	values := make([]interfaces.RequiredToolConfig, len(*requiredTools))
	for i, tool := range *requiredTools {
		values[i] = interfaces.RequiredToolConfig{
			Name:        tool.Name,
			Command:     tool.Command,
			Purpose:     stringValue(tool.Purpose),
			VersionArgs: stringSliceValue(tool.VersionArgs),
		}
	}
	return values
}

func bundledFilesInternalFromAPI(bundledFiles *[]factoryapi.BundledFile) []interfaces.BundledFileConfig {
	if bundledFiles == nil {
		return nil
	}
	values := make([]interfaces.BundledFileConfig, len(*bundledFiles))
	for i, file := range *bundledFiles {
		values[i] = interfaces.BundledFileConfig{
			ID:         interfaces.CanonicalBundledFileID(stringValue(file.Id), file.TargetPath),
			Type:       string(file.Type),
			TargetPath: file.TargetPath,
			Content: interfaces.BundledFileContentConfig{
				Encoding: string(file.Content.Encoding),
				Inline:   file.Content.Inline,
			},
		}
	}
	return values
}

func factoryGuardsInternalFromAPI(guards *[]factoryapi.FactoryGuard) []interfaces.FactoryGuardConfig {
	if guards == nil {
		return nil
	}
	values := make([]interfaces.FactoryGuardConfig, len(*guards))
	for i, guard := range *guards {
		values[i] = interfaces.FactoryGuardConfig{
			Type:          internalFactoryGuardTypeFromPublicFactoryGuard(guard.Type),
			ModelProvider: internalFactoryWorkerModelProviderFromPublic(&guard.ModelProvider),
			Model:         stringValue(guard.Model),
			RefreshWindow: guard.RefreshWindow,
		}
	}
	return values
}

func inputGuardInternalFromAPI(guards *[]factoryapi.InputGuard, fieldPath string) (*interfaces.InputGuardConfig, error) {
	if guards == nil || len(*guards) == 0 {
		return nil, nil
	}
	if len(*guards) > 1 {
		return nil, fmt.Errorf("map %s: expected at most 1 guard, got %d", fieldPath, len(*guards))
	}
	guard := (*guards)[0]
	return &interfaces.InputGuardConfig{
		Type:        internalFactoryGuardTypeFromPublicInputGuard(guard.Type),
		MatchInput:  stringValue(guard.MatchInput),
		ParentInput: stringValue(guard.ParentInput),
		SpawnedBy:   stringValue(guard.SpawnedBy),
	}, nil
}

func enumStringValue[T ~string](value *T) string {
	if value == nil {
		return ""
	}
	return string(*value)
}

func workstationGuardsInternalFromAPI(guards *[]factoryapi.WorkstationGuard) []interfaces.GuardConfig {
	if guards == nil {
		return nil
	}
	values := make([]interfaces.GuardConfig, len(*guards))
	for i, guard := range *guards {
		values[i] = interfaces.GuardConfig{
			Type:              internalFactoryGuardTypeFromPublicWorkstationGuard(guard.Type),
			Workstation:       stringValue(guard.Workstation),
			MaxVisits:         intValue(guard.MaxVisits),
			MaxVisitsArgument: stringValue(guard.MaxVisitsArgument),
			MatchConfig:       guardMatchConfigInternalFromAPI(guard.MatchConfig),
		}
	}
	return values
}

func guardMatchConfigInternalFromAPI(matchConfig *factoryapi.GuardMatchConfig) *interfaces.GuardMatchConfig {
	if matchConfig == nil {
		return nil
	}
	return &interfaces.GuardMatchConfig{
		InputKey: matchConfig.InputKey,
	}
}

const (
	publicFactoryInputKindDefault                = "DEFAULT"
	publicFactoryWorkerTypeModel                 = "MODEL_WORKER"
	publicFactoryWorkerTypeScript                = "SCRIPT_WORKER"
	publicFactoryWorkerTypeHosted                = "HOSTED_WORKER"
	publicFactoryWorkerModelLocalityLocal        = "LOCAL"
	publicFactoryWorkerModelLocalityCloud        = "CLOUD"
	publicFactoryResourceTypeModel               = "MODEL"
	publicFactoryResourceTypeProviderQuota       = "PROVIDER_QUOTA"
	publicFactoryResourceTypeInvocationSlot      = "INVOCATION_SLOT"
	publicFactoryModelOperationContentTypeText   = "TEXT"
	publicFactoryModelOperationContentTypeImage  = "IMAGE"
	publicFactoryModelOperationContentTypeAudio  = "AUDIO"
	publicFactoryModelOperationContentTypeJSON   = "JSON"
	publicFactoryModelOperationContentTypeBinary = "BINARY"
	publicFactoryWorkerProviderScriptWrap        = "SCRIPT_WRAP"
	publicFactoryWorkstationKindStandard         = "STANDARD"
	publicFactoryWorkstationKindRepeater         = "REPEATER"
	publicFactoryWorkstationKindCron             = "CRON"
	publicFactoryWorkstationKindPoller           = "POLLER"
	publicFactoryWorkstationTypeInference        = "INFERENCE_RUN"
	publicFactoryWorkstationTypeAgent            = "AGENT_RUN"
	publicFactoryWorkstationTypeScript           = "SCRIPT_RUN"
	publicFactoryWorkstationTypePoller           = "POLLER_RUN"
	publicFactoryWorkstationTypeModel            = "MODEL_WORKSTATION"
	publicFactoryWorkstationTypeInvoke           = "MODEL_INVOKE"
	publicFactoryWorkstationTypeLogical          = "LOGICAL_MOVE"
	publicFactoryGuardTypeVisitCount             = "VISIT_COUNT"
	publicFactoryGuardTypeMatchesFields          = "MATCHES_FIELDS"
	publicFactoryGuardTypeAllChildrenComplete    = "ALL_CHILDREN_COMPLETE"
	publicFactoryGuardTypeAnyChildFailed         = "ANY_CHILD_FAILED"
	publicFactoryGuardTypeSameName               = "SAME_NAME"
	publicFactoryGuardTypeSameTraceID            = "SAME_TRACE_ID"
	publicFactoryGuardTypeInferenceThrottle      = "INFERENCE_THROTTLE_GUARD"
)

var publicFactoryInputKindAliases = map[string]string{
	"DEFAULT": publicFactoryInputKindDefault,
}

var publicFactoryGuardTypeAliases = map[string]string{
	publicFactoryGuardTypeVisitCount:          publicFactoryGuardTypeVisitCount,
	publicFactoryGuardTypeMatchesFields:       publicFactoryGuardTypeMatchesFields,
	publicFactoryGuardTypeAllChildrenComplete: publicFactoryGuardTypeAllChildrenComplete,
	publicFactoryGuardTypeAnyChildFailed:      publicFactoryGuardTypeAnyChildFailed,
	publicFactoryGuardTypeSameName:            publicFactoryGuardTypeSameName,
	publicFactoryGuardTypeSameTraceID:         publicFactoryGuardTypeSameTraceID,
	publicFactoryGuardTypeInferenceThrottle:   publicFactoryGuardTypeInferenceThrottle,
}

var publicFactoryModelOperationContentTypeAliases = map[string]string{
	publicFactoryModelOperationContentTypeText:   publicFactoryModelOperationContentTypeText,
	publicFactoryModelOperationContentTypeImage:  publicFactoryModelOperationContentTypeImage,
	publicFactoryModelOperationContentTypeAudio:  publicFactoryModelOperationContentTypeAudio,
	publicFactoryModelOperationContentTypeJSON:   publicFactoryModelOperationContentTypeJSON,
	publicFactoryModelOperationContentTypeBinary: publicFactoryModelOperationContentTypeBinary,
}

var publicFactoryResourceTypeAliases = map[string]string{
	publicFactoryResourceTypeModel:          publicFactoryResourceTypeModel,
	publicFactoryResourceTypeProviderQuota:  publicFactoryResourceTypeProviderQuota,
	publicFactoryResourceTypeInvocationSlot: publicFactoryResourceTypeInvocationSlot,
}

var publicFactoryRootGuardTypeAliases = map[string]string{
	publicFactoryGuardTypeInferenceThrottle: publicFactoryGuardTypeInferenceThrottle,
}

var publicFactoryWorkstationGuardTypeAliases = map[string]string{
	publicFactoryGuardTypeVisitCount:    publicFactoryGuardTypeVisitCount,
	publicFactoryGuardTypeMatchesFields: publicFactoryGuardTypeMatchesFields,
}

var publicFactoryInputGuardTypeAliases = map[string]string{
	publicFactoryGuardTypeVisitCount:          publicFactoryGuardTypeVisitCount,
	publicFactoryGuardTypeAllChildrenComplete: publicFactoryGuardTypeAllChildrenComplete,
	publicFactoryGuardTypeAnyChildFailed:      publicFactoryGuardTypeAnyChildFailed,
	publicFactoryGuardTypeSameName:            publicFactoryGuardTypeSameName,
	publicFactoryGuardTypeSameTraceID:         publicFactoryGuardTypeSameTraceID,
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

func publicFactoryResourceTypeFromInternal(value string) string {
	if canonical := interfaces.PermissivePublicFactoryResourceType(value); canonical != "" {
		return canonical
	}
	return strings.TrimSpace(value)
}

func internalFactoryResourceTypeFromPublic(value string) string {
	if canonical := interfaces.StrictPublicFactoryResourceType(value); canonical != "" {
		return canonical
	}
	return strings.TrimSpace(value)
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
	case string(interfaces.GuardTypeSameTraceID), publicFactoryGuardTypeSameTraceID:
		return publicFactoryGuardTypeSameTraceID
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
	case publicFactoryGuardTypeSameTraceID:
		return interfaces.GuardTypeSameTraceID
	case publicFactoryGuardTypeInferenceThrottle:
		return interfaces.GuardTypeInferenceThrottle
	default:
		return interfaces.GuardType(strings.TrimSpace(string(value)))
	}
}

func internalFactoryGuardTypeFromPublicWorkstationGuard(value factoryapi.WorkstationGuardType) interfaces.GuardType {
	return internalFactoryGuardTypeFromPublic(factoryapi.GuardType(value))
}

func internalFactoryGuardTypeFromPublicInputGuard(value factoryapi.InputGuardType) interfaces.GuardType {
	return internalFactoryGuardTypeFromPublic(factoryapi.GuardType(value))
}

func internalFactoryGuardTypeFromPublicFactoryGuard(value factoryapi.FactoryGuardType) interfaces.GuardType {
	return internalFactoryGuardTypeFromPublic(factoryapi.GuardType(value))
}

func publicWorkstationGuardTypeFromInternal(value interfaces.GuardType) factoryapi.WorkstationGuardType {
	return factoryapi.WorkstationGuardType(publicFactoryGuardTypeStringFromInternal(value))
}

func publicInputGuardTypeFromInternal(value interfaces.GuardType) factoryapi.InputGuardType {
	return factoryapi.InputGuardType(publicFactoryGuardTypeStringFromInternal(value))
}

func publicFactoryRootGuardTypeFromInternal(value interfaces.GuardType) factoryapi.FactoryGuardType {
	return factoryapi.FactoryGuardType(publicFactoryGuardTypeStringFromInternal(value))
}
