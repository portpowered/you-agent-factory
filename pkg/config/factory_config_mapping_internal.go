// backendsizecheck:ignore-file canonical public factory contract internal mapping remains consolidated until dedicated mapper seams are extracted from pkg/config.
// pkgmaintcheck:ignore-file-lines canonical public factory contract internal mapping remains consolidated until dedicated mapper seams are extracted from pkg/config.
package config

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factoryresource "github.com/portpowered/infinite-you/pkg/factory/resource"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	contentcontract "github.com/portpowered/infinite-you/pkg/transports/mapping/workcontent"
	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"
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
	cfg.InvocationReturn = invocationReturnInternalFromAPI(apiCfg.InvocationReturn)
	cfg.InvocationSignature = invocationSignatureInternalFromAPI(apiCfg.InvocationSignature)
	orchestrator, err := orchestratorInternalFromAPI(apiCfg.Orchestrator)
	if err != nil {
		return interfaces.FactoryConfig{}, err
	}
	cfg.Orchestrator = orchestrator
	if apiCfg.WorkTypes != nil {
		cfg.WorkTypes = workTypesInternalFromAPI(*apiCfg.WorkTypes)
	}
	if apiCfg.Resources != nil {
		cfg.Resources = resourcesInternalFromAPI(*apiCfg.Resources)
	}
	if apiCfg.SupportingFiles != nil {
		cfg.ResourceManifest = resourceManifestInternalFromAPI(apiCfg.SupportingFiles)
	}
	if apiCfg.Layout != nil {
		cfg.Layout = factoryLayoutInternalFromAPI(apiCfg.Layout)
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

func invocationReturnInternalFromAPI(value *factoryapi.InvocationReturn) *interfaces.InvocationReturnConfig {
	if value == nil {
		return nil
	}
	return &interfaces.InvocationReturnConfig{
		Policy:        string(value.Policy),
		WorkTypeName:  stringValue(value.WorkTypeName),
		TerminalState: stringValue(value.TerminalState),
		WorkName:      stringValue(value.WorkName),
	}
}

func invocationSignatureInternalFromAPI(value *factoryapi.FactoryInvocationSignature) *interfaces.InvocationSignatureConfig {
	if value == nil {
		return nil
	}
	return &interfaces.InvocationSignatureConfig{
		Parameters:                 invocationParametersInternalFromAPI(value.Parameters),
		UnknownNamedArgumentPolicy: enumStringValue(value.UnknownNamedArgumentPolicy),
		OutputContract:             invocationOutputContractInternalFromAPI(value.OutputContract),
		Examples:                   invocationExamplesInternalFromAPI(value.Examples),
	}
}

func invocationParametersInternalFromAPI(parameters *[]factoryapi.FactoryInvocationParameter) []interfaces.InvocationParameterConfig {
	if parameters == nil {
		return nil
	}
	values := make([]interfaces.InvocationParameterConfig, len(*parameters))
	for i, parameter := range *parameters {
		values[i] = interfaces.InvocationParameterConfig{
			Name:          parameter.Name,
			Description:   stringValue(parameter.Description),
			ExternalName:  stringValue(parameter.ExternalName),
			Aliases:       stringSliceValue(parameter.Aliases),
			TypeHint:      enumStringValue(parameter.TypeHint),
			ValueMode:     enumStringValue(parameter.ValueMode),
			Required:      boolValue(parameter.Required),
			Sensitive:     boolValue(parameter.Sensitive),
			Choices:       stringSliceValue(parameter.Choices),
			DefaultValue:  stringValue(parameter.DefaultValue),
			DefaultValues: stringSliceValue(parameter.DefaultValues),
			Bindings:      invocationParameterBindingsInternalFromAPI(parameter.Bindings),
		}
	}
	return values
}

func invocationParameterBindingsInternalFromAPI(bindings *[]factoryapi.FactoryInvocationParameterBinding) []interfaces.InvocationParameterBindingConfig {
	if bindings == nil {
		return nil
	}
	values := make([]interfaces.InvocationParameterBindingConfig, len(*bindings))
	for i, binding := range *bindings {
		values[i] = interfaces.InvocationParameterBindingConfig{
			Kind:     string(binding.Kind),
			Position: intValue(binding.Position),
		}
	}
	return values
}

func invocationOutputContractInternalFromAPI(value *factoryapi.FactoryInvocationOutputContract) *interfaces.InvocationOutputContractConfig {
	if value == nil {
		return nil
	}
	return &interfaces.InvocationOutputContractConfig{
		Mode:          enumStringValue(value.Mode),
		PathParameter: stringValue(value.PathParameter),
		ContentType:   stringValue(value.ContentType),
		FileExtension: stringValue(value.FileExtension),
		Description:   stringValue(value.Description),
	}
}

func invocationExamplesInternalFromAPI(examples *[]factoryapi.FactoryInvocationExample) []interfaces.InvocationExampleConfig {
	if examples == nil {
		return nil
	}
	values := make([]interfaces.InvocationExampleConfig, len(*examples))
	for i, example := range *examples {
		values[i] = interfaces.InvocationExampleConfig{
			Name:        example.Name,
			Description: stringValue(example.Description),
			Argv:        stringSliceValue(example.Argv),
			Stdin:       stringValue(example.Stdin),
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
				ID:   stringValue(state.Id),
				Name: state.Name,
				Type: interfaces.StateType(state.Type),
			}
		}
		values[i] = interfaces.WorkTypeConfig{
			ID:               stringValue(workType.Id),
			Name:             workType.Name,
			States:           states,
			HandlingBehavior: workTypeHandlingBehaviorInternalFromAPI(workType.HandlingBehavior),
		}
	}
	return values
}

func resourcesInternalFromAPI(resources []factoryapi.Resource) []factoryresource.Config {
	values := make([]factoryresource.Config, len(resources))
	for i, resource := range resources {
		values[i] = factoryresource.Config{
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

func factoryLayoutInternalFromAPI(layout *factoryapi.FactoryLayout) *interfaces.FactoryLayoutConfig {
	if layout == nil {
		return nil
	}
	return &interfaces.FactoryLayoutConfig{
		SchemaVersion: int(layout.SchemaVersion),
		Nodes:         factoryLayoutNodesInternalFromAPI(layout.Nodes),
		Edges:         factoryLayoutEdgesInternalFromAPI(layout.Edges),
		Groups:        factoryLayoutGroupsInternalFromAPI(layout.Groups),
		Annotations:   factoryLayoutAnnotationsInternalFromAPI(layout.Annotations),
		Viewport:      factoryLayoutViewportInternalFromAPI(layout.Viewport),
		Preferences:   factoryLayoutPreferencesInternalFromAPI(layout.Preferences),
	}
}

func factoryLayoutAnnotationsInternalFromAPI(annotations *[]factoryapi.FactoryLayoutAnnotation) []interfaces.FactoryLayoutAnnotationConfig {
	if annotations == nil {
		return nil
	}
	values := make([]interfaces.FactoryLayoutAnnotationConfig, len(*annotations))
	for i, annotation := range *annotations {
		values[i] = interfaces.FactoryLayoutAnnotationConfig{
			ID:       annotation.Id,
			Kind:     string(annotation.Kind),
			Position: factoryLayoutAnnotationPositionInternalFromAPI(annotation.Position),
			Size:     factoryLayoutAnnotationSizeInternalFromAPI(annotation.Size),
			Note:     factoryLayoutNoteInternalFromAPI(annotation.Note),
			Image:    factoryLayoutImageInternalFromAPI(annotation.Image),
		}
	}
	return values
}

func factoryLayoutAnnotationPositionInternalFromAPI(position factoryapi.FactoryLayoutAnnotationPosition) interfaces.FactoryLayoutPointConfig {
	return interfaces.FactoryLayoutPointConfig{X: float64(position.X), Y: float64(position.Y)}
}

func factoryLayoutAnnotationSizeInternalFromAPI(size *factoryapi.FactoryLayoutAnnotationSize) *interfaces.FactoryLayoutSizeConfig {
	if size == nil {
		return nil
	}
	return &interfaces.FactoryLayoutSizeConfig{Width: float64(size.Width), Height: float64(size.Height)}
}

func factoryLayoutNoteInternalFromAPI(note *factoryapi.FactoryLayoutNote) *interfaces.FactoryLayoutNoteConfig {
	if note == nil {
		return nil
	}
	return &interfaces.FactoryLayoutNoteConfig{
		Title: stringValue(note.Title),
		Body:  note.Body,
		Tone:  string(note.Tone),
	}
}

func factoryLayoutImageInternalFromAPI(image *factoryapi.FactoryLayoutImage) *interfaces.FactoryLayoutImageConfig {
	if image == nil {
		return nil
	}
	return &interfaces.FactoryLayoutImageConfig{
		Source: interfaces.FactoryLayoutImageSourceConfig{
			Kind:      string(image.Source.Kind),
			MediaType: string(image.Source.MediaType),
			Data:      base64.StdEncoding.EncodeToString(image.Source.Data),
		},
		AlternativeText: image.AlternativeText,
	}
}

func factoryLayoutNodesInternalFromAPI(nodes *[]factoryapi.FactoryLayoutNode) []interfaces.FactoryLayoutNodeConfig {
	if nodes == nil {
		return nil
	}
	values := make([]interfaces.FactoryLayoutNodeConfig, len(*nodes))
	for i, node := range *nodes {
		values[i] = interfaces.FactoryLayoutNodeConfig{
			ID:         node.Id,
			Position:   factoryLayoutPointInternalFromAPI(node.Position),
			Size:       factoryLayoutSizeInternalFromAPI(node.Size),
			Locked:     node.Locked,
			EmptyState: factoryLayoutEmptyStateInternalFromAPI(node.EmptyState),
		}
	}
	return values
}

func factoryLayoutEmptyStateInternalFromAPI(emptyState *factoryapi.FactoryLayoutEmptyState) *interfaces.FactoryLayoutEmptyStateConfig {
	if emptyState == nil {
		return nil
	}
	return &interfaces.FactoryLayoutEmptyStateConfig{
		Text:  stringValue(emptyState.Text),
		Image: factoryLayoutImageInternalFromAPI(emptyState.Image),
	}
}

func factoryLayoutEdgesInternalFromAPI(edges *[]factoryapi.FactoryLayoutEdge) []interfaces.FactoryLayoutEdgeConfig {
	if edges == nil {
		return nil
	}
	values := make([]interfaces.FactoryLayoutEdgeConfig, len(*edges))
	for i, edge := range *edges {
		values[i] = interfaces.FactoryLayoutEdgeConfig{
			ID:            edge.Id,
			Waypoints:     factoryLayoutPointsInternalFromAPI(edge.Waypoints),
			LabelPosition: factoryLayoutPointPtrInternalFromAPI(edge.LabelPosition),
		}
	}
	return values
}

func factoryLayoutGroupsInternalFromAPI(groups *[]factoryapi.FactoryLayoutGroup) []interfaces.FactoryLayoutGroupConfig {
	if groups == nil {
		return nil
	}
	values := make([]interfaces.FactoryLayoutGroupConfig, len(*groups))
	for i, group := range *groups {
		values[i] = interfaces.FactoryLayoutGroupConfig{
			ID:            group.Id,
			Label:         stringValue(group.Label),
			Bounds:        factoryLayoutBoundsInternalFromAPI(group.Bounds),
			NodeIDs:       append([]string(nil), group.NodeIds...),
			ParentGroupID: group.ParentGroupId,
			Color:         stringValue(group.Color),
			Locked:        group.Locked,
		}
	}
	return values
}

func factoryLayoutViewportInternalFromAPI(viewport *factoryapi.FactoryLayoutViewport) *interfaces.FactoryLayoutViewportConfig {
	if viewport == nil {
		return nil
	}
	return &interfaces.FactoryLayoutViewportConfig{
		X:    float64(viewport.X),
		Y:    float64(viewport.Y),
		Zoom: float64(viewport.Zoom),
	}
}

func factoryLayoutPreferencesInternalFromAPI(preferences *factoryapi.FactoryLayoutPreferences) *interfaces.FactoryLayoutPreferencesConfig {
	if preferences == nil {
		return nil
	}
	return &interfaces.FactoryLayoutPreferencesConfig{
		Direction: enumStringValue(preferences.Direction),
	}
}

func factoryLayoutPointInternalFromAPI(point factoryapi.FactoryLayoutPoint) interfaces.FactoryLayoutPointConfig {
	return interfaces.FactoryLayoutPointConfig{
		X: float64(point.X),
		Y: float64(point.Y),
	}
}

func factoryLayoutPointPtrInternalFromAPI(point *factoryapi.FactoryLayoutPoint) *interfaces.FactoryLayoutPointConfig {
	if point == nil {
		return nil
	}
	value := factoryLayoutPointInternalFromAPI(*point)
	return &value
}

func factoryLayoutPointsInternalFromAPI(points *[]factoryapi.FactoryLayoutPoint) []interfaces.FactoryLayoutPointConfig {
	if points == nil {
		return nil
	}
	values := make([]interfaces.FactoryLayoutPointConfig, len(*points))
	for i, point := range *points {
		values[i] = factoryLayoutPointInternalFromAPI(point)
	}
	return values
}

func factoryLayoutSizeInternalFromAPI(size *factoryapi.FactoryLayoutSize) *interfaces.FactoryLayoutSizeConfig {
	if size == nil {
		return nil
	}
	return &interfaces.FactoryLayoutSizeConfig{
		Width:  float64(size.Width),
		Height: float64(size.Height),
	}
}

func factoryLayoutBoundsInternalFromAPI(bounds factoryapi.FactoryLayoutBounds) interfaces.FactoryLayoutBoundsConfig {
	return interfaces.FactoryLayoutBoundsConfig{
		X:      float64(bounds.X),
		Y:      float64(bounds.Y),
		Width:  float64(bounds.Width),
		Height: float64(bounds.Height),
	}
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

func workersInternalFromAPI(workers []factoryapi.Worker) ([]workerconfig.Config, error) {
	values := make([]workerconfig.Config, len(workers))
	for i, worker := range workers {
		converted, err := WorkerConfigFromOpenAPI(worker)
		if err != nil {
			return nil, fmt.Errorf("map factory.workers[%d]: %w", i, err)
		}
		values[i] = converted
	}
	return values, nil
}

func workerInternalFromAPI(worker factoryapi.Worker) workerconfig.Config {
	return workerconfig.Config{
		ID:               stringValue(worker.Id),
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
		AgentTools:       agentWorkerToolsInternalFromAPI(worker.AgentTools),
		Body:             stringValue(worker.Body),
	}
}

func modelOperationsInternalFromAPI(operations *[]factoryapi.ModelOperation) []workerconfig.ModelOperation {
	if operations == nil {
		return nil
	}
	values := make([]workerconfig.ModelOperation, len(*operations))
	for i, operation := range *operations {
		values[i] = workerconfig.ModelOperation{
			Name:    operation.Name,
			Inputs:  modelOperationSlotsInternalFromAPI(operation.Inputs),
			Outputs: modelOperationSlotsInternalFromAPI(operation.Outputs),
		}
	}
	return values
}

func modelOperationSlotsInternalFromAPI(slots *[]factoryapi.ModelOperationSlot) []workerconfig.ModelOperationSlot {
	if slots == nil {
		return nil
	}
	values := make([]workerconfig.ModelOperationSlot, len(*slots))
	for i, slot := range *slots {
		values[i] = workerconfig.ModelOperationSlot{
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
func WorkerConfigFromOpenAPI(worker factoryapi.Worker) (workerconfig.Config, error) {
	cfg := workerInternalFromAPI(worker)
	openCodeAgent, err := openCodeAgentInternalFromAPI(worker.OpenCodeAgent, fmt.Sprintf("factory.workers[%q]", worker.Name))
	if err != nil {
		return workerconfig.Config{}, err
	}
	cfg.OpenCodeAgent = openCodeAgent
	return cfg, nil
}

func openCodeAgentInternalFromAPI(agent *string, fieldPath string) (string, error) {
	if agent == nil {
		return "", nil
	}
	if err := validateOpenCodeAgentField(fieldPath, *agent); err != nil {
		return "", err
	}
	return *agent, nil
}

func hostedWorkerAuthInternalFromAPI(auth *factoryapi.HostedWorkerAuth) *workerconfig.HostedWorkerAuthConfig {
	if auth == nil {
		return nil
	}
	return &workerconfig.HostedWorkerAuthConfig{
		SecretRef: stringValue(auth.SecretRef),
	}
}

func hostedLinearWorkerInternalFromAPI(cfg *factoryapi.HostedLinearWorkerConfig) *workerconfig.HostedLinearWorkerConfig {
	if cfg == nil {
		return nil
	}
	return &workerconfig.HostedLinearWorkerConfig{
		PollInterval: stringValue(cfg.PollInterval),
		TeamIDs:      stringSliceValue(cfg.TeamIds),
		StateIDs:     stringSliceValue(cfg.StateIds),
		Mapping:      hostedLinearWorkerMappingInternalFromAPI(cfg.Mapping),
		Claim:        hostedLinearWorkerClaimInternalFromAPI(cfg.Claim),
	}
}

func hostedLinearWorkerMappingInternalFromAPI(mapping *factoryapi.HostedLinearWorkerMapping) workerconfig.HostedLinearWorkerMappingConfig {
	if mapping == nil {
		return workerconfig.HostedLinearWorkerMappingConfig{}
	}
	return workerconfig.HostedLinearWorkerMappingConfig{
		WorkType: stringValue(mapping.WorkType),
		State:    stringValue(mapping.State),
	}
}

func hostedLinearWorkerClaimInternalFromAPI(claim *factoryapi.HostedLinearWorkerClaim) *workerconfig.HostedLinearWorkerClaimConfig {
	if claim == nil {
		return nil
	}
	return &workerconfig.HostedLinearWorkerClaimConfig{
		AssigneeField: stringValue(claim.AssigneeField),
	}
}

func agentWorkerToolsInternalFromAPI(cfg *factoryapi.AgentWorkerToolsConfig) *workerconfig.AgentToolsConfig {
	if cfg == nil {
		return nil
	}
	return &workerconfig.AgentToolsConfig{
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
	openCodeAgent, err := openCodeAgentInternalFromAPI(workstation.OpenCodeAgent, fieldPath)
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
		Resources:             resourceRequirementsInternalFromAPI(workstation.Resources),
		CopyReferencedScripts: boolValue(workstation.CopyReferencedScripts),
		Guards:                workstationGuardsInternalFromAPI(workstation.Guards),
		StopWords:             stringSliceValue(workstation.StopWords),
		Body:                  stringValue(workstation.Body),
		WorkingDirectory:      stringValue(workstation.WorkingDirectory),
		Worktree:              stringValue(workstation.Worktree),
		Env:                   stringMapValue(workstation.Env),
		OpenCodeAgent:         openCodeAgent,
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

func workstationLimitsInternalFromAPI(limits *factoryapi.WorkstationLimits) interfaces.WorkstationLimits {
	if limits == nil {
		return interfaces.WorkstationLimits{}
	}
	return interfaces.WorkstationLimits{
		MaxRetries:       intValue(limits.MaxRetries),
		MaxExecutionTime: stringValue(limits.MaxExecutionTime),
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
			Type:          internalFactoryGuardTypeFromPublicFactoryGuard(guard.Type),
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

func resourceRequirementsInternalFromAPI(resources *[]factoryapi.ResourceRequirement) []factoryresource.Config {
	if resources == nil {
		return nil
	}
	values := make([]factoryresource.Config, len(*resources))
	for i, resource := range *resources {
		values[i] = factoryresource.Config{
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

func workstationGuardsInternalFromAPI(guards *[]factoryapi.WorkstationGuard) []interfaces.GuardConfig {
	if guards == nil {
		return nil
	}
	values := make([]interfaces.GuardConfig, len(*guards))
	for i, guard := range *guards {
		values[i] = interfaces.GuardConfig{
			Type:        internalFactoryGuardTypeFromPublicWorkstationGuard(guard.Type),
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

func internalFactoryWorkerTypeFromPublic(value factoryapi.WorkerType) string {
	if runtimeType := interfaces.InternalRuntimeWorkerTypeFromPublic(string(value)); runtimeType != "" {
		return runtimeType
	}
	return strings.TrimSpace(string(value))
}

func publicFactoryWorkerModelProviderFromInternal(value string) factoryapi.WorkerModelProvider {
	return factoryapi.WorkerModelProvider(interfaces.PublicWorkerModelProviderFromInternalRuntime(value))
}

func publicFactoryWorkerModelLocalityFromInternal(value string) factoryapi.WorkerModelLocality {
	return factoryapi.WorkerModelLocality(interfaces.PermissivePublicFactoryWorkerModelLocality(value))
}

func internalFactoryWorkerModelProviderFromPublic(value *factoryapi.WorkerModelProvider) string {
	if value == nil {
		return ""
	}
	if internal, ok := interfaces.InternalModelProviderFromPublicWorkerModelProvider(string(*value)); ok {
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
	return factoryapi.WorkerProvider(interfaces.PublicWorkerProviderFromInternalRuntime(value))
}

func publicFactoryHostedWorkerProviderFromInternal(value string) string {
	return interfaces.PermissivePublicFactoryHostedWorkerProvider(value)
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
	return factoryapi.ModelOperationContentType(interfaces.PermissivePublicFactoryWorkerModelOperationContentType(value))
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
	return factoryapi.WorkstationKind(interfaces.CanonicalPublicWorkstationKind(kind))
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
	case publicFactoryWorkstationKindPoller:
		return interfaces.WorkstationKindPoller
	default:
		return interfaces.WorkstationKind(strings.TrimSpace(string(*kind)))
	}
}

func publicFactoryWorkstationTypeFromInternal(workstation interfaces.FactoryWorkstationConfig, workerType string) factoryapi.WorkstationType {
	return factoryapi.WorkstationType(interfaces.PublicWorkstationTypeFromInternalRuntime(workstation.Type, workerType, workstation.Kind))
}

func internalFactoryWorkstationTypeFromPublic(value *factoryapi.WorkstationType) string {
	if value == nil {
		return ""
	}
	if runtimeType := interfaces.InternalRuntimeWorkstationTypeFromPublic(string(*value)); runtimeType != "" || interfaces.PermissivePublicFactoryWorkstationType(string(*value)) == interfaces.WorkstationTypePoller {
		return runtimeType
	}
	return strings.TrimSpace(string(*value))
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

func orchestratorInternalFromAPI(value *factoryapi.FactoryOrchestrator) (*interfaces.FactoryOrchestratorConfig, error) {
	if value == nil {
		return nil, nil
	}
	kind := interfaces.StrictPublicFactoryOrchestratorKind(string(value.Kind))
	if kind == "" {
		return &interfaces.FactoryOrchestratorConfig{Kind: string(value.Kind)}, nil
	}
	cfg := &interfaces.FactoryOrchestratorConfig{
		Kind: kind,
	}
	if value.Petri != nil {
		cfg.Petri = &interfaces.FactoryOrchestratorPetriConfig{}
	}
	if value.Javascript != nil {
		jsCfg, err := orchestratorJavaScriptInternalFromAPI(*value.Javascript)
		if err != nil {
			return nil, err
		}
		cfg.JavaScript = jsCfg
	}
	return cfg, nil
}

func orchestratorJavaScriptInternalFromAPI(value factoryapi.FactoryOrchestratorJavaScriptConfig) (*interfaces.FactoryOrchestratorJavaScriptConfig, error) {
	cfg := &interfaces.FactoryOrchestratorJavaScriptConfig{
		Dialect:    stringValue(value.Dialect),
		SourceRef:  stringValue(value.SourceRef),
		SourceHash: stringValue(value.SourceHash),
		Entrypoint: stringValue(value.Entrypoint),
	}
	if value.Metadata != nil {
		cfg.Metadata = map[string]string(*value.Metadata)
	}
	if value.InlineSource != nil {
		cfg.InlineSource = &interfaces.FactoryOrchestratorJavaScriptInlineSource{
			Encoding: string(value.InlineSource.Encoding),
			Inline:   value.InlineSource.Inline,
		}
	}
	if value.ArgsSchema != nil {
		raw, err := json.Marshal(value.ArgsSchema)
		if err != nil {
			return nil, err
		}
		cfg.ArgsSchema = raw
	}
	if value.DefaultPolicy != nil {
		raw, err := json.Marshal(value.DefaultPolicy)
		if err != nil {
			return nil, err
		}
		cfg.DefaultPolicy = raw
	}
	if value.Agents != nil {
		cfg.Agents = make(map[string]interfaces.FactoryOrchestratorJavaScriptAgent, len(*value.Agents))
		for id, agent := range *value.Agents {
			cfg.Agents[id] = interfaces.FactoryOrchestratorJavaScriptAgent{Preset: agent.Preset}
		}
	}
	return cfg, nil
}

func orchestratorAPIFromInternal(cfg *interfaces.FactoryConfig) *factoryapi.FactoryOrchestrator {
	if cfg == nil || cfg.Orchestrator == nil {
		return nil
	}
	kind := interfaces.EffectiveOrchestratorKind(cfg)
	apiKind := factoryapi.FactoryOrchestratorKind(interfaces.StrictPublicFactoryOrchestratorKind(kind))
	result := &factoryapi.FactoryOrchestrator{
		Kind: apiKind,
	}
	if cfg.Orchestrator.Petri != nil || kind == interfaces.OrchestratorKindPetri {
		result.Petri = &factoryapi.FactoryOrchestratorPetriConfig{}
	}
	if cfg.Orchestrator.JavaScript != nil {
		result.Javascript = orchestratorJavaScriptAPIFromInternal(cfg.Orchestrator.JavaScript)
	}
	return result
}

// ProjectEffectiveOrchestratorForAPIRead fills the compatibility PETRI orchestrator
// projection when a factory has no authored orchestrator block.
func ProjectEffectiveOrchestratorForAPIRead(api factoryapi.Factory, cfg *interfaces.FactoryConfig) factoryapi.Factory {
	if api.Orchestrator != nil {
		return api
	}
	if interfaces.EffectiveOrchestratorKind(cfg) == interfaces.OrchestratorKindPetri {
		api.Orchestrator = defaultPetriOrchestratorAPI()
	}
	return api
}

func defaultPetriOrchestratorAPI() *factoryapi.FactoryOrchestrator {
	kind := factoryapi.PETRI
	return &factoryapi.FactoryOrchestrator{
		Kind:  kind,
		Petri: &factoryapi.FactoryOrchestratorPetriConfig{},
	}
}

func orchestratorJavaScriptAPIFromInternal(cfg *interfaces.FactoryOrchestratorJavaScriptConfig) *factoryapi.FactoryOrchestratorJavaScriptConfig {
	if cfg == nil {
		return nil
	}
	result := &factoryapi.FactoryOrchestratorJavaScriptConfig{
		Dialect:    stringPtrIfNotEmpty(cfg.Dialect),
		SourceRef:  stringPtrIfNotEmpty(cfg.SourceRef),
		SourceHash: stringPtrIfNotEmpty(cfg.SourceHash),
		Entrypoint: stringPtrIfNotEmpty(cfg.Entrypoint),
	}
	if len(cfg.Metadata) > 0 {
		metadata := factoryapi.StringMap(cfg.Metadata)
		result.Metadata = &metadata
	}
	if cfg.InlineSource != nil {
		result.InlineSource = &factoryapi.FactoryOrchestratorJavaScriptInlineSource{
			Encoding: factoryapi.FactoryOrchestratorJavaScriptInlineSourceEncoding(cfg.InlineSource.Encoding),
			Inline:   cfg.InlineSource.Inline,
		}
	}
	if len(cfg.ArgsSchema) > 0 {
		var argsSchema map[string]any
		if err := json.Unmarshal(cfg.ArgsSchema, &argsSchema); err == nil {
			result.ArgsSchema = &argsSchema
		}
	}
	if len(cfg.DefaultPolicy) > 0 {
		var defaultPolicy map[string]any
		if err := json.Unmarshal(cfg.DefaultPolicy, &defaultPolicy); err == nil {
			result.DefaultPolicy = &defaultPolicy
		}
	}
	if len(cfg.Agents) > 0 {
		agents := make(map[string]factoryapi.FactoryOrchestratorJavaScriptAgent, len(cfg.Agents))
		for id, agent := range cfg.Agents {
			agents[id] = factoryapi.FactoryOrchestratorJavaScriptAgent{Preset: agent.Preset}
		}
		result.Agents = &agents
	}
	return result
}

func isDefaultPetriOrchestratorAPI(value *factoryapi.FactoryOrchestrator) bool {
	if value == nil {
		return true
	}
	if value.Kind != factoryapi.PETRI {
		return false
	}
	if value.Javascript != nil {
		return false
	}
	if value.Petri == nil {
		return true
	}
	return true
}

func cloneFactoryOrchestratorConfig(cfg *interfaces.FactoryOrchestratorConfig) *interfaces.FactoryOrchestratorConfig {
	if cfg == nil {
		return nil
	}
	cloned := &interfaces.FactoryOrchestratorConfig{
		Kind: cfg.Kind,
	}
	if cfg.Petri != nil {
		cloned.Petri = &interfaces.FactoryOrchestratorPetriConfig{}
	}
	if cfg.JavaScript != nil {
		js := *cfg.JavaScript
		if cfg.JavaScript.InlineSource != nil {
			inline := *cfg.JavaScript.InlineSource
			js.InlineSource = &inline
		}
		if len(cfg.JavaScript.Metadata) > 0 {
			js.Metadata = make(map[string]string, len(cfg.JavaScript.Metadata))
			for key, value := range cfg.JavaScript.Metadata {
				js.Metadata[key] = value
			}
		}
		if len(cfg.JavaScript.ArgsSchema) > 0 {
			js.ArgsSchema = append(json.RawMessage(nil), cfg.JavaScript.ArgsSchema...)
		}
		if len(cfg.JavaScript.DefaultPolicy) > 0 {
			js.DefaultPolicy = append(json.RawMessage(nil), cfg.JavaScript.DefaultPolicy...)
		}
		if len(cfg.JavaScript.Agents) > 0 {
			js.Agents = make(map[string]interfaces.FactoryOrchestratorJavaScriptAgent, len(cfg.JavaScript.Agents))
			for id, agent := range cfg.JavaScript.Agents {
				js.Agents[id] = agent
			}
		}
		cloned.JavaScript = &js
	}
	return cloned
}
