// Factory worker and workstation projections from generated API models to internal definitions.
package factoryconfig

import (
	"fmt"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	contentcontract "github.com/portpowered/infinite-you/pkg/transports/mapping/workcontent"
)

func workersInternalFromAPI(workers []factoryapi.Worker) ([]interfaces.FactoryWorkerConfig, error) {
	values := make([]interfaces.FactoryWorkerConfig, len(workers))
	for i, worker := range workers {
		converted, err := WorkerConfigFromOpenAPI(worker)
		if err != nil {
			return nil, fmt.Errorf("map factory.workers[%d]: %w", i, err)
		}
		values[i] = converted
	}
	return values, nil
}

func workerInternalFromAPI(worker factoryapi.Worker) interfaces.FactoryWorkerConfig {
	return interfaces.FactoryWorkerConfig{
		ID:               stringValue(worker.Id),
		Name:             worker.Name,
		Type:             internalFactoryWorkerTypeFromPublic(valueOrEmpty(worker.Type)),
		Provider:         internalFactoryHostedWorkerProviderFromPublic(string(valueOrEmpty(worker.Provider))),
		Model:            stringValue(worker.Model),
		ModelProvider:    internalFactoryWorkerModelProviderFromPublic(worker.ModelProvider),
		ReasoningEffort:  stringValue(worker.ReasoningEffort),
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
		AgentTools:       agentWorkerToolsInternalFromAPI(worker.AgentTools),
		Body:             stringValue(worker.Body),
	}
}

// WorkerConfigFromOpenAPI converts a generated OpenAPI worker model into the
// internal runtime config representation.
func WorkerConfigFromOpenAPI(worker factoryapi.Worker) (interfaces.FactoryWorkerConfig, error) {
	cfg := workerInternalFromAPI(worker)
	description, err := nameValueInternalFromAPI(worker.Description, fmt.Sprintf("factory.workers[%q].description", worker.Name))
	if err != nil {
		return interfaces.FactoryWorkerConfig{}, err
	}
	cfg.Description = description
	return cfg, nil
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

func agentWorkerToolsInternalFromAPI(cfg *factoryapi.AgentWorkerToolsConfig) *interfaces.AgentToolsConfig {
	if cfg == nil {
		return nil
	}
	return &interfaces.AgentToolsConfig{
		Policy: string(cfg.Policy),
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
		WorkerTypeName:        stringValue(workstation.Worker),
		Operation:             stringValue(workstation.Operation),
		OperationBindings:     workstationOperationBindingsInternalFromAPI(workstation.OperationBindings),
		Type:                  internalFactoryWorkstationTypeFromPublic(workstation.Type),
		PromptFile:            stringValue(workstation.PromptFile),
		OutputSchema:          stringValue(workstation.OutputSchema),
		OutputContract:        stringValue(workstation.OutputContract),
		OutcomeFormat:         enumStringValue(workstation.OutcomeFormat),
		Limits:                workstationLimitsInternalFromAPI(workstation.Limits),
		WorkPropagation:       workPropagationInternalFromAPI(workstation.WorkPropagation),
		Cron:                  workstationCronInternalFromAPI(workstation.Cron),
		Inputs:                inputs,
		Outputs:               outputs,
		ClassificationRoutes:  classificationRoutes,
		OnContinue:            onContinue,
		OnRejection:           onRejection,
		OnFailure:             onFailure,
		ExpectedArtifacts:     expectedArtifactsInternalFromAPI(workstation.ExpectedArtifacts),
		Resources:             resourceRequirementsInternalFromAPI(workstation.Resources),
		CopyReferencedScripts: boolValue(workstation.CopyReferencedScripts),
		Guards:                workstationGuardsInternalFromAPI(workstation.Guards),
		StopWords:             stringSliceValue(workstation.StopWords),
		Body:                  stringValue(workstation.Body),
		WorkingDirectory:      stringValue(workstation.WorkingDirectory),
		Worktree:              stringValue(workstation.Worktree),
		Env:                   stringMapValue(workstation.Env),
	}
	description, err := nameValueInternalFromAPI(workstation.Description, fieldPath+".description")
	if err != nil {
		return interfaces.FactoryWorkstationConfig{}, err
	}
	cfg.Description = description
	if workstation.Type != nil {
		cfg.Type = internalFactoryWorkstationTypeFromPublic(workstation.Type)
	}
	if workstation.Behavior != nil {
		cfg.Kind = internalFactoryWorkstationKindFromPublic(workstation.Behavior)
	}
	normalizeCanonicalWorkstationRuntime(&cfg)
	return cfg, nil
}

func workstationLimitsInternalFromAPI(limits *factoryapi.WorkstationLimits) interfaces.WorkstationLimits {
	if limits == nil {
		return interfaces.WorkstationLimits{}
	}
	return interfaces.WorkstationLimits{
		MaxRetries:                          intValue(limits.MaxRetries),
		MaxExecutionTime:                    stringValue(limits.MaxExecutionTime),
		MaxGeneratedWorkItems:               intValue(limits.MaxGeneratedWorkItems),
		MaxGeneratedWorkItemsArgument:       stringValue(limits.MaxGeneratedWorkItemsArgument),
		MaxGeneratedWorkItemsArgumentOffset: intValue(limits.MaxGeneratedWorkItemsArgumentOffset),
	}
}

func workPropagationInternalFromAPI(value *factoryapi.WorkPropagation) *interfaces.WorkPropagationConfig {
	if value == nil {
		return nil
	}
	return &interfaces.WorkPropagationConfig{
		Mode: interfaces.WorkPropagationMode(value.Mode),
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
			Config:         contentcontract.PartsFromGenerated(binding.Config),
			DefaultContent: contentcontract.PartsFromGenerated(binding.DefaultContent),
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
		Schedule:       stringValue(cron.Schedule),
		Every:          stringValue(cron.Every),
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
