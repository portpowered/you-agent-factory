package cliprovider

import (
	"errors"
	"os/exec"
	"strings"
	"testing"

	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

func TestRegisteredCLIProviders_IncludeEverySupportedProvider(t *testing.T) {
	wantCommands := map[CLIProviderIdentity]string{
		CLIProviderIdentityClaude:   string(modelprovider.ProviderClaude),
		CLIProviderIdentityCodex:    string(modelprovider.ProviderCodex),
		CLIProviderIdentityGemini:   string(modelprovider.ProviderGemini),
		CLIProviderIdentityKiro:     string(modelprovider.ProviderKiro),
		CLIProviderIdentityCursor:   string(modelprovider.ProviderCursor),
		CLIProviderIdentityOpenCode: string(modelprovider.ProviderOpenCode),
		CLIProviderIdentityPi:       string(modelprovider.ProviderPi),
	}

	registrations := RegisteredCLIProviders()
	if len(registrations) != len(wantCommands) {
		t.Fatalf("RegisteredCLIProviders() len = %d, want %d", len(registrations), len(wantCommands))
	}

	seen := make(map[CLIProviderIdentity]struct{}, len(wantCommands))
	for _, registration := range registrations {
		wantCommand, ok := wantCommands[registration.Identity]
		if !ok {
			t.Fatalf("unexpected provider identity %q", registration.Identity)
		}
		if registration.Command != wantCommand {
			t.Fatalf("identity %q command = %q, want %q", registration.Identity, registration.Command, wantCommand)
		}
		seen[registration.Identity] = struct{}{}
	}

	for identity := range wantCommands {
		if _, ok := seen[identity]; !ok {
			t.Fatalf("missing provider identity %q", identity)
		}
	}
}

func TestRegisteredCLIProviders_PreferenceRanksAreUnique(t *testing.T) {
	seen := make(map[int]CLIProviderIdentity)
	for _, registration := range RegisteredCLIProviders() {
		if previous, ok := seen[registration.PreferenceRank]; ok {
			t.Fatalf(
				"duplicate preference rank %d: %q and %q",
				registration.PreferenceRank,
				previous,
				registration.Identity,
			)
		}
		seen[registration.PreferenceRank] = registration.Identity
	}
}

func TestRegisteredCLIProviders_PreferenceRankOrder(t *testing.T) {
	wantOrder := []CLIProviderIdentity{
		CLIProviderIdentityCodex,
		CLIProviderIdentityClaude,
		CLIProviderIdentityCursor,
		CLIProviderIdentityOpenCode,
		CLIProviderIdentityGemini,
		CLIProviderIdentityKiro,
		CLIProviderIdentityPi,
	}

	registrations := RegisteredCLIProviders()
	if len(registrations) != len(wantOrder) {
		t.Fatalf("RegisteredCLIProviders() len = %d, want %d", len(registrations), len(wantOrder))
	}

	for i, wantIdentity := range wantOrder {
		if registrations[i].Identity != wantIdentity {
			t.Fatalf("rank[%d] identity = %q, want %q", i, registrations[i].Identity, wantIdentity)
		}
	}
}

func TestCLIProviderRegistrationLookups_AreStableAcrossRepeatedCalls(t *testing.T) {
	first := RegisteredCLIProviders()
	second := RegisteredCLIProviders()
	if len(first) != len(second) {
		t.Fatalf("registry length drift: first=%d second=%d", len(first), len(second))
	}
	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("registry entry[%d] drift: first=%#v second=%#v", i, first[i], second[i])
		}
	}

	for _, registration := range first {
		for attempt := 0; attempt < 3; attempt++ {
			byIdentity, ok := CLIProviderRegistrationByIdentity(registration.Identity)
			if !ok {
				t.Fatalf("identity lookup miss for %q on attempt %d", registration.Identity, attempt)
			}
			if byIdentity != registration {
				t.Fatalf("identity lookup drift for %q: got %#v want %#v", registration.Identity, byIdentity, registration)
			}

			byCommand, ok := CLIProviderRegistrationByCommand(registration.Command)
			if !ok {
				t.Fatalf("command lookup miss for %q on attempt %d", registration.Command, attempt)
			}
			if byCommand != registration {
				t.Fatalf("command lookup drift for %q: got %#v want %#v", registration.Command, byCommand, registration)
			}
		}
	}
}

func TestCLIProviderIdentity_CompatibleWithProviderBackendScopeNaming(t *testing.T) {
	for _, registration := range RegisteredCLIProviders() {
		scope := operatorsettings.DeriveProviderBackendScopeID(
			CLIProviderScopeSegment(registration.Identity),
			"account",
			"workspace",
		)
		if !strings.HasPrefix(scope, "provider-") {
			t.Fatalf("identity %q scope = %q, want provider-* prefix", registration.Identity, scope)
		}
		if !strings.Contains(scope, string(registration.Identity)) {
			t.Fatalf("identity %q scope = %q, want identity segment preserved", registration.Identity, scope)
		}
	}
}

type cliProviderProbeCase struct {
	name            string
	command         string
	presentCommands map[string]bool
	wantAvailable   bool
	wantReason      string
}

type executableLocatorFunc func(string) (string, error)

func (locate executableLocatorFunc) LookPath(command string) (string, error) {
	return locate(command)
}

func fakeCLIProviderProbeCases() []cliProviderProbeCase {
	commands := []string{
		string(modelprovider.ProviderClaude),
		string(modelprovider.ProviderCodex),
		string(modelprovider.ProviderCursor),
		string(modelprovider.ProviderOpenCode),
		string(modelprovider.ProviderGemini),
		string(modelprovider.ProviderKiro),
	}
	cases := make([]cliProviderProbeCase, 0, len(commands)*2)
	for _, command := range commands {
		cases = append(cases,
			cliProviderProbeCase{
				name:            command + " present",
				command:         command,
				presentCommands: map[string]bool{command: true},
				wantAvailable:   true,
			},
			cliProviderProbeCase{
				name:            command + " absent",
				command:         command,
				presentCommands: map[string]bool{},
				wantAvailable:   false,
				wantReason:      string(workerexecution.WorkFailureTypeMissingExecutable),
			},
		)
	}
	return cases
}

func assertCLIProviderProbeAvailability(t *testing.T, tc cliProviderProbeCase) {
	t.Helper()

	var probedCommands []string
	locator := executableLocatorFunc(func(file string) (string, error) {
		probedCommands = append(probedCommands, file)
		if tc.presentCommands[file] {
			return "/fake/bin/" + file, nil
		}
		return "", exec.ErrNotFound
	})

	registration, ok := CLIProviderRegistrationByCommand(tc.command)
	if !ok {
		t.Fatalf("CLIProviderRegistrationByCommand(%q) miss", tc.command)
	}

	got := ProbeCLIProviderAvailability(locator, registration)
	if got.Registration != registration {
		t.Fatalf("registration = %#v, want %#v (provider/command diagnostic)", got.Registration, registration)
	}
	if got.Available != tc.wantAvailable {
		t.Fatalf("available = %v, want %v", got.Available, tc.wantAvailable)
	}
	if got.UnavailableReason != tc.wantReason {
		t.Fatalf("unavailable reason = %q, want %q", got.UnavailableReason, tc.wantReason)
	}
	if len(probedCommands) != 1 || probedCommands[0] != tc.command {
		t.Fatalf("lookPath commands = %#v, want [%q]", probedCommands, tc.command)
	}
}

func TestProbeCLIProviderAvailability_FakePATHPresentAndAbsentPerCommand(t *testing.T) {
	for _, tc := range fakeCLIProviderProbeCases() {
		t.Run(tc.name, func(t *testing.T) {
			assertCLIProviderProbeAvailability(t, tc)
		})
	}
}

func TestProbeCLIProviderAvailability_FailsClosedWithoutExecutableLocator(t *testing.T) {
	registration, ok := CLIProviderRegistrationByIdentity(CLIProviderIdentityCodex)
	if !ok {
		t.Fatal("codex registration is missing")
	}
	got := ProbeCLIProviderAvailability(nil, registration)
	if got.Available || got.UnavailableReason != string(workerexecution.WorkFailureTypeMissingExecutable) {
		t.Fatalf("availability = %#v, want missing executable classification", got)
	}
}

type cliProviderDiscoveryCase struct {
	name              string
	presentCommands   map[string]bool
	wantSelected      *CLIProviderIdentity
	wantUnavailable   []CLIProviderIdentity
	wantProbeCommands []string
}

func fakeCLIProviderDiscoveryCases() []cliProviderDiscoveryCase {
	return []cliProviderDiscoveryCase{
		{
			name: "all providers present prefers highest rank codex",
			presentCommands: map[string]bool{
				string(modelprovider.ProviderCodex):    true,
				string(modelprovider.ProviderClaude):   true,
				string(modelprovider.ProviderCursor):   true,
				string(modelprovider.ProviderOpenCode): true,
				string(modelprovider.ProviderGemini):   true,
				string(modelprovider.ProviderKiro):     true,
				string(modelprovider.ProviderPi):       true,
			},
			wantSelected: ptrCLIProviderIdentity(CLIProviderIdentityCodex),
			wantProbeCommands: []string{
				string(modelprovider.ProviderCodex),
				string(modelprovider.ProviderClaude),
				string(modelprovider.ProviderCursor),
				string(modelprovider.ProviderOpenCode),
				string(modelprovider.ProviderGemini),
				string(modelprovider.ProviderKiro),
				string(modelprovider.ProviderPi),
			},
		},
		{
			name: "codex absent falls through to claude",
			presentCommands: map[string]bool{
				string(modelprovider.ProviderClaude):   true,
				string(modelprovider.ProviderCursor):   true,
				string(modelprovider.ProviderOpenCode): true,
				string(modelprovider.ProviderGemini):   true,
				string(modelprovider.ProviderKiro):     true,
			},
			wantSelected: ptrCLIProviderIdentity(CLIProviderIdentityClaude),
			wantUnavailable: []CLIProviderIdentity{
				CLIProviderIdentityCodex,
			},
		},
		{
			name: "codex and claude absent falls through to cursor",
			presentCommands: map[string]bool{
				string(modelprovider.ProviderCursor):   true,
				string(modelprovider.ProviderOpenCode): true,
				string(modelprovider.ProviderGemini):   true,
			},
			wantSelected: ptrCLIProviderIdentity(CLIProviderIdentityCursor),
			wantUnavailable: []CLIProviderIdentity{
				CLIProviderIdentityCodex,
				CLIProviderIdentityClaude,
			},
		},
		{
			name: "only lower ranked providers present selects gemini before kiro",
			presentCommands: map[string]bool{
				string(modelprovider.ProviderGemini): true,
				string(modelprovider.ProviderKiro):   true,
			},
			wantSelected: ptrCLIProviderIdentity(CLIProviderIdentityGemini),
			wantUnavailable: []CLIProviderIdentity{
				CLIProviderIdentityCodex,
				CLIProviderIdentityClaude,
				CLIProviderIdentityCursor,
				CLIProviderIdentityOpenCode,
			},
		},
		{
			name:            "no providers on path selects none and classifies all missing",
			presentCommands: map[string]bool{},
			wantSelected:    nil,
			wantUnavailable: []CLIProviderIdentity{
				CLIProviderIdentityCodex,
				CLIProviderIdentityClaude,
				CLIProviderIdentityCursor,
				CLIProviderIdentityOpenCode,
				CLIProviderIdentityGemini,
				CLIProviderIdentityKiro,
				CLIProviderIdentityPi,
			},
		},
	}
}

func registeredCLIProviderProbeCommands() []string {
	return []string{
		string(modelprovider.ProviderCodex),
		string(modelprovider.ProviderClaude),
		string(modelprovider.ProviderCursor),
		string(modelprovider.ProviderOpenCode),
		string(modelprovider.ProviderGemini),
		string(modelprovider.ProviderKiro),
		string(modelprovider.ProviderPi),
	}
}

func assertCLIProviderDiscoverySelected(t *testing.T, got CLIProviderDiscovery, wantSelected *CLIProviderIdentity) {
	t.Helper()

	if wantSelected == nil {
		if got.Selected != nil {
			t.Fatalf("selected = %#v, want nil", got.Selected)
		}
		return
	}
	if got.Selected == nil {
		t.Fatalf("selected = nil, want %q", *wantSelected)
	}
	if got.Selected.Identity != *wantSelected {
		t.Fatalf("selected identity = %q, want %q", got.Selected.Identity, *wantSelected)
	}
}

func assertCLIProviderDiscoveryUnavailable(
	t *testing.T,
	availability []CLIProviderAvailability,
	wantUnavailable []CLIProviderIdentity,
) {
	t.Helper()

	if len(availability) != len(RegisteredCLIProviders()) {
		t.Fatalf("availability len = %d, want %d", len(availability), len(RegisteredCLIProviders()))
	}

	unavailable := make(map[CLIProviderIdentity]struct{})
	for _, item := range availability {
		if item.Available {
			continue
		}
		if item.UnavailableReason != string(workerexecution.WorkFailureTypeMissingExecutable) {
			t.Fatalf("identity %q unavailable reason = %q, want %q",
				item.Registration.Identity,
				item.UnavailableReason,
				workerexecution.WorkFailureTypeMissingExecutable,
			)
		}
		unavailable[item.Registration.Identity] = struct{}{}
	}

	for _, identity := range wantUnavailable {
		if _, ok := unavailable[identity]; !ok {
			t.Fatalf("identity %q not classified unavailable", identity)
		}
	}
}

func assertProbedCLIProviderCommands(t *testing.T, probedCommands, wantProbeCommands []string) {
	t.Helper()

	if len(probedCommands) != len(wantProbeCommands) {
		t.Fatalf("lookPath commands = %#v, want %#v", probedCommands, wantProbeCommands)
	}
	for i, wantCommand := range wantProbeCommands {
		if probedCommands[i] != wantCommand {
			t.Fatalf("lookPath command[%d] = %q, want %q", i, probedCommands[i], wantCommand)
		}
	}
}

func assertCLIProviderDiscovery(t *testing.T, tc cliProviderDiscoveryCase) {
	t.Helper()

	var probedCommands []string
	locator := executableLocatorFunc(func(file string) (string, error) {
		probedCommands = append(probedCommands, file)
		if tc.presentCommands[file] {
			return "/fake/bin/" + file, nil
		}
		return "", exec.ErrNotFound
	})

	got := DiscoverRegisteredCLIProvider(locator)
	assertCLIProviderDiscoverySelected(t, got, tc.wantSelected)
	assertCLIProviderDiscoveryUnavailable(t, got.Availability, tc.wantUnavailable)

	wantProbeCommands := tc.wantProbeCommands
	if wantProbeCommands == nil {
		wantProbeCommands = registeredCLIProviderProbeCommands()
	}
	assertProbedCLIProviderCommands(t, probedCommands, wantProbeCommands)
}

func TestDiscoverRegisteredCLIProvider_FakePATHDiscoveryTables(t *testing.T) {
	for _, tc := range fakeCLIProviderDiscoveryCases() {
		t.Run(tc.name, func(t *testing.T) {
			assertCLIProviderDiscovery(t, tc)
		})
	}
}

func ptrCLIProviderIdentity(id CLIProviderIdentity) *CLIProviderIdentity {
	return &id
}

func TestProbeRegisteredCLIProviderAvailability_FakePATHReturnsRankOrder(t *testing.T) {
	presentCommands := map[string]bool{
		string(modelprovider.ProviderCodex):    true,
		string(modelprovider.ProviderClaude):   true,
		string(modelprovider.ProviderCursor):   true,
		string(modelprovider.ProviderOpenCode): true,
		string(modelprovider.ProviderGemini):   true,
		string(modelprovider.ProviderKiro):     true,
		string(modelprovider.ProviderPi):       true,
	}
	locator := executableLocatorFunc(func(file string) (string, error) {
		if presentCommands[file] {
			return "/fake/bin/" + file, nil
		}
		return "", exec.ErrNotFound
	})

	wantOrder := []CLIProviderIdentity{
		CLIProviderIdentityCodex,
		CLIProviderIdentityClaude,
		CLIProviderIdentityCursor,
		CLIProviderIdentityOpenCode,
		CLIProviderIdentityGemini,
		CLIProviderIdentityKiro,
		CLIProviderIdentityPi,
	}

	got := ProbeRegisteredCLIProviderAvailability(locator)
	if len(got) != len(wantOrder) {
		t.Fatalf("availability len = %d, want %d", len(got), len(wantOrder))
	}
	for i, wantIdentity := range wantOrder {
		if got[i].Registration.Identity != wantIdentity {
			t.Fatalf("availability[%d] identity = %q, want %q", i, got[i].Registration.Identity, wantIdentity)
		}
		if !got[i].Available {
			t.Fatalf("availability[%d] identity %q not available on Fake-PATH", i, wantIdentity)
		}
	}
}

func TestProbeCLIProviderAvailability_IsDeterministicForFixedFakePATH(t *testing.T) {
	presentCommands := map[string]bool{
		string(modelprovider.ProviderCodex):    true,
		string(modelprovider.ProviderClaude):   false,
		string(modelprovider.ProviderCursor):   true,
		string(modelprovider.ProviderOpenCode): false,
		string(modelprovider.ProviderGemini):   true,
		string(modelprovider.ProviderKiro):     false,
	}

	locator := executableLocatorFunc(func(file string) (string, error) {
		if presentCommands[file] {
			return "/fake/bin/" + file, nil
		}
		return "", errors.New("executable file not found in $PATH")
	})

	first := ProbeRegisteredCLIProviderAvailability(locator)
	second := ProbeRegisteredCLIProviderAvailability(locator)
	if len(first) != len(second) {
		t.Fatalf("probe length drift: first=%d second=%d", len(first), len(second))
	}
	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("probe[%d] drift: first=%#v second=%#v", i, first[i], second[i])
		}
	}
}
