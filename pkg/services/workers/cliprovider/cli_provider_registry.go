package cliprovider

import (
	"sort"
	"strings"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

// CLIProviderAvailability reports whether one registered CLI provider command is
// resolvable on PATH without executing the provider or customer work.
type CLIProviderAvailability struct {
	Registration      CLIProviderRegistration
	Available         bool
	UnavailableReason string
}

// CLIProviderIdentity is the stable registry identity for one supported agent CLI
// provider. Values align with Operator Settings provider-scope segments.
type CLIProviderIdentity string

const (
	CLIProviderIdentityCodex    CLIProviderIdentity = "codex"
	CLIProviderIdentityClaude   CLIProviderIdentity = "claude"
	CLIProviderIdentityCursor   CLIProviderIdentity = "cursor"
	CLIProviderIdentityOpenCode CLIProviderIdentity = "opencode"
	CLIProviderIdentityGemini   CLIProviderIdentity = "gemini"
	CLIProviderIdentityKiro     CLIProviderIdentity = "kiro"
	CLIProviderIdentityPi       CLIProviderIdentity = "pi"
)

// CLIProviderRegistration describes one supported agent CLI provider in the
// deterministic discovery catalog.
//
// PreferenceRank establishes a total order for discovery: lower rank means higher
// preference. Ranks are fixed at registration time and do not depend on PATH
// enumeration or host filesystem ordering.
type CLIProviderRegistration struct {
	Identity       CLIProviderIdentity
	Command        string
	PreferenceRank int
}

// registeredCLIProviders is the canonical CLI provider catalog. Preference ranks
// are assigned in discovery priority order:
//
//	Codex (10) → Claude (20) → Cursor (30) → OpenCode (40) → Gemini (50) → Kiro (60)
var registeredCLIProviders = []CLIProviderRegistration{
	{
		Identity:       CLIProviderIdentityCodex,
		Command:        string(modelprovider.ProviderCodex),
		PreferenceRank: 10,
	},
	{
		Identity:       CLIProviderIdentityClaude,
		Command:        string(modelprovider.ProviderClaude),
		PreferenceRank: 20,
	},
	{
		Identity:       CLIProviderIdentityCursor,
		Command:        string(modelprovider.ProviderCursor),
		PreferenceRank: 30,
	},
	{
		Identity:       CLIProviderIdentityOpenCode,
		Command:        string(modelprovider.ProviderOpenCode),
		PreferenceRank: 40,
	},
	{
		Identity:       CLIProviderIdentityGemini,
		Command:        string(modelprovider.ProviderGemini),
		PreferenceRank: 50,
	},
	{
		Identity:       CLIProviderIdentityKiro,
		Command:        string(modelprovider.ProviderKiro),
		PreferenceRank: 60,
	},
	{
		Identity:       CLIProviderIdentityPi,
		Command:        string(modelprovider.ProviderPi),
		PreferenceRank: 65,
	},
}

var (
	cliProvidersByIdentity = buildCLIProvidersByIdentity(registeredCLIProviders)
	cliProvidersByCommand  = buildCLIProvidersByCommand(registeredCLIProviders)
)

func buildCLIProvidersByIdentity(registrations []CLIProviderRegistration) map[CLIProviderIdentity]CLIProviderRegistration {
	byIdentity := make(map[CLIProviderIdentity]CLIProviderRegistration, len(registrations))
	for _, registration := range registrations {
		byIdentity[registration.Identity] = registration
	}
	return byIdentity
}

func buildCLIProvidersByCommand(registrations []CLIProviderRegistration) map[string]CLIProviderRegistration {
	byCommand := make(map[string]CLIProviderRegistration, len(registrations))
	for _, registration := range registrations {
		byCommand[registration.Command] = registration
	}
	return byCommand
}

// RegisteredCLIProviders returns every supported agent CLI provider sorted by
// deterministic preference rank.
func RegisteredCLIProviders() []CLIProviderRegistration {
	sorted := append([]CLIProviderRegistration(nil), registeredCLIProviders...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].PreferenceRank != sorted[j].PreferenceRank {
			return sorted[i].PreferenceRank < sorted[j].PreferenceRank
		}
		return sorted[i].Identity < sorted[j].Identity
	})
	return sorted
}

// CLIProviderRegistrationByIdentity returns one registry entry for a stable
// provider identity.
func CLIProviderRegistrationByIdentity(id CLIProviderIdentity) (CLIProviderRegistration, bool) {
	registration, ok := cliProvidersByIdentity[NormalizeCLIProviderIdentity(string(id))]
	return registration, ok
}

// CLIProviderRegistrationByCommand returns one registry entry for a canonical
// CLI dispatch command name.
func CLIProviderRegistrationByCommand(command string) (CLIProviderRegistration, bool) {
	registration, ok := cliProvidersByCommand[strings.TrimSpace(command)]
	return registration, ok
}

// NormalizeCLIProviderIdentity trims and lowercases operator-supplied provider
// identity inputs into the registry vocabulary.
func NormalizeCLIProviderIdentity(id string) CLIProviderIdentity {
	return CLIProviderIdentity(strings.ToLower(strings.TrimSpace(id)))
}

// CLIProviderScopeSegment returns the provider segment used by Operator Settings
// provider backend scope derivation for one registry identity.
func CLIProviderScopeSegment(id CLIProviderIdentity) string {
	return string(NormalizeCLIProviderIdentity(string(id)))
}

// ProbeCLIProviderAvailability checks whether one registered provider command
// is resolvable on PATH. Probes perform only command-resolution checks.
func ProbeCLIProviderAvailability(
	locator platformprocess.ExecutableLocator,
	registration CLIProviderRegistration,
) CLIProviderAvailability {
	if locator == nil {
		return unavailableCLIProvider(registration)
	}
	if _, err := locator.LookPath(registration.Command); err != nil {
		return unavailableCLIProvider(registration)
	}
	return CLIProviderAvailability{
		Registration: registration,
		Available:    true,
	}
}

func unavailableCLIProvider(registration CLIProviderRegistration) CLIProviderAvailability {
	return CLIProviderAvailability{
		Registration:      registration,
		Available:         false,
		UnavailableReason: string(workerexecution.WorkFailureTypeMissingExecutable),
	}
}

// ProbeRegisteredCLIProviderAvailability probes every registered CLI provider in
// deterministic preference-rank order.
func ProbeRegisteredCLIProviderAvailability(locator platformprocess.ExecutableLocator) []CLIProviderAvailability {
	registrations := RegisteredCLIProviders()
	results := make([]CLIProviderAvailability, 0, len(registrations))
	for _, registration := range registrations {
		results = append(results, ProbeCLIProviderAvailability(locator, registration))
	}
	return results
}

// CLIProviderDiscovery reports ranked availability probes and the first
// available registered provider in preference order, if any.
type CLIProviderDiscovery struct {
	Selected     *CLIProviderRegistration
	Availability []CLIProviderAvailability
}

// DiscoverRegisteredCLIProvider probes every registered CLI provider in rank
// order and selects the first available registration. When no command is on
// PATH, Selected is nil and every probe is classified unavailable.
func DiscoverRegisteredCLIProvider(locator platformprocess.ExecutableLocator) CLIProviderDiscovery {
	availability := ProbeRegisteredCLIProviderAvailability(locator)
	discovery := CLIProviderDiscovery{Availability: availability}
	for i := range availability {
		if !availability[i].Available {
			continue
		}
		selected := availability[i].Registration
		discovery.Selected = &selected
		break
	}
	return discovery
}
