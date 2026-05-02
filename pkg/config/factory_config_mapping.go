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
	generated      factoryapi.Factory
	normalizedJSON []byte
}

const generatedFactoryBoundaryErrorPrefix = "decode factory generated-schema boundary"

type retiredBoundaryField struct {
	key         string
	replacement string
}

var retiredFactoryBoundaryFields = []retiredBoundaryField{
	{key: "project", replacement: "use id"},
	{key: "factoryDir", replacement: "use factoryDirectory"},
	{key: "factory_dir", replacement: "use factoryDirectory"},
	{key: "resourceManifest", replacement: "use supportingFiles"},
	{key: "resource_manifest", replacement: "use supportingFiles"},
	{key: "workflowId", replacement: "remove workflowId"},
	{key: "workflow_id", replacement: "remove workflowId"},
}

var retiredWorkerBoundaryFields = []retiredBoundaryField{
	{key: "provider", replacement: "use executorProvider"},
	{key: "model_provider", replacement: "use modelProvider"},
	{key: "sessionId", replacement: "remove sessionId; provider sessions are runtime-owned"},
	{key: "session_id", replacement: "remove sessionId; provider sessions are runtime-owned"},
	{key: "concurrency", replacement: "remove concurrency; use resources to limit concurrent work"},
}

var retiredWorkstationBoundaryFields = []retiredBoundaryField{
	{key: "kind", replacement: "use behavior"},
	{key: "runtimeType", replacement: "use type"},
	{key: "runtime_type", replacement: "use type"},
	{key: "resourceUsage", replacement: "use resources"},
	{key: "resource_usage", replacement: "use resources"},
	{key: "resource-usage", replacement: "use resources"},
	{key: "stopToken", replacement: "use stopWords"},
	{key: "stop_token", replacement: "use stopWords"},
	{key: "runtimeStopWords", replacement: "use stopWords"},
	{key: "runtime_stop_words", replacement: "use stopWords"},
	{key: "timeout", replacement: "use limits.maxExecutionTime"},
}

func rejectRetiredFanInField(data []byte) error {
	var payload struct {
		Workstations []map[string]json.RawMessage `json:"workstations"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil
	}
	for index, workstation := range payload.Workstations {
		if _, ok := workstation["join"]; ok {
			return fmt.Errorf("workstations[%d].join is not supported; use per-input guards", index)
		}
	}
	return nil
}

func rejectRetiredExhaustionRulesField(data []byte) error {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil
	}
	if _, ok := payload["exhaustionRules"]; ok {
		return fmt.Errorf("exhaustion_rules is retired; use a guarded LOGICAL_MOVE workstation with a visit_count guard instead")
	}
	if _, ok := payload["exhaustion_rules"]; ok {
		return fmt.Errorf("exhaustion_rules is retired; use a guarded LOGICAL_MOVE workstation with a visit_count guard instead")
	}
	return nil
}

func rejectRetiredCronIntervalField(data []byte) error {
	var payload struct {
		Workstations []struct {
			Cron *interfaces.CronConfig `json:"cron"`
		} `json:"workstations"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil
	}
	for index, workstation := range payload.Workstations {
		if workstation.Cron != nil && workstation.Cron.HasUnsupportedInterval() {
			return fmt.Errorf("workstations[%d].cron.interval is not supported; use cron.schedule", index)
		}
	}
	return nil
}

func rejectRetiredGeneratedBoundaryAliases(data []byte) error {
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil
	}
	if err := rejectRetiredBoundaryFields(payload, "factory", retiredFactoryBoundaryFields); err != nil {
		return err
	}
	if err := rejectRetiredWorkerBoundaryAliases(payload); err != nil {
		return err
	}
	if err := rejectRetiredWorkstationBoundaryAliases(payload); err != nil {
		return err
	}
	return nil
}

func rejectRetiredWorkerBoundaryAliases(root map[string]any) error {
	workers, ok := root["workers"].([]any)
	if !ok {
		return nil
	}
	for index, item := range workers {
		worker, ok := item.(map[string]any)
		if !ok {
			continue
		}
		basePath := fmt.Sprintf("workers[%d]", index)
		if err := rejectRetiredBoundaryFields(worker, basePath, retiredWorkerBoundaryFields); err != nil {
			return err
		}
		definition, ok := worker["definition"].(map[string]any)
		if !ok {
			continue
		}
		if err := rejectRetiredBoundaryFields(definition, basePath+".definition", retiredWorkerBoundaryFields); err != nil {
			return err
		}
	}
	return nil
}

func rejectRetiredWorkstationBoundaryAliases(root map[string]any) error {
	workstations, ok := root["workstations"].([]any)
	if !ok {
		return nil
	}
	for index, item := range workstations {
		workstation, ok := item.(map[string]any)
		if !ok {
			continue
		}
		basePath := fmt.Sprintf("workstations[%d]", index)
		if err := rejectRetiredBoundaryFields(workstation, basePath, retiredWorkstationBoundaryFields); err != nil {
			return err
		}
		if err := rejectRetiredCronBoundaryAliases(workstation["cron"], basePath+".cron"); err != nil {
			return err
		}
		definition, ok := workstation["definition"].(map[string]any)
		if !ok {
			continue
		}
		if err := rejectRetiredBoundaryFields(definition, basePath+".definition", retiredWorkstationBoundaryFields); err != nil {
			return err
		}
		if err := rejectRetiredCronBoundaryAliases(definition["cron"], basePath+".definition.cron"); err != nil {
			return err
		}
	}
	return nil
}

func rejectRetiredCronBoundaryAliases(raw any, path string) error {
	cron, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	return rejectRetiredBoundaryFields(cron, path, []retiredBoundaryField{
		{key: "trigger_at_start", replacement: "use triggerAtStart"},
		{key: "expiry_window", replacement: "use expiryWindow"},
	})
}

func rejectRetiredBoundaryFields(container map[string]any, path string, fields []retiredBoundaryField) error {
	for _, field := range fields {
		if _, ok := container[field.key]; ok {
			return fmt.Errorf("%s.%s is not supported; %s", path, field.key, field.replacement)
		}
	}
	return nil
}

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
	applyOpenAPICronCompatibility(&cfg, boundary.normalizedJSON)
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
		generated:      apiCfg,
		normalizedJSON: normalizedData,
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
				Name:     resource.Name,
				Capacity: resource.Capacity,
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

func factoryInternalFromAPI(apiCfg factoryapi.Factory) (interfaces.FactoryConfig, error) {
	cfg := interfaces.FactoryConfig{Name: string(apiCfg.Name)}
	if apiCfg.Id != nil {
		cfg.Project = *apiCfg.Id
	}
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
		values[i] = interfaces.WorkTypeConfig{Name: workType.Name, States: states}
	}
	return values
}

func resourcesInternalFromAPI(resources []factoryapi.Resource) []interfaces.ResourceConfig {
	values := make([]interfaces.ResourceConfig, len(resources))
	for i, resource := range resources {
		values[i] = interfaces.ResourceConfig{
			Name:     resource.Name,
			Capacity: resource.Capacity,
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
		Model:            stringValue(worker.Model),
		ModelProvider:    internalFactoryWorkerModelProviderFromPublic(worker.ModelProvider),
		ExecutorProvider: internalFactoryWorkerProviderFromPublic(worker.ExecutorProvider),
		Command:          stringValue(worker.Command),
		Args:             stringSliceValue(worker.Args),
		Resources:        resourceRequirementsInternalFromAPI(worker.Resources),
		Timeout:          stringValue(worker.Timeout),
		StopToken:        stringValue(worker.StopToken),
		SkipPermissions:  boolValue(worker.SkipPermissions),
		Body:             stringValue(worker.Body),
	}
}

// WorkerConfigFromOpenAPI converts a generated OpenAPI worker model into the
// internal runtime config representation.
func WorkerConfigFromOpenAPI(worker factoryapi.Worker) (interfaces.WorkerConfig, error) {
	return workerInternalFromAPI(worker), nil
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
	outputs, err := workstationIOsInternalFromAPI(workstation.Outputs, fieldPath+".outputs")
	if err != nil {
		return interfaces.FactoryWorkstationConfig{}, err
	}
	onContinue, err := workstationIOPtrInternalFromAPI(workstation.OnContinue, fieldPath+".on_continue")
	if err != nil {
		return interfaces.FactoryWorkstationConfig{}, err
	}
	onRejection, err := workstationIOPtrInternalFromAPI(workstation.OnRejection, fieldPath+".on_rejection")
	if err != nil {
		return interfaces.FactoryWorkstationConfig{}, err
	}
	onFailure, err := workstationIOPtrInternalFromAPI(workstation.OnFailure, fieldPath+".on_failure")
	if err != nil {
		return interfaces.FactoryWorkstationConfig{}, err
	}
	cfg := interfaces.FactoryWorkstationConfig{
		ID:                    stringValue(workstation.Id),
		Name:                  workstation.Name,
		WorkerTypeName:        workstation.Worker,
		Type:                  internalFactoryWorkstationTypeFromPublic(workstation.Type),
		PromptFile:            stringValue(workstation.PromptFile),
		OutputSchema:          stringValue(workstation.OutputSchema),
		Limits:                workstationLimitsInternalFromAPI(workstation.Limits),
		Cron:                  workstationCronInternalFromAPI(workstation.Cron),
		Inputs:                inputs,
		Outputs:               outputs,
		OnContinue:            onContinue,
		OnRejection:           onRejection,
		OnFailure:             onFailure,
		Resources:             resourceRequirementsInternalFromAPI(workstation.Resources),
		CopyReferencedScripts: boolValue(workstation.CopyReferencedScripts),
		Guards:                workstationGuardsInternalFromAPI(workstation.Guards),
		StopWords:             stringSliceValue(workstation.StopWords),
		Body:                  stringValue(workstation.Body),
		PromptTemplate:        stringValue(workstation.PromptTemplate),
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

func workstationIOPtrInternalFromAPI(cfg *factoryapi.WorkstationIO, fieldPath string) (*interfaces.IOConfig, error) {
	if cfg == nil {
		return nil, nil
	}
	value, err := workstationIOInternalFromAPI(*cfg, fieldPath)
	if err != nil {
		return nil, err
	}
	return &value, nil
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

func workstationAPIFromInternal(workstation interfaces.FactoryWorkstationConfig) factoryapi.Workstation {
	normalized := CloneWorkstationConfig(workstation)
	NormalizeWorkstationExecutionLimit(&normalized)

	apiWorkstation := factoryapi.Workstation{
		Name:                  normalized.Name,
		Worker:                normalized.WorkerTypeName,
		Inputs:                workstationIOsAPIFromInternal(normalized.Inputs),
		Outputs:               workstationIOsAPIFromInternal(normalized.Outputs),
		Cron:                  workstationCronAPIFromInternal(normalized.Cron),
		OnContinue:            workstationIOPtrAPIFromInternal(normalized.OnContinue),
		OnRejection:           workstationIOPtrAPIFromInternal(normalized.OnRejection),
		OnFailure:             workstationIOPtrAPIFromInternal(normalized.OnFailure),
		Resources:             resourceRequirementsAPIFromInternal(normalized.Resources),
		CopyReferencedScripts: boolPtrIfTrue(normalized.CopyReferencedScripts),
		Guards:                workstationGuardsAPIFromInternal(normalized.Guards),
		StopWords:             stringSlicePtr(mergeCanonicalStopWords(normalized.StopWords, normalized.RuntimeStopWords)),
		Env:                   stringMapPtr(normalized.Env),
		Body:                  stringPtrIfNotEmpty(normalized.Body),
		Limits:                workstationLimitsAPIFromInternal(normalized.Limits),
		OutputSchema:          stringPtrIfNotEmpty(normalized.OutputSchema),
		PromptFile:            stringPtrIfNotEmpty(normalized.PromptFile),
		PromptTemplate:        stringPtrIfNotEmpty(normalized.PromptTemplate),
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
			Content: factoryapi.BundledFileContent{
				Encoding: factoryapi.BundledFileContentEncoding(file.Content.Encoding),
				Inline:   file.Content.Inline,
			},
		}
	}
	return &values
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
		Name:             def.Name,
		Args:             stringSlicePtr(def.Args),
		Body:             stringPtrIfNotEmpty(def.Body),
		Command:          stringPtrIfNotEmpty(def.Command),
		Model:            stringPtrIfNotEmpty(def.Model),
		ModelProvider:    workerModelProviderPtrIfNotEmpty(def.ModelProvider),
		ExecutorProvider: workerProviderPtrIfNotEmpty(def.ExecutorProvider),
		Resources:        resourceRequirementsAPIFromInternal(def.Resources),
		SkipPermissions:  boolPtrIfTrue(def.SkipPermissions),
		StopToken:        stringPtrIfNotEmpty(def.StopToken),
		Timeout:          stringPtrIfNotEmpty(def.Timeout),
	}
}

// WorkerConfigToOpenAPI converts an internal worker config into the generated
// OpenAPI worker model.
func WorkerConfigToOpenAPI(worker interfaces.WorkerConfig) factoryapi.Worker {
	return *workerDefinitionAPIFromInternal(&worker)
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

func workstationIOPtrAPIFromInternal(cfg *interfaces.IOConfig) *factoryapi.WorkstationIO {
	if cfg == nil {
		return nil
	}
	apiIO := workstationIOAPIFromInternal(*cfg)
	return &apiIO
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

func workerProviderPtrIfNotEmpty(value string) *factoryapi.WorkerProvider {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	enumValue := publicFactoryWorkerProviderFromInternal(value)
	return &enumValue
}

func workstationTypePtrIfNotEmpty(value string) *factoryapi.WorkstationType {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	enumValue := publicFactoryWorkstationTypeFromInternal(value)
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
