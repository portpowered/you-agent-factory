package cliprovider

import (
	"strings"
	"testing"

	workertaxonomy "github.com/portpowered/infinite-you/pkg/workers/taxonomy"

	modelprovider "github.com/portpowered/infinite-you/pkg/models/provider"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
)

type cliProviderExplicitPrecedenceCase struct {
	name            string
	input           CLIProviderSelectionInput
	presentCommands map[string]bool
	wantSource      CLIProviderSelectionSource
	wantIdentity    CLIProviderIdentity
	wantFailure     bool
	wantFailureCode CLIProviderSelectionFailureCode
}

func fakeCLIProviderExplicitPrecedenceCases() []cliProviderExplicitPrecedenceCase {
	allPresent := map[string]bool{
		string(modelprovider.Codex):    true,
		string(modelprovider.Claude):   true,
		string(modelprovider.Cursor):   true,
		string(modelprovider.OpenCode): true,
		string(modelprovider.Gemini):   true,
		string(modelprovider.Kiro):     true,
	}
	return []cliProviderExplicitPrecedenceCase{
		{
			name: "explicit beats factory default",
			input: CLIProviderSelectionInput{
				ExplicitInvocation: string(modelprovider.Claude),
				FactoryDefault:     string(modelprovider.Codex),
			},
			presentCommands: allPresent,
			wantSource:      CLIProviderSelectionSourceExplicitInvocation,
			wantIdentity:    CLIProviderIdentityClaude,
		},
		{
			name: "explicit beats system default",
			input: CLIProviderSelectionInput{
				ExplicitInvocation: string(modelprovider.Gemini),
				SystemDefault:      string(modelprovider.Codex),
			},
			presentCommands: allPresent,
			wantSource:      CLIProviderSelectionSourceExplicitInvocation,
			wantIdentity:    CLIProviderIdentityGemini,
		},
		{
			name: "explicit beats discovery even when lower-ranked and absent on PATH",
			input: CLIProviderSelectionInput{
				ExplicitInvocation: string(modelprovider.Kiro),
				FactoryDefault:     string(modelprovider.Codex),
				SystemDefault:      string(modelprovider.Claude),
			},
			presentCommands: map[string]bool{
				string(modelprovider.Codex): true,
			},
			wantSource:   CLIProviderSelectionSourceExplicitInvocation,
			wantIdentity: CLIProviderIdentityKiro,
		},
		{
			name: "unsupported explicit DEFAULT falls through to factory without deprecated model default injection",
			input: CLIProviderSelectionInput{
				ExplicitInvocation: workertaxonomy.ModelProviderDefault,
				FactoryDefault:     string(modelprovider.Cursor),
			},
			presentCommands: allPresent,
			wantSource:      CLIProviderSelectionSourceFactoryDefault,
			wantIdentity:    CLIProviderIdentityCursor,
		},
		{
			name: "unsupported explicit deprecated openai alias falls through to system default",
			input: CLIProviderSelectionInput{
				ExplicitInvocation: "openai",
				SystemDefault:      string(modelprovider.Gemini),
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
				string(modelprovider.Gemini): true,
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
				string(modelprovider.OpenCode): true,
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
		string(modelprovider.Codex):    true,
		string(modelprovider.Claude):   true,
		string(modelprovider.Cursor):   true,
		string(modelprovider.OpenCode): true,
		string(modelprovider.Gemini):   true,
		string(modelprovider.Kiro):     true,
	}
	return []cliProviderConfiguredDefaultPrecedenceCase{
		{
			name: "factory default beats system default",
			input: CLIProviderSelectionInput{
				FactoryDefault: string(modelprovider.Cursor),
				SystemDefault:  string(modelprovider.Codex),
			},
			presentCommands: allPresent,
			wantSource:      CLIProviderSelectionSourceFactoryDefault,
			wantIdentity:    CLIProviderIdentityCursor,
		},
		{
			name: "factory default beats discovery even when absent on PATH",
			input: CLIProviderSelectionInput{
				FactoryDefault: string(modelprovider.Kiro),
				SystemDefault:  string(modelprovider.Claude),
			},
			presentCommands: map[string]bool{
				string(modelprovider.Codex): true,
			},
			wantSource:   CLIProviderSelectionSourceFactoryDefault,
			wantIdentity: CLIProviderIdentityKiro,
		},
		{
			name: "system default beats discovery",
			input: CLIProviderSelectionInput{
				SystemDefault: string(modelprovider.Gemini),
			},
			presentCommands: map[string]bool{
				string(modelprovider.Codex): true,
			},
			wantSource:   CLIProviderSelectionSourceSystemDefault,
			wantIdentity: CLIProviderIdentityGemini,
		},
		{
			name: "system default beats discovery without consulting lower-ranked available providers",
			input: CLIProviderSelectionInput{
				SystemDefault: string(modelprovider.OpenCode),
			},
			presentCommands: map[string]bool{
				string(modelprovider.Codex):  true,
				string(modelprovider.Gemini): true,
			},
			wantSource:   CLIProviderSelectionSourceSystemDefault,
			wantIdentity: CLIProviderIdentityOpenCode,
		},
		{
			name: "empty factory falls through to system default over discovery",
			input: CLIProviderSelectionInput{
				FactoryDefault: "   ",
				SystemDefault:  string(modelprovider.Claude),
			},
			presentCommands: map[string]bool{
				string(modelprovider.Codex): true,
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
	name            string
	input           CLIProviderSelectionInput
	presentCommands map[string]bool
	registrations   []CLIProviderRegistration
	wantIdentity    CLIProviderIdentity
}

func fakeCLIProviderDiscoveryPrecedenceCases() []cliProviderDiscoveryPrecedenceCase {
	allPresent := map[string]bool{
		string(modelprovider.Codex):    true,
		string(modelprovider.Claude):   true,
		string(modelprovider.Cursor):   true,
		string(modelprovider.OpenCode): true,
		string(modelprovider.Gemini):   true,
		string(modelprovider.Kiro):     true,
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
				string(modelprovider.Claude):   true,
				string(modelprovider.Cursor):   true,
				string(modelprovider.OpenCode): true,
				string(modelprovider.Gemini):   true,
				string(modelprovider.Kiro):     true,
			},
			wantIdentity: CLIProviderIdentityClaude,
		},
		{
			name: "subset available selects highest preference cursor over gemini and kiro",
			presentCommands: map[string]bool{
				string(modelprovider.Cursor):   true,
				string(modelprovider.OpenCode): true,
				string(modelprovider.Gemini):   true,
				string(modelprovider.Kiro):     true,
			},
			wantIdentity: CLIProviderIdentityCursor,
		},
		{
			name: "only lower ranked providers present selects gemini before kiro",
			presentCommands: map[string]bool{
				string(modelprovider.Gemini): true,
				string(modelprovider.Kiro):   true,
			},
			wantIdentity: CLIProviderIdentityGemini,
		},
		{
			name: "single available provider selects that provider",
			presentCommands: map[string]bool{
				string(modelprovider.OpenCode): true,
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
				ExplicitInvocation: workertaxonomy.ModelProviderDefault,
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
				FactoryDefault:     workertaxonomy.ModelProviderDefault,
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

type cliProviderFullPrecedenceMatrixCase struct {
	name                    string
	input                   CLIProviderSelectionInput
	presentCommands         map[string]bool
	wantSource              CLIProviderSelectionSource
	wantIdentity            CLIProviderIdentity
	wantFailure             bool
	wantFailureCode         CLIProviderSelectionFailureCode
	assertRegistrationOrder bool
	forbidIdentities        []CLIProviderIdentity
}

func fakeCLIProviderFullPrecedenceMatrixCases() []cliProviderFullPrecedenceMatrixCase {
	allPresent := map[string]bool{
		string(modelprovider.Codex):    true,
		string(modelprovider.Claude):   true,
		string(modelprovider.Cursor):   true,
		string(modelprovider.OpenCode): true,
		string(modelprovider.Gemini):   true,
		string(modelprovider.Kiro):     true,
	}
	return []cliProviderFullPrecedenceMatrixCase{
		{
			name: "edge explicit invocation wins over factory system and discovery",
			input: CLIProviderSelectionInput{
				ExplicitInvocation: string(modelprovider.Claude),
				FactoryDefault:     string(modelprovider.Codex),
				SystemDefault:      string(modelprovider.Gemini),
			},
			presentCommands: allPresent,
			wantSource:      CLIProviderSelectionSourceExplicitInvocation,
			wantIdentity:    CLIProviderIdentityClaude,
		},
		{
			name: "edge factory default wins when explicit unset",
			input: CLIProviderSelectionInput{
				FactoryDefault: string(modelprovider.Cursor),
				SystemDefault:  string(modelprovider.Codex),
			},
			presentCommands: allPresent,
			wantSource:      CLIProviderSelectionSourceFactoryDefault,
			wantIdentity:    CLIProviderIdentityCursor,
		},
		{
			name: "edge system default wins when explicit and factory unset",
			input: CLIProviderSelectionInput{
				SystemDefault: string(modelprovider.Gemini),
			},
			presentCommands: map[string]bool{
				string(modelprovider.Codex): true,
			},
			wantSource:   CLIProviderSelectionSourceSystemDefault,
			wantIdentity: CLIProviderIdentityGemini,
		},
		{
			name: "edge discovery wins when all configured defaults unset",
			presentCommands: map[string]bool{
				string(modelprovider.Cursor):   true,
				string(modelprovider.OpenCode): true,
				string(modelprovider.Gemini):   true,
			},
			wantSource:              CLIProviderSelectionSourceDiscovery,
			wantIdentity:            CLIProviderIdentityCursor,
			assertRegistrationOrder: true,
		},
		{
			name:            "edge empty selection returns NO_AGENT_HARNESS",
			presentCommands: map[string]bool{},
			wantFailure:     true,
			wantFailureCode: CLIProviderSelectionFailureNoAgentHarness,
		},
		{
			name: "forbidden deprecated DEFAULT openai and legacy values do not inject codex fallback",
			input: CLIProviderSelectionInput{
				ExplicitInvocation: workertaxonomy.ModelProviderDefault,
				FactoryDefault:     "openai",
				SystemDefault:      "legacy-model-default",
			},
			presentCommands:  map[string]bool{},
			wantFailure:      true,
			wantFailureCode:  CLIProviderSelectionFailureNoAgentHarness,
			forbidIdentities: []CLIProviderIdentity{CLIProviderIdentityCodex},
		},
	}
}

func reversedCLIProviderRegistrations() []CLIProviderRegistration {
	reversed := RegisteredCLIProviders()
	for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
		reversed[i], reversed[j] = reversed[j], reversed[i]
	}
	return reversed
}

func selectCLIProviderForMatrixCase(
	tc cliProviderFullPrecedenceMatrixCase,
	registrations []CLIProviderRegistration,
) CLIProviderSelectionResult {
	discovery := fakeCLIProviderDiscoveryView(tc.presentCommands)
	if registrations != nil {
		discovery.Registrations = registrations
	}
	return SelectCLIProvider(tc.input, discovery)
}

func assertCLIProviderMatrixFailure(t *testing.T, tc cliProviderFullPrecedenceMatrixCase, result CLIProviderSelectionResult) {
	t.Helper()

	if result.OK() {
		t.Fatalf("result = %#v, want failure", result)
	}
	if result.Selected != nil {
		t.Fatalf("selected = %#v, want nil without deprecated model-default injection", result.Selected)
	}
	if result.Failure == nil || result.Failure.Code != tc.wantFailureCode {
		t.Fatalf("failure = %#v, want code %q", result.Failure, tc.wantFailureCode)
	}
	for _, forbidden := range tc.forbidIdentities {
		if result.Selected != nil && result.Selected.Identity == forbidden {
			t.Fatalf("selected identity = %q, want forbidden deprecated fallback %q", result.Selected.Identity, forbidden)
		}
	}
}

func assertCLIProviderMatrixSuccess(t *testing.T, tc cliProviderFullPrecedenceMatrixCase, result CLIProviderSelectionResult) {
	t.Helper()

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

func assertCLIProviderMatrixRegistrationOrder(t *testing.T, tc cliProviderFullPrecedenceMatrixCase) {
	t.Helper()

	reversedResult := selectCLIProviderForMatrixCase(tc, reversedCLIProviderRegistrations())
	if !reversedResult.OK() {
		t.Fatalf("reversed registrations result = %#v, want success", reversedResult)
	}
	if reversedResult.Source != tc.wantSource {
		t.Fatalf("reversed registrations source = %q, want %q", reversedResult.Source, tc.wantSource)
	}
	if reversedResult.Selected == nil || reversedResult.Selected.Identity != tc.wantIdentity {
		t.Fatalf(
			"reversed registrations selected = %#v, want identity %q independent of slice order",
			reversedResult.Selected,
			tc.wantIdentity,
		)
	}
}

func assertCLIProviderFullPrecedenceMatrix(t *testing.T, tc cliProviderFullPrecedenceMatrixCase) {
	t.Helper()

	result := selectCLIProviderForMatrixCase(tc, nil)
	if tc.wantFailure {
		assertCLIProviderMatrixFailure(t, tc, result)
		return
	}

	assertCLIProviderMatrixSuccess(t, tc, result)
	if tc.assertRegistrationOrder {
		assertCLIProviderMatrixRegistrationOrder(t, tc)
	}
}

func TestSelectCLIProvider_FullPrecedenceMatrixTables(t *testing.T) {
	for _, tc := range fakeCLIProviderFullPrecedenceMatrixCases() {
		t.Run(tc.name, func(t *testing.T) {
			assertCLIProviderFullPrecedenceMatrix(t, tc)
		})
	}
}

func TestSelectCLIProvider_DiscoveryIgnoresRegistrationSliceOrder(t *testing.T) {
	reversed := RegisteredCLIProviders()
	for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
		reversed[i], reversed[j] = reversed[j], reversed[i]
	}

	discovery := fakeCLIProviderDiscoveryView(map[string]bool{
		string(modelprovider.Codex):  true,
		string(modelprovider.Gemini): true,
		string(modelprovider.Kiro):   true,
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
		string(modelprovider.Gemini): true,
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
		FactoryDefault: string(modelprovider.Codex),
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
		string(modelprovider.Cursor): true,
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

func TestDefaultCLIProviderDiscoveryView_UsesInjectedProbe(t *testing.T) {
	discovery := DefaultCLIProviderDiscoveryView(func(registration CLIProviderRegistration) CLIProviderAvailability {
		available := registration.Identity == CLIProviderIdentityGemini
		if available {
			return CLIProviderAvailability{
				Registration: registration,
				Available:    true,
			}
		}
		return CLIProviderAvailability{
			Registration:      registration,
			Available:         false,
			UnavailableReason: string(workerexecution.WorkFailureTypeMissingExecutable),
		}
	})

	result := SelectCLIProvider(CLIProviderSelectionInput{}, discovery)

	if !result.OK() {
		t.Fatalf("result = %#v, want discovery success", result)
	}
	if result.Source != CLIProviderSelectionSourceDiscovery {
		t.Fatalf("source = %q, want %q", result.Source, CLIProviderSelectionSourceDiscovery)
	}
	if result.Selected == nil || result.Selected.Identity != CLIProviderIdentityGemini {
		t.Fatalf("selected = %#v, want gemini via default discovery view", result.Selected)
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
				UnavailableReason: string(workerexecution.WorkFailureTypeMissingExecutable),
			}
		},
	}
}
