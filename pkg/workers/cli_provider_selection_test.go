package workers

import (
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestSelectCLIProvider_AcceptsLayeredPrecedenceInputs(t *testing.T) {
	discovery := fakeCLIProviderDiscoveryView(map[string]bool{
		string(interfaces.ModelProviderGemini): true,
	})

	result := SelectCLIProvider(CLIProviderSelectionInput{
		ExplicitInvocation: "claude",
		FactoryDefault:     "codex",
		SystemDefault:      "gemini",
	}, discovery)

	if !result.OK() {
		t.Fatalf("result = %#v, want success", result)
	}
	if result.Source != CLIProviderSelectionSourceExplicitInvocation {
		t.Fatalf("source = %q, want %q", result.Source, CLIProviderSelectionSourceExplicitInvocation)
	}
	if result.Selected == nil || result.Selected.Identity != CLIProviderIdentityClaude {
		t.Fatalf("selected = %#v, want claude", result.Selected)
	}
}

func TestSelectCLIProvider_ReturnsStructuredSuccess(t *testing.T) {
	discovery := fakeCLIProviderDiscoveryView(nil)

	result := SelectCLIProvider(CLIProviderSelectionInput{
		FactoryDefault: string(interfaces.ModelProviderCodex),
	}, discovery)

	if result.Failure != nil {
		t.Fatalf("failure = %#v, want nil", result.Failure)
	}
	if result.Selected == nil {
		t.Fatal("selected = nil, want codex registration")
	}
	if result.Selected.Identity != CLIProviderIdentityCodex {
		t.Fatalf("selected identity = %q, want %q", result.Selected.Identity, CLIProviderIdentityCodex)
	}
	if result.Source != CLIProviderSelectionSourceFactoryDefault {
		t.Fatalf("source = %q, want %q", result.Source, CLIProviderSelectionSourceFactoryDefault)
	}
}

func TestSelectCLIProvider_ReturnsStructuredFailure(t *testing.T) {
	discovery := fakeCLIProviderDiscoveryView(nil)

	result := SelectCLIProvider(CLIProviderSelectionInput{}, discovery)

	if result.OK() {
		t.Fatalf("result = %#v, want failure", result)
	}
	if result.Selected != nil {
		t.Fatalf("selected = %#v, want nil", result.Selected)
	}
	if result.Failure == nil {
		t.Fatal("failure = nil, want structured failure")
	}
	if result.Failure.Code != CLIProviderSelectionFailureNoAgentHarness {
		t.Fatalf("failure code = %q, want %q", result.Failure.Code, CLIProviderSelectionFailureNoAgentHarness)
	}
	if result.Failure.Message == "" {
		t.Fatal("failure message = empty, want actionable text")
	}
	if result.Failure.Guidance == "" {
		t.Fatal("failure guidance = empty, want actionable text")
	}
}

func TestSelectCLIProvider_UsesInjectedDiscoveryProbeView(t *testing.T) {
	discovery := fakeCLIProviderDiscoveryView(map[string]bool{
		string(interfaces.ModelProviderCursor): true,
	})

	result := SelectCLIProvider(CLIProviderSelectionInput{}, discovery)

	if !result.OK() {
		t.Fatalf("result = %#v, want discovery success", result)
	}
	if result.Source != CLIProviderSelectionSourceDiscovery {
		t.Fatalf("source = %q, want %q", result.Source, CLIProviderSelectionSourceDiscovery)
	}
	if result.Selected == nil || result.Selected.Identity != CLIProviderIdentityCursor {
		t.Fatalf("selected = %#v, want cursor", result.Selected)
	}
}

func TestFormatCLIProviderSelectionFailure_IncludesCodeAndGuidance(t *testing.T) {
	formatted := FormatCLIProviderSelectionFailure(CLIProviderSelectionFailure{
		Code:     CLIProviderSelectionFailureNoAgentHarness,
		Message:  "no supported agent provider harness was selected",
		Guidance: "install a supported agent CLI provider on PATH",
	})

	if formatted == "" {
		t.Fatal("formatted failure = empty")
	}
	for _, want := range []string{
		"NO_AGENT_HARNESS",
		"no supported agent provider harness was selected",
		"install a supported agent CLI provider on PATH",
	} {
		if !strings.Contains(formatted, want) {
			t.Fatalf("formatted = %q, want substring %q", formatted, want)
		}
	}
}

func fakeCLIProviderDiscoveryView(presentCommands map[string]bool) CLIProviderDiscoveryView {
	return CLIProviderDiscoveryView{
		Registrations: RegisteredCLIProviders(),
		Probe: func(registration CLIProviderRegistration) CLIProviderAvailability {
			available := presentCommands != nil && presentCommands[registration.Command]
			if available {
				return CLIProviderAvailability{
					Registration: registration,
					Available:    true,
				}
			}
			return CLIProviderAvailability{
				Registration:      registration,
				Available:         false,
				UnavailableReason: string(interfaces.WorkFailureTypeMissingExecutable),
			}
		},
	}
}
