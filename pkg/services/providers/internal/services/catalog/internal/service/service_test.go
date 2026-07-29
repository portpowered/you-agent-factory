package service_test

import (
	"context"
	"errors"
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
	if len(list.Providers) != 4 {
		t.Fatalf("len(Providers) = %d, want 4", len(list.Providers))
	}

	byID := indexProviders(list.Providers)
	for _, id := range []providers.ID{
		providers.IDAntigravity,
		providers.IDClaude,
		providers.IDCodex,
		providers.IDCursor,
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
	if antigravity.Readiness != providers.ReadinessReady {
		t.Fatalf("antigravity readiness = %q, want %q", antigravity.Readiness, providers.ReadinessReady)
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

	first.Providers[0].DisplayName = "mutated"
	if len(first.Providers[0].Aliases) > 0 {
		first.Providers[0].Aliases[0] = "mutated"
	}
	if len(first.Providers[0].Capabilities) > 0 {
		first.Providers[0].Capabilities[0] = providers.CapabilityUsage
	}

	second, err := service.ListProviders(context.Background(), providers.ListProvidersRequest{})
	if err != nil {
		t.Fatalf("second ListProviders() = %v", err)
	}
	if second.Providers[0].DisplayName == "mutated" {
		t.Fatalf("second list display name = %q, want detached copy", second.Providers[0].DisplayName)
	}
	if len(second.Providers[0].Aliases) > 0 && second.Providers[0].Aliases[0] == "mutated" {
		t.Fatal("second list aliases share mutation from first result")
	}
	if len(second.Providers[0].Capabilities) > 0 &&
		second.Providers[0].Capabilities[0] == providers.CapabilityUsage &&
		first.Providers[0].Capabilities[0] == providers.CapabilityUsage {
		t.Fatal("second list capabilities share mutation from first result")
	}
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

	codex := indexProviders(list.Providers)[providers.IDCodex]
	if codex.DisplayName != "Codex" {
		t.Fatalf("codex display name = %q, want Codex", codex.DisplayName)
	}
	if codex.Availability != providers.AvailabilitySelectable {
		t.Fatalf("codex availability = %q, want selectable", codex.Availability)
	}
	if codex.Readiness != providers.ReadinessReady {
		t.Fatalf("codex readiness = %q, want ready", codex.Readiness)
	}
	if !slices.Contains(codex.Capabilities, providers.CapabilityPromptSubmission) {
		t.Fatalf("codex capabilities = %#v, want prompt_submission", codex.Capabilities)
	}

	cursor := indexProviders(list.Providers)[providers.IDCursor]
	if cursor.ID != providers.IDCursor {
		t.Fatalf("cursor id = %q, want %q", cursor.ID, providers.IDCursor)
	}
	if len(cursor.Aliases) != 0 {
		t.Fatalf("cursor aliases = %#v, want none", cursor.Aliases)
	}
	if cursor.DisplayName != "Cursor CLI" {
		t.Fatalf("cursor display name = %q, want Cursor CLI", cursor.DisplayName)
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
	if got.Provider.Readiness != providers.ReadinessReady {
		t.Fatalf("readiness = %q, want ready", got.Provider.Readiness)
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
		{name: "alias", id: "cursor", want: providers.IDCursor},
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
