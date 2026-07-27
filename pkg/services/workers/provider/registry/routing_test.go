package registry

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type recordingExecutableLocator struct {
	commands []string
	missing  string
}

func (l *recordingExecutableLocator) LookPath(command string) (string, error) {
	l.commands = append(l.commands, command)
	if command == l.missing {
		return "", errors.New("not found")
	}
	return "/bin/" + command, nil
}

func TestResolveRunnerSelectionUsesRegistryIdentityAndPrecedence(t *testing.T) {
	t.Parallel()
	providers := newBuiltInRegistry(t)

	tests := []struct {
		name        string
		workstation string
		factory     string
		worker      string
		wantID      string
		wantSource  workers.RunnerSelectionSource
	}{
		{
			name:        "manifest alias",
			workstation: " AGENT ",
			factory:     "gemini",
			worker:      "claude",
			wantID:      workers.RunnerIDCursorCLI,
			wantSource:  workers.RunnerSelectionSourceWorkstation,
		},
		{
			name:       "published alias",
			factory:    "kiro-cli",
			worker:     "claude",
			wantID:     workers.RunnerIDKiro,
			wantSource: workers.RunnerSelectionSourceFactory,
		},
		{
			name:       "legacy public model provider alias",
			worker:     "openai",
			wantID:     workers.RunnerIDCodex,
			wantSource: workers.RunnerSelectionSourceLegacyProvider,
		},
		{
			name:       "legacy anthropic model provider alias",
			worker:     "anthropic",
			wantID:     "claude",
			wantSource: workers.RunnerSelectionSourceLegacyProvider,
		},
		{
			name:       "legacy cursor runner compatibility",
			worker:     workers.RunnerIDCursorCLI,
			wantID:     workers.RunnerIDCursorCLI,
			wantSource: workers.RunnerSelectionSourceLegacyProvider,
		},
		{
			name:       "default",
			wantID:     workers.RunnerIDCodex,
			wantSource: workers.RunnerSelectionSourceDefault,
		},
		{
			name:       "unresolved authored provider template defers to runtime default",
			worker:     "${branchProvider}",
			wantID:     workers.RunnerIDCodex,
			wantSource: workers.RunnerSelectionSourceDefault,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := providers.ResolveRunnerSelection(test.workstation, test.factory, test.worker)
			if err != nil {
				t.Fatalf("ResolveRunnerSelection() error = %v", err)
			}
			if got.RunnerID != test.wantID || got.Source != test.wantSource {
				t.Fatalf("ResolveRunnerSelection() = %#v, want id=%q source=%q", got, test.wantID, test.wantSource)
			}
		})
	}
}

func TestCompatibilityAliasesUseRegistryIdentityAuthority(t *testing.T) {
	t.Parallel()
	providers := newBuiltInRegistry(t)

	tests := []struct {
		alias         string
		wantCanonical string
		wantRunner    string
	}{
		{alias: "anthropic", wantCanonical: "claude", wantRunner: "claude"},
		{alias: workers.RunnerIDCursorCLI, wantCanonical: "cursor", wantRunner: workers.RunnerIDCursorCLI},
		{alias: "kiro-cli", wantCanonical: "kiro", wantRunner: workers.RunnerIDKiro},
		{alias: "openai", wantCanonical: "codex", wantRunner: workers.RunnerIDCodex},
	}
	for _, test := range tests {
		test := test
		t.Run(test.alias, func(t *testing.T) {
			t.Parallel()
			canonical, err := providers.CanonicalIdentity(test.alias)
			if err != nil {
				t.Fatalf("CanonicalIdentity(%q) error = %v", test.alias, err)
			}
			runner, err := providers.RunnerID(test.alias)
			if err != nil {
				t.Fatalf("RunnerID(%q) error = %v", test.alias, err)
			}
			if canonical != test.wantCanonical || runner != test.wantRunner {
				t.Fatalf(
					"compatibility identity %q = (canonical %q, runner %q), want (%q, %q)",
					test.alias,
					canonical,
					runner,
					test.wantCanonical,
					test.wantRunner,
				)
			}
		})
	}
}

func TestResolveRunnerSelectionRejectsUnknownAndNonSelectableWithoutFallback(t *testing.T) {
	t.Parallel()
	providers := newBuiltInRegistry(t)

	for _, identity := range []string{"unknown", "agy"} {
		identity := identity
		t.Run(identity, func(t *testing.T) {
			t.Parallel()
			_, err := providers.ResolveRunnerSelection("", "", identity)
			if err == nil || !strings.Contains(err.Error(), `provider "`+identity+`"`) {
				t.Fatalf("ResolveRunnerSelection(%q) error = %v", identity, err)
			}
		})
	}
}

func TestResolveRunnerSelectionUsesExternalIntegrationCanonicalIdentity(t *testing.T) {
	t.Parallel()
	registrations, err := BuiltInRegistrations()
	if err != nil {
		t.Fatalf("BuiltInRegistrations() error = %v", err)
	}
	manifest := externalManifest(t, "customer.provider", "customer")
	registrations = append(registrations, ExternalRegistration(manifest, integrationFor(manifest)))
	providers, err := New(registrations...)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	selection, err := providers.ResolveRunnerSelection("customer", "", "")
	if err != nil {
		t.Fatalf("ResolveRunnerSelection(external) error = %v", err)
	}
	if selection.RunnerID != "customer.provider" ||
		selection.Source != workers.RunnerSelectionSourceWorkstation {
		t.Fatalf(
			"ResolveRunnerSelection(external) = %#v, want canonical registered integration",
			selection,
		)
	}
	if providers.UsesNativeRunner(selection.RunnerID) {
		t.Fatal("UsesNativeRunner(external) = true, want conductor route")
	}
	if !providers.UsesNativeRunner(workers.RunnerIDCodex) {
		t.Fatal("UsesNativeRunner(codex) = false, want native runner route")
	}
	if providers.UsesNativeRunner("gemini") {
		t.Fatal("UsesNativeRunner(gemini) = true, want conductor route for migrated Gemini")
	}
	if providers.UsesNativeRunner("cursor") || providers.UsesNativeRunner(workers.RunnerIDCursorCLI) {
		t.Fatal("UsesNativeRunner(cursor) = true, want conductor route for migrated Cursor")
	}
	if _, err := providers.Integration("customer"); err != nil {
		t.Fatalf("Integration(external) error = %v", err)
	}
	if _, err := providers.RunnerID("customer"); err == nil ||
		!strings.Contains(err.Error(), "not available through the provider-native runner path") {
		t.Fatalf("RunnerID(external) error = %v, want native-path rejection", err)
	}
}

func TestRunnerMetadataUsesManifestCapabilities(t *testing.T) {
	t.Parallel()
	providers := newBuiltInRegistry(t)

	metadata, err := providers.RunnerMetadata("codex")
	if err != nil {
		t.Fatalf("RunnerMetadata() error = %v", err)
	}
	if metadata.ID != workers.RunnerIDCodex || metadata.DisplayName != "Codex" {
		t.Fatalf("RunnerMetadata() = %#v", metadata)
	}
	wantBaseline := []workers.RunnerBaselineCapability{
		workers.RunnerBaselineCapabilityPromptSubmission,
		workers.RunnerBaselineCapabilityToolExecution,
	}
	if !reflect.DeepEqual(metadata.Capabilities.Baseline, wantBaseline) {
		t.Fatalf("baseline = %#v, want %#v", metadata.Capabilities.Baseline, wantBaseline)
	}
	assertOptionalCapabilityStatus(
		t,
		metadata,
		workers.RunnerOptionalCapabilityStructuredOutput,
		workers.RunnerOptionalCapabilityStatusSupported,
	)

	gemini, err := providers.RunnerMetadata("gemini")
	if err != nil {
		t.Fatalf("RunnerMetadata(gemini) error = %v", err)
	}
	assertOptionalCapabilityStatus(
		t,
		gemini,
		workers.RunnerOptionalCapabilitySessionResume,
		workers.RunnerOptionalCapabilityStatusUnsupported,
	)
}

func TestValidateRunnerPrerequisitesUsesManifestExecutableAndAlias(t *testing.T) {
	t.Parallel()
	providers := newBuiltInRegistry(t)
	locator := &recordingExecutableLocator{}

	if err := providers.ValidateRunnerPrerequisites(locator, "agent"); err != nil {
		t.Fatalf("ValidateRunnerPrerequisites() error = %v", err)
	}
	if !reflect.DeepEqual(locator.commands, []string{"agent"}) {
		t.Fatalf("commands = %#v, want manifest executable", locator.commands)
	}

	locator.missing = "kiro-cli"
	err := providers.ValidateRunnerPrerequisites(locator, "kiro")
	if err == nil || !strings.Contains(err.Error(), `requires "kiro-cli" on PATH`) {
		t.Fatalf("ValidateRunnerPrerequisites(kiro) error = %v", err)
	}
}

func assertOptionalCapabilityStatus(
	t *testing.T,
	metadata workers.RunnerMetadata,
	capability workers.RunnerOptionalCapability,
	want workers.RunnerOptionalCapabilityStatus,
) {
	t.Helper()
	for _, support := range metadata.Capabilities.Optional {
		if support.Capability == capability {
			if support.Status != want {
				t.Fatalf("%s status = %q, want %q", capability, support.Status, want)
			}
			return
		}
	}
	t.Fatalf("metadata missing optional capability %q", capability)
}

func newBuiltInRegistry(t *testing.T) *Registry {
	t.Helper()
	registrations, err := BuiltInRegistrations()
	if err != nil {
		t.Fatalf("BuiltInRegistrations() error = %v", err)
	}
	providers, err := New(registrations...)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return providers
}
