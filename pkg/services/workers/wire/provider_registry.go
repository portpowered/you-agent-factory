package wire

import (
	"context"
	"fmt"
	"sort"
	"strings"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// NewProviderRegistry adapts providers.Service into the legacy Workers
// selection contract used by Runtime construction and preflight. Identity,
// alias, catalog, and prerequisite authority remain on Providers; this adapter
// only projects Providers-owned facts into Workers runner vocabulary.
func NewProviderRegistry(ctx context.Context, service providers.Service) (workers.ProviderRegistry, error) {
	if service == nil {
		return nil, fmt.Errorf("construct Workers provider catalog: Providers service is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &providerRegistry{service: service}, nil
}

type providerRegistry struct {
	service providers.Service
}

func (*providerRegistry) UsesNativeRunner(string) bool { return true }

func (registry *providerRegistry) CanonicalIdentity(identity string) (string, error) {
	resolved, err := registry.service.ResolveIdentity(
		context.Background(),
		providers.ResolveIdentityRequest{Identity: identity},
	)
	if err != nil {
		return "", err
	}
	return resolved.ID.String(), nil
}

func (registry *providerRegistry) RunnerIdentities() []string {
	listed, err := registry.service.ListProviders(context.Background(), providers.ListProvidersRequest{})
	if err != nil {
		return nil
	}
	identities := make([]string, 0, len(listed.Providers))
	for _, descriptor := range listed.Providers {
		if descriptor.Availability != providers.AvailabilitySelectable {
			continue
		}
		identities = append(identities, runnerIdentity(descriptor.ID))
	}
	sort.Strings(identities)
	return identities
}

func (registry *providerRegistry) RunnerMetadata(identity string) (workers.RunnerMetadata, error) {
	resolved, err := registry.service.ResolveIdentity(
		context.Background(),
		providers.ResolveIdentityRequest{Identity: identity},
	)
	if err != nil {
		return workers.RunnerMetadata{}, err
	}
	runnerID := runnerIdentity(resolved.ID)
	if metadata, ok := workers.BuiltInRunnerMetadata(runnerID); ok {
		return metadata, nil
	}
	listed, err := registry.service.ListProviders(context.Background(), providers.ListProvidersRequest{})
	if err != nil {
		return workers.RunnerMetadata{}, err
	}
	var descriptor providers.Descriptor
	found := false
	for _, entry := range listed.Providers {
		if entry.ID == resolved.ID {
			descriptor = entry
			found = true
			break
		}
	}
	if !found {
		return workers.RunnerMetadata{}, fmt.Errorf("%w: %q", providers.ErrUnknownProvider, identity)
	}
	optional := []workers.RunnerOptionalCapabilitySupport{}
	for _, capability := range []struct {
		provider providers.Capability
		worker   workers.RunnerOptionalCapability
	}{
		{providers.CapabilityImageInput, workers.RunnerOptionalCapabilityImageInput},
		{providers.CapabilitySessionResume, workers.RunnerOptionalCapabilitySessionResume},
		{providers.CapabilityStructuredOutput, workers.RunnerOptionalCapabilityStructuredOutput},
	} {
		status := workers.RunnerOptionalCapabilityStatusUnsupported
		for _, supported := range descriptor.Capabilities {
			if supported == capability.provider {
				status = workers.RunnerOptionalCapabilityStatusSupported
				break
			}
		}
		optional = append(optional, workers.RunnerOptionalCapabilitySupport{
			Capability: capability.worker,
			Status:     status,
		})
	}
	return workers.RunnerMetadata{
		ID:           runnerID,
		DisplayName:  descriptor.DisplayName,
		Capabilities: workers.NewCapabilities(optional...),
	}, nil
}

func (registry *providerRegistry) ValidateRunnerPrerequisites(
	_ platformprocess.ExecutableLocator,
	identity string,
) error {
	resolved, err := registry.service.ResolveIdentity(
		context.Background(),
		providers.ResolveIdentityRequest{Identity: identity},
	)
	if err != nil {
		return err
	}
	return registry.service.ValidatePrerequisites(
		context.Background(),
		providers.ValidatePrerequisitesRequest{ID: resolved.ID},
	)
}

func (registry *providerRegistry) ResolveRunnerSelection(
	workstation string,
	factory string,
	model string,
) (workers.ResolvedRunnerSelection, error) {
	resolved, err := registry.service.ResolveSelection(
		context.Background(),
		providers.ResolveSelectionRequest{
			Workstation:   workstation,
			Factory:       factory,
			ModelProvider: model,
		},
	)
	if err != nil {
		return workers.ResolvedRunnerSelection{}, err
	}
	return workers.ResolvedRunnerSelection{
		RunnerID: runnerIdentity(resolved.Provider),
		Source:   selectionSource(resolved.Source),
	}, nil
}

func runnerIdentity(identity providers.ID) string {
	switch identity {
	case providers.IDAntigravity:
		return workers.RunnerIDAntigravity
	default:
		return strings.ToLower(strings.TrimSpace(identity.String()))
	}
}

func selectionSource(source providers.SelectionSource) workers.RunnerSelectionSource {
	switch source {
	case providers.SelectionSourceWorkstation:
		return workers.RunnerSelectionSourceWorkstation
	case providers.SelectionSourceFactory:
		return workers.RunnerSelectionSourceFactory
	case providers.SelectionSourceLegacyProvider:
		return workers.RunnerSelectionSourceLegacyProvider
	default:
		return workers.RunnerSelectionSourceDefault
	}
}

var _ workers.ProviderRegistry = (*providerRegistry)(nil)
