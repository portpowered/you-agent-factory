package service

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	modelproviders "github.com/portpowered/infinite-you/packages/model-providers"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

func projectPublishedCatalog() ([]providers.Descriptor, error) {
	var catalog publishedProviderCatalog
	if err := json.Unmarshal(modelproviders.CatalogJSON(), &catalog); err != nil {
		return nil, fmt.Errorf("parse published provider catalog: %w", err)
	}
	return projectManifests(catalog.Providers)
}

type publishedProviderCatalog struct {
	Providers []publishedProviderManifest `json:"providers"`
}

type publishedProviderManifest struct {
	Aliases                             []string                       `json:"aliases"`
	Discovery                           publishedProviderDiscovery     `json:"discovery"`
	DisplayName                         publishedNameValue             `json:"displayName"`
	ID                                  string                         `json:"id"`
	ImplementationAvailability          string                         `json:"implementationAvailability"`
	KnownLimits                         []publishedKnownLimit          `json:"knownLimits"`
	MaximumExecutionCapabilities        publishedExecutionCapabilities `json:"maximumExecutionCapabilities"`
	MaximumResponseFidelityCapabilities publishedResponseCapabilities  `json:"maximumResponseFidelityCapabilities"`
	Models                              []publishedModel               `json:"models"`
	TechnicalSupportLevel               string                         `json:"technicalSupportLevel"`
	Tools                               []publishedTool                `json:"tools"`
}

type publishedNameValue struct {
	Value string `json:"value"`
}

type publishedProviderDiscovery struct {
	ConfigurationKeys []string                `json:"configurationKeys"`
	EndpointKinds     []string                `json:"endpointKinds"`
	ExecutableNames   []string                `json:"executableNames"`
	Prerequisites     []publishedPrerequisite `json:"prerequisites"`
}

type publishedPrerequisite struct {
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type publishedModel struct {
	ID         string              `json:"id"`
	Efforts    []string            `json:"efforts"`
	Modalities []publishedModality `json:"modalities"`
}

type publishedModality struct {
	Direction string `json:"direction"`
	Kind      string `json:"modality"`
	Support   string `json:"support"`
	Transport string `json:"transport"`
}

type publishedTool struct {
	Name        string `json:"name"`
	Support     string `json:"support"`
	Description string `json:"description"`
}

type publishedKnownLimit struct {
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Unit        string `json:"unit"`
	Description string `json:"description"`
	Maximum     *int64 `json:"maximum"`
	Default     *int64 `json:"default"`
	Value       string `json:"value"`
}

type publishedExecutionCapabilities struct {
	PromptSubmission bool `json:"promptSubmission"`
	ImageInput       bool `json:"imageInput"`
	SessionResume    bool `json:"sessionResume"`
	StructuredOutput bool `json:"structuredOutput"`
}

type publishedResponseCapabilities struct {
	NativeStreaming    bool `json:"nativeStreaming"`
	MessageDeltas      bool `json:"messageDeltas"`
	MessageSnapshots   bool `json:"messageSnapshots"`
	ReasoningSummaries bool `json:"reasoningSummaries"`
	ToolLifecycle      bool `json:"toolLifecycle"`
	ToolOutputDeltas   bool `json:"toolOutputDeltas"`
	FileChanges        bool `json:"fileChanges"`
	Plans              bool `json:"plans"`
	Usage              bool `json:"usage"`
	StableItemIDs      bool `json:"stableItemIds"`
	ProviderReconnect  bool `json:"providerReconnect"`
}

func projectManifests(manifests []publishedProviderManifest) ([]providers.Descriptor, error) {
	descriptors := make([]providers.Descriptor, 0, len(manifests))
	for _, manifest := range manifests {
		descriptor, err := projectManifest(manifest)
		if err != nil {
			return nil, err
		}
		descriptors = append(descriptors, descriptor)
	}
	sort.Slice(descriptors, func(i, j int) bool {
		return descriptors[i].ID.String() < descriptors[j].ID.String()
	})
	return descriptors, nil
}

func projectManifest(manifest publishedProviderManifest) (providers.Descriptor, error) {
	canonicalID := canonicalProvidersID(manifest.ID)
	if err := canonicalID.Validate(); err != nil {
		return providers.Descriptor{}, fmt.Errorf("project provider %q: %w", manifest.ID, err)
	}
	availability := projectAvailability(manifest)
	models, err := projectModels(manifest.ID, manifest.Models)
	if err != nil {
		return providers.Descriptor{}, err
	}
	tools, err := projectTools(manifest.ID, manifest.Tools)
	if err != nil {
		return providers.Descriptor{}, err
	}
	knownLimits, err := projectKnownLimits(manifest.ID, manifest.KnownLimits)
	if err != nil {
		return providers.Descriptor{}, err
	}
	return providers.Descriptor{
		ID:                         canonicalID,
		Aliases:                    projectAliases(manifest.ID, manifest.Aliases, canonicalID),
		DisplayName:                localizedValue(manifest.DisplayName),
		Availability:               availability,
		Readiness:                  projectReadiness(availability),
		TechnicalSupportLevel:      projectTechnicalSupportLevel(manifest.TechnicalSupportLevel),
		ImplementationAvailability: projectImplementationAvailability(manifest.ImplementationAvailability),
		Prerequisites:              projectStaticPrerequisites(manifest),
		Models:                     models,
		Tools:                      tools,
		KnownLimits:                knownLimits,
		Capabilities:               projectCapabilities(manifest),
	}, nil
}

func canonicalProvidersID(manifestID string) providers.ID {
	switch strings.ToLower(strings.TrimSpace(manifestID)) {
	case "antigravity":
		return providers.IDAntigravity
	default:
		return providers.ID(manifestID)
	}
}

func projectAliases(manifestID string, aliases []string, canonical providers.ID) []string {
	seen := make(map[string]struct{}, len(aliases)+1)
	collected := make([]string, 0, len(aliases)+1)
	add := func(value string) {
		normalized := strings.ToLower(strings.TrimSpace(value))
		if normalized == "" || normalized == canonical.String() {
			return
		}
		if _, ok := seen[normalized]; ok {
			return
		}
		seen[normalized] = struct{}{}
		collected = append(collected, normalized)
	}
	for _, alias := range aliases {
		add(alias)
	}
	add(manifestID)
	sort.Strings(collected)
	return collected
}

func projectAvailability(manifest publishedProviderManifest) providers.Availability {
	switch manifest.TechnicalSupportLevel {
	case "not-supported":
		return providers.AvailabilityNotSupported
	}
	switch manifest.ImplementationAvailability {
	case "catalog-only":
		return providers.AvailabilityCatalogOnly
	case "bundled", "externally-supplied":
		return providers.AvailabilitySelectable
	default:
		return providers.AvailabilitySupportedButUnavailable
	}
}

func projectReadiness(availability providers.Availability) providers.Readiness {
	switch availability {
	case providers.AvailabilitySelectable:
		return providers.ReadinessReady
	default:
		return providers.ReadinessUnavailable
	}
}

func projectStaticPrerequisites(manifest publishedProviderManifest) []providers.Prerequisite {
	displayName := localizedValue(manifest.DisplayName)
	if displayName == "" {
		displayName = strings.TrimSpace(manifest.ID)
	}
	structuredExecutableNames := make(map[string]struct{})
	for _, prerequisite := range manifest.Discovery.Prerequisites {
		if strings.EqualFold(strings.TrimSpace(prerequisite.Kind), "executable") {
			structuredExecutableNames[strings.ToLower(strings.TrimSpace(prerequisite.Name))] = struct{}{}
		}
	}
	prerequisites := make([]providers.Prerequisite, 0,
		len(manifest.Discovery.ConfigurationKeys)+
			len(manifest.Discovery.EndpointKinds)+
			len(manifest.Discovery.ExecutableNames)+
			len(manifest.Discovery.Prerequisites),
	)
	for _, key := range manifest.Discovery.ConfigurationKeys {
		prerequisites = append(prerequisites, providers.Prerequisite{
			Kind:        providers.PrerequisiteConfiguration,
			Name:        strings.TrimSpace(key),
			Status:      providers.PrerequisiteSatisfied,
			Description: fmt.Sprintf("%s lists configuration key %q.", displayName, strings.TrimSpace(key)),
		})
	}
	for _, kind := range manifest.Discovery.EndpointKinds {
		kindName := strings.TrimSpace(kind)
		prerequisites = append(prerequisites, providers.Prerequisite{
			Kind:        providers.PrerequisiteConfiguration,
			Name:        kindName,
			Status:      providers.PrerequisiteSatisfied,
			Description: fmt.Sprintf("%s supports %s transport.", displayName, kindName),
		})
	}
	for _, executable := range manifest.Discovery.ExecutableNames {
		executable = strings.TrimSpace(executable)
		if _, exists := structuredExecutableNames[strings.ToLower(executable)]; exists {
			continue
		}
		prerequisites = append(prerequisites, providers.Prerequisite{
			Kind:        providers.PrerequisiteDependency,
			Name:        executable,
			Status:      providers.PrerequisiteSatisfied,
			Description: fmt.Sprintf("%s uses the %q executable.", displayName, executable),
		})
	}
	for _, prerequisite := range manifest.Discovery.Prerequisites {
		kind, ok := projectPrerequisiteKind(prerequisite.Kind)
		if !ok {
			continue
		}
		prerequisites = append(prerequisites, providers.Prerequisite{
			Kind:        kind,
			Name:        strings.TrimSpace(prerequisite.Name),
			Status:      providers.PrerequisiteSatisfied,
			Description: strings.TrimSpace(prerequisite.Description),
		})
	}
	if len(prerequisites) == 0 {
		return nil
	}
	sort.Slice(prerequisites, func(i, j int) bool {
		left := prerequisiteSortKey(prerequisites[i])
		right := prerequisiteSortKey(prerequisites[j])
		return left < right
	})
	return prerequisites
}

func projectPrerequisiteKind(kind string) (providers.PrerequisiteKind, bool) {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "authentication":
		return providers.PrerequisiteAuthentication, true
	case "configuration":
		return providers.PrerequisiteConfiguration, true
	case "executable":
		return providers.PrerequisiteExecutable, true
	case "workspace":
		return providers.PrerequisiteWorkspace, true
	default:
		return "", false
	}
}

func projectTechnicalSupportLevel(value string) providers.TechnicalSupportLevel {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "production":
		return providers.TechnicalSupportProduction
	case "experimental":
		return providers.TechnicalSupportExperimental
	case "not-supported":
		return providers.TechnicalSupportNotSupported
	default:
		return ""
	}
}

func projectImplementationAvailability(value string) providers.ImplementationAvailability {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "bundled":
		return providers.ImplementationBundled
	case "externally-supplied":
		return providers.ImplementationExternallySupplied
	case "catalog-only":
		return providers.ImplementationCatalogOnly
	default:
		return ""
	}
}

func projectModels(providerID string, manifests []publishedModel) ([]providers.ModelDescriptor, error) {
	models := make([]providers.ModelDescriptor, 0, len(manifests))
	seen := make(map[string]struct{}, len(manifests))
	for _, manifest := range manifests {
		id := strings.TrimSpace(manifest.ID)
		if id == "" {
			return nil, fmt.Errorf("provider %q: model id is required", providerID)
		}
		if _, exists := seen[id]; exists {
			return nil, fmt.Errorf("provider %q: duplicate model id %q", providerID, id)
		}
		seen[id] = struct{}{}
		efforts, err := projectEfforts(providerID, id, manifest.Efforts)
		if err != nil {
			return nil, err
		}
		modalities, err := projectModalities(providerID, id, manifest.Modalities)
		if err != nil {
			return nil, err
		}
		models = append(models, providers.ModelDescriptor{ID: id, Efforts: efforts, Modalities: modalities})
	}
	sort.Slice(models, func(i, j int) bool { return models[i].ID < models[j].ID })
	return models, nil
}

func projectEfforts(providerID, modelID string, values []string) ([]providers.ReasoningEffort, error) {
	seen := make(map[string]struct{}, len(values))
	result := make([]providers.ReasoningEffort, 0, len(values))
	for _, value := range values {
		canonical, ok := providers.ReasoningEffort(strings.TrimSpace(value)).Canonical()
		if !ok || canonical == "" {
			return nil, fmt.Errorf("provider %q model %q: unknown effort %q", providerID, modelID, value)
		}
		if _, exists := seen[canonical]; exists {
			return nil, fmt.Errorf("provider %q model %q: duplicate effort %q", providerID, modelID, canonical)
		}
		seen[canonical] = struct{}{}
		result = append(result, providers.ReasoningEffort(canonical))
	}
	sort.Slice(result, func(i, j int) bool { return effortSortKey(result[i]) < effortSortKey(result[j]) })
	return result, nil
}

func effortSortKey(value providers.ReasoningEffort) int {
	for index, candidate := range []string{"minimal", "low", "medium", "high", "xhigh", "max"} {
		if string(value) == candidate {
			return index
		}
	}
	return len("minimal")
}

func projectModalities(providerID, modelID string, values []publishedModality) ([]providers.Modality, error) {
	seen := make(map[string]struct{}, len(values))
	result := make([]providers.Modality, 0, len(values))
	for _, value := range values {
		direction := providers.ModalityDirection(strings.TrimSpace(value.Direction))
		kind := providers.ModalityKind(strings.TrimSpace(value.Kind))
		support := providers.ModalitySupport(strings.TrimSpace(value.Support))
		transport := providers.ModalityTransport(strings.TrimSpace(value.Transport))
		if direction != providers.ModalityDirectionInput && direction != providers.ModalityDirectionOutput {
			return nil, fmt.Errorf("provider %q model %q: unknown modality direction %q", providerID, modelID, value.Direction)
		}
		if !isKnownModalityKind(kind) {
			return nil, fmt.Errorf("provider %q model %q: unknown modality %q", providerID, modelID, value.Kind)
		}
		if support != providers.ModalitySupported && support != providers.ModalityUnsupported {
			return nil, fmt.Errorf("provider %q model %q: unknown modality support %q", providerID, modelID, value.Support)
		}
		if !isKnownModalityTransport(transport) {
			return nil, fmt.Errorf("provider %q model %q: unknown modality transport %q", providerID, modelID, value.Transport)
		}
		if (support == providers.ModalityUnsupported) != (transport == providers.ModalityTransportNone) {
			return nil, fmt.Errorf("provider %q model %q modality %s/%s has inconsistent support and transport", providerID, modelID, direction, kind)
		}
		key := string(direction) + "\x00" + string(kind)
		if _, exists := seen[key]; exists {
			return nil, fmt.Errorf("provider %q model %q: duplicate modality %s/%s", providerID, modelID, direction, kind)
		}
		seen[key] = struct{}{}
		result = append(result, providers.Modality{Direction: direction, Kind: kind, Support: support, Transport: transport})
	}
	sort.Slice(result, func(i, j int) bool {
		return modalitySortKey(result[i]) < modalitySortKey(result[j])
	})
	return result, nil
}

func isKnownModalityKind(kind providers.ModalityKind) bool {
	switch kind {
	case providers.ModalityText, providers.ModalityImage, providers.ModalityAudio, providers.ModalityVideo:
		return true
	default:
		return false
	}
}

func isKnownModalityTransport(transport providers.ModalityTransport) bool {
	switch transport {
	case providers.ModalityTransportInline, providers.ModalityTransportFilePath, providers.ModalityTransportNone:
		return true
	default:
		return false
	}
}

func modalitySortKey(modality providers.Modality) string {
	return strings.Join([]string{string(modality.Direction), string(modality.Kind), string(modality.Support), string(modality.Transport)}, "\x00")
}

func projectTools(providerID string, values []publishedTool) ([]providers.Tool, error) {
	result := make([]providers.Tool, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		name := strings.TrimSpace(value.Name)
		if name == "" || strings.TrimSpace(value.Description) == "" {
			return nil, fmt.Errorf("provider %q: incomplete tool record %q", providerID, name)
		}
		support := providers.ToolSupport(strings.TrimSpace(value.Support))
		if support != providers.ToolSupported && support != providers.ToolUnsupported {
			return nil, fmt.Errorf("provider %q tool %q: unknown support %q", providerID, name, value.Support)
		}
		if _, exists := seen[name]; exists {
			return nil, fmt.Errorf("provider %q: duplicate tool %q", providerID, name)
		}
		seen[name] = struct{}{}
		result = append(result, providers.Tool{Name: name, Support: support, Description: strings.TrimSpace(value.Description)})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func projectKnownLimits(providerID string, values []publishedKnownLimit) ([]providers.KnownLimit, error) {
	result := make([]providers.KnownLimit, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		name := strings.TrimSpace(value.Name)
		kind := providers.KnownLimitKind(strings.TrimSpace(value.Kind))
		if name == "" || strings.TrimSpace(value.Unit) == "" || strings.TrimSpace(value.Description) == "" {
			return nil, fmt.Errorf("provider %q: incomplete known-limit record %q", providerID, name)
		}
		if _, exists := seen[name]; exists {
			return nil, fmt.Errorf("provider %q: duplicate known limit %q", providerID, name)
		}
		seen[name] = struct{}{}
		if err := validateKnownLimitValue(providerID, name, kind, value); err != nil {
			return nil, err
		}
		result = append(result, providers.KnownLimit{
			Name: name, Kind: kind, Unit: strings.TrimSpace(value.Unit),
			Description: strings.TrimSpace(value.Description), Maximum: cloneInt64(value.Maximum),
			Default: cloneInt64(value.Default), Value: strings.TrimSpace(value.Value),
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func validateKnownLimitValue(providerID, name string, kind providers.KnownLimitKind, value publishedKnownLimit) error {
	switch kind {
	case providers.KnownLimitMaximum:
		if err := validateMaximumKnownLimitValue(value); err != nil {
			return fmt.Errorf("provider %q known limit %q: %w", providerID, name, err)
		}
	case providers.KnownLimitDefault:
		if err := validateDefaultKnownLimitValue(value); err != nil {
			return fmt.Errorf("provider %q known limit %q: %w", providerID, name, err)
		}
	case providers.KnownLimitBehavior:
		if err := validateBehaviorKnownLimitValue(value); err != nil {
			return fmt.Errorf("provider %q known limit %q: %w", providerID, name, err)
		}
	default:
		return fmt.Errorf("provider %q known limit %q: unknown kind %q", providerID, name, kind)
	}
	return nil
}

func validateMaximumKnownLimitValue(value publishedKnownLimit) error {
	if value.Maximum == nil || *value.Maximum <= 0 || value.Default != nil || strings.TrimSpace(value.Value) != "" {
		return fmt.Errorf("maximum record is incomplete or invalid")
	}
	return nil
}

func validateDefaultKnownLimitValue(value publishedKnownLimit) error {
	if value.Default == nil || *value.Default <= 0 || value.Maximum != nil || strings.TrimSpace(value.Value) != "" {
		return fmt.Errorf("default record is incomplete or invalid")
	}
	return nil
}

func validateBehaviorKnownLimitValue(value publishedKnownLimit) error {
	if strings.TrimSpace(value.Value) == "" || value.Maximum != nil || value.Default != nil {
		return fmt.Errorf("behavior record is incomplete or invalid")
	}
	return nil
}

func cloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func prerequisiteSortKey(prerequisite providers.Prerequisite) string {
	return strings.Join([]string{
		string(prerequisite.Kind),
		prerequisite.Name,
		string(prerequisite.Status),
		prerequisite.Description,
	}, "\x00")
}

func projectCapabilities(manifest publishedProviderManifest) []providers.Capability {
	execution := manifest.MaximumExecutionCapabilities
	response := manifest.MaximumResponseFidelityCapabilities
	capabilities := make([]providers.Capability, 0, len(allProviderCapabilities()))
	appendIf := func(enabled bool, capability providers.Capability) {
		if enabled {
			capabilities = append(capabilities, capability)
		}
	}
	appendIf(execution.PromptSubmission, providers.CapabilityPromptSubmission)
	appendIf(execution.ImageInput, providers.CapabilityImageInput)
	appendIf(execution.SessionResume, providers.CapabilitySessionResume)
	appendIf(execution.StructuredOutput, providers.CapabilityStructuredOutput)
	appendIf(response.NativeStreaming, providers.CapabilityNativeStreaming)
	appendIf(response.MessageDeltas, providers.CapabilityMessageDeltas)
	appendIf(response.MessageSnapshots, providers.CapabilityMessageSnapshots)
	appendIf(response.ReasoningSummaries, providers.CapabilityReasoningSummaries)
	appendIf(response.ToolLifecycle, providers.CapabilityToolLifecycle)
	appendIf(response.ToolOutputDeltas, providers.CapabilityToolOutputDeltas)
	appendIf(response.FileChanges, providers.CapabilityFileChanges)
	appendIf(response.Plans, providers.CapabilityPlans)
	appendIf(response.Usage, providers.CapabilityUsage)
	appendIf(response.StableItemIDs, providers.CapabilityStableItemIDs)
	appendIf(response.ProviderReconnect, providers.CapabilityProviderReconnect)
	return capabilities
}

func localizedValue(value publishedNameValue) string {
	return strings.TrimSpace(value.Value)
}

func allProviderCapabilities() []providers.Capability {
	return []providers.Capability{
		providers.CapabilityPromptSubmission,
		providers.CapabilityImageInput,
		providers.CapabilitySessionResume,
		providers.CapabilityStructuredOutput,
		providers.CapabilityNativeStreaming,
		providers.CapabilityMessageDeltas,
		providers.CapabilityMessageSnapshots,
		providers.CapabilityReasoningSummaries,
		providers.CapabilityToolLifecycle,
		providers.CapabilityToolOutputDeltas,
		providers.CapabilityFileChanges,
		providers.CapabilityPlans,
		providers.CapabilityUsage,
		providers.CapabilityStableItemIDs,
		providers.CapabilityProviderReconnect,
	}
}
