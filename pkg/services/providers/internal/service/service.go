// Package service implements the published Providers root Service by
// delegating catalog list/get to the parent-private catalog subservice.
package service

import (
	"context"
	"fmt"
	"sort"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	acp "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/acp"
	catalog "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
)

// Service fulfills the published Providers root contract.
type Service struct {
	catalog     catalog.Service
	execution   execution.Service
	acp         acp.Service
	packagedACP []providers.ACPIntegration
	lifecycles  []providers.Lifecycle
}

var _ providers.Service = (*Service)(nil)

// New constructs an inert Providers root facade over its two private sibling
// capabilities.
func New(catalogService catalog.Service, executionService execution.Service) (providers.Service, error) {
	return newService(catalogService, executionService, nil, nil)
}

// NewWithACP constructs the production Providers root with its persistent ACP
// subservice and exact lifecycle roles.
func NewWithACP(
	catalogService catalog.Service,
	executionService execution.Service,
	acpService acp.Service,
	packagedACP []providers.ACPIntegration,
	lifecycles ...providers.Lifecycle,
) (providers.Service, error) {
	return newService(catalogService, executionService, acpService, packagedACP, lifecycles...)
}

func newService(
	catalogService catalog.Service,
	executionService execution.Service,
	acpService acp.Service,
	packagedACP []providers.ACPIntegration,
	lifecycles ...providers.Lifecycle,
) (providers.Service, error) {
	if catalogService == nil {
		return nil, fmt.Errorf("construct Providers: catalog is required")
	}
	if executionService == nil {
		return nil, fmt.Errorf("construct Providers: execution is required")
	}
	for index, lifecycle := range lifecycles {
		if lifecycle == nil {
			return nil, fmt.Errorf("construct Providers: lifecycle %d is required", index)
		}
	}
	return &Service{catalog: catalogService, execution: executionService, acp: acpService, packagedACP: cloneACPIntegrations(packagedACP), lifecycles: append([]providers.Lifecycle(nil), lifecycles...)}, nil
}

func (s *Service) ListProviders(
	ctx context.Context,
	request providers.ListProvidersRequest,
) (providers.ListProvidersResult, error) {
	listed, err := s.catalog.ListProviders(ctx, request)
	if err != nil {
		return providers.ListProvidersResult{}, err
	}
	byID := make(map[providers.ID]int, len(listed.Providers))
	for index, descriptor := range listed.Providers {
		byID[descriptor.ID] = index
	}
	if s.acp == nil {
		return listed, nil
	}
	for _, integration := range s.acp.Integrations() {
		descriptor := acpDescriptor(integration)
		if index, exists := byID[descriptor.ID]; exists {
			listed.Providers[index] = descriptor
		} else {
			listed.Providers = append(listed.Providers, descriptor)
		}
	}
	sort.Slice(listed.Providers, func(i, j int) bool { return listed.Providers[i].ID < listed.Providers[j].ID })
	return listed, nil
}

func (s *Service) GetProvider(
	ctx context.Context,
	request providers.GetProviderRequest,
) (providers.GetProviderResult, error) {
	if s.acp != nil {
		if canonical, ok := s.acp.Resolve(request.ID); ok {
			for _, integration := range s.acp.Integrations() {
				if integration.Name == canonical {
					return providers.GetProviderResult{Provider: acpDescriptor(integration)}, nil
				}
			}
		}
	}
	return s.catalog.GetProvider(ctx, request)
}

func (s *Service) Execute(
	ctx context.Context,
	request providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	if err := request.Validate(); err != nil {
		return providers.ExecuteResult{}, providers.ExecuteFailure{
			Kind:    providers.ExecuteFailureKindInvalidRequest,
			Message: err.Error(),
		}
	}
	request.ReasoningEffort, _ = providers.ReasoningEffort(request.ReasoningEffort).Canonical()
	if s.acp != nil {
		if canonical, ok := s.acp.Resolve(request.Provider); ok {
			if request.ReasoningEffort != "" {
				return providers.ExecuteResult{}, providers.ExecuteFailure{
					Kind: providers.ExecuteFailureKindInvalidRequest,
					Message: fmt.Sprintf(
						"ACP provider %q selects reasoning effort through its exact advertised model id; omit reasoningEffort and choose the intended model",
						canonical,
					),
				}
			}
			request.Provider = canonical
			return s.acp.Execute(ctx, canonical, request)
		}
	}
	canonicalProvider, err := s.catalog.ResolveProviderID(request.Provider)
	if err != nil {
		return providers.ExecuteResult{}, err
	}
	request.Provider = canonicalProvider
	if request.Provider == providers.IDClaude && request.ReasoningEffort == "minimal" {
		return providers.ExecuteResult{}, providers.ExecuteFailure{
			Kind:    providers.ExecuteFailureKindInvalidRequest,
			Message: `Claude does not support reasoning effort "minimal"`,
		}
	}
	if request.Provider == providers.IDAntigravity && request.ReasoningEffort != "" {
		return providers.ExecuteResult{}, providers.ExecuteFailure{
			Kind:    providers.ExecuteFailureKindInvalidRequest,
			Message: "Agy does not support a separate reasoning effort",
		}
	}
	return s.execution.Execute(ctx, request)
}

func (s *Service) ConfigureACPIntegrations(ctx context.Context, configured []providers.ACPIntegration) error {
	if s.acp == nil {
		return fmt.Errorf("configure ACP integrations: ACP service is unavailable")
	}
	return s.acp.Configure(ctx, effectiveACPIntegrations(s.packagedACP, configured))
}

func (s *Service) Close(ctx context.Context) error {
	var first error
	for _, lifecycle := range s.lifecycles {
		if err := lifecycle.Close(ctx); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func cloneACPIntegrations(values []providers.ACPIntegration) []providers.ACPIntegration {
	result := make([]providers.ACPIntegration, len(values))
	for index := range values {
		result[index] = values[index].Clone()
	}
	return result
}

func effectiveACPIntegrations(packaged, configured []providers.ACPIntegration) []providers.ACPIntegration {
	values := cloneACPIntegrations(packaged)
	for _, integration := range configured {
		replaced := false
		for index := range values {
			if values[index].Name == integration.Name {
				values[index] = integration.Clone()
				replaced = true
				break
			}
		}
		if !replaced {
			values = append(values, integration.Clone())
		}
	}
	return values
}

func acpDescriptor(integration providers.ACPIntegration) providers.Descriptor {
	return providers.Descriptor{
		ID: integration.Name, Aliases: append([]string(nil), integration.Aliases...), DisplayName: integration.Name.String(),
		Availability: providers.AvailabilitySelectable, Readiness: providers.ReadinessReady,
		Capabilities: []providers.Capability{providers.CapabilityPromptSubmission, providers.CapabilityImageInput, providers.CapabilitySessionResume, providers.CapabilityNativeStreaming, providers.CapabilityMessageDeltas, providers.CapabilityReasoningSummaries, providers.CapabilityToolLifecycle, providers.CapabilityFileChanges, providers.CapabilityPlans, providers.CapabilityUsage},
	}
}

var _ providers.ACPConfiguration = (*Service)(nil)
