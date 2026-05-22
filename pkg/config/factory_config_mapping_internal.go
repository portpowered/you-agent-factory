package config

import (
	"fmt"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func factoryInternalFromAPI(apiCfg factoryapi.Factory) (interfaces.FactoryConfig, error) {
	cfg := interfaces.FactoryConfig{Name: string(apiCfg.Name)}
	if apiCfg.Id != nil {
		cfg.Project = *apiCfg.Id
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
		ModelLocality:    internalFactoryWorkerModelLocalityFromPublic(worker.ModelLocality),
		ExecutorProvider: internalFactoryWorkerProviderFromPublic(worker.ExecutorProvider),
		Operations:       modelOperationsInternalFromAPI(worker.Operations),
		Command:          stringValue(worker.Command),
		Args:             stringSliceValue(worker.Args),
		Resources:        resourceRequirementsInternalFromAPI(worker.Resources),
		Timeout:          stringValue(worker.Timeout),
		StopToken:        stringValue(worker.StopToken),
		SkipPermissions:  boolValue(worker.SkipPermissions),
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
	onContinue, err := optionalWorkstationIOsInternalFromAPI(workstation.OnContinue, fieldPath+".onContinue")
	if err != nil {
		return interfaces.FactoryWorkstationConfig{}, err
	}
	onRejection, err := optionalWorkstationIOsInternalFromAPI(workstation.OnRejection, fieldPath+".onRejection")
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
