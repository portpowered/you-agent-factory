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

func TestCatalogPublishesCanonicalCapabilityFacts(t *testing.T) {
	t.Parallel()

	catalog, err := modelproviders.Catalog()
	if err != nil {
		t.Fatalf("Catalog() error = %v", err)
	}
	byID := make(map[string]generated.ProviderManifest, len(catalog.Providers))
	for _, provider := range catalog.Providers {
		byID[provider.Id] = provider
		if len(derefSlice(provider.Models)) == 0 || len(derefSlice(provider.Tools)) == 0 {
			t.Fatalf("provider %q does not publish model and tool facts", provider.Id)
		}
		if len(derefSlice(provider.Discovery.Prerequisites)) != 3 {
			t.Fatalf("provider %q prerequisite count = %d, want 3", provider.Id, len(derefSlice(provider.Discovery.Prerequisites)))
		}
	}

	codex, ok := byID["codex"]
	if !ok {
		t.Fatal("codex is missing from the catalog")
	}
	codexModels := derefSlice(codex.Models)
	if len(codexModels) != 1 || codexModels[0].Id != "gpt-5.6" {
		t.Fatalf("codex models = %#v, want only gpt-5.6", codexModels)
	}
	if got := effortStrings(codexModels[0].Efforts); !wantStringSlice(got, []string{"minimal", "low", "medium", "high", "xhigh", "max"}) {
		t.Fatalf("codex efforts = %v, want minimal through max; comparison = %t", got, wantStringSlice(got, []string{"minimal", "low", "medium", "high", "xhigh", "max"}))
	}
	if modality := findModality(codexModels[0].Modalities, "input", "audio"); modality == nil || modality.Support != generated.ProviderModalitySupportUnsupported || modality.Transport != generated.None {
		t.Fatalf("codex audio input = %#v, want explicitly unsupported/none", modality)
	}
	if modality := findModality(codexModels[0].Modalities, "input", "video"); modality == nil || modality.Support != generated.ProviderModalitySupportUnsupported || modality.Transport != generated.None {
		t.Fatalf("codex video input = %#v, want explicitly unsupported/none", modality)
	}
	codexLimits := derefSlice(codex.KnownLimits)
	if len(codexLimits) != 1 || codexLimits[0].Name != "referenced_image_paths" || codexLimits[0].Maximum == nil || *codexLimits[0].Maximum != 5 {
		t.Fatalf("codex image-path limit = %#v, want maximum 5", codexLimits)
	}

	agy, ok := byID["antigravity"]
	if !ok {
		t.Fatal("antigravity is missing from the catalog")
	}
	agyModels := derefSlice(agy.Models)
	if got := effortStrings(agyModels[0].Efforts); !wantStringSlice(got, []string{"low", "medium", "high"}) {
		t.Fatalf("AGY efforts = %v, want low, medium, high", got)
	}
	if modality := findModality(agyModels[0].Modalities, "input", "audio"); modality == nil || modality.Support != generated.ProviderModalitySupportSupported || modality.Transport != generated.FilePath {
		t.Fatalf("AGY audio input = %#v, want supported/file_path", modality)
	}
	if modality := findModality(agyModels[0].Modalities, "input", "video"); modality == nil || modality.Support != generated.ProviderModalitySupportSupported || modality.Transport != generated.FilePath {
		t.Fatalf("AGY video input = %#v, want supported/file_path", modality)
	}
	agyLimits := derefSlice(agy.KnownLimits)
	if len(agyLimits) != 2 || agyLimits[0].Name != "add_dir_workspace" || agyLimits[1].Name != "print_timeout" {
		t.Fatalf("AGY known limits = %#v, want stable name order", agyLimits)
	}
}

func derefSlice[T any](value *[]T) []T {
	if value == nil {
		return nil
	}
	return *value
}

func effortStrings(values []generated.ProviderEffort) []string {
	result := make([]string, len(values))
	for index, value := range values {
		result[index] = string(value)
	}
	return result
}

func wantStringSlice(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range got {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}

func findModality(values []generated.ProviderModality, direction, kind string) *generated.ProviderModality {
	for index := range values {
		if string(values[index].Direction) == direction && string(values[index].Modality) == kind {
			return &values[index]
		}
	}
	return nil
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
	firstModels := derefSlice(firstCatalog.Providers[0].Models)
	firstModels[0].Id = "mutated-model"
	firstModels[0].Efforts[0] = "mutated-effort"
	firstLimits := derefSlice(firstCatalog.Providers[0].KnownLimits)
	if firstLimits[1].Default != nil {
		*firstLimits[1].Default = 999
	}

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
	thirdModels := derefSlice(thirdCatalog.Providers[0].Models)
	if thirdModels[0].Id == "mutated-model" || thirdModels[0].Efforts[0] == "mutated-effort" {
		t.Fatal("mutating parsed model facts affected a later caller")
	}
	thirdLimits := derefSlice(thirdCatalog.Providers[0].KnownLimits)
	if thirdLimits[1].Default == nil || *thirdLimits[1].Default != 300 {
		t.Fatal("mutating parsed limit facts affected a later caller")
	}
}
