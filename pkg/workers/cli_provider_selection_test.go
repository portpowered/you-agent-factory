package workers

import (
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

type cliProviderExplicitPrecedenceCase struct {
	name               string
	input              CLIProviderSelectionInput
	presentCommands    map[string]bool
	wantSource         CLIProviderSelectionSource
	wantIdentity       CLIProviderIdentity
	wantFailure        bool
	wantFailureCode    CLIProviderSelectionFailureCode
}

func fakeCLIProviderExplicitPrecedenceCases() []cliProviderExplicitPrecedenceCase {
	allPresent := map[string]bool{
		string(interfaces.ModelProviderCodex):    true,
		string(interfaces.ModelProviderClaude):   true,
		string(interfaces.ModelProviderCursor):   true,
		string(interfaces.ModelProviderOpenCode): true,
		string(interfaces.ModelProviderGemini):   true,
		string(interfaces.ModelProviderKiro):     true,
	}
	return []cliProviderExplicitPrecedenceCase{
		{
			name: "explicit beats factory default",
			input: CLIProviderSelectionInput{
				ExplicitInvocation: string(interfaces.ModelProviderClaude),
				FactoryDefault:     string(interfaces.ModelProviderCodex),
			},
			presentCommands: allPresent,
			wantSource:      CLIProviderSelectionSourceExplicitInvocation,
			wantIdentity:    CLIProviderIdentityClaude,
		},
		{
			name: "explicit beats system default",
			input: CLIProviderSelectionInput{
				ExplicitInvocation: string(interfaces.ModelProviderGemini),
				SystemDefault:      string(interfaces.ModelProviderCodex),
			},
			presentCommands: allPresent,
			wantSource:      CLIProviderSelectionSourceExplicitInvocation,
			wantIdentity:    CLIProviderIdentityGemini,
		},
		{
			name: "explicit beats discovery even when lower-ranked and absent on PATH",
			input: CLIProviderSelectionInput{
				ExplicitInvocation: string(interfaces.ModelProviderKiro),
				FactoryDefault:     string(interfaces.ModelProviderCodex),
				SystemDefault:      string(interfaces.ModelProviderClaude),
			},
			presentCommands: map[string]bool{
				string(interfaces.ModelProviderCodex): true,
			},
			wantSource:   CLIProviderSelectionSourceExplicitInvocation,
			wantIdentity: CLIProviderIdentityKiro,
		},
		{
			name: "unsupported explicit DEFAULT falls through to factory without deprecated model default injection",
			input: CLIProviderSelectionInput{
				ExplicitInvocation: interfaces.WorkerModelProviderDefault,
				FactoryDefault:     string(interfaces.ModelProviderCursor),
			},
			presentCommands: allPresent,
			wantSource:      CLIProviderSelectionSourceFactoryDefault,
			wantIdentity:    CLIProviderIdentityCursor,
		},
		{
			name: "unsupported explicit deprecated openai alias falls through to system default",
			input: CLIProviderSelectionInput{
				ExplicitInvocation: "openai",
				SystemDefault:      string(interfaces.ModelProviderGemini),
			},
			presentCommands: allPresent,
			wantSource:      CLIProviderSelectionSourceSystemDefault,
			wantIdentity:    CLIProviderIdentityGemini,
		},
		{
			name: "empty explicit falls through to discovery",
			input: CLIProviderSelectionInput{
				ExplicitInvocation: "   ",
			},
			presentCommands: map[string]bool{
				string(interfaces.ModelProviderGemini): true,
			},
			wantSource:   CLIProviderSelectionSourceDiscovery,
			wantIdentity: CLIProviderIdentityGemini,
		},
		{
			name: "unknown explicit falls through to discovery without inventing a provider",
			input: CLIProviderSelectionInput{
				ExplicitInvocation: "legacy-model-default",
			},
			presentCommands: map[string]bool{
				string(interfaces.ModelProviderOpenCode): true,
			},
			wantSource:   CLIProviderSelectionSourceDiscovery,
			wantIdentity: CLIProviderIdentityOpenCode,
		},
	}
}

func assertCLIProviderExplicitPrecedence(t *testing.T, tc cliProviderExplicitPrecedenceCase) {
	t.Helper()

	discovery := fakeCLIProviderDiscoveryView(tc.presentCommands)
	result := SelectCLIProvider(tc.input, discovery)

	if tc.wantFailure {
		if result.OK() {
			t.Fatalf("result = %#v, want failure", result)
		}
		if result.Failure == nil || result.Failure.Code != tc.wantFailureCode {
			t.Fatalf("failure = %#v, want code %q", result.Failure, tc.wantFailureCode)
		}
		return
	}

	if !result.OK() {
		t.Fatalf("result = %#v, want success", result)
	}
	if result.Source != tc.wantSource {
		t.Fatalf("source = %q, want %q", result.Source, tc.wantSource)
	}
	if result.Selected == nil || result.Selected.Identity != tc.wantIdentity {
		t.Fatalf("selected = %#v, want identity %q", result.Selected, tc.wantIdentity)
	}
}

func TestSelectCLIProvider_ExplicitInvocationPrecedenceTables(t *testing.T) {
	for _, tc := range fakeCLIProviderExplicitPrecedenceCases() {
		t.Run(tc.name, func(t *testing.T) {
			assertCLIProviderExplicitPrecedence(t, tc)
		})
	}
}

type cliProviderConfiguredDefaultPrecedenceCase struct {
	name            string
	input           CLIProviderSelectionInput
	presentCommands map[string]bool
	wantSource      CLIProviderSelectionSource
	wantIdentity    CLIProviderIdentity
}

func fakeCLIProviderConfiguredDefaultPrecedenceCases() []cliProviderConfiguredDefaultPrecedenceCase {
	allPresent := map[string]bool{
		string(interfaces.ModelProviderCodex):    true,
		string(interfaces.ModelProviderClaude):   true,
		string(interfaces.ModelProviderCursor):   true,
		string(interfaces.ModelProviderOpenCode): true,
		string(interfaces.ModelProviderGemini):   true,
		string(interfaces.ModelProviderKiro):     true,
	}
	return []cliProviderConfiguredDefaultPrecedenceCase{
		{
			name: "factory default beats system default",
			input: CLIProviderSelectionInput{
				FactoryDefault: string(interfaces.ModelProviderCursor),
				SystemDefault:  string(interfaces.ModelProviderCodex),
			},
			presentCommands: allPresent,
			wantSource:      CLIProviderSelectionSourceFactoryDefault,
			wantIdentity:    CLIProviderIdentityCursor,
		},
		{
			name: "factory default beats discovery even when absent on PATH",
			input: CLIProviderSelectionInput{
				FactoryDefault: string(interfaces.ModelProviderKiro),
				SystemDefault:  string(interfaces.ModelProviderClaude),
			},
			presentCommands: map[string]bool{
				string(interfaces.ModelProviderCodex): true,
			},
			wantSource:   CLIProviderSelectionSourceFactoryDefault,
			wantIdentity: CLIProviderIdentityKiro,
		},
		{
			name: "system default beats discovery",
			input: CLIProviderSelectionInput{
				SystemDefault: string(interfaces.ModelProviderGemini),
			},
			presentCommands: map[string]bool{
				string(interfaces.ModelProviderCodex): true,
			},
			wantSource:   CLIProviderSelectionSourceSystemDefault,
			wantIdentity: CLIProviderIdentityGemini,
		},
		{
			name: "system default beats discovery without consulting lower-ranked available providers",
			input: CLIProviderSelectionInput{
				SystemDefault: string(interfaces.ModelProviderOpenCode),
			},
			presentCommands: map[string]bool{
				string(interfaces.ModelProviderCodex):  true,
				string(interfaces.ModelProviderGemini): true,
			},
			wantSource:   CLIProviderSelectionSourceSystemDefault,
			wantIdentity: CLIProviderIdentityOpenCode,
		},
		{
			name: "empty factory falls through to system default over discovery",
			input: CLIProviderSelectionInput{
				FactoryDefault: "   ",
				SystemDefault:  string(interfaces.ModelProviderClaude),
			},
			presentCommands: map[string]bool{
				string(interfaces.ModelProviderCodex): true,
			},
			wantSource:   CLIProviderSelectionSourceSystemDefault,
			wantIdentity: CLIProviderIdentityClaude,
		},
	}
}

func assertCLIProviderConfiguredDefaultPrecedence(t *testing.T, tc cliProviderConfiguredDefaultPrecedenceCase) {
	t.Helper()

	discovery := fakeCLIProviderDiscoveryView(tc.presentCommands)
	result := SelectCLIProvider(tc.input, discovery)

	if !result.OK() {
		t.Fatalf("result = %#v, want success", result)
	}
	if result.Source != tc.wantSource {
		t.Fatalf("source = %q, want %q", result.Source, tc.wantSource)
	}
	if result.Selected == nil || result.Selected.Identity != tc.wantIdentity {
		t.Fatalf("selected = %#v, want identity %q", result.Selected, tc.wantIdentity)
	}
}

func TestSelectCLIProvider_FactoryAndSystemDefaultPrecedenceTables(t *testing.T) {
	for _, tc := range fakeCLIProviderConfiguredDefaultPrecedenceCases() {
		t.Run(tc.name, func(t *testing.T) {
			assertCLIProviderConfiguredDefaultPrecedence(t, tc)
		})
	}
}

type cliProviderDiscoveryPrecedenceCase struct {
	name              string
	input             CLIProviderSelectionInput
	presentCommands   map[string]bool
	registrations     []CLIProviderRegistration
	wantIdentity      CLIProviderIdentity
}

func fakeCLIProviderDiscoveryPrecedenceCases() []cliProviderDiscoveryPrecedenceCase {
	allPresent := map[string]bool{
		string(interfaces.ModelProviderCodex):    true,
		string(interfaces.ModelProviderClaude):   true,
		string(interfaces.ModelProviderCursor):   true,
		string(interfaces.ModelProviderOpenCode): true,
		string(interfaces.ModelProviderGemini):   true,
		string(interfaces.ModelProviderKiro):     true,
	}
	return []cliProviderDiscoveryPrecedenceCase{
		{
			name:            "all providers available selects highest preference codex",
			presentCommands: allPresent,
			wantIdentity:    CLIProviderIdentityCodex,
		},
		{
			name: "codex absent falls through to claude",
			presentCommands: map[string]bool{
				string(interfaces.ModelProviderClaude):   true,
				string(interfaces.ModelProviderCursor):   true,
				string(interfaces.ModelProviderOpenCode): true,
				string(interfaces.ModelProviderGemini):   true,
				string(interfaces.ModelProviderKiro):     true,
			},
			wantIdentity: CLIProviderIdentityClaude,
		},
		{
			name: "subset available selects highest preference cursor over gemini and kiro",
			presentCommands: map[string]bool{
				string(interfaces.ModelProviderCursor):   true,
				string(interfaces.ModelProviderOpenCode): true,
				string(interfaces.ModelProviderGemini):   true,
				string(interfaces.ModelProviderKiro):     true,
			},
			wantIdentity: CLIProviderIdentityCursor,
		},
		{
			name: "only lower ranked providers present selects gemini before kiro",
			presentCommands: map[string]bool{
				string(interfaces.ModelProviderGemini): true,
				string(interfaces.ModelProviderKiro):   true,
			},
			wantIdentity: CLIProviderIdentityGemini,
		},
		{
			name: "single available provider selects that provider",
			presentCommands: map[string]bool{
				string(interfaces.ModelProviderOpenCode): true,
			},
			wantIdentity: CLIProviderIdentityOpenCode,
		},
	}
}

func assertCLIProviderDiscoveryPrecedence(t *testing.T, tc cliProviderDiscoveryPrecedenceCase) {
	t.Helper()

	discovery := fakeCLIProviderDiscoveryView(tc.presentCommands)
	if tc.registrations != nil {
		discovery.Registrations = tc.registrations
	}
	result := SelectCLIProvider(tc.input, discovery)

	if !result.OK() {
		t.Fatalf("result = %#v, want success", result)
	}
	if result.Source != CLIProviderSelectionSourceDiscovery {
		t.Fatalf("source = %q, want %q", result.Source, CLIProviderSelectionSourceDiscovery)
	}
	if result.Selected == nil || result.Selected.Identity != tc.wantIdentity {
		t.Fatalf("selected = %#v, want identity %q", result.Selected, tc.wantIdentity)
	}
}

func TestSelectCLIProvider_DiscoveryPreferenceRankTables(t *testing.T) {
	for _, tc := range fakeCLIProviderDiscoveryPrecedenceCases() {
		t.Run(tc.name, func(t *testing.T) {
			assertCLIProviderDiscoveryPrecedence(t, tc)
		})
	}
}

type cliProviderNoAgentHarnessCase struct {
	name            string
	input           CLIProviderSelectionInput
	presentCommands map[string]bool
	wantGuidance    []string
}

func fakeCLIProviderNoAgentHarnessCases() []cliProviderNoAgentHarnessCase {
	return []cliProviderNoAgentHarnessCase{
		{
			name:            "all defaults unset and no providers available",
			presentCommands: map[string]bool{},
			wantGuidance: []string{
				"install a supported agent CLI provider on PATH",
				"set an explicit invocation provider",
				"configure a factory default provider",
				"configure a system default provider",
			},
		},
		{
			name: "unsupported explicit falls through to empty discovery without deprecated model default injection",
			input: CLIProviderSelectionInput{
				ExplicitInvocation: interfaces.WorkerModelProviderDefault,
			},
			presentCommands: map[string]bool{},
			wantGuidance: []string{
				"install a supported agent CLI provider on PATH",
			},
		},
		{
			name: "unsupported factory and system defaults fall through to empty discovery",
			input: CLIProviderSelectionInput{
				FactoryDefault: "openai",
				SystemDefault:  "legacy-model-default",
			},
			presentCommands: map[string]bool{},
			wantGuidance: []string{
				"install a supported agent CLI provider on PATH",
			},
		},
		{
			name: "whitespace defaults with providers absent on PATH",
			input: CLIProviderSelectionInput{
				ExplicitInvocation: "   ",
				FactoryDefault:     "\t",
				SystemDefault:      " ",
			},
			presentCommands: map[string]bool{},
			wantGuidance: []string{
				"install a supported agent CLI provider on PATH",
			},
		},
		{
			name: "unsupported values at all layers with no providers available",
			input: CLIProviderSelectionInput{
				ExplicitInvocation: "openai",
				FactoryDefault:     interfaces.WorkerModelProviderDefault,
				SystemDefault:      "legacy-model-default",
			},
			presentCommands: map[string]bool{},
			wantGuidance: []string{
				"install a supported agent CLI provider on PATH",
			},
		},
	}
}

func assertCLIProviderNoAgentHarness(t *testing.T, tc cliProviderNoAgentHarnessCase) {
	t.Helper()

	discovery := fakeCLIProviderDiscoveryView(tc.presentCommands)
	result := SelectCLIProvider(tc.input, discovery)

	if result.OK() {
		t.Fatalf("result = %#v, want failure", result)
	}
	if result.Selected != nil {
		t.Fatalf("selected = %#v, want nil without deprecated model-default injection", result.Selected)
	}
	if result.Failure == nil {
		t.Fatal("failure = nil, want structured NO_AGENT_HARNESS failure")
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
	for _, want := range tc.wantGuidance {
		if !strings.Contains(result.Failure.Guidance, want) {
			t.Fatalf("guidance = %q, want substring %q", result.Failure.Guidance, want)
		}
	}
	formatted := FormatCLIProviderSelectionFailure(*result.Failure)
	if !strings.Contains(formatted, string(CLIProviderSelectionFailureNoAgentHarness)) {
		t.Fatalf("formatted = %q, want machine-readable code", formatted)
	}
}

func TestSelectCLIProvider_NoAgentHarnessTables(t *testing.T) {
	for _, tc := range fakeCLIProviderNoAgentHarnessCases() {
		t.Run(tc.name, func(t *testing.T) {
			assertCLIProviderNoAgentHarness(t, tc)
		})
	}
}

func TestSelectCLIProvider_DiscoveryIgnoresRegistrationSliceOrder(t *testing.T) {
	reversed := RegisteredCLIProviders()
	for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
		reversed[i], reversed[j] = reversed[j], reversed[i]
	}

	discovery := fakeCLIProviderDiscoveryView(map[string]bool{
		string(interfaces.ModelProviderCodex):  true,
		string(interfaces.ModelProviderGemini): true,
		string(interfaces.ModelProviderKiro):   true,
	})
	discovery.Registrations = reversed

	result := SelectCLIProvider(CLIProviderSelectionInput{}, discovery)

	if !result.OK() {
		t.Fatalf("result = %#v, want success", result)
	}
	if result.Source != CLIProviderSelectionSourceDiscovery {
		t.Fatalf("source = %q, want %q", result.Source, CLIProviderSelectionSourceDiscovery)
	}
	if result.Selected == nil || result.Selected.Identity != CLIProviderIdentityCodex {
		t.Fatalf("selected = %#v, want codex despite reversed registration slice", result.Selected)
	}
}

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
