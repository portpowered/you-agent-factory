// Factory worker and workstation projections from internal definitions to the generated API.
package factoryconfig

import (
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	workercompatibility "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	contentcontract "github.com/portpowered/infinite-you/pkg/transports/mapping/workcontent"
)

func workersAPIFromInternal(workers []interfaces.FactoryWorkerConfig, workstations []interfaces.FactoryWorkstationConfig) *[]factoryapi.Worker {
	if len(workers) == 0 {
		return nil
	}
	result := make([]factoryapi.Worker, len(workers))
	for i, worker := range workers {
		result[i] = *workerDefinitionAPIFromInternalWithUsage(&worker, workstations)
	}
	return &result
}

func workerTypesByName(workers []interfaces.FactoryWorkerConfig) map[string]string {
	if len(workers) == 0 {
		return nil
	}
	workerTypes := make(map[string]string, len(workers))
	for _, worker := range workers {
		workerTypes[worker.Name] = worker.Type
	}
	return workerTypes
}

func workstationsAPIFromInternal(workstations []interfaces.FactoryWorkstationConfig, workerTypes map[string]string) *[]factoryapi.Workstation {
	if len(workstations) == 0 {
		return nil
	}
	result := make([]factoryapi.Workstation, 0, len(workstations))
	for _, workstation := range workstations {
		result = append(result, workstationAPIFromInternal(workstation, workerTypes[workstation.WorkerTypeName]))
	}
	return &result
}

func expectedArtifactsAPIFromInternal(
	values []interfaces.ExpectedArtifactConfig,
) *[]factoryapi.ExpectedArtifact {
	if len(values) == 0 {
		return nil
	}
	result := make([]factoryapi.ExpectedArtifact, len(values))
	for index, value := range values {
		result[index] = factoryapi.ExpectedArtifact{
			Name:     value.Name,
			Pattern:  value.Pattern,
			NonEmpty: boolPtrIfTrue(value.NonEmpty),
		}
	}
	return &result
}

func resourceRequirementsAPIFromInternal(resources []interfaces.ResourceConfig) *[]factoryapi.ResourceRequirement {
	if len(resources) == 0 {
		return nil
	}
	values := make([]factoryapi.ResourceRequirement, len(resources))
	for i, resource := range resources {
		values[i] = factoryapi.ResourceRequirement{
			Name:     resource.Name,
			Capacity: resource.Capacity,
		}
	}
	return &values
}

func modelOperationsAPIFromInternal(operations []interfaces.ModelOperation) *[]factoryapi.ModelOperation {
	if len(operations) == 0 {
		return nil
	}
	values := make([]factoryapi.ModelOperation, len(operations))
	for i, operation := range operations {
		values[i] = factoryapi.ModelOperation{
			Name:    operation.Name,
			Inputs:  modelOperationSlotsAPIFromInternal(operation.Inputs),
			Outputs: modelOperationSlotsAPIFromInternal(operation.Outputs),
		}
	}
	return &values
}

func modelOperationSlotsAPIFromInternal(slots []interfaces.ModelOperationSlot) *[]factoryapi.ModelOperationSlot {
	if len(slots) == 0 {
		return nil
	}
	values := make([]factoryapi.ModelOperationSlot, len(slots))
	for i, slot := range slots {
		values[i] = factoryapi.ModelOperationSlot{
			Name:         slot.Name,
			ContentTypes: modelOperationContentTypesAPIFromInternal(slot.ContentTypes),
			Required:     boolPtrIfTrue(slot.Required),
		}
	}
	return &values
}

func modelOperationContentTypesAPIFromInternal(contentTypes []string) []factoryapi.ModelOperationContentType {
	if len(contentTypes) == 0 {
		return nil
	}
	values := make([]factoryapi.ModelOperationContentType, len(contentTypes))
	for i, contentType := range contentTypes {
		values[i] = publicFactoryModelOperationContentTypeFromInternal(contentType)
	}
	return values
}

func workstationAPIFromInternal(workstation interfaces.FactoryWorkstationConfig, workerType string) factoryapi.Workstation {
	normalized := interfaces.CloneWorkstationConfig(workstation)
	NormalizeWorkstationExecutionLimit(&normalized)
	promptBody := normalized.PromptTemplate
	if promptBody == "" {
		promptBody = normalized.Body
	}

	apiWorkstation := factoryapi.Workstation{
		Name:                  normalized.Name,
		Description:           NameValueAPIFromInternal(normalized.Description),
		Worker:                stringPtrIfNotEmpty(normalized.WorkerTypeName),
		Inputs:                workstationIOsAPIFromInternal(normalized.Inputs),
		Outputs:               optionalWorkstationIOsAPIFromInternal(normalized.Outputs),
		ClassificationRoutes:  classificationRoutesAPIFromInternal(normalized.ClassificationRoutes),
		Cron:                  workstationCronAPIFromInternal(normalized.Cron),
		OnContinue:            optionalWorkstationIOsAPIFromInternal(normalized.OnContinue),
		OnRejection:           optionalWorkstationIOsAPIFromInternal(normalized.OnRejection),
		OnFailure:             optionalWorkstationIOsAPIFromInternal(normalized.OnFailure),
		ExpectedArtifacts:     expectedArtifactsAPIFromInternal(normalized.ExpectedArtifacts),
		Resources:             resourceRequirementsAPIFromInternal(normalized.Resources),
		CopyReferencedScripts: boolPtrIfTrue(normalized.CopyReferencedScripts),
		Guards:                workstationGuardsAPIFromInternal(normalized.Guards),
		StopWords:             stringSlicePtr(mergeStopWords(normalized.StopWords, normalized.RuntimeStopWords)),
		Env:                   stringMapPtr(normalized.Env),
		Body:                  stringPtrIfNotEmpty(promptBody),
		Limits:                workstationLimitsAPIFromInternal(normalized.Limits),
		WorkPropagation:       workPropagationAPIFromInternal(normalized.WorkPropagation),
		OutputSchema:          stringPtrIfNotEmpty(normalized.OutputSchema),
		OutputContract:        stringPtrIfNotEmpty(normalized.OutputContract),
		OutcomeFormat:         workstationOutcomeFormatPtrIfNotEmpty(normalized.OutcomeFormat),
		Operation:             stringPtrIfNotEmpty(normalized.Operation),
		OperationBindings:     workstationOperationBindingsAPIFromInternal(normalized.OperationBindings),
		PromptFile:            stringPtrIfNotEmpty(normalized.PromptFile),
		Type:                  workstationTypePtrIfNotEmpty(normalized, workerType),
	}
	if normalized.ID != "" {
		apiWorkstation.Id = stringPtr(normalized.ID)
	}
	if normalized.Kind != "" {
		behavior := publicFactoryWorkstationKindFromInternal(normalized.Kind)
		apiWorkstation.Behavior = &behavior
	}
	if normalized.WorkingDirectory != "" {
		apiWorkstation.WorkingDirectory = stringPtr(normalized.WorkingDirectory)
	}
	if normalized.Worktree != "" {
		apiWorkstation.Worktree = stringPtr(normalized.Worktree)
	}
	return apiWorkstation
}

func classificationRoutesAPIFromInternal(routes []interfaces.ClassificationRouteConfig) *[]factoryapi.ClassificationRoute {
	if len(routes) == 0 {
		return nil
	}
	values := make([]factoryapi.ClassificationRoute, len(routes))
	for i, route := range routes {
		values[i] = factoryapi.ClassificationRoute{
			Label:   route.Label,
			Outputs: workstationIOsAPIFromInternal(route.Outputs),
		}
	}
	return &values
}

// WorkstationConfigToOpenAPI converts an internal workstation config into the
// generated OpenAPI model.
func WorkstationConfigToOpenAPI(workstation interfaces.FactoryWorkstationConfig) factoryapi.Workstation {
	return WorkstationConfigToOpenAPIWithWorkerType(workstation, "")
}

// WorkstationConfigToOpenAPIWithWorkerType converts an internal workstation
// config into the generated OpenAPI model using worker-type context for
// behavior-aware workstation taxonomy projection.
func WorkstationConfigToOpenAPIWithWorkerType(workstation interfaces.FactoryWorkstationConfig, workerType string) factoryapi.Workstation {
	return workstationAPIFromInternal(workstation, workerType)
}

func workerDefinitionAPIFromInternalWithUsage(def *interfaces.FactoryWorkerConfig, workstations []interfaces.FactoryWorkstationConfig) *factoryapi.Worker {
	if def == nil {
		return nil
	}
	return &factoryapi.Worker{
		Id:               stringPtrIfNotEmpty(def.ID),
		Description:      NameValueAPIFromInternal(def.Description),
		Type:             workerTypePtrForFactoryUsage(def, workstations),
		Provider:         hostedWorkerProviderPtrIfNotEmpty(def.Provider),
		Name:             def.Name,
		Args:             stringSlicePtr(def.Args),
		Auth:             hostedWorkerAuthAPIFromInternal(def.Auth),
		Body:             stringPtrIfNotEmpty(def.Body),
		Command:          stringPtrIfNotEmpty(def.Command),
		Linear:           hostedLinearWorkerAPIFromInternal(def.Linear),
		AgentTools:       agentWorkerToolsAPIFromInternal(def.AgentTools),
		Model:            stringPtrIfNotEmpty(def.Model),
		ModelProvider:    workerModelProviderPtrIfNotEmpty(def.ModelProvider),
		ReasoningEffort:  stringPtrIfNotEmpty(def.ReasoningEffort),
		ModelLocality:    workerModelLocalityPtrIfNotEmpty(def.ModelLocality),
		ExecutorProvider: workerProviderPtrIfNotEmpty(def.ExecutorProvider),
		Operations:       modelOperationsAPIFromInternal(def.Operations),
		Resources:        resourceRequirementsAPIFromInternal(def.Resources),
		SkipPermissions:  boolPtrIfTrue(def.SkipPermissions),
		StopToken:        stringPtrIfNotEmpty(def.StopToken),
		Timeout:          stringPtrIfNotEmpty(def.Timeout),
	}
}

func hostedWorkerAuthAPIFromInternal(auth *interfaces.HostedWorkerAuthConfig) *factoryapi.HostedWorkerAuth {
	if auth == nil {
		return nil
	}
	return &factoryapi.HostedWorkerAuth{
		SecretRef: stringPtrIfNotEmpty(auth.SecretRef),
	}
}

func hostedLinearWorkerAPIFromInternal(cfg *interfaces.HostedLinearWorkerConfig) *factoryapi.HostedLinearWorkerConfig {
	if cfg == nil {
		return nil
	}
	return &factoryapi.HostedLinearWorkerConfig{
		PollInterval: stringPtrIfNotEmpty(cfg.PollInterval),
		TeamIds:      stringSlicePtr(cfg.TeamIDs),
		StateIds:     stringSlicePtr(cfg.StateIDs),
		Mapping:      hostedLinearWorkerMappingAPIFromInternal(cfg.Mapping),
		Claim:        hostedLinearWorkerClaimAPIFromInternal(cfg.Claim),
	}
}

func hostedLinearWorkerMappingAPIFromInternal(mapping interfaces.HostedLinearWorkerMappingConfig) *factoryapi.HostedLinearWorkerMapping {
	return &factoryapi.HostedLinearWorkerMapping{
		WorkType: stringPtrIfNotEmpty(mapping.WorkType),
		State:    stringPtrIfNotEmpty(mapping.State),
	}
}

func hostedLinearWorkerClaimAPIFromInternal(claim *interfaces.HostedLinearWorkerClaimConfig) *factoryapi.HostedLinearWorkerClaim {
	if claim == nil {
		return nil
	}
	return &factoryapi.HostedLinearWorkerClaim{
		AssigneeField: stringPtrIfNotEmpty(claim.AssigneeField),
	}
}

func agentWorkerToolsAPIFromInternal(cfg *interfaces.AgentToolsConfig) *factoryapi.AgentWorkerToolsConfig {
	if cfg == nil {
		return nil
	}
	policy := interfaces.NormalizeAgentToolPolicy(cfg.Policy)
	if policy == interfaces.AgentToolPolicyDisabled {
		return &factoryapi.AgentWorkerToolsConfig{
			Policy: factoryapi.AgentWorkerToolPolicyDISABLED,
		}
	}
	switch policy {
	case interfaces.AgentToolPolicyReadOnly:
		return &factoryapi.AgentWorkerToolsConfig{
			Policy: factoryapi.AgentWorkerToolPolicyREADONLY,
		}
	case interfaces.AgentToolPolicyEnabled:
		return &factoryapi.AgentWorkerToolsConfig{
			Policy: factoryapi.AgentWorkerToolPolicyENABLED,
		}
	default:
		return &factoryapi.AgentWorkerToolsConfig{
			Policy: factoryapi.AgentWorkerToolPolicy(policy),
		}
	}
}

// WorkerConfigToOpenAPI converts an internal worker config into the generated
// OpenAPI worker model.
func WorkerConfigToOpenAPI(worker interfaces.FactoryWorkerConfig) factoryapi.Worker {
	return WorkerConfigToOpenAPIWithFactoryUsage(worker, nil)
}

// WorkerConfigToOpenAPIWithFactoryUsage converts an internal worker config into
// the generated OpenAPI worker model using workstation references for
// behavior-aware worker taxonomy projection.
func WorkerConfigToOpenAPIWithFactoryUsage(worker interfaces.FactoryWorkerConfig, workstations []interfaces.FactoryWorkstationConfig) factoryapi.Worker {
	return *workerDefinitionAPIFromInternalWithUsage(&worker, workstations)
}

func workstationLimitsAPIFromInternal(limits interfaces.WorkstationLimits) *factoryapi.WorkstationLimits {
	if limits.MaxRetries == 0 && limits.MaxExecutionTime == "" && limits.MaxGeneratedWorkItems == 0 &&
		limits.MaxGeneratedWorkItemsArgument == "" && limits.MaxGeneratedWorkItemsArgumentOffset == 0 {
		return nil
	}
	return &factoryapi.WorkstationLimits{
		MaxExecutionTime:                    stringPtrIfNotEmpty(limits.MaxExecutionTime),
		MaxRetries:                          intPtrIfNonZero(limits.MaxRetries),
		MaxGeneratedWorkItems:               intPtrIfNonZero(limits.MaxGeneratedWorkItems),
		MaxGeneratedWorkItemsArgument:       stringPtrIfNotEmpty(limits.MaxGeneratedWorkItemsArgument),
		MaxGeneratedWorkItemsArgumentOffset: intPtrIfNonZero(limits.MaxGeneratedWorkItemsArgumentOffset),
	}
}

func workPropagationAPIFromInternal(value *interfaces.WorkPropagationConfig) *factoryapi.WorkPropagation {
	if value == nil {
		return nil
	}
	return &factoryapi.WorkPropagation{
		Mode: factoryapi.WorkPropagationMode(value.Mode),
	}
}

func workstationOperationBindingsAPIFromInternal(bindings []interfaces.ModelOperationBinding) *[]factoryapi.WorkstationOperationBinding {
	if len(bindings) == 0 {
		return nil
	}
	values := make([]factoryapi.WorkstationOperationBinding, len(bindings))
	for i, binding := range bindings {
		values[i] = factoryapi.WorkstationOperationBinding{
			Slot:           binding.Slot,
			Config:         contentcontract.GeneratedPtrFromParts(binding.Config),
			DefaultContent: contentcontract.GeneratedPtrFromParts(binding.DefaultContent),
			Selector:       workstationOperationBindingSelectorAPIFromInternal(binding.Selector),
		}
	}
	return &values
}

func workstationOperationBindingSelectorAPIFromInternal(selector *interfaces.ModelOperationBindingSelector) *factoryapi.WorkstationOperationBindingSelector {
	if selector == nil {
		return nil
	}
	return &factoryapi.WorkstationOperationBindingSelector{
		Slot:  stringPtrIfNotEmpty(selector.Slot),
		Label: stringPtrIfNotEmpty(selector.Label),
		Type:  modelOperationContentTypePtrIfNotEmpty(selector.Type),
		Role:  stringPtrIfNotEmpty(selector.Role),
	}
}

func modelOperationContentTypePtrIfNotEmpty(value string) *factoryapi.ModelOperationContentType {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	enumValue := publicFactoryModelOperationContentTypeFromInternal(value)
	return &enumValue
}

func workstationCronAPIFromInternal(cron *interfaces.CronConfig) *factoryapi.WorkstationCron {
	if cron == nil {
		return nil
	}
	return &factoryapi.WorkstationCron{
		Every:          stringPtrIfNotEmpty(cron.Every),
		ExpiryWindow:   stringPtrIfNotEmpty(cron.ExpiryWindow),
		Jitter:         stringPtrIfNotEmpty(cron.Jitter),
		Schedule:       stringPtrIfNotEmpty(cron.Schedule),
		TriggerAtStart: boolPtrIfTrue(cron.TriggerAtStart),
	}
}

func workstationIOsAPIFromInternal(configs []interfaces.IOConfig) []factoryapi.WorkstationIO {
	values := make([]factoryapi.WorkstationIO, len(configs))
	for i, cfg := range configs {
		values[i] = workstationIOAPIFromInternal(cfg)
	}
	return values
}

func optionalWorkstationIOsAPIFromInternal(configs []interfaces.IOConfig) *[]factoryapi.WorkstationIO {
	if len(configs) == 0 {
		return nil
	}
	values := workstationIOsAPIFromInternal(configs)
	return &values
}

func workstationIOAPIFromInternal(cfg interfaces.IOConfig) factoryapi.WorkstationIO {
	apiIO := factoryapi.WorkstationIO{
		State:    cfg.StateName,
		WorkType: cfg.WorkTypeName,
	}
	if cfg.Guard != nil {
		guards := []factoryapi.InputGuard{inputGuardAPIFromInternal(*cfg.Guard)}
		apiIO.Guards = &guards
	}
	return apiIO
}

func workerTypePtrForFactoryUsage(def *interfaces.FactoryWorkerConfig, workstations []interfaces.FactoryWorkstationConfig) *factoryapi.WorkerType {
	if def == nil || strings.TrimSpace(def.Type) == "" {
		return nil
	}
	publicType := workercompatibility.PublicWorkerTypeForFactoryUsage(*def, compatibilityWorkstations(workstations))
	enumValue := factoryapi.WorkerType(publicType)
	return &enumValue
}

func compatibilityWorkstations(values []interfaces.FactoryWorkstationConfig) []workercompatibility.Workstation {
	if len(values) == 0 {
		return nil
	}
	workstations := make([]workercompatibility.Workstation, len(values))
	for i, value := range values {
		workstations[i] = workercompatibility.Workstation{
			Name: value.Name, Type: value.Type, Kind: value.Kind, WorkerTypeName: value.WorkerTypeName,
		}
	}
	return workstations
}

func workerModelProviderPtrIfNotEmpty(value string) *factoryapi.WorkerModelProvider {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	enumValue := publicFactoryWorkerModelProviderFromInternal(value)
	return &enumValue
}

func workerModelLocalityPtrIfNotEmpty(value string) *factoryapi.WorkerModelLocality {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	enumValue := publicFactoryWorkerModelLocalityFromInternal(value)
	return &enumValue
}

func workerProviderPtrIfNotEmpty(value string) *factoryapi.WorkerProvider {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	enumValue := publicFactoryWorkerProviderFromInternal(value)
	return &enumValue
}

func hostedWorkerProviderPtrIfNotEmpty(value string) *factoryapi.HostedWorkerProvider {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	converted := factoryapi.HostedWorkerProvider(interfaces.PermissivePublicFactoryHostedWorkerProvider(value))
	return &converted
}

func workstationOutcomeFormatPtrIfNotEmpty(value string) *factoryapi.WorkstationOutcomeFormat {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	converted := factoryapi.WorkstationOutcomeFormat(interfaces.PermissivePublicFactoryWorkstationOutcomeFormat(value))
	return &converted
}

func workstationTypePtrIfNotEmpty(workstation interfaces.FactoryWorkstationConfig, workerType string) *factoryapi.WorkstationType {
	publicType := interfaces.PublicWorkstationTypeFromInternalRuntime(workstation.Type, workerType, workstation.Kind)
	if strings.TrimSpace(publicType) == "" {
		return nil
	}
	enumValue := publicFactoryWorkstationTypeFromInternal(workstation, workerType)
	return &enumValue
}

func publicFactoryWorkerModelProviderFromInternal(value string) factoryapi.WorkerModelProvider {
	return factoryapi.WorkerModelProvider(interfaces.PublicWorkerModelProviderFromInternalRuntime(value))
}

func publicFactoryWorkerModelLocalityFromInternal(value string) factoryapi.WorkerModelLocality {
	return factoryapi.WorkerModelLocality(interfaces.PermissivePublicFactoryWorkerModelLocality(value))
}

func publicFactoryWorkerProviderFromInternal(value string) factoryapi.WorkerProvider {
	return factoryapi.WorkerProvider(interfaces.PublicWorkerProviderFromInternalRuntime(value))
}

func publicFactoryHostedWorkerProviderFromInternal(value string) string {
	return interfaces.PermissivePublicFactoryHostedWorkerProvider(value)
}

func publicFactoryModelOperationContentTypeFromInternal(value string) factoryapi.ModelOperationContentType {
	return factoryapi.ModelOperationContentType(interfaces.PermissivePublicFactoryWorkerModelOperationContentType(value))
}

func publicFactoryWorkstationKindFromInternal(kind interfaces.WorkstationKind) factoryapi.WorkstationKind {
	return factoryapi.WorkstationKind(interfaces.CanonicalPublicWorkstationKind(kind))
}

func publicFactoryWorkstationTypeFromInternal(workstation interfaces.FactoryWorkstationConfig, workerType string) factoryapi.WorkstationType {
	return factoryapi.WorkstationType(interfaces.PublicWorkstationTypeFromInternalRuntime(workstation.Type, workerType, workstation.Kind))
}
