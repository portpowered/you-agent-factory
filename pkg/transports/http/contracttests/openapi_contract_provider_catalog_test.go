package apicontract_test

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestProviderCatalogContract_RepresentativeFixtureValidatesAndRoundTrips(t *testing.T) {
	t.Parallel()

	fixture := loadProviderCatalogFixture(t)
	doc := loadValidatedOpenAPIContract(t)
	if err := doc.Components.Schemas["ProviderCatalog"].Value.VisitJSON(fixture); err != nil {
		t.Fatalf("validate representative ProviderCatalog fixture: %v", err)
	}

	data, err := json.Marshal(fixture)
	if err != nil {
		t.Fatalf("marshal representative fixture: %v", err)
	}
	var catalog factoryapi.ProviderCatalog
	if err := json.Unmarshal(data, &catalog); err != nil {
		t.Fatalf("unmarshal generated ProviderCatalog: %v", err)
	}
	if catalog.FormatVersion != factoryapi.ProviderCatalogFormatVersionV1 {
		t.Fatalf("format version = %q, want 1.0.0", catalog.FormatVersion)
	}
	if len(catalog.Providers) != 2 {
		t.Fatalf("providers = %d, want 2", len(catalog.Providers))
	}
	alpha := catalog.Providers[0]
	if alpha.Id != "example-alpha" ||
		alpha.TechnicalSupportLevel != factoryapi.ProviderTechnicalSupportLevelExperimental ||
		alpha.ImplementationAvailability != factoryapi.ProviderImplementationAvailabilityExternallySupplied {
		t.Fatalf("generated provider metadata changed meaning: %#v", alpha)
	}
	if alpha.Deprecation == nil || alpha.Deprecation.ReplacementProviderId == nil ||
		*alpha.Deprecation.ReplacementProviderId != "example-next" {
		t.Fatalf("generated deprecation metadata changed meaning: %#v", alpha.Deprecation)
	}

	roundTrip, err := json.Marshal(catalog)
	if err != nil {
		t.Fatalf("marshal generated ProviderCatalog: %v", err)
	}
	var roundTripValue any
	if err := json.Unmarshal(roundTrip, &roundTripValue); err != nil {
		t.Fatalf("decode generated ProviderCatalog JSON: %v", err)
	}
	if !reflect.DeepEqual(roundTripValue, fixture) {
		t.Fatalf("generated Go round trip changed the public fixture\n got: %#v\nwant: %#v", roundTripValue, fixture)
	}
}

func TestProviderManifestContract_RejectsOperationalDiscoveryData(t *testing.T) {
	t.Parallel()

	doc := loadValidatedOpenAPIContract(t)
	schema := doc.Components.Schemas["ProviderManifest"].Value
	for _, field := range []string{"readiness", "credentialValue", "environmentValue", "endpointAddress", "machinePath", "pricing"} {
		field := field
		t.Run(field, func(t *testing.T) {
			t.Parallel()
			manifest := providerManifestFromFixture(t)
			manifest["discovery"].(map[string]any)[field] = "not-public-contract-data"
			if err := schema.VisitJSON(manifest); err == nil {
				t.Fatalf("ProviderManifest accepted forbidden discovery field %q", field)
			}
		})
	}
}

func TestProviderManifestContract_RequiresCoherentDeprecationShape(t *testing.T) {
	t.Parallel()

	doc := loadValidatedOpenAPIContract(t)
	schema := doc.Components.Schemas["ProviderManifest"].Value
	manifest := providerManifestFromFixture(t)
	manifest["deprecation"] = map[string]any{
		"replacementProviderId": "example-next",
	}
	if err := schema.VisitJSON(manifest); err == nil {
		t.Fatal("ProviderManifest accepted deprecation metadata without deprecatedSince and reason")
	}
}

func TestProviderCatalogContract_UsesClosedPublicationVocabulary(t *testing.T) {
	t.Parallel()

	doc := loadValidatedOpenAPIContract(t)
	assertSchemaEnum := func(schemaName string, want []any) {
		t.Helper()
		got := doc.Components.Schemas[schemaName].Value.Enum
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s enum = %#v, want %#v", schemaName, got, want)
		}
	}
	assertSchemaEnum("ProviderTechnicalSupportLevel", []any{"production", "experimental", "not-supported"})
	assertSchemaEnum("ProviderImplementationAvailability", []any{"bundled", "externally-supplied", "catalog-only"})
}

func loadProviderCatalogFixture(t *testing.T) map[string]any {
	t.Helper()

	data, err := os.ReadFile("../../../../api/testdata/provider-catalog-contract.json")
	if err != nil {
		t.Fatalf("read representative ProviderCatalog fixture: %v", err)
	}
	var fixture map[string]any
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatalf("decode representative ProviderCatalog fixture: %v", err)
	}
	return fixture
}

func providerManifestFromFixture(t *testing.T) map[string]any {
	t.Helper()

	fixture := loadProviderCatalogFixture(t)
	providers, ok := fixture["providers"].([]any)
	if !ok || len(providers) == 0 {
		t.Fatal("representative ProviderCatalog fixture has no providers")
	}
	manifest, ok := providers[0].(map[string]any)
	if !ok {
		t.Fatal("representative ProviderManifest fixture is not an object")
	}
	return manifest
}
