package config

import (
	"fmt"
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/workcontent"
	"strings"
)

func factoryInternalFromAPI(apiCfg factoryapi.Factory) (interfaces.FactoryConfig, error) {
	cfg := interfaces.FactoryConfig{Name: string(apiCfg.Name)}
	if apiCfg.Id != nil {
		cfg.Project = *apiCfg.Id
	}
	if apiCfg.Version != nil {
		cfg.Version = &interfaces.FactoryVersion{
			Logical:  apiCfg.Version.Logical.Int64(),
			Physical: apiCfg.Version.Physical.UTC(),
		}
	}
	cfg.Guards = factoryGuardsInternalFromAPI(apiCfg.Guards)
	if apiCfg.InputTypes != nil {
		cfg.InputTypes = inputTypesInternalFromAPI(*apiCfg.InputTypes)
	}
	if apiCfg.WorkTypes != nil {
		cfg.WorkTypes = workTypesInternalFromAPI(*apiCfg.WorkTypes)
	}
	if apiCfg.Resources != nil {
		cfg.Resources = resourcesInternalFromAPI(*apiCfg.Resources)
	}
	if apiCfg.SupportingFiles != nil {
		cfg.ResourceManifest = resourceManifestInternalFromAPI(apiCfg.SupportingFiles)
	}
	if apiCfg.Workers != nil {
		workers, err := workersInternalFromAPI(*apiCfg.Workers)
		if err != nil {
			return interfaces.FactoryConfig{}, err
		}
		cfg.Workers = workers
	}
	if apiCfg.Workstations != nil {
		workstations, err := workstationsInternalFromAPI(*apiCfg.Workstations)
		if err != nil {
			return interfaces.FactoryConfig{}, err
		}
		cfg.Workstations = workstations
	}
	return cfg, nil
}

// FactoryConfigFromOpenAPI converts the generated OpenAPI factory model into
// the internal config representation.
func FactoryConfigFromOpenAPI(apiCfg factoryapi.Factory) (interfaces.FactoryConfig, error) {
	return factoryInternalFromAPI(apiCfg)
}

func inputTypesInternalFromAPI(inputTypes []factoryapi.InputType) []interfaces.InputTypeConfig {
	values := make([]interfaces.InputTypeConfig, len(inputTypes))
	for i, inputType := range inputTypes {
		values[i] = interfaces.InputTypeConfig{
			Name: inputType.Name,
			Type: internalFactoryInputKindFromPublic(inputType.Type),
		}
	}
	return values
}

func workTypeHandlingBehaviorInternalFromAPI(behaviors *[]factoryapi.WorkTypeHandlingBehavior) []string {
	if behaviors == nil || len(*behaviors) == 0 {
		return nil
	}
	values := make([]string, 0, len(*behaviors))
	for _, behavior := range *behaviors {
		canonical := interfaces.StrictPublicWorkTypeHandlingBehavior(string(behavior))
		if canonical == "" {
			continue
		}
		values = append(values, canonical)
	}
	if len(values) == 0 {
		return nil
	}
	return values
}

func workTypesInternalFromAPI(workTypes []factoryapi.WorkType) []interfaces.WorkTypeConfig {
	values := make([]interfaces.WorkTypeConfig, len(workTypes))
	for i, workType := range workTypes {
		states := make([]interfaces.StateConfig, len(workType.States))
		for si, state := range workType.States {
			states[si] = interfaces.StateConfig{
				Name: state.Name,
				Type: interfaces.StateType(state.Type),
			}
		}
		values[i] = interfaces.WorkTypeConfig{
			Name:             workType.Name,
			States:           states,
			HandlingBehavior: workTypeHandlingBehaviorInternalFromAPI(workType.HandlingBehavior),
		}
	}
	return values
}

func resourcesInternalFromAPI(resources []factoryapi.Resource) []interfaces.ResourceConfig {
	values := make([]interfaces.ResourceConfig, len(resources))
	for i, resource := range resources {
		values[i] = interfaces.ResourceConfig{
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

func workersInternalFromAPI(workers []factoryapi.Worker) ([]interfaces.WorkerConfig, error) {
	values := make([]interfaces.WorkerConfig, len(workers))
	for i, worker := range workers {
		converted, err := WorkerConfigFromOpenAPI(worker)
		if err != nil {
			return nil, fmt.Errorf("map factory.workers[%d]: %w", i, err)
		}
		values[i] = converted
	}
	return values, nil
}

func workerInternalFromAPI(worker factoryapi.Worker) interfaces.WorkerConfig {
	return interfaces.WorkerConfig{
		Name:             worker.Name,
		Type:             internalFactoryWorkerTypeFromPublic(valueOrEmpty(worker.Type)),
		Provider:         internalFactoryHostedWorkerProviderFromPublic(string(valueOrEmpty(worker.Provider))),
		Model:            stringValue(worker.Model),
		ModelProvider:    internalFactoryWorkerModelProviderFromPublic(worker.ModelProvider),
		ModelLocality:    internalFactoryWorkerModelLocalityFromPublic(worker.ModelLocality),
		ExecutorProvider: internalFactoryWorkerProviderFromPublic(worker.ExecutorProvider),
		Operations:       modelOperationsInternalFromAPI(worker.Operations),
		Command:          stringValue(worker.Command),
		Args:             stringSliceValue(worker.Args),
		Resources:        resourceRequirementsInternalFromAPI(worker.Resources),
		Timeout:          stringValue(worker.Timeout),
		StopToken:        stringValue(worker.StopToken),
		SkipPermissions:  boolValue(worker.SkipPermissions),
		Auth:             hostedWorkerAuthInternalFromAPI(worker.Auth),
		Linear:           hostedLinearWorkerInternalFromAPI(worker.Linear),
		Body:             stringValue(worker.Body),
	}
}

func modelOperationsInternalFromAPI(operations *[]factoryapi.ModelOperation) []interfaces.ModelOperation {
	if operations == nil {
		return nil
	}
	values := make([]interfaces.ModelOperation, len(*operations))
	for i, operation := range *operations {
		values[i] = interfaces.ModelOperation{
			Name:    operation.Name,
			Inputs:  modelOperationSlotsInternalFromAPI(operation.Inputs),
			Outputs: modelOperationSlotsInternalFromAPI(operation.Outputs),
		}
	}
	return values
}

func modelOperationSlotsInternalFromAPI(slots *[]factoryapi.ModelOperationSlot) []interfaces.ModelOperationSlot {
	if slots == nil {
		return nil
	}
	values := make([]interfaces.ModelOperationSlot, len(*slots))
	for i, slot := range *slots {
		values[i] = interfaces.ModelOperationSlot{
			Name:         slot.Name,
			ContentTypes: modelOperationContentTypesInternalFromAPI(slot.ContentTypes),
			Required:     boolValue(slot.Required),
		}
	}
	return values
}

func modelOperationContentTypesInternalFromAPI(contentTypes []factoryapi.ModelOperationContentType) []string {
	if len(contentTypes) == 0 {
		return nil
	}
	values := make([]string, len(contentTypes))
	for i, contentType := range contentTypes {
		values[i] = internalFactoryModelOperationContentTypeFromPublic(contentType)
	}
	return values
}

// WorkerConfigFromOpenAPI converts a generated OpenAPI worker model into the
// internal runtime config representation.
func WorkerConfigFromOpenAPI(worker factoryapi.Worker) (interfaces.WorkerConfig, error) {
	return workerInternalFromAPI(worker), nil
}

func hostedWorkerAuthInternalFromAPI(auth *factoryapi.HostedWorkerAuth) *interfaces.HostedWorkerAuthConfig {
	if auth == nil {
		return nil
	}
	return &interfaces.HostedWorkerAuthConfig{
		SecretRef: stringValue(auth.SecretRef),
	}
}

func hostedLinearWorkerInternalFromAPI(cfg *factoryapi.HostedLinearWorkerConfig) *interfaces.HostedLinearWorkerConfig {
	if cfg == nil {
		return nil
	}
	return &interfaces.HostedLinearWorkerConfig{
		PollInterval: stringValue(cfg.PollInterval),
		TeamIDs:      stringSliceValue(cfg.TeamIds),
		StateIDs:     stringSliceValue(cfg.StateIds),
		Mapping:      hostedLinearWorkerMappingInternalFromAPI(cfg.Mapping),
		Claim:        hostedLinearWorkerClaimInternalFromAPI(cfg.Claim),
	}
}

func hostedLinearWorkerMappingInternalFromAPI(mapping *factoryapi.HostedLinearWorkerMapping) interfaces.HostedLinearWorkerMappingConfig {
	if mapping == nil {
		return interfaces.HostedLinearWorkerMappingConfig{}
	}
	return interfaces.HostedLinearWorkerMappingConfig{
		WorkType: stringValue(mapping.WorkType),
		State:    stringValue(mapping.State),
	}
}

func hostedLinearWorkerClaimInternalFromAPI(claim *factoryapi.HostedLinearWorkerClaim) *interfaces.HostedLinearWorkerClaimConfig {
	if claim == nil {
		return nil
	}
	return &interfaces.HostedLinearWorkerClaimConfig{
		AssigneeField: stringValue(claim.AssigneeField),
	}
}

func workstationsInternalFromAPI(workstations []factoryapi.Workstation) ([]interfaces.FactoryWorkstationConfig, error) {
	values := make([]interfaces.FactoryWorkstationConfig, len(workstations))
	for i, workstation := range workstations {
		converted, err := workstationInternalFromAPI(workstation, fmt.Sprintf("factory.workstations[%d]", i))
		if err != nil {
			return nil, err
		}
		values[i] = converted
	}
	return values, nil
}

func workstationInternalFromAPI(workstation factoryapi.Workstation, fieldPath string) (interfaces.FactoryWorkstationConfig, error) {
	inputs, err := workstationIOsInternalFromAPI(workstation.Inputs, fieldPath+".inputs")
	if err != nil {
		return interfaces.FactoryWorkstationConfig{}, err
	}
	outputs, err := optionalWorkstationIOsInternalFromAPI(workstation.Outputs, fieldPath+".outputs")
	if err != nil {
		return interfaces.FactoryWorkstationConfig{}, err
	}
	onContinue, err := optionalWorkstationIOsInternalFromAPI(workstation.OnContinue, fieldPath+".onContinue")
	if err != nil {
		return interfaces.FactoryWorkstationConfig{}, err
	}
	onRejection, err := optionalWorkstationIOsInternalFromAPI(workstation.OnRejection, fieldPath+".onRejection")
	if err != nil {
		return interfaces.FactoryWorkstationConfig{}, err
	}
	classificationRoutes, err := classificationRoutesInternalFromAPI(workstation.ClassificationRoutes, fieldPath+".classificationRoutes")
	if err != nil {
		return interfaces.FactoryWorkstationConfig{}, err
	}
	onFailure, err := optionalWorkstationIOsInternalFromAPI(workstation.OnFailure, fieldPath+".onFailure")
	if err != nil {
		return interfaces.FactoryWorkstationConfig{}, err
	}
	cfg := interfaces.FactoryWorkstationConfig{
		ID:                    stringValue(workstation.Id),
		Name:                  workstation.Name,
		WorkerTypeName:        workstation.Worker,
		Operation:             stringValue(workstation.Operation),
		OperationBindings:     workstationOperationBindingsInternalFromAPI(workstation.OperationBindings),
		Type:                  internalFactoryWorkstationTypeFromPublic(workstation.Type),
		PromptFile:            stringValue(workstation.PromptFile),
		OutputSchema:          stringValue(workstation.OutputSchema),
		Limits:                workstationLimitsInternalFromAPI(workstation.Limits),
		Cron:                  workstationCronInternalFromAPI(workstation.Cron),
		Inputs:                inputs,
		Outputs:               outputs,
		ClassificationRoutes:  classificationRoutes,
		OnContinue:            onContinue,
		OnRejection:           onRejection,
		OnFailure:             onFailure,
		Resources:             resourceRequirementsInternalFromAPI(workstation.Resources),
		CopyReferencedScripts: boolValue(workstation.CopyReferencedScripts),
		Guards:                workstationGuardsInternalFromAPI(workstation.Guards),
		StopWords:             stringSliceValue(workstation.StopWords),
		Body:                  stringValue(workstation.Body),
		WorkingDirectory:      stringValue(workstation.WorkingDirectory),
		Worktree:              stringValue(workstation.Worktree),
		Env:                   stringMapValue(workstation.Env),
	}
	if workstation.Type != nil {
		cfg.Type = internalFactoryWorkstationTypeFromPublic(workstation.Type)
	}
	if workstation.Behavior != nil {
		cfg.Kind = internalFactoryWorkstationKindFromPublic(workstation.Behavior)
	}
	normalizeCanonicalWorkstationRuntime(&cfg)
	return cfg, nil
}

// WorkstationConfigFromOpenAPI converts a generated OpenAPI workstation model
// into the internal config representation.
func WorkstationConfigFromOpenAPI(workstation factoryapi.Workstation) (interfaces.FactoryWorkstationConfig, error) {
	return workstationInternalFromAPI(workstation, fmt.Sprintf("factory.workstations[%q]", workstation.Name))
}

func workstationLimitsInternalFromAPI(limits *factoryapi.WorkstationLimits) interfaces.WorkstationLimits {
	if limits == nil {
		return interfaces.WorkstationLimits{}
	}
	return interfaces.WorkstationLimits{
		MaxRetries:       intValue(limits.MaxRetries),
		MaxExecutionTime: stringValue(limits.MaxExecutionTime),
	}
}

func workstationOperationBindingsInternalFromAPI(bindings *[]factoryapi.WorkstationOperationBinding) []interfaces.ModelOperationBinding {
	if bindings == nil {
		return nil
	}
	values := make([]interfaces.ModelOperationBinding, len(*bindings))
	for i, binding := range *bindings {
		values[i] = interfaces.ModelOperationBinding{
			Slot:           binding.Slot,
			Selector:       workstationOperationBindingSelectorInternalFromAPI(binding.Selector),
			Config:         workcontent.PartsFromGenerated(binding.Config),
			DefaultContent: workcontent.PartsFromGenerated(binding.DefaultContent),
		}
	}
	return values
}

func workstationOperationBindingSelectorInternalFromAPI(selector *factoryapi.WorkstationOperationBindingSelector) *interfaces.ModelOperationBindingSelector {
	if selector == nil {
		return nil
	}
	return &interfaces.ModelOperationBindingSelector{
		Slot:  stringValue(selector.Slot),
		Label: stringValue(selector.Label),
		Type:  internalFactoryModelOperationContentTypeFromPublic(valueOrEmpty(selector.Type)),
		Role:  stringValue(selector.Role),
	}
}

func workstationCronInternalFromAPI(cron *factoryapi.WorkstationCron) *interfaces.CronConfig {
	if cron == nil {
		return nil
	}
	return &interfaces.CronConfig{
		Schedule:       cron.Schedule,
		TriggerAtStart: boolValue(cron.TriggerAtStart),
		Jitter:         stringValue(cron.Jitter),
		ExpiryWindow:   stringValue(cron.ExpiryWindow),
	}
}

func workstationIOsInternalFromAPI(configs []factoryapi.WorkstationIO, fieldPath string) ([]interfaces.IOConfig, error) {
	values := make([]interfaces.IOConfig, len(configs))
	for i, cfg := range configs {
		converted, err := workstationIOInternalFromAPI(cfg, fmt.Sprintf("%s[%d]", fieldPath, i))
		if err != nil {
			return nil, err
		}
		values[i] = converted
	}
	return values, nil
}

func optionalWorkstationIOsInternalFromAPI(configs *[]factoryapi.WorkstationIO, fieldPath string) ([]interfaces.IOConfig, error) {
	if configs == nil {
		return nil, nil
	}
	return workstationIOsInternalFromAPI(*configs, fieldPath)
}

func classificationRoutesInternalFromAPI(
	routes *[]factoryapi.ClassificationRoute,
	fieldPath string,
) ([]interfaces.ClassificationRouteConfig, error) {
	if routes == nil {
		return nil, nil
	}
	values := make([]interfaces.ClassificationRouteConfig, len(*routes))
	for i, route := range *routes {
		outputs, err := workstationIOsInternalFromAPI(route.Outputs, fmt.Sprintf("%s[%d].outputs", fieldPath, i))
		if err != nil {
			return nil, err
		}
		values[i] = interfaces.ClassificationRouteConfig{
			Label:   route.Label,
			Outputs: outputs,
		}
	}
	return values, nil
}

func factoryGuardsInternalFromAPI(guards *[]factoryapi.FactoryGuard) []interfaces.FactoryGuardConfig {
	if guards == nil {
		return nil
	}
	values := make([]interfaces.FactoryGuardConfig, len(*guards))
	for i, guard := range *guards {
		values[i] = interfaces.FactoryGuardConfig{
			Type:          internalFactoryGuardTypeFromPublic(guard.Type),
			ModelProvider: internalFactoryWorkerModelProviderFromPublic(&guard.ModelProvider),
			Model:         stringValue(guard.Model),
			RefreshWindow: guard.RefreshWindow,
		}
	}
	return values
}

func workstationIOInternalFromAPI(cfg factoryapi.WorkstationIO, fieldPath string) (interfaces.IOConfig, error) {
	guard, err := inputGuardInternalFromAPI(cfg.Guards, fieldPath+".guards")
	if err != nil {
		return interfaces.IOConfig{}, err
	}
	return interfaces.IOConfig{
		WorkTypeName: cfg.WorkType,
		StateName:    cfg.State,
		Guard:        guard,
	}, nil
}

func inputGuardInternalFromAPI(guards *[]factoryapi.Guard, fieldPath string) (*interfaces.InputGuardConfig, error) {
	if guards == nil || len(*guards) == 0 {
		return nil, nil
	}
	if len(*guards) > 1 {
		return nil, fmt.Errorf("map %s: expected at most 1 guard, got %d", fieldPath, len(*guards))
	}
	guard := (*guards)[0]
	return &interfaces.InputGuardConfig{
		Type:        internalFactoryGuardTypeFromPublic(guard.Type),
		MatchInput:  stringValue(guard.MatchInput),
		ParentInput: stringValue(guard.ParentInput),
		SpawnedBy:   stringValue(guard.SpawnedBy),
	}, nil
}

func resourceRequirementsInternalFromAPI(resources *[]factoryapi.ResourceRequirement) []interfaces.ResourceConfig {
	if resources == nil {
		return nil
	}
	values := make([]interfaces.ResourceConfig, len(*resources))
	for i, resource := range *resources {
		values[i] = interfaces.ResourceConfig{
			Name:     resource.Name,
			Capacity: resource.Capacity,
		}
	}
	return values
}

func enumStringValue[T ~string](value *T) string {
	if value == nil {
		return ""
	}
	return string(*value)
}

func workstationGuardsInternalFromAPI(guards *[]factoryapi.Guard) []interfaces.GuardConfig {
	if guards == nil {
		return nil
	}
	values := make([]interfaces.GuardConfig, len(*guards))
	for i, guard := range *guards {
		values[i] = interfaces.GuardConfig{
			Type:        internalFactoryGuardTypeFromPublic(guard.Type),
			Workstation: stringValue(guard.Workstation),
			MaxVisits:   intValue(guard.MaxVisits),
			MatchConfig: guardMatchConfigInternalFromAPI(guard.MatchConfig),
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

func publicFactoryWorkerModelLocalityFromInternal(value string) factoryapi.WorkerModelLocality {
	return interfaces.GeneratedPublicFactoryWorkerModelLocality(value)
}

func internalFactoryWorkerModelProviderFromPublic(value *factoryapi.WorkerModelProvider) string {
	if value == nil {
		return ""
	}
	if internal, ok := interfaces.InternalModelProviderFromPublicWorkerModelProvider(*value); ok {
		return string(internal)
	}
	return strings.TrimSpace(string(*value))
}

func internalFactoryWorkerModelLocalityFromPublic(value *factoryapi.WorkerModelLocality) string {
	if value == nil {
		return ""
	}
	if canonical := interfaces.StrictPublicFactoryWorkerModelLocality(string(*value)); canonical != "" {
		return canonical
	}
	return strings.TrimSpace(string(*value))
}

func publicFactoryWorkerProviderFromInternal(value string) factoryapi.WorkerProvider {
	return interfaces.GeneratedPublicFactoryWorkerProvider(value)
}

func publicFactoryHostedWorkerProviderFromInternal(value string) string {
	return string(interfaces.GeneratedPublicFactoryHostedWorkerProvider(value))
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

func internalFactoryHostedWorkerProviderFromPublic(value string) string {
	if canonical := interfaces.StrictPublicFactoryHostedWorkerProvider(value); canonical != "" {
		return canonical
	}
	return strings.TrimSpace(value)
}

func publicFactoryModelOperationContentTypeFromInternal(value string) factoryapi.ModelOperationContentType {
	return interfaces.GeneratedPublicFactoryWorkerModelOperationContentType(value)
}

func internalFactoryModelOperationContentTypeFromPublic(value factoryapi.ModelOperationContentType) string {
	if canonical := interfaces.StrictPublicFactoryWorkerModelOperationContentType(string(value)); canonical != "" {
		return canonical
	}
	return strings.TrimSpace(string(value))
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

func publicFactoryWorkstationKindFromInternal(kind interfaces.WorkstationKind) factoryapi.WorkstationKind {
	return interfaces.GeneratedPublicWorkstationKind(kind)
}

func internalFactoryWorkstationKindFromPublic(kind *factoryapi.WorkstationKind) interfaces.WorkstationKind {
	if kind == nil {
		return ""
	}
	switch interfaces.StrictPublicWorkstationKind(string(*kind)) {
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

