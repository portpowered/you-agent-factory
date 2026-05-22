package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/workcontent"
)

// FactoryConfigMapper maps between on-disk factory configuration payloads and
// canonical in-memory config structures.
type FactoryConfigMapper struct{}

// NewFactoryConfigMapper returns the canonical mapper used across config loading
// and serialization paths.
func NewFactoryConfigMapper() *FactoryConfigMapper {
	return &FactoryConfigMapper{}
}

type generatedFactoryBoundary struct {
	generated factoryapi.Factory
}

const generatedFactoryBoundaryErrorPrefix = "decode factory generated-schema boundary"

// Expand parses and normalizes a user-provided factory payload into the internal
// canonical configuration representation.
func (m *FactoryConfigMapper) Expand(data []byte) (*interfaces.FactoryConfig, error) {
	boundary, err := decodeGeneratedFactoryBoundaryJSON(data)
	if err != nil {
		return nil, err
	}

	cfg, err := FactoryConfigFromOpenAPI(boundary.generated)
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

func decodeGeneratedFactoryBoundaryJSON(data []byte) (generatedFactoryBoundary, error) {
	if err := rejectRetiredGeneratedBoundaryAliases(data); err != nil {
		return generatedFactoryBoundary{}, fmt.Errorf("%s: %w", generatedFactoryBoundaryErrorPrefix, err)
	}
	normalizedData, err := normalizeFactoryInputJSON(data)
	if err != nil {
		return generatedFactoryBoundary{}, fmt.Errorf("%s: %w", generatedFactoryBoundaryErrorPrefix, err)
	}
	if err := rejectRetiredExhaustionRulesField(normalizedData); err != nil {
		return generatedFactoryBoundary{}, fmt.Errorf("%s: %w", generatedFactoryBoundaryErrorPrefix, err)
	}
	if err := rejectRetiredFanInField(normalizedData); err != nil {
		return generatedFactoryBoundary{}, fmt.Errorf("%s: %w", generatedFactoryBoundaryErrorPrefix, err)
	}
	if err := rejectRetiredCronIntervalField(normalizedData); err != nil {
		return generatedFactoryBoundary{}, fmt.Errorf("%s: %w", generatedFactoryBoundaryErrorPrefix, err)
	}

	apiCfg, err := decodeGeneratedFactoryBoundary(normalizedData)
	if err != nil {
		return generatedFactoryBoundary{}, fmt.Errorf("%s: %w", generatedFactoryBoundaryErrorPrefix, err)
	}
	return generatedFactoryBoundary{
		generated: apiCfg,
	}, nil
}

func decodeGeneratedFactoryBoundary(data []byte) (factoryapi.Factory, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	var apiCfg factoryapi.Factory
	if err := decoder.Decode(&apiCfg); err != nil {
		return factoryapi.Factory{}, fmt.Errorf("unmarshal factory api model: %w", err)
	}
	if err := ensureFactoryBoundaryEOF(decoder); err != nil {
		return factoryapi.Factory{}, err
	}
	if err := validateGeneratedFactoryBoundary(apiCfg); err != nil {
		return factoryapi.Factory{}, err
	}
	return apiCfg, nil
}

func ensureFactoryBoundaryEOF(decoder *json.Decoder) error {
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != nil {
		if err == io.EOF {
			return nil
		}
		return fmt.Errorf("unmarshal factory api model: %w", err)
	}
	return fmt.Errorf("unmarshal factory api model: unexpected trailing JSON value")
}

func validateGeneratedFactoryBoundary(apiCfg factoryapi.Factory) error {
	if strings.TrimSpace(string(apiCfg.Name)) == "" {
		return fmt.Errorf("factory.name is required")
	}
	return nil
}

// Flatten serializes an internal factory configuration into canonical JSON that is
// stable for persisted output and downstream tooling.
func (m *FactoryConfigMapper) Flatten(cfg *interfaces.FactoryConfig) ([]byte, error) {
	apiCfg := factoryAPIFromInternalConfig(cfg)

	raw, err := json.Marshal(apiCfg)
	if err != nil {
		return nil, fmt.Errorf("marshal factory api model: %w", err)
	}

	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, fmt.Errorf("decode factory api payload: %w", err)
	}
	canonical := normalizeFactoryOutputJSONKeys(decoded)
	dropSupportedPortableBundledInlineContent(canonical)
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return nil, fmt.Errorf("normalize factory config keys: %w", err)
	}
	return encoded, nil
}

func factoryAPIFromInternalConfig(cfg *interfaces.FactoryConfig) factoryapi.Factory {
	if cfg == nil {
		return factoryapi.Factory{}
	}

	apiCfg := factoryapi.Factory{Name: factoryReferenceName(cfg)}
	if cfg.Project != "" {
		apiCfg.Id = stringPtr(cfg.Project)
	}
	if len(cfg.Guards) > 0 {
		apiCfg.Guards = factoryGuardsAPIFromInternal(cfg.Guards)
	}
	if len(cfg.InputTypes) > 0 {
		inputTypes := make([]factoryapi.InputType, len(cfg.InputTypes))
		for i, inputType := range cfg.InputTypes {
			inputTypes[i] = factoryapi.InputType{
				Name: inputType.Name,
				Type: publicFactoryInputKindFromInternal(inputType.Type),
			}
		}
		apiCfg.InputTypes = &inputTypes
	}
	if len(cfg.WorkTypes) > 0 {
		workTypes := make([]factoryapi.WorkType, len(cfg.WorkTypes))
		for i, workType := range cfg.WorkTypes {
			states := make([]factoryapi.WorkState, len(workType.States))
			for si, state := range workType.States {
				states[si] = factoryapi.WorkState{
					Name: state.Name,
					Type: factoryapi.WorkStateType(state.Type),
				}
			}
			workTypes[i] = factoryapi.WorkType{
				Name:   workType.Name,
				States: states,
			}
		}
		apiCfg.WorkTypes = &workTypes
	}
	if len(cfg.Resources) > 0 {
		resources := make([]factoryapi.Resource, len(cfg.Resources))
		for i, resource := range cfg.Resources {
			resources[i] = factoryapi.Resource{
				Name:       resource.Name,
				Type:       resourceTypePtrIfNotEmpty(resource.Type),
				Capacity:   resource.Capacity,
				Model:      stringPtrIfNotEmpty(resource.Model),
				Backend:    stringPtrIfNotEmpty(resource.Backend),
				LoadPolicy: stringPtrIfNotEmpty(resource.LoadPolicy),
				Provider:   stringPtrIfNotEmpty(resource.Provider),
			}
		}
		apiCfg.Resources = &resources
	}
	if cfg.ResourceManifest != nil {
		apiCfg.SupportingFiles = resourceManifestAPIFromInternal(cfg.ResourceManifest)
	}
	if len(cfg.Workers) > 0 {
		workers := make([]factoryapi.Worker, len(cfg.Workers))
		for i, worker := range cfg.Workers {
			workers[i] = *workerDefinitionAPIFromInternal(&worker)
		}
		apiCfg.Workers = &workers
	}
	if len(cfg.Workstations) > 0 {
		workstations := make([]factoryapi.Workstation, 0, len(cfg.Workstations))
		for _, workstation := range cfg.Workstations {
			workstations = append(workstations, workstationAPIFromInternal(workstation))
		}
		apiCfg.Workstations = &workstations
	}
	return apiCfg
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

// FactoryConfigToOpenAPI converts the internal factory config into the generated
// OpenAPI model without passing through normalized on-disk JSON.
func FactoryConfigToOpenAPI(cfg *interfaces.FactoryConfig) factoryapi.Factory {
	return factoryAPIFromInternalConfig(cfg)
}

func workstationAPIFromInternal(workstation interfaces.FactoryWorkstationConfig) factoryapi.Workstation {
	normalized := CloneWorkstationConfig(workstation)
	NormalizeWorkstationExecutionLimit(&normalized)
	promptBody := normalized.PromptTemplate
	if promptBody == "" {
		promptBody = normalized.Body
	}

	apiWorkstation := factoryapi.Workstation{
		Name:                  normalized.Name,
		Worker:                normalized.WorkerTypeName,
		Inputs:                workstationIOsAPIFromInternal(normalized.Inputs),
		Outputs:               workstationIOsAPIFromInternal(normalized.Outputs),
		Cron:                  workstationCronAPIFromInternal(normalized.Cron),
		OnContinue:            optionalWorkstationIOsAPIFromInternal(normalized.OnContinue),
		OnRejection:           optionalWorkstationIOsAPIFromInternal(normalized.OnRejection),
		OnFailure:             optionalWorkstationIOsAPIFromInternal(normalized.OnFailure),
		Resources:             resourceRequirementsAPIFromInternal(normalized.Resources),
		CopyReferencedScripts: boolPtrIfTrue(normalized.CopyReferencedScripts),
		Guards:                workstationGuardsAPIFromInternal(normalized.Guards),
		StopWords:             stringSlicePtr(mergeCanonicalStopWords(normalized.StopWords, normalized.RuntimeStopWords)),
		Env:                   stringMapPtr(normalized.Env),
		Body:                  stringPtrIfNotEmpty(promptBody),
		Limits:                workstationLimitsAPIFromInternal(normalized.Limits),
		OutputSchema:          stringPtrIfNotEmpty(normalized.OutputSchema),
		Operation:             stringPtrIfNotEmpty(normalized.Operation),
		OperationBindings:     workstationOperationBindingsAPIFromInternal(normalized.OperationBindings),
		PromptFile:            stringPtrIfNotEmpty(normalized.PromptFile),
		Type:                  workstationTypePtrIfNotEmpty(normalized.Type),
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

func shouldOmitSupportedPortableBundledInline(file interfaces.BundledFileConfig) bool {
	if !isSupportedPortableBundledFile(file) {
		return false
	}
	switch file.Type {
	case interfaces.BundledFileTypeScript, interfaces.BundledFileTypeDoc, interfaces.BundledFileTypeInput:
		return true
	default:
		return false
	}
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
		if !shouldOmitSupportedPortableBundledInline(interfaces.BundledFileConfig{
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
	return workstationAPIFromInternal(workstation)
}

func workerDefinitionAPIFromInternal(def *interfaces.WorkerConfig) *factoryapi.Worker {
	if def == nil {
		return nil
	}
	return &factoryapi.Worker{
		Type:             workerTypePtrIfNotEmpty(def.Type),
		Provider:         hostedWorkerProviderPtrIfNotEmpty(def.Provider),
		Name:             def.Name,
		Args:             stringSlicePtr(def.Args),
		Auth:             hostedWorkerAuthAPIFromInternal(def.Auth),
		Body:             stringPtrIfNotEmpty(def.Body),
		Command:          stringPtrIfNotEmpty(def.Command),
		Linear:           hostedLinearWorkerAPIFromInternal(def.Linear),
		Model:            stringPtrIfNotEmpty(def.Model),
		ModelProvider:    workerModelProviderPtrIfNotEmpty(def.ModelProvider),
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

// WorkerConfigToOpenAPI converts an internal worker config into the generated
// OpenAPI worker model.
func WorkerConfigToOpenAPI(worker interfaces.WorkerConfig) factoryapi.Worker {
	return *workerDefinitionAPIFromInternal(&worker)
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

func mergeCanonicalStopWords(base []string, extra []string) []string {
	if len(base) == 0 {
		return append([]string(nil), extra...)
	}
	if len(extra) == 0 {
		return append([]string(nil), base...)
	}
	out := append([]string(nil), base...)
	seen := make(map[string]struct{}, len(base)+len(extra))
	for _, word := range base {
		seen[word] = struct{}{}
	}
	for _, word := range extra {
		if _, ok := seen[word]; ok {
			continue
		}
		out = append(out, word)
		seen[word] = struct{}{}
	}
	return out
}

func workstationLimitsAPIFromInternal(limits interfaces.WorkstationLimits) *factoryapi.WorkstationLimits {
	if limits.MaxRetries == 0 && limits.MaxExecutionTime == "" {
		return nil
	}
	return &factoryapi.WorkstationLimits{
		MaxExecutionTime: stringPtrIfNotEmpty(limits.MaxExecutionTime),
		MaxRetries:       intPtrIfNonZero(limits.MaxRetries),
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
			Config:         workcontent.GeneratedPtrFromParts(binding.Config),
			DefaultContent: workcontent.GeneratedPtrFromParts(binding.DefaultContent),
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
		ExpiryWindow:   stringPtrIfNotEmpty(cron.ExpiryWindow),
		Jitter:         stringPtrIfNotEmpty(cron.Jitter),
		Schedule:       cron.Schedule,
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
		guards := []factoryapi.Guard{inputGuardAPIFromInternal(*cfg.Guard)}
		apiIO.Guards = &guards
	}
	return apiIO
}

func inputGuardAPIFromInternal(guard interfaces.InputGuardConfig) factoryapi.Guard {
	apiGuard := factoryapi.Guard{
		Type: publicFactoryGuardTypeFromInternal(guard.Type),
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

func workstationGuardsAPIFromInternal(guards []interfaces.GuardConfig) *[]factoryapi.Guard {
	if len(guards) == 0 {
		return nil
	}
	values := make([]factoryapi.Guard, len(guards))
	for i, guard := range guards {
		values[i] = factoryapi.Guard{
			Type:        publicFactoryGuardTypeFromInternal(guard.Type),
			Workstation: stringPtrIfNotEmpty(guard.Workstation),
			MaxVisits:   intPtrIfNonZero(guard.MaxVisits),
			MatchConfig: guardMatchConfigAPIFromInternal(guard.MatchConfig),
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
			Type:          publicFactoryGuardTypeFromInternal(guard.Type),
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

func workerTypePtrIfNotEmpty(value string) *factoryapi.WorkerType {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	enumValue := publicFactoryWorkerTypeFromInternal(value)
	return &enumValue
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
	enumValue := factoryapi.HostedWorkerProvider(publicFactoryHostedWorkerProviderFromInternal(value))
	return &enumValue
}

func workstationTypePtrIfNotEmpty(value string) *factoryapi.WorkstationType {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	enumValue := publicFactoryWorkstationTypeFromInternal(value)
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
	if value == "" {
		return nil
	}
	return stringPtr(value)
}

func stringSlicePtr(values []string) *[]string {
	if len(values) == 0 {
		return nil
	}
	copied := append([]string(nil), values...)
	return &copied
}

func stringMapPtr(values map[string]string) *factoryapi.StringMap {
	if len(values) == 0 {
		return nil
	}
	copied := make(factoryapi.StringMap, len(values))
	for key, value := range values {
		copied[key] = value
	}
	return &copied
}

func intPtrIfNonZero(value int) *int {
	if value == 0 {
		return nil
	}
	return &value
}

func boolPtrIfTrue(value bool) *bool {
	if !value {
		return nil
	}
	return &value
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func stringSliceValue(values *[]string) []string {
	if values == nil {
		return nil
	}
	return append([]string(nil), (*values)...)
}

func stringMapValue(values *factoryapi.StringMap) map[string]string {
	if values == nil {
		return nil
	}
	out := make(map[string]string, len(*values))
	for key, value := range *values {
		out[key] = value
	}
	return out
}

func intValue(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func boolValue(value *bool) bool {
	if value == nil {
		return false
	}
	return *value
}
