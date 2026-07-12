package workers

import (
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/config/systemconfig"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestRegisteredCLIProviders_IncludeEverySupportedProvider(t *testing.T) {
	wantCommands := map[CLIProviderIdentity]string{
		CLIProviderIdentityClaude:   string(interfaces.ModelProviderClaude),
		CLIProviderIdentityCodex:    string(interfaces.ModelProviderCodex),
		CLIProviderIdentityGemini:   string(interfaces.ModelProviderGemini),
		CLIProviderIdentityKiro:     string(interfaces.ModelProviderKiro),
		CLIProviderIdentityCursor:   string(interfaces.ModelProviderCursor),
		CLIProviderIdentityOpenCode: string(interfaces.ModelProviderOpenCode),
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

func TestRegisteredCLIProviders_PreferenceRankOrder(t *testing.T) {
	wantOrder := []CLIProviderIdentity{
		CLIProviderIdentityCodex,
		CLIProviderIdentityClaude,
		CLIProviderIdentityCursor,
		CLIProviderIdentityOpenCode,
		CLIProviderIdentityGemini,
		CLIProviderIdentityKiro,
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
		scope := systemconfig.DeriveProviderBackendScopeID(
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

func TestProbeCLIProviderAvailability_FakePATHPresentAndAbsentPerCommand(t *testing.T) {
	originalLookPath := lookPath
	defer func() {
		lookPath = originalLookPath
	}()

	cases := []struct {
		name            string
		command         string
		presentCommands map[string]bool
		wantAvailable   bool
		wantReason      string
	}{
		{
			name:            "claude present",
			command:         string(interfaces.ModelProviderClaude),
			presentCommands: map[string]bool{string(interfaces.ModelProviderClaude): true},
			wantAvailable:   true,
		},
		{
			name:            "claude absent",
			command:         string(interfaces.ModelProviderClaude),
			presentCommands: map[string]bool{},
			wantAvailable:   false,
			wantReason:      string(interfaces.WorkFailureTypeMissingExecutable),
		},
		{
			name:            "codex present",
			command:         string(interfaces.ModelProviderCodex),
			presentCommands: map[string]bool{string(interfaces.ModelProviderCodex): true},
			wantAvailable:   true,
		},
		{
			name:            "codex absent",
			command:         string(interfaces.ModelProviderCodex),
			presentCommands: map[string]bool{},
			wantAvailable:   false,
			wantReason:      string(interfaces.WorkFailureTypeMissingExecutable),
		},
		{
			name:            "cursor present",
			command:         string(interfaces.ModelProviderCursor),
			presentCommands: map[string]bool{string(interfaces.ModelProviderCursor): true},
			wantAvailable:   true,
		},
		{
			name:            "cursor absent",
			command:         string(interfaces.ModelProviderCursor),
			presentCommands: map[string]bool{},
			wantAvailable:   false,
			wantReason:      string(interfaces.WorkFailureTypeMissingExecutable),
		},
		{
			name:            "opencode present",
			command:         string(interfaces.ModelProviderOpenCode),
			presentCommands: map[string]bool{string(interfaces.ModelProviderOpenCode): true},
			wantAvailable:   true,
		},
		{
			name:            "opencode absent",
			command:         string(interfaces.ModelProviderOpenCode),
			presentCommands: map[string]bool{},
			wantAvailable:   false,
			wantReason:      string(interfaces.WorkFailureTypeMissingExecutable),
		},
		{
			name:            "gemini present",
			command:         string(interfaces.ModelProviderGemini),
			presentCommands: map[string]bool{string(interfaces.ModelProviderGemini): true},
			wantAvailable:   true,
		},
		{
			name:            "gemini absent",
			command:         string(interfaces.ModelProviderGemini),
			presentCommands: map[string]bool{},
			wantAvailable:   false,
			wantReason:      string(interfaces.WorkFailureTypeMissingExecutable),
		},
		{
			name:            "kiro present",
			command:         string(interfaces.ModelProviderKiro),
			presentCommands: map[string]bool{string(interfaces.ModelProviderKiro): true},
			wantAvailable:   true,
		},
		{
			name:            "kiro absent",
			command:         string(interfaces.ModelProviderKiro),
			presentCommands: map[string]bool{},
			wantAvailable:   false,
			wantReason:      string(interfaces.WorkFailureTypeMissingExecutable),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var probedCommands []string
			lookPath = func(file string) (string, error) {
				probedCommands = append(probedCommands, file)
				if tc.presentCommands[file] {
					return "/fake/bin/" + file, nil
				}
				return "", exec.ErrNotFound
			}

			registration, ok := CLIProviderRegistrationByCommand(tc.command)
			if !ok {
				t.Fatalf("CLIProviderRegistrationByCommand(%q) miss", tc.command)
			}

			got := ProbeCLIProviderAvailability(registration)
			if got.Available != tc.wantAvailable {
				t.Fatalf("available = %v, want %v", got.Available, tc.wantAvailable)
			}
			if got.UnavailableReason != tc.wantReason {
				t.Fatalf("unavailable reason = %q, want %q", got.UnavailableReason, tc.wantReason)
			}
			if len(probedCommands) != 1 || probedCommands[0] != tc.command {
				t.Fatalf("lookPath commands = %#v, want [%q]", probedCommands, tc.command)
			}
		})
	}
}

func TestProbeCLIProviderAvailability_IsDeterministicForFixedFakePATH(t *testing.T) {
	originalLookPath := lookPath
	defer func() {
		lookPath = originalLookPath
	}()

	presentCommands := map[string]bool{
		string(interfaces.ModelProviderCodex):    true,
		string(interfaces.ModelProviderClaude):   false,
		string(interfaces.ModelProviderCursor):   true,
		string(interfaces.ModelProviderOpenCode): false,
		string(interfaces.ModelProviderGemini):   true,
		string(interfaces.ModelProviderKiro):     false,
	}

	lookPath = func(file string) (string, error) {
		if presentCommands[file] {
			return "/fake/bin/" + file, nil
		}
		return "", errors.New("executable file not found in $PATH")
	}

	first := ProbeRegisteredCLIProviderAvailability()
	second := ProbeRegisteredCLIProviderAvailability()
	if len(first) != len(second) {
		t.Fatalf("probe length drift: first=%d second=%d", len(first), len(second))
	}
	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("probe[%d] drift: first=%#v second=%#v", i, first[i], second[i])
		}
	}
}
