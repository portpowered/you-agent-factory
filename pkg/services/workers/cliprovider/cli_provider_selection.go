package cliprovider

import (
	"fmt"
	"sort"
	"strings"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
)

// CLIProviderAvailabilityProbe reports whether one registered provider is
// available without executing provider commands or customer work.
type CLIProviderAvailabilityProbe func(registration CLIProviderRegistration) CLIProviderAvailability

// CLIProviderDiscoveryView supplies ranked registry entries and injectable
// availability probes for supported-provider discovery.
type CLIProviderDiscoveryView struct {
	Registrations []CLIProviderRegistration
	Probe         CLIProviderAvailabilityProbe
}

// ExecutableAvailabilityProbe binds the exact process edge used to resolve
// registered provider commands. A missing locator fails closed.
func ExecutableAvailabilityProbe(locator platformprocess.ExecutableLocator) CLIProviderAvailabilityProbe {
	return func(registration CLIProviderRegistration) CLIProviderAvailability {
		return ProbeCLIProviderAvailability(locator, registration)
	}
}

// DefaultCLIProviderDiscoveryView returns the canonical registry with the given
// probe function. A missing probe fails closed rather than selecting a host
// process dependency inside the Workers service.
func DefaultCLIProviderDiscoveryView(probe CLIProviderAvailabilityProbe) CLIProviderDiscoveryView {
	if probe == nil {
		probe = ExecutableAvailabilityProbe(nil)
	}
	return CLIProviderDiscoveryView{
		Registrations: RegisteredCLIProviders(),
		Probe:         probe,
	}
}

// CLIProviderSelectionInput carries layered provider defaults resolved in fixed
// precedence: explicit invocation, factory default, system default, then
// supported-provider discovery.
type CLIProviderSelectionInput struct {
	ExplicitInvocation string
	FactoryDefault     string
	SystemDefault      string
}

// CLIProviderSelectionSource reports which precedence layer selected a provider.
type CLIProviderSelectionSource string

const (
	CLIProviderSelectionSourceExplicitInvocation CLIProviderSelectionSource = "explicit_invocation"
	CLIProviderSelectionSourceFactoryDefault     CLIProviderSelectionSource = "factory_default"
	CLIProviderSelectionSourceSystemDefault      CLIProviderSelectionSource = "system_default"
	CLIProviderSelectionSourceDiscovery          CLIProviderSelectionSource = "discovery"
)

// CLIProviderSelectionFailureCode is the machine-readable outcome when selection
// cannot resolve a supported provider.
type CLIProviderSelectionFailureCode string

const (
	CLIProviderSelectionFailureNoAgentHarness CLIProviderSelectionFailureCode = "NO_AGENT_HARNESS"
)

// CLIProviderSelectionFailure is the structured empty-selection outcome.
type CLIProviderSelectionFailure struct {
	Code     CLIProviderSelectionFailureCode
	Message  string
	Guidance string
}

// CLIProviderSelectionResult is the sole resolver output for agent-provider
// selection at the registry and shared config/service boundary.
type CLIProviderSelectionResult struct {
	Selected *CLIProviderRegistration
	Failure  *CLIProviderSelectionFailure
	Source   CLIProviderSelectionSource
}

// OK reports whether selection resolved a supported provider identity.
func (result CLIProviderSelectionResult) OK() bool {
	return result.Selected != nil && result.Failure == nil
}

// SelectCLIProvider resolves one supported agent CLI provider from layered
// inputs. Precedence is always explicit invocation, then factory default, then
// system default, then supported-provider discovery. Selection never injects
// deprecated model defaults or depends on filesystem enumeration order.
func SelectCLIProvider(
	input CLIProviderSelectionInput,
	discovery CLIProviderDiscoveryView,
) CLIProviderSelectionResult {
	if selected, ok := resolveConfiguredCLIProvider(input.ExplicitInvocation); ok {
		return cliProviderSelectionSuccess(selected, CLIProviderSelectionSourceExplicitInvocation)
	}
	if selected, ok := resolveConfiguredCLIProvider(input.FactoryDefault); ok {
		return cliProviderSelectionSuccess(selected, CLIProviderSelectionSourceFactoryDefault)
	}
	if selected, ok := resolveConfiguredCLIProvider(input.SystemDefault); ok {
		return cliProviderSelectionSuccess(selected, CLIProviderSelectionSourceSystemDefault)
	}
	if selected, ok := selectDiscoveredCLIProvider(discovery); ok {
		return cliProviderSelectionSuccess(selected, CLIProviderSelectionSourceDiscovery)
	}
	return CLIProviderSelectionResult{
		Failure: noAgentHarnessSelectionFailure(),
	}
}

func cliProviderSelectionSuccess(
	registration CLIProviderRegistration,
	source CLIProviderSelectionSource,
) CLIProviderSelectionResult {
	selected := registration
	return CLIProviderSelectionResult{
		Selected: &selected,
		Source:   source,
	}
}

func resolveConfiguredCLIProvider(value string) (CLIProviderRegistration, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return CLIProviderRegistration{}, false
	}
	if registration, ok := CLIProviderRegistrationByIdentity(NormalizeCLIProviderIdentity(trimmed)); ok {
		return registration, true
	}
	if registration, ok := CLIProviderRegistrationByCommand(trimmed); ok {
		return registration, true
	}
	return CLIProviderRegistration{}, false
}

func selectDiscoveredCLIProvider(view CLIProviderDiscoveryView) (CLIProviderRegistration, bool) {
	registrations := view.Registrations
	if len(registrations) == 0 {
		registrations = RegisteredCLIProviders()
	} else {
		registrations = sortCLIProviderRegistrationsByPreferenceRank(registrations)
	}
	probe := view.Probe
	if probe == nil {
		probe = ExecutableAvailabilityProbe(nil)
	}
	for _, registration := range registrations {
		if probe(registration).Available {
			return registration, true
		}
	}
	return CLIProviderRegistration{}, false
}

func sortCLIProviderRegistrationsByPreferenceRank(
	registrations []CLIProviderRegistration,
) []CLIProviderRegistration {
	sorted := append([]CLIProviderRegistration(nil), registrations...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].PreferenceRank != sorted[j].PreferenceRank {
			return sorted[i].PreferenceRank < sorted[j].PreferenceRank
		}
		return sorted[i].Identity < sorted[j].Identity
	})
	return sorted
}

func noAgentHarnessSelectionFailure() *CLIProviderSelectionFailure {
	return &CLIProviderSelectionFailure{
		Code:    CLIProviderSelectionFailureNoAgentHarness,
		Message: "no supported agent provider harness was selected",
		Guidance: strings.Join([]string{
			"install a supported agent CLI provider on PATH",
			"or set an explicit invocation provider",
			"or configure a factory default provider",
			"or configure a system default provider",
		}, "; "),
	}
}

// FormatCLIProviderSelectionFailure renders one structured selection failure for
// operator-facing diagnostics.
func FormatCLIProviderSelectionFailure(failure CLIProviderSelectionFailure) string {
	if failure.Code == "" {
		return failure.Message
	}
	if failure.Guidance == "" {
		return fmt.Sprintf("%s: %s", failure.Code, failure.Message)
	}
	return fmt.Sprintf("%s: %s (%s)", failure.Code, failure.Message, failure.Guidance)
}
