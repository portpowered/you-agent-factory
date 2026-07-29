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

// NewWorkersRegistry projects the Providers catalog into the narrow Workers
// selection contract. Invocation stays on providers.Service; this snapshot
// carries identity, capability, and readiness facts only.
func NewWorkersRegistry(ctx context.Context, service providers.Service) (workers.ProviderRegistry, error) {
	if service == nil {
		return nil, fmt.Errorf("construct Workers provider catalog: Providers service is required")
	}
	listed, err := service.ListProviders(ctx, providers.ListProvidersRequest{})
	if err != nil {
		return nil, fmt.Errorf("construct Workers provider catalog: %w", err)
	}
	registry := &workersRegistry{
		descriptors: make(map[string]providers.Descriptor, len(listed.Providers)),
		aliases:     make(map[string]string),
	}
	for _, descriptor := range listed.Providers {
		canonical := runnerIdentity(descriptor.ID.String())
		registry.descriptors[canonical] = descriptor.Clone()
		registry.aliases[strings.ToLower(descriptor.ID.String())] = canonical
		registry.aliases[canonical] = canonical
		for _, alias := range descriptor.Aliases {
			registry.aliases[strings.ToLower(strings.TrimSpace(alias))] = canonical
		}
	}
	// Preserve the legacy public aliases at the composition boundary.
	registry.aliases["openai"] = workers.RunnerIDCodex
	registry.aliases["anthropic"] = "claude"
	registry.aliases["agent"] = workers.RunnerIDCursorCLI
	registry.aliases["cursor"] = workers.RunnerIDCursorCLI
	registry.aliases["kiro-cli"] = workers.RunnerIDKiro
	return registry, nil
}

type workersRegistry struct {
	descriptors map[string]providers.Descriptor
	aliases     map[string]string
}

func (*workersRegistry) UsesNativeRunner(string) bool { return true }

func (registry *workersRegistry) CanonicalIdentity(identity string) (string, error) {
	canonical, err := registry.runnerCanonicalIdentity(identity)
	if err != nil {
		return "", err
	}
	switch canonical {
	case workers.RunnerIDCursorCLI:
		return "cursor", nil
	default:
		return canonical, nil
	}
}

func (registry *workersRegistry) runnerCanonicalIdentity(identity string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(identity))
	if normalized == "" {
		return "", fmt.Errorf("provider %q is invalid", identity)
	}
	canonical, ok := registry.aliases[normalized]
	if !ok {
		return "", fmt.Errorf("provider %q is unknown", identity)
	}
	return canonical, nil
}

func (registry *workersRegistry) RunnerIdentities() []string {
	identities := make([]string, 0, len(registry.descriptors))
	for identity := range registry.descriptors {
		identities = append(identities, identity)
	}
	sort.Strings(identities)
	return identities
}

func (registry *workersRegistry) RunnerMetadata(identity string) (workers.RunnerMetadata, error) {
	canonical, err := registry.runnerCanonicalIdentity(identity)
	if err != nil {
		return workers.RunnerMetadata{}, err
	}
	if metadata, ok := workers.BuiltInRunnerMetadata(canonical); ok {
		return metadata, nil
	}
	descriptor := registry.descriptors[canonical]
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
		optional = append(optional, workers.RunnerOptionalCapabilitySupport{Capability: capability.worker, Status: status})
	}
	return workers.RunnerMetadata{ID: canonical, DisplayName: descriptor.DisplayName, Capabilities: workers.NewCapabilities(optional...)}, nil
}

func (registry *workersRegistry) ValidateRunnerPrerequisites(_ platformprocess.ExecutableLocator, identity string) error {
	canonical, err := registry.runnerCanonicalIdentity(identity)
	if err != nil {
		return err
	}
	descriptor := registry.descriptors[canonical]
	if descriptor.Availability != providers.AvailabilitySelectable || descriptor.Readiness == providers.ReadinessUnavailable {
		return fmt.Errorf("provider %q is unavailable", identity)
	}
	return nil
}

func (registry *workersRegistry) ResolveRunnerSelection(workstation, factory, model string) (workers.ResolvedRunnerSelection, error) {
	selection := workers.ResolveRunnerSelection(workstation, factory, model)
	raw := selection.RunnerID
	if workstation == "" && factory == "" && strings.TrimSpace(model) != "" {
		raw = model
		selection.Source = workers.RunnerSelectionSourceLegacyProvider
	}
	canonical, err := registry.runnerCanonicalIdentity(raw)
	if err != nil {
		return workers.ResolvedRunnerSelection{}, err
	}
	selection.RunnerID = canonical
	return selection, nil
}

func runnerIdentity(identity string) string {
	switch strings.ToLower(strings.TrimSpace(identity)) {
	case providers.IDCursor.String():
		return workers.RunnerIDCursorCLI
	case providers.IDKiro.String():
		return workers.RunnerIDKiro
	default:
		return strings.ToLower(strings.TrimSpace(identity))
	}
}

var _ workers.ProviderRegistry = (*workersRegistry)(nil)
