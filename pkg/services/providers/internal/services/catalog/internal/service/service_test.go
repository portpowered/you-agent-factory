package service_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	catalog "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog"
	internalservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog/internal/service"
)

func TestListProvidersReturnsCompleteEnumeration(t *testing.T) {
	t.Parallel()

	service, err := internalservice.New()
	if err != nil {
		t.Fatalf("New() = %v", err)
	}

	list, err := service.ListProviders(context.Background(), providers.ListProvidersRequest{})
	if err != nil {
		t.Fatalf("ListProviders() = %v", err)
	}
	if len(list.Providers) != 3 {
		t.Fatalf("len(Providers) = %d, want 3", len(list.Providers))
	}

	byID := indexProviders(list.Providers)
	for _, id := range []providers.ID{
		providers.IDAntigravity,
		providers.IDClaude,
		providers.IDCodex,
	} {
		if _, ok := byID[id]; !ok {
			t.Fatalf("Providers missing %q", id)
		}
	}
}

func TestListProvidersOrderIsDeterministic(t *testing.T) {
	t.Parallel()

	service, err := internalservice.New()
	if err != nil {
		t.Fatalf("New() = %v", err)
	}

	first, err := service.ListProviders(context.Background(), providers.ListProvidersRequest{})
	if err != nil {
		t.Fatalf("first ListProviders() = %v", err)
	}
	second, err := service.ListProviders(context.Background(), providers.ListProvidersRequest{})
	if err != nil {
		t.Fatalf("second ListProviders() = %v", err)
	}

	ids := providerIDs(first.Providers)
	if !slices.IsSorted(ids) {
		t.Fatalf("provider IDs = %v, want canonical sorted order", ids)
	}
	if !slices.Equal(providerIDs(second.Providers), ids) {
		t.Fatalf("repeated list order = %v, want %v", providerIDs(second.Providers), ids)
	}
}

func TestNewAddsAndReplacesContributedDescriptors(t *testing.T) {
	t.Parallel()

	service, err := internalservice.New(internalservice.WithDescriptors(
		providers.Descriptor{ID: providers.IDCodex, DisplayName: "Configured Codex", Availability: providers.AvailabilitySelectable, Readiness: providers.ReadinessReady},
		providers.Descriptor{ID: "cursor-acp", DisplayName: "cursor-acp", Availability: providers.AvailabilitySelectable, Readiness: providers.ReadinessReady},
	))
	if err != nil {
		t.Fatalf("New(WithDescriptors) = %v", err)
	}
	list, err := service.ListProviders(context.Background(), providers.ListProvidersRequest{})
	if err != nil {
		t.Fatalf("ListProviders() = %v", err)
	}
	indexed := indexProviders(list.Providers)
	if indexed[providers.IDCodex].DisplayName != "Configured Codex" {
		t.Fatalf("replacement = %#v", indexed[providers.IDCodex])
	}
	if indexed["cursor-acp"].ID != "cursor-acp" {
		t.Fatalf("contributed descriptor missing: %#v", indexed)
	}
}

func TestNewAppliesCapabilityOverrideToPublishedDescriptor(t *testing.T) {
	t.Parallel()

	capabilities := []providers.Capability{providers.CapabilityPromptSubmission}
	service, err := internalservice.New(internalservice.WithCapabilityOverrides(
		catalog.CapabilityOverride{
			Provider:     providers.IDCodex,
			Capabilities: capabilities,
		},
	))
	if err != nil {
		t.Fatalf("New(WithCapabilityOverrides) = %v", err)
	}

	capabilities[0] = providers.CapabilityUsage
	registered, err := service.RegistrationProvider(providers.IDCodex)
	if err != nil {
		t.Fatalf("RegistrationProvider(codex) = %v", err)
	}
	if !slices.Equal(registered.Capabilities, []providers.Capability{providers.CapabilityPromptSubmission}) {
		t.Fatalf("overridden capabilities = %v, want prompt_submission only", registered.Capabilities)
	}

	registered.Capabilities[0] = providers.CapabilityUsage
	reloaded, err := service.RegistrationProvider(providers.IDCodex)
	if err != nil {
		t.Fatalf("second RegistrationProvider(codex) = %v", err)
	}
	if !slices.Equal(reloaded.Capabilities, []providers.Capability{providers.CapabilityPromptSubmission}) {
		t.Fatalf("reloaded capabilities = %v, want detached prompt_submission only", reloaded.Capabilities)
	}
}

func TestNewRejectsCapabilityOverrideForUnknownPublishedProvider(t *testing.T) {
	t.Parallel()

	service, err := internalservice.New(internalservice.WithCapabilityOverrides(
		catalog.CapabilityOverride{
			Provider:     "not-published",
			Capabilities: []providers.Capability{providers.CapabilityPromptSubmission},
		},
	))
	if service != nil || err == nil || !strings.Contains(err.Error(), "unknown provider") {
		t.Fatalf("New(unknown capability override) = (%#v, %v), want construction error", service, err)
	}
}

func TestListProvidersIncludesExperimentalSelectableEntries(t *testing.T) {
	t.Parallel()

	service, err := internalservice.New()
	if err != nil {
		t.Fatalf("New() = %v", err)
	}

	list, err := service.ListProviders(context.Background(), providers.ListProvidersRequest{})
	if err != nil {
		t.Fatalf("ListProviders() = %v", err)
	}

	antigravity := indexProviders(list.Providers)[providers.IDAntigravity]
	if antigravity.Availability != providers.AvailabilitySelectable {
		t.Fatalf("antigravity availability = %q, want %q", antigravity.Availability, providers.AvailabilitySelectable)
	}
	if antigravity.Readiness != providers.ReadinessUnverified {
		t.Fatalf("antigravity readiness = %q, want %q", antigravity.Readiness, providers.ReadinessUnverified)
	}
}

func TestListProvidersReturnsDetachedValues(t *testing.T) {
	t.Parallel()

	service, err := internalservice.New()
	if err != nil {
		t.Fatalf("New() = %v", err)
	}

	first, err := service.ListProviders(context.Background(), providers.ListProvidersRequest{})
	if err != nil {
		t.Fatalf("first ListProviders() = %v", err)
	}
	if len(first.Providers) == 0 {
		t.Fatal("first ListProviders() returned no providers")
	}
	mutateProviderForDetachTest(&first.Providers[0])

	second, err := service.ListProviders(context.Background(), providers.ListProvidersRequest{})
	if err != nil {
		t.Fatalf("second ListProviders() = %v", err)
	}
	assertDetachedProvider(t, first.Providers[0], second.Providers[0])
}

func TestListProvidersProjectsIdentityMetadataAndCapabilities(t *testing.T) {
	t.Parallel()

	service, err := internalservice.New()
	if err != nil {
		t.Fatalf("New() = %v", err)
	}

	list, err := service.ListProviders(context.Background(), providers.ListProvidersRequest{})
	if err != nil {
		t.Fatalf("ListProviders() = %v", err)
	}

	indexed := indexProviders(list.Providers)
	assertCodexCatalogFacts(t, indexed[providers.IDCodex])
	assertAntigravityCatalogFacts(t, indexed[providers.IDAntigravity])
	assertClaudeCatalogFacts(t, indexed[providers.IDClaude])

	if _, ok := indexed[providers.IDCursor]; ok {
		t.Fatal("native cursor must not be present in the selectable provider catalog")
	}
}

func mutateProviderForDetachTest(provider *providers.Descriptor) {
	provider.DisplayName = "mutated"
	if len(provider.Aliases) > 0 {
		provider.Aliases[0] = "mutated"
	}
	provider.Models[0].ID = "mutated-model"
	provider.Models[0].Efforts = append(provider.Models[0].Efforts, "mutated-effort")
	for index := range provider.KnownLimits {
		if provider.KnownLimits[index].Default != nil {
			*provider.KnownLimits[index].Default = 999
		}
	}
	if len(provider.Capabilities) > 0 {
		provider.Capabilities[0] = providers.CapabilityUsage
	}
}

func assertDetachedProvider(t *testing.T, first, second providers.Descriptor) {
	t.Helper()
	if second.DisplayName == "mutated" {
		t.Fatalf("second list display name = %q, want detached copy", second.DisplayName)
	}
	if len(second.Aliases) > 0 {
		if second.Aliases[0] == "mutated" {
			t.Fatal("second list aliases share mutation from first result")
		}
	}
	if len(second.Capabilities) > 0 {
		if second.Capabilities[0] == providers.CapabilityUsage && first.Capabilities[0] == providers.CapabilityUsage {
			t.Fatal("second list capabilities share mutation from first result")
		}
	}
	if second.Models[0].ID == "mutated-model" || slices.Contains(second.Models[0].Efforts, "mutated-effort") {
		t.Fatal("second list model facts share mutation from first result")
	}
	for _, limit := range second.KnownLimits {
		if limit.Default != nil && *limit.Default != 300 {
			t.Fatal("second list limit facts share mutation from first result")
		}
	}
}

func assertCodexCatalogFacts(t *testing.T, codex providers.Descriptor) {
	t.Helper()
	if codex.DisplayName != "Codex" || codex.Availability != providers.AvailabilitySelectable || codex.Readiness != providers.ReadinessUnverified {
		t.Fatalf("codex identity = %#v, want Codex/selectable/unverified", codex)
	}
	if !slices.Contains(codex.Capabilities, providers.CapabilityPromptSubmission) {
		t.Fatalf("codex capabilities = %#v, want prompt_submission", codex.Capabilities)
	}
	assertCapabilities(t, codex, []providers.Capability{
		providers.CapabilityPromptSubmission,
		providers.CapabilityImageInput,
		providers.CapabilitySessionResume,
		providers.CapabilityStructuredOutput,
		providers.CapabilityPermissionBypass,
		providers.CapabilityNativeStreaming,
		providers.CapabilityMessageSnapshots,
		providers.CapabilityReasoningSummaries,
		providers.CapabilityToolLifecycle,
		providers.CapabilityToolOutputDeltas,
		providers.CapabilityFileChanges,
		providers.CapabilityPlans,
		providers.CapabilityUsage,
		providers.CapabilityStableItemIDs,
	})
	if codex.TechnicalSupportLevel != providers.TechnicalSupportProduction || codex.ImplementationAvailability != providers.ImplementationBundled {
		t.Fatalf("codex publication posture = %q/%q, want production/bundled", codex.TechnicalSupportLevel, codex.ImplementationAvailability)
	}
	assertPrerequisiteNames(t, codex, []string{
		"authentication/account-authentication",
		"configuration/stdio",
		"executable/codex",
		"workspace/writable-workspace",
	})
	assertToolNames(t, codex, []string{"filesystem", "shell", "web_search"})
	wantModels := []string{"gpt-5.6", "gpt-5.6-luna", "gpt-5.6-sol", "gpt-5.6-terra"}
	if len(codex.Models) != len(wantModels) {
		t.Fatalf("codex models = %#v, want exact IDs %v", codex.Models, wantModels)
		return
	}
	for index, model := range codex.Models {
		if model.ID != wantModels[index] {
			t.Fatalf("codex model[%d] = %q, want %q", index, model.ID, wantModels[index])
		}
		if !slices.Equal(model.Efforts, []providers.ReasoningEffort{"minimal", "low", "medium", "high", "xhigh", "max"}) {
			t.Fatalf("codex %s efforts = %v, want canonical order", model.ID, model.Efforts)
		}
		assertProviderModality(t, "codex audio input", model, providers.ModalityAudio, providers.ModalityUnsupported, providers.ModalityTransportNone)
		assertProviderModality(t, "codex video input", model, providers.ModalityVideo, providers.ModalityUnsupported, providers.ModalityTransportNone)
	}
	if len(codex.KnownLimits) != 1 || codex.KnownLimits[0].Maximum == nil || *codex.KnownLimits[0].Maximum != 5 {
		t.Fatalf("codex known limits = %#v, want referenced image-path maximum 5", codex.KnownLimits)
	}
}

func assertAntigravityCatalogFacts(t *testing.T, agy providers.Descriptor) {
	t.Helper()
	wantModels := []string{
		"claude-opus-4-6-thinking", "claude-sonnet-4-6", "gemini-3.1-pro-high",
		"gemini-3.1-pro-low", "gemini-3.5-flash-high", "gemini-3.5-flash-low",
		"gemini-3.5-flash-medium", "gemini-3.6-flash-high", "gemini-3.6-flash-low",
		"gemini-3.6-flash-medium", "gpt-oss-120b-medium",
	}
	if len(agy.Models) != len(wantModels) {
		t.Fatalf("AGY models = %d, want %d", len(agy.Models), len(wantModels))
		return
	}
	for index, model := range agy.Models {
		if model.ID != wantModels[index] {
			t.Fatalf("AGY model[%d] = %q, want %q", index, model.ID, wantModels[index])
		}
		if len(model.Efforts) != 0 {
			t.Fatalf("AGY %s efforts = %v, want explicit empty model-encoded effort list", model.ID, model.Efforts)
		}
	}
	for _, kind := range []providers.ModalityKind{providers.ModalityAudio, providers.ModalityVideo} {
		assertProviderModality(t, fmt.Sprintf("AGY %s input", kind), agy.Models[0], kind, providers.ModalitySupported, providers.ModalityTransportFilePath)
	}
	if len(agy.KnownLimits) != 3 || agy.KnownLimits[0].Name != "add_dir_workspace" || agy.KnownLimits[1].Name != "effort_selection" || agy.KnownLimits[2].Name != "print_timeout" {
		t.Fatalf("AGY known limits = %#v, want stable name order", agy.KnownLimits)
	}
	assertCapabilities(t, agy, []providers.Capability{
		providers.CapabilityPromptSubmission,
		providers.CapabilitySessionResume,
		providers.CapabilityPermissionBypass,
		providers.CapabilityMessageSnapshots,
	})
	assertAntigravityPrerequisites(t, agy)
	assertToolNames(t, agy, []string{"filesystem", "image_generation", "shell"})
}

func assertAntigravityPrerequisites(t *testing.T, agy providers.Descriptor) {
	t.Helper()
	assertPrerequisiteNames(t, agy, []string{
		"authentication/account-authentication",
		"configuration/stdio",
		"executable/agy",
		"workspace/writable-workspace",
	})
	prerequisiteKinds := make(map[providers.PrerequisiteKind]bool, len(agy.Prerequisites))
	for _, prerequisite := range agy.Prerequisites {
		prerequisiteKinds[prerequisite.Kind] = true
	}
	for _, kind := range []providers.PrerequisiteKind{providers.PrerequisiteAuthentication, providers.PrerequisiteExecutable, providers.PrerequisiteWorkspace} {
		if !prerequisiteKinds[kind] {
			t.Fatalf("AGY prerequisites = %#v, missing %q", agy.Prerequisites, kind)
		}
	}
	for _, prerequisite := range agy.Prerequisites {
		if prerequisite.Status != providers.PrerequisiteRequired {
			t.Fatalf("AGY prerequisite %s/%s status = %q, want required", prerequisite.Kind, prerequisite.Name, prerequisite.Status)
		}
	}
}

func assertClaudeCatalogFacts(t *testing.T, claude providers.Descriptor) {
	t.Helper()
	if claude.Readiness != providers.ReadinessUnverified {
		t.Fatalf("Claude readiness = %q, want unverified for an unprobed catalog", claude.Readiness)
	}
	wantModels := []string{"claude-opus-4-6-thinking", "claude-sonnet-4-20250514", "claude-sonnet-5"}
	if len(claude.Models) != len(wantModels) {
		t.Fatalf("Claude models = %#v, want exact IDs %v", claude.Models, wantModels)
	}
	for index, model := range claude.Models {
		if model.ID != wantModels[index] {
			t.Fatalf("Claude model[%d] = %q, want %q", index, model.ID, wantModels[index])
		}
		if !slices.Equal(model.Efforts, []providers.ReasoningEffort{"low", "medium", "high", "xhigh", "max"}) {
			t.Fatalf("Claude %s efforts = %v, want canonical order", model.ID, model.Efforts)
		}
		assertProviderModality(t, "Claude text input", model, providers.ModalityText, providers.ModalitySupported, providers.ModalityTransportInline)
		assertProviderModality(t, "Claude audio input", model, providers.ModalityAudio, providers.ModalityUnsupported, providers.ModalityTransportNone)
		assertProviderModality(t, "Claude video input", model, providers.ModalityVideo, providers.ModalityUnsupported, providers.ModalityTransportNone)
	}
	assertCapabilities(t, claude, []providers.Capability{
		providers.CapabilityPromptSubmission,
		providers.CapabilitySessionResume,
		providers.CapabilityPermissionBypass,
		providers.CapabilityNativeStreaming,
		providers.CapabilityMessageDeltas,
		providers.CapabilityMessageSnapshots,
		providers.CapabilityToolLifecycle,
		providers.CapabilityToolOutputDeltas,
		providers.CapabilityStableItemIDs,
	})
	assertPrerequisiteNames(t, claude, []string{
		"authentication/account-authentication",
		"configuration/stdio",
		"executable/claude",
		"workspace/writable-workspace",
	})
	assertToolNames(t, claude, []string{"filesystem", "shell", "web_search"})
	for _, prerequisite := range claude.Prerequisites {
		if prerequisite.Status != providers.PrerequisiteRequired {
			t.Fatalf("Claude prerequisite %s/%s status = %q, want required", prerequisite.Kind, prerequisite.Name, prerequisite.Status)
		}
	}
}

func assertCapabilities(t *testing.T, descriptor providers.Descriptor, want []providers.Capability) {
	t.Helper()
	if !slices.Equal(descriptor.Capabilities, want) {
		t.Fatalf("%s capabilities = %v, want %v", descriptor.ID, descriptor.Capabilities, want)
	}
}

func assertPrerequisiteNames(t *testing.T, descriptor providers.Descriptor, want []string) {
	t.Helper()
	got := make([]string, len(descriptor.Prerequisites))
	for index, prerequisite := range descriptor.Prerequisites {
		got[index] = string(prerequisite.Kind) + "/" + prerequisite.Name
	}
	if !slices.Equal(got, want) {
		t.Fatalf("%s prerequisites = %v, want %v", descriptor.ID, got, want)
	}
}

func assertToolNames(t *testing.T, descriptor providers.Descriptor, want []string) {
	t.Helper()
	got := make([]string, len(descriptor.Tools))
	for index, tool := range descriptor.Tools {
		got[index] = tool.Name
		if tool.Support != providers.ToolSupported {
			t.Fatalf("%s tool %q support = %q, want supported", descriptor.ID, tool.Name, tool.Support)
		}
	}
	if !slices.Equal(got, want) {
		t.Fatalf("%s tools = %v, want %v", descriptor.ID, got, want)
	}
}

func assertProviderModality(t *testing.T, label string, model providers.ModelDescriptor, kind providers.ModalityKind, support providers.ModalitySupport, transport providers.ModalityTransport) {
	t.Helper()
	modality := findProviderModality(model.Modalities, providers.ModalityDirectionInput, kind)
	if modality == nil || modality.Support != support || modality.Transport != transport {
		t.Fatalf("%s = %#v, want %s/%s", label, modality, support, transport)
	}
}

func TestGetProviderResolvesCanonicalID(t *testing.T) {
	t.Parallel()

	service, err := internalservice.New()
	if err != nil {
		t.Fatalf("New() = %v", err)
	}

	got, err := service.GetProvider(context.Background(), providers.GetProviderRequest{ID: providers.IDCodex})
	if err != nil {
		t.Fatalf("GetProvider(codex) = %v", err)
	}
	if got.Provider.ID != providers.IDCodex {
		t.Fatalf("provider id = %q, want %q", got.Provider.ID, providers.IDCodex)
	}
	if got.Provider.DisplayName != "Codex" {
		t.Fatalf("display name = %q, want Codex", got.Provider.DisplayName)
	}
	if got.Provider.Availability != providers.AvailabilitySelectable {
		t.Fatalf("availability = %q, want selectable", got.Provider.Availability)
	}
	if got.Provider.Readiness != providers.ReadinessUnverified {
		t.Fatalf("readiness = %q, want unverified", got.Provider.Readiness)
	}
	if !slices.Contains(got.Provider.Capabilities, providers.CapabilityPromptSubmission) {
		t.Fatalf("capabilities = %#v, want prompt_submission", got.Provider.Capabilities)
	}
}

func TestResolveProviderIDUsesStaticCanonicalAuthority(t *testing.T) {
	t.Parallel()

	probeCalls := 0
	service, err := internalservice.New(internalservice.WithProbeQuery(func(
		context.Context,
		providers.Descriptor,
	) (catalog.ProbeFacts, error) {
		probeCalls++
		return catalog.ProbeFacts{}, nil
	}))
	if err != nil {
		t.Fatalf("NewService() = %v", err)
	}

	tests := []struct {
		name string
		id   providers.ID
		want providers.ID
		err  error
	}{
		{name: "canonical", id: providers.IDCodex, want: providers.IDCodex},
		{name: "retired native cursor", id: "cursor", err: providers.ErrUnknownProvider},
		{name: "invalid", err: providers.ErrInvalidID},
		{name: "unknown", id: "missing", err: providers.ErrUnknownProvider},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, resolveErr := service.ResolveProviderID(test.id)
			if !errors.Is(resolveErr, test.err) {
				t.Fatalf(
					"ResolveProviderID(%q) error = %v, want %v",
					test.id,
					resolveErr,
					test.err,
				)
			}
			if got != test.want {
				t.Fatalf("ResolveProviderID(%q) = %q, want %q", test.id, got, test.want)
			}
		})
	}
	if probeCalls != 0 {
		t.Fatalf("ResolveProviderID() probe calls = %d, want 0", probeCalls)
	}
}

func TestRegistrationProviderReturnsStaticDetachedCatalogFacts(t *testing.T) {
	t.Parallel()

	probeCalls := 0
	service, err := internalservice.New(internalservice.WithProbeQuery(func(
		context.Context,
		providers.Descriptor,
	) (catalog.ProbeFacts, error) {
		probeCalls++
		return catalog.ProbeFacts{}, nil
	}))
	if err != nil {
		t.Fatalf("New() = %v", err)
	}

	codex, err := service.RegistrationProvider(providers.IDCodex)
	if err != nil {
		t.Fatalf("RegistrationProvider(codex) = %v", err)
	}
	claude, err := service.RegistrationProvider(providers.IDClaude)
	if err != nil {
		t.Fatalf("RegistrationProvider(claude) = %v", err)
	}
	if codex.ID != providers.IDCodex ||
		codex.Availability != providers.AvailabilitySelectable ||
		!slices.Contains(codex.Capabilities, providers.CapabilityStructuredOutput) {
		t.Fatalf("Codex registration facts = %#v", codex)
	}
	if claude.ID != providers.IDClaude ||
		claude.Availability != providers.AvailabilitySelectable ||
		!slices.Contains(claude.Capabilities, providers.CapabilityMessageDeltas) {
		t.Fatalf("Claude registration facts = %#v", claude)
	}
	if probeCalls != 0 {
		t.Fatalf("RegistrationProvider() probe calls = %d, want 0", probeCalls)
	}

	codex.Capabilities[0] = providers.CapabilityUsage
	reloaded, err := service.RegistrationProvider(providers.IDCodex)
	if err != nil {
		t.Fatalf("second RegistrationProvider(codex) = %v", err)
	}
	if reflect.DeepEqual(reloaded.Capabilities, codex.Capabilities) {
		t.Fatal("RegistrationProvider() returned catalog-owned capability slice")
	}
}

func TestGetProviderReturnsDetachedValues(t *testing.T) {
	t.Parallel()

	service, err := internalservice.New()
	if err != nil {
		t.Fatalf("New() = %v", err)
	}

	first, err := service.GetProvider(context.Background(), providers.GetProviderRequest{ID: providers.IDCodex})
	if err != nil {
		t.Fatalf("first GetProvider() = %v", err)
	}
	second, err := service.GetProvider(context.Background(), providers.GetProviderRequest{ID: providers.IDCodex})
	if err != nil {
		t.Fatalf("second GetProvider() = %v", err)
	}
	if !reflect.DeepEqual(first.Provider, second.Provider) {
		t.Fatalf("repeated get = %#v, want %#v", first.Provider, second.Provider)
	}

	first.Provider.DisplayName = "mutated"
	if len(first.Provider.Aliases) > 0 {
		first.Provider.Aliases[0] = "mutated"
	}
	if len(first.Provider.Capabilities) > 0 {
		first.Provider.Capabilities[0] = providers.CapabilityUsage
	}

	third, err := service.GetProvider(context.Background(), providers.GetProviderRequest{ID: providers.IDCodex})
	if err != nil {
		t.Fatalf("third GetProvider() = %v", err)
	}
	if third.Provider.DisplayName == "mutated" {
		t.Fatalf("third get display name = %q, want detached copy", third.Provider.DisplayName)
	}
	if len(third.Provider.Aliases) > 0 && third.Provider.Aliases[0] == "mutated" {
		t.Fatal("third get aliases share mutation from first result")
	}
	if len(third.Provider.Capabilities) > 0 &&
		third.Provider.Capabilities[0] == providers.CapabilityUsage &&
		first.Provider.Capabilities[0] == providers.CapabilityUsage {
		t.Fatal("third get capabilities share mutation from first result")
	}
}

func TestGetProviderTypedFailures(t *testing.T) {
	t.Parallel()

	service, err := internalservice.New()
	if err != nil {
		t.Fatalf("New() = %v", err)
	}

	assertGetErrorIs(t, service, providers.GetProviderRequest{}, providers.ErrInvalidID)
	assertGetErrorIs(
		t,
		service,
		providers.GetProviderRequest{ID: providers.ID("unknown-provider")},
		providers.ErrUnknownProvider,
	)
	assertGetErrorIs(
		t,
		service,
		providers.GetProviderRequest{ID: providers.IDCodex + "-stale"},
		providers.ErrUnknownProvider,
	)

	got, err := service.GetProvider(context.Background(), providers.GetProviderRequest{ID: providers.IDAntigravity})
	if err != nil {
		t.Fatalf("GetProvider(agy) = %v", err)
	}
	if got.Provider.Availability != providers.AvailabilitySelectable {
		t.Fatalf("agy availability = %q, want %q", got.Provider.Availability, providers.AvailabilitySelectable)
	}

	list, err := service.ListProviders(context.Background(), providers.ListProvidersRequest{})
	if err != nil {
		t.Fatalf("ListProviders() = %v", err)
	}
	agy, ok := indexProviders(list.Providers)[providers.IDAntigravity]
	if !ok {
		t.Fatal("ListProviders() missing agy provider")
	}
	if agy.Availability != providers.AvailabilitySelectable {
		t.Fatalf("agy availability = %q, want %q", agy.Availability, providers.AvailabilitySelectable)
	}
}

func TestGetProviderBlocksOnMissingPrerequisiteProbeFacts(t *testing.T) {
	t.Parallel()

	service, err := internalservice.New(
		internalservice.WithProbeQuery(func(
			_ context.Context,
			descriptor providers.Descriptor,
		) (catalog.ProbeFacts, error) {
			if descriptor.ID != providers.IDCodex {
				return catalog.ProbeFacts{
					Readiness:     descriptor.Readiness,
					Prerequisites: descriptor.Prerequisites,
				}, nil
			}
			return catalog.ProbeFacts{
				Readiness: providers.ReadinessUnavailable,
				Prerequisites: []providers.Prerequisite{{
					Kind:        providers.PrerequisiteDependency,
					Name:        "codex",
					Status:      providers.PrerequisiteMissing,
					Description: "install codex CLI",
				}},
			}, nil
		}),
	)
	if err != nil {
		t.Fatalf("New() = %v", err)
	}

	assertGetErrorIs(t, service, providers.GetProviderRequest{ID: providers.IDCodex}, providers.ErrProviderUnavailable)

	list, err := service.ListProviders(context.Background(), providers.ListProvidersRequest{})
	if err != nil {
		t.Fatalf("ListProviders() = %v", err)
	}
	codex := indexProviders(list.Providers)[providers.IDCodex]
	if codex.Readiness != providers.ReadinessUnavailable {
		t.Fatalf("codex readiness = %q, want unavailable", codex.Readiness)
	}
	if len(codex.Prerequisites) != 1 ||
		codex.Prerequisites[0].Status != providers.PrerequisiteMissing {
		t.Fatalf("codex prerequisites = %#v, want one missing prerequisite", codex.Prerequisites)
	}
}

func TestProbeFailureSurfacesUnavailableCatalogFacts(t *testing.T) {
	t.Parallel()

	service, err := internalservice.New(
		internalservice.WithProbeQuery(func(
			_ context.Context,
			descriptor providers.Descriptor,
		) (catalog.ProbeFacts, error) {
			if descriptor.ID == providers.IDCodex {
				return catalog.ProbeFacts{}, errors.New("native probe stderr: /Users/customer/.codex/output")
			}
			return catalog.ProbeFacts{
				Readiness:     descriptor.Readiness,
				Prerequisites: descriptor.Prerequisites,
			}, nil
		}),
	)
	if err != nil {
		t.Fatalf("New() = %v", err)
	}

	list, err := service.ListProviders(context.Background(), providers.ListProvidersRequest{})
	if err != nil {
		t.Fatalf("ListProviders() = %v", err)
	}
	codex := indexProviders(list.Providers)[providers.IDCodex]
	if codex.Readiness != providers.ReadinessUnavailable {
		t.Fatalf("codex readiness = %q, want unavailable after probe failure", codex.Readiness)
	}
	if len(codex.Prerequisites) != 1 ||
		codex.Prerequisites[0].Status != providers.PrerequisiteMissing {
		t.Fatalf("codex prerequisites = %#v, want probe-class missing prerequisite", codex.Prerequisites)
	}
	if strings.Contains(codex.Prerequisites[0].Description, "/Users/") {
		t.Fatalf("probe failure description leaked native output: %q", codex.Prerequisites[0].Description)
	}

	assertGetErrorIs(t, service, providers.GetProviderRequest{ID: providers.IDCodex}, providers.ErrProviderUnavailable)
}

func assertGetErrorIs(
	t *testing.T,
	service interface {
		GetProvider(
			context.Context,
			providers.GetProviderRequest,
		) (providers.GetProviderResult, error)
	},
	request providers.GetProviderRequest,
	want error,
) {
	t.Helper()

	result, err := service.GetProvider(context.Background(), request)
	if err == nil {
		t.Fatalf("GetProvider(%#v) = %#v, want error %v", request, result, want)
	}
	if !errors.Is(err, want) {
		t.Fatalf("GetProvider(%#v) error = %v, want %v", request, err, want)
	}
	if !reflect.DeepEqual(result.Provider, providers.Descriptor{}) {
		t.Fatalf("GetProvider(%#v) provider = %#v, want zero descriptor on failure", request, result.Provider)
	}
}

func findProviderModality(values []providers.Modality, direction providers.ModalityDirection, kind providers.ModalityKind) *providers.Modality {
	for index := range values {
		if values[index].Direction == direction && values[index].Kind == kind {
			return &values[index]
		}
	}
	return nil
}

func indexProviders(descriptors []providers.Descriptor) map[providers.ID]providers.Descriptor {
	byID := make(map[providers.ID]providers.Descriptor, len(descriptors))
	for _, descriptor := range descriptors {
		byID[descriptor.ID] = descriptor
	}
	return byID
}

func providerIDs(descriptors []providers.Descriptor) []string {
	ids := make([]string, len(descriptors))
	for i, descriptor := range descriptors {
		ids[i] = descriptor.ID.String()
	}
	return ids
}
