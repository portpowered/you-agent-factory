package cliprovider

import (
	"os/exec"
	"sort"
	"strings"

	modelprovider "github.com/portpowered/infinite-you/pkg/models/provider"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
)

// CLIProviderAvailability reports whether one registered CLI provider command is
// resolvable on PATH without executing the provider or customer work.
type CLIProviderAvailability struct {
	Registration      CLIProviderRegistration
	Available         bool
	UnavailableReason string
}

// CLIProviderIdentity is the stable registry identity for one supported agent CLI
// provider. Values align with systemconfig provider-scope segments.
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
		Command:        string(modelprovider.Codex),
		PreferenceRank: 10,
	},
	{
		Identity:       CLIProviderIdentityClaude,
		Command:        string(modelprovider.Claude),
		PreferenceRank: 20,
	},
	{
		Identity:       CLIProviderIdentityCursor,
		Command:        string(modelprovider.Cursor),
		PreferenceRank: 30,
	},
	{
		Identity:       CLIProviderIdentityOpenCode,
		Command:        string(modelprovider.OpenCode),
		PreferenceRank: 40,
	},
	{
		Identity:       CLIProviderIdentityGemini,
		Command:        string(modelprovider.Gemini),
		PreferenceRank: 50,
	},
	{
		Identity:       CLIProviderIdentityKiro,
		Command:        string(modelprovider.Kiro),
		PreferenceRank: 60,
	},
	{
		Identity:       CLIProviderIdentityPi,
		Command:        string(modelprovider.Pi),
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

// CLIProviderScopeSegment returns the provider segment used by systemconfig
// provider backend scope derivation for one registry identity.
func CLIProviderScopeSegment(id CLIProviderIdentity) string {
	return string(NormalizeCLIProviderIdentity(string(id)))
}

var lookPath = exec.LookPath

// ProbeCLIProviderAvailability checks whether one registered provider command
// is resolvable on PATH. Probes perform only command-resolution checks.
func ProbeCLIProviderAvailability(registration CLIProviderRegistration) CLIProviderAvailability {
	if _, err := lookPath(registration.Command); err != nil {
		return CLIProviderAvailability{
			Registration:      registration,
			Available:         false,
			UnavailableReason: string(workerexecution.WorkFailureTypeMissingExecutable),
		}
	}
	return CLIProviderAvailability{
		Registration: registration,
		Available:    true,
	}
}

// ProbeRegisteredCLIProviderAvailability probes every registered CLI provider in
// deterministic preference-rank order.
func ProbeRegisteredCLIProviderAvailability() []CLIProviderAvailability {
	registrations := RegisteredCLIProviders()
	results := make([]CLIProviderAvailability, 0, len(registrations))
	for _, registration := range registrations {
		results = append(results, ProbeCLIProviderAvailability(registration))
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
func DiscoverRegisteredCLIProvider() CLIProviderDiscovery {
	availability := ProbeRegisteredCLIProviderAvailability()
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
