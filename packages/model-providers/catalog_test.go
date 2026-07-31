package modelproviders_test

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	modelproviders "github.com/portpowered/infinite-you/packages/model-providers"
	generated "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestPublishedBytesMatchGeneratedArtifacts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
		got  []byte
	}{
		{name: "catalog", path: "generated/catalog.json", got: modelproviders.CatalogJSON()},
		{name: "manifest schema", path: "generated/provider-manifest.schema.json", got: modelproviders.ProviderManifestSchemaJSON()},
		{name: "catalog schema", path: "generated/provider-catalog.schema.json", got: modelproviders.ProviderCatalogSchemaJSON()},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			want, err := os.ReadFile(test.path)
			if err != nil {
				t.Fatalf("read generated artifact: %v", err)
			}
			if !bytes.Equal(test.got, want) {
				t.Fatalf("embedded bytes differ from %s", test.path)
			}
			if !json.Valid(test.got) {
				t.Fatalf("%s is not valid JSON", test.path)
			}
		})
	}
}

func TestCatalogReturnsGeneratedContractProjection(t *testing.T) {
	t.Parallel()

	catalog, err := modelproviders.Catalog()
	if err != nil {
		t.Fatalf("Catalog() error = %v", err)
	}
	if catalog.FormatVersion != generated.ProviderCatalogFormatVersionV1 {
		t.Fatalf("FormatVersion = %q, want %q", catalog.FormatVersion, generated.ProviderCatalogFormatVersionV1)
	}
	if len(catalog.Providers) != 3 {
		t.Fatalf("provider count = %d, want 3", len(catalog.Providers))
	}
	if catalog.Providers[0].Id != "antigravity" || catalog.Providers[len(catalog.Providers)-1].Id != "codex" {
		t.Fatalf("providers are not in canonical ID order: first = %q, last = %q", catalog.Providers[0].Id, catalog.Providers[len(catalog.Providers)-1].Id)
	}
}

func TestPublishedValuesAreDetachedAcrossCallers(t *testing.T) {
	t.Parallel()

	byteAccessors := []struct {
		name string
		read func() []byte
	}{
		{name: "catalog", read: modelproviders.CatalogJSON},
		{name: "manifest schema", read: modelproviders.ProviderManifestSchemaJSON},
		{name: "catalog schema", read: modelproviders.ProviderCatalogSchemaJSON},
	}
	for _, accessor := range byteAccessors {
		firstBytes := accessor.read()
		secondBytes := accessor.read()
		firstBytes[0] ^= 0xff
		if !bytes.Equal(secondBytes, accessor.read()) {
			t.Fatalf("mutating returned %s bytes affected a later caller", accessor.name)
		}
	}

	firstCatalog, err := modelproviders.Catalog()
	if err != nil {
		t.Fatalf("first Catalog() error = %v", err)
	}
	secondCatalog, err := modelproviders.Catalog()
	if err != nil {
		t.Fatalf("second Catalog() error = %v", err)
	}
	firstCatalog.Providers[0].Id = "mutated"
	firstCatalog.Providers[0].Aliases = append(firstCatalog.Providers[0].Aliases, "mutated")

	thirdCatalog, err := modelproviders.Catalog()
	if err != nil {
		t.Fatalf("third Catalog() error = %v", err)
	}
	if secondCatalog.Providers[0].Id != "antigravity" || thirdCatalog.Providers[0].Id != "antigravity" {
		t.Fatal("mutating one parsed catalog affected another caller")
	}
	if len(thirdCatalog.Providers[0].Aliases) != 0 {
		t.Fatal("mutating parsed aliases affected a later caller")
	}
}
