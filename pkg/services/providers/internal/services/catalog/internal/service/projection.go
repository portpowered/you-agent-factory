package service

import (
	"fmt"
	"sort"
	"strings"

	modelproviders "github.com/portpowered/infinite-you/packages/model-providers"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func projectPublishedCatalog() ([]providers.Descriptor, error) {
	catalog, err := modelproviders.Catalog()
	if err != nil {
		return nil, err
	}
	return projectManifests(catalog.Providers)
}

func projectManifests(manifests []factoryapi.ProviderManifest) ([]providers.Descriptor, error) {
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

// ProjectManifestsForTest projects manifests for focused catalog characterization tests.
func ProjectManifestsForTest(manifests []factoryapi.ProviderManifest) ([]providers.Descriptor, error) {
	return projectManifests(manifests)
}

func projectManifest(manifest factoryapi.ProviderManifest) (providers.Descriptor, error) {
	canonicalID := canonicalProvidersID(manifest.Id)
	if err := canonicalID.Validate(); err != nil {
		return providers.Descriptor{}, fmt.Errorf("project provider %q: %w", manifest.Id, err)
	}
	availability := projectAvailability(manifest)
	return providers.Descriptor{
		ID:            canonicalID,
		Aliases:       projectAliases(manifest.Id, manifest.Aliases, canonicalID),
		DisplayName:   localizedValue(manifest.DisplayName),
		Availability:  availability,
		Readiness:     projectReadiness(availability),
		Prerequisites: projectStaticPrerequisites(manifest),
		Capabilities:  projectCapabilities(manifest),
	}, nil
}

func canonicalProvidersID(manifestID string) providers.ID {
	switch strings.ToLower(strings.TrimSpace(manifestID)) {
	case "cursor":
		return providers.IDCursor
	case "kiro":
		return providers.IDKiro
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

func projectAvailability(manifest factoryapi.ProviderManifest) providers.Availability {
	switch manifest.TechnicalSupportLevel {
	case factoryapi.ProviderTechnicalSupportLevelNotSupported:
		return providers.AvailabilityNotSupported
	}
	switch manifest.ImplementationAvailability {
	case factoryapi.ProviderImplementationAvailabilityCatalogOnly:
		return providers.AvailabilityCatalogOnly
	case factoryapi.ProviderImplementationAvailabilityBundled,
		factoryapi.ProviderImplementationAvailabilityExternallySupplied:
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

func projectStaticPrerequisites(manifest factoryapi.ProviderManifest) []providers.Prerequisite {
	displayName := localizedValue(manifest.DisplayName)
	if displayName == "" {
		displayName = strings.TrimSpace(manifest.Id)
	}
	prerequisites := make([]providers.Prerequisite, 0,
		len(manifest.Discovery.ConfigurationKeys)+
			len(manifest.Discovery.EndpointKinds)+
			len(manifest.Discovery.ExecutableNames),
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
		kindName := strings.TrimSpace(string(kind))
		prerequisites = append(prerequisites, providers.Prerequisite{
			Kind:        providers.PrerequisiteConfiguration,
			Name:        kindName,
			Status:      providers.PrerequisiteSatisfied,
			Description: fmt.Sprintf("%s supports %s transport.", displayName, kindName),
		})
	}
	for _, executable := range manifest.Discovery.ExecutableNames {
		executable = strings.TrimSpace(executable)
		prerequisites = append(prerequisites, providers.Prerequisite{
			Kind:        providers.PrerequisiteDependency,
			Name:        executable,
			Status:      providers.PrerequisiteSatisfied,
			Description: fmt.Sprintf("%s uses the %q executable.", displayName, executable),
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

func prerequisiteSortKey(prerequisite providers.Prerequisite) string {
	return strings.Join([]string{
		string(prerequisite.Kind),
		prerequisite.Name,
		string(prerequisite.Status),
		prerequisite.Description,
	}, "\x00")
}

func projectCapabilities(manifest factoryapi.ProviderManifest) []providers.Capability {
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
	appendIf(response.StableItemIds, providers.CapabilityStableItemIDs)
	appendIf(response.ProviderReconnect, providers.CapabilityProviderReconnect)
	return capabilities
}

func localizedValue(value factoryapi.NameValue) string {
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
