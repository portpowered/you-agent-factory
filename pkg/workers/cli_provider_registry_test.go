package workers

import (
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
