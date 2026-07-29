package wire

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

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
	registry := &workersRegistry{
		service:     service,
		descriptors: make(map[string]providers.Descriptor),
		aliases:     make(map[string]string),
	}
	if err := registry.refresh(ctx); err != nil {
		return nil, fmt.Errorf("construct Workers provider catalog: %w", err)
	}
	return registry, nil
}

type workersRegistry struct {
	mu          sync.RWMutex
	service     providers.Service
	descriptors map[string]providers.Descriptor
	aliases     map[string]string
}

func (registry *workersRegistry) refresh(ctx context.Context) error {
	listed, err := registry.service.ListProviders(ctx, providers.ListProvidersRequest{})
	if err != nil {
		return err
	}
	descriptors := make(map[string]providers.Descriptor, len(listed.Providers))
	aliases := make(map[string]string)
	for _, descriptor := range listed.Providers {
		canonical := runnerIdentity(descriptor.ID.String())
		descriptors[canonical] = descriptor.Clone()
		aliases[strings.ToLower(descriptor.ID.String())] = canonical
		aliases[canonical] = canonical
		for _, alias := range descriptor.Aliases {
			aliases[strings.ToLower(strings.TrimSpace(alias))] = canonical
		}
	}
	// Preserve the legacy public aliases at the composition boundary.
	aliases["openai"] = workers.RunnerIDCodex
	aliases["anthropic"] = "claude"
	aliases["agent"] = workers.RunnerIDCursorCLI
	aliases["cursor"] = workers.RunnerIDCursorCLI
	aliases["kiro-cli"] = workers.RunnerIDKiro
	registry.mu.Lock()
	registry.descriptors, registry.aliases = descriptors, aliases
	registry.mu.Unlock()
	return nil
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
	if err := registry.refresh(context.Background()); err != nil {
		return "", err
	}
	normalized := strings.ToLower(strings.TrimSpace(identity))
	if normalized == "" {
		return "", fmt.Errorf("provider %q is invalid", identity)
	}
	registry.mu.RLock()
	canonical, ok := registry.aliases[normalized]
	registry.mu.RUnlock()
	if !ok {
		return "", fmt.Errorf("provider %q is unknown", identity)
	}
	return canonical, nil
}

func (registry *workersRegistry) RunnerIdentities() []string {
	_ = registry.refresh(context.Background())
	registry.mu.RLock()
	defer registry.mu.RUnlock()
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
	registry.mu.RLock()
	descriptor := registry.descriptors[canonical]
	registry.mu.RUnlock()
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
	registry.mu.RLock()
	descriptor := registry.descriptors[canonical]
	registry.mu.RUnlock()
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
