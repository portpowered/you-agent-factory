package service_test

import (
	"context"
	"reflect"
	"slices"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
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
	if len(list.Providers) != 8 {
		t.Fatalf("len(Providers) = %d, want 8", len(list.Providers))
	}

	byID := indexProviders(list.Providers)
	for _, id := range []providers.ID{
		providers.IDAgy,
		providers.IDClaude,
		providers.IDCodex,
		providers.IDCursor,
		providers.IDGemini,
		providers.IDKiro,
		providers.IDOpenCode,
		providers.IDPi,
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

func TestListProvidersIncludesNonSelectableEntries(t *testing.T) {
	t.Parallel()

	service, err := internalservice.New()
	if err != nil {
		t.Fatalf("New() = %v", err)
	}

	list, err := service.ListProviders(context.Background(), providers.ListProvidersRequest{})
	if err != nil {
		t.Fatalf("ListProviders() = %v", err)
	}

	agy := indexProviders(list.Providers)[providers.IDAgy]
	if agy.Availability != providers.AvailabilityNotSupported {
		t.Fatalf("agy availability = %q, want %q", agy.Availability, providers.AvailabilityNotSupported)
	}
	if agy.Readiness != providers.ReadinessUnavailable {
		t.Fatalf("agy readiness = %q, want %q", agy.Readiness, providers.ReadinessUnavailable)
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
	if !slices.Contains(cursor.Aliases, "cursor") {
		t.Fatalf("cursor aliases = %#v, want cursor manifest id alias", cursor.Aliases)
	}
	if cursor.DisplayName != "Cursor CLI" {
		t.Fatalf("cursor display name = %q, want Cursor CLI", cursor.DisplayName)
	}

	kiro := indexProviders(list.Providers)[providers.IDKiro]
	if !slices.Contains(kiro.Aliases, "kiro") {
		t.Fatalf("kiro aliases = %#v, want kiro manifest id alias", kiro.Aliases)
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

func TestGetProviderResolvesAcceptedAliases(t *testing.T) {
	t.Parallel()

	service, err := internalservice.New()
	if err != nil {
		t.Fatalf("New() = %v", err)
	}

	cases := []struct {
		name      string
		requestID providers.ID
		wantID    providers.ID
		wantName  string
		wantAlias string
	}{
		{
			name:      "cursor manifest id alias",
			requestID: providers.ID("cursor"),
			wantID:    providers.IDCursor,
			wantName:  "Cursor CLI",
			wantAlias: "cursor",
		},
		{
			name:      "kiro manifest id alias",
			requestID: providers.ID("kiro"),
			wantID:    providers.IDKiro,
			wantName:  "Kiro CLI",
			wantAlias: "kiro",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			byAlias, err := service.GetProvider(context.Background(), providers.GetProviderRequest{ID: testCase.requestID})
			if err != nil {
				t.Fatalf("GetProvider(%q) = %v", testCase.requestID, err)
			}
			byCanonical, err := service.GetProvider(context.Background(), providers.GetProviderRequest{ID: testCase.wantID})
			if err != nil {
				t.Fatalf("GetProvider(%q) = %v", testCase.wantID, err)
			}

			if byAlias.Provider.ID != testCase.wantID {
				t.Fatalf("alias lookup id = %q, want %q", byAlias.Provider.ID, testCase.wantID)
			}
			if byAlias.Provider.DisplayName != testCase.wantName {
				t.Fatalf("alias lookup display name = %q, want %q", byAlias.Provider.DisplayName, testCase.wantName)
			}
			if !slices.Contains(byAlias.Provider.Aliases, testCase.wantAlias) {
				t.Fatalf("alias lookup aliases = %#v, want %q", byAlias.Provider.Aliases, testCase.wantAlias)
			}
			if !reflect.DeepEqual(byAlias.Provider, byCanonical.Provider) {
				t.Fatalf("alias lookup = %#v, want canonical lookup %#v", byAlias.Provider, byCanonical.Provider)
			}
		})
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
