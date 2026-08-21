// Factory mapping helpers for the remaining work-type, resource, worker, workstation, and compatibility projections.
package factoryconfig

import (
	"sort"
	"strings"

	"github.com/portpowered/infinite-you/pkg/transports/http/apitypes"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/optional"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	workercompatibility "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	contentcontract "github.com/portpowered/infinite-you/pkg/transports/mapping/workcontent"
)

func inputTypesAPIFromInternal(inputTypes []interfaces.InputTypeConfig) *[]factoryapi.InputType {
	if len(inputTypes) == 0 {
		return nil
	}
	result := make([]factoryapi.InputType, len(inputTypes))
	for i, inputType := range inputTypes {
		result[i] = factoryapi.InputType{
			Name: inputType.Name,
			Type: publicFactoryInputKindFromInternal(inputType.Type),
		}
	}
	return &result
}

func invocationReturnAPIFromInternal(value *interfaces.InvocationReturnConfig) *factoryapi.InvocationReturn {
	if value == nil {
		return nil
	}
	result := &factoryapi.InvocationReturn{
		Policy: factoryapi.InvocationReturnPolicy(value.Policy),
	}
	if strings.TrimSpace(value.WorkTypeName) != "" {
		result.WorkTypeName = stringPtrIfNotEmpty(value.WorkTypeName)

	}
	if strings.TrimSpace(value.TerminalState) != "" {
		result.TerminalState = stringPtrIfNotEmpty(value.TerminalState)
	}
	if strings.TrimSpace(value.WorkName) != "" {
		result.WorkName = stringPtrIfNotEmpty(value.WorkName)
	}
	return result
}

func workTypeHandlingBehaviorAPIFromInternal(behaviors []string) *[]factoryapi.WorkTypeHandlingBehavior {
	if len(behaviors) == 0 {
		return nil
	}
	values := make([]factoryapi.WorkTypeHandlingBehavior, 0, len(behaviors))
	for _, behavior := range behaviors {
		canonical := interfaces.StrictPublicWorkTypeHandlingBehavior(behavior)
		if canonical == "" {
			continue
		}
		values = append(values, factoryapi.WorkTypeHandlingBehavior(canonical))
	}
	if len(values) == 0 {
		return nil
	}
	return &values
}

func workTypesAPIFromInternal(workTypes []interfaces.WorkTypeConfig) *[]factoryapi.WorkType {
	if len(workTypes) == 0 {
		return nil
	}
	result := make([]factoryapi.WorkType, len(workTypes))
	for i, workType := range workTypes {
		states := make([]factoryapi.WorkState, len(workType.States))
		for stateIndex, state := range workType.States {
			states[stateIndex] = factoryapi.WorkState{
				Id:   stringPtrIfNotEmpty(state.ID),
				Name: state.Name,
				Type: factoryapi.WorkStateType(state.Type),
			}
		}
		result[i] = factoryapi.WorkType{
			Id:                stringPtrIfNotEmpty(workType.ID),
			Name:              workType.Name,
			Description:       NameValueAPIFromInternal(workType.Description),
			States:            states,
			HandlingBehavior:  workTypeHandlingBehaviorAPIFromInternal(workType.HandlingBehavior),
			ExpectedArtifacts: expectedArtifactsAPIFromInternal(workType.ExpectedArtifacts),
		}
	}
	return &result
}

func resourcesAPIFromInternal(resources []interfaces.ResourceConfig) *[]factoryapi.Resource {
	if len(resources) == 0 {
		return nil
	}
	result := make([]factoryapi.Resource, len(resources))
	for i, resource := range resources {
		result[i] = factoryapi.Resource{
			Id:         stringPtrIfNotEmpty(resource.ID),
			Name:       resource.Name,
			Type:       resourceTypePtrIfNotEmpty(resource.Type),
			Capacity:   resource.Capacity,
			Model:      stringPtrIfNotEmpty(resource.Model),
			Backend:    stringPtrIfNotEmpty(resource.Backend),
			LoadPolicy: stringPtrIfNotEmpty(resource.LoadPolicy),
			Provider:   stringPtrIfNotEmpty(resource.Provider),
		}
	}
	return &result
}

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

func factoryReferenceName(cfg *interfaces.FactoryConfig) factoryapi.FactoryName {
	if cfg != nil && strings.TrimSpace(cfg.Name) != "" {
		return factoryapi.FactoryName(cfg.Name)
	}
	if cfg != nil && strings.TrimSpace(cfg.Project) != "" {
		return factoryapi.FactoryName(cfg.Project)
	}
	return factoryapi.FactoryName("factory")
}

// FactoryConfigToOpenAPI converts a valid internal factory config into the
// generated OpenAPI model without passing through normalized on-disk JSON.
// It rejects values that cannot be represented by the public contract.
func FactoryConfigToOpenAPI(cfg *interfaces.FactoryConfig) (factoryapi.Factory, error) {
	return factoryAPIFromInternalConfig(cfg)
}

func hybridLogicalTimestampPtr(version *interfaces.FactoryVersion) *factoryapi.HybridLogicalTimestamp {
	if version == nil {
		return nil
	}
	return &factoryapi.HybridLogicalTimestamp{
		Logical:  apitypes.Int64String(version.Logical),
		Physical: version.Physical.UTC(),
	}
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

func resourceManifestAPIFromInternal(manifest *interfaces.PortableResourceManifestConfig) *factoryapi.ResourceManifest {
	if manifest == nil {
		return nil
	}
	return &factoryapi.ResourceManifest{
		RequiredTools: requiredToolsAPIFromInternal(manifest.RequiredTools),
		BundledFiles:  bundledFilesAPIFromInternal(manifest.BundledFiles),
	}
}

func requiredToolsAPIFromInternal(requiredTools []interfaces.RequiredToolConfig) *[]factoryapi.RequiredTool {
	if len(requiredTools) == 0 {
		return nil
	}
	values := make([]factoryapi.RequiredTool, len(requiredTools))
	for i, tool := range requiredTools {
		values[i] = factoryapi.RequiredTool{
			Name:        tool.Name,
			Command:     tool.Command,
			Purpose:     stringPtrIfNotEmpty(tool.Purpose),
			VersionArgs: stringSlicePtr(tool.VersionArgs),
		}
	}
	return &values
}

func bundledFilesAPIFromInternal(bundledFiles []interfaces.BundledFileConfig) *[]factoryapi.BundledFile {
	if len(bundledFiles) == 0 {
		return nil
	}
	sorted := append([]interfaces.BundledFileConfig(nil), bundledFiles...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].TargetPath < sorted[j].TargetPath
	})
	values := make([]factoryapi.BundledFile, len(sorted))
	for i, file := range sorted {
		values[i] = factoryapi.BundledFile{
			Id:         stringPtrIfNotEmpty(interfaces.CanonicalBundledFileID(file.ID, file.TargetPath)),
			Type:       factoryapi.BundledFileType(file.Type),
			TargetPath: file.TargetPath,
			Content:    bundledFileContentAPIFromInternal(file),
		}
	}
	return &values
}

func bundledFileContentAPIFromInternal(file interfaces.BundledFileConfig) factoryapi.BundledFileContent {
	return factoryapi.BundledFileContent{
		Encoding: factoryapi.BundledFileContentEncoding(file.Content.Encoding),
		Inline:   file.Content.Inline,
	}
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

func dropSupportedPortableBundledInlineContent(payload any) {
	root, ok := payload.(map[string]any)
	if !ok {
		return
	}
	supportingFiles, ok := root["supportingFiles"].(map[string]any)
	if !ok {
		return
	}
	bundledFiles, ok := supportingFiles["bundledFiles"].([]any)
	if !ok {
		return
	}
	for _, entry := range bundledFiles {
		bundledFile, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		fileType, _ := bundledFile["type"].(string)
		targetPath, _ := bundledFile["targetPath"].(string)
		if !interfaces.ShouldOmitSupportedPortableBundledInline(interfaces.BundledFileConfig{
			Type:       fileType,
			TargetPath: targetPath,
		}) {
			continue
		}
		content, ok := bundledFile["content"].(map[string]any)
		if !ok {
			continue
		}
		inline, _ := content["inline"].(string)
		if strings.TrimSpace(inline) != "" {
			continue
		}
		delete(content, "inline")
	}
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
			Name: slot.Name,

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

func inputGuardAPIFromInternal(guard interfaces.InputGuardConfig) factoryapi.InputGuard {
	apiGuard := factoryapi.InputGuard{
		Type: publicInputGuardTypeFromInternal(guard.Type),
	}
	if guard.MatchInput != "" {
		apiGuard.MatchInput = stringPtr(guard.MatchInput)
	}
	if guard.ParentInput != "" {
		apiGuard.ParentInput = stringPtr(guard.ParentInput)
	}
	if guard.SpawnedBy != "" {
		apiGuard.SpawnedBy = stringPtr(guard.SpawnedBy)
	}
	return apiGuard
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

func resourceTypePtrIfNotEmpty(value string) *factoryapi.ResourceType {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	canonical := factoryapi.ResourceType(publicFactoryResourceTypeFromInternal(value))
	return &canonical
}

func workstationGuardsAPIFromInternal(guards []interfaces.GuardConfig) *[]factoryapi.WorkstationGuard {
	if len(guards) == 0 {
		return nil
	}
	values := make([]factoryapi.WorkstationGuard, len(guards))
	for i, guard := range guards {
		values[i] = factoryapi.WorkstationGuard{
			Type:              publicWorkstationGuardTypeFromInternal(guard.Type),
			Workstation:       stringPtrIfNotEmpty(guard.Workstation),
			MaxVisits:         intPtrIfNonZero(guard.MaxVisits),
			MaxVisitsArgument: stringPtrIfNotEmpty(guard.MaxVisitsArgument),
			MatchConfig:       guardMatchConfigAPIFromInternal(guard.MatchConfig),
		}
	}
	return &values
}

func factoryGuardsAPIFromInternal(guards []interfaces.FactoryGuardConfig) *[]factoryapi.FactoryGuard {
	if len(guards) == 0 {
		return nil
	}
	values := make([]factoryapi.FactoryGuard, len(guards))
	for i, guard := range guards {
		values[i] = factoryapi.FactoryGuard{
			Type:          publicFactoryRootGuardTypeFromInternal(guard.Type),
			ModelProvider: publicFactoryWorkerModelProviderFromInternal(guard.ModelProvider),
			Model:         stringPtrIfNotEmpty(guard.Model),
			RefreshWindow: guard.RefreshWindow,
		}
	}
	return &values
}

func guardMatchConfigAPIFromInternal(matchConfig *interfaces.GuardMatchConfig) *factoryapi.GuardMatchConfig {
	if matchConfig == nil {
		return nil
	}
	return &factoryapi.GuardMatchConfig{
		InputKey: matchConfig.InputKey,
	}
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

func modelOperationContentTypePtrIfNotEmpty(value string) *factoryapi.ModelOperationContentType {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	enumValue := publicFactoryModelOperationContentTypeFromInternal(value)
	return &enumValue
}

func valueOrEmpty[T ~string](value *T) T {
	if value == nil {
		return ""
	}
	return *value
}

func stringPtr(value string) *string {
	return &value
}

func stringPtrIfNotEmpty(value string) *string {
	return optional.NonEmptyStringPtr(value)
}

func stringSlicePtr(values []string) *[]string {
	return optional.CopiedStringsPtr(values)
}

func stringMapPtr(values map[string]string) *factoryapi.StringMap {
	if len(values) == 0 {
		return nil
	}
	copied := factoryapi.StringMap(cloneStringMap(values))
	return &copied
}

func intPtrIfNonZero(value int) *int {
	return optional.NonZeroIntPtr(value)
}

func boolPtrIfTrue(value bool) *bool {
	return optional.TrueBoolPtr(value)
}

func stringValue(value *string) string {
	return optional.StringValue(value)
}

func stringSliceValue(values *[]string) []string {
	return optional.StringsValue(values)
}

func stringMapValue(values *factoryapi.StringMap) map[string]string {
	return optional.StringMapValue(values)
}

func intValue(value *int) int {
	return optional.IntValue(value)
}

func boolValue(value *bool) bool {
	return optional.BoolValue(value)
}
