package modelproviders_test

import (
	"bytes"
	"encoding/json"
	"fmt"
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
	wantCodexModels := []string{"gpt-5.6", "gpt-5.6-luna", "gpt-5.6-sol", "gpt-5.6-terra"}
	if len(codexModels) != len(wantCodexModels) {
		t.Fatalf("codex models = %#v, want exact IDs %v", codexModels, wantCodexModels)
	}
	for index, model := range codexModels {
		if model.Id != wantCodexModels[index] {
			t.Fatalf("codex model[%d] = %q, want %q", index, model.Id, wantCodexModels[index])
		}
		if got := effortStrings(model.Efforts); !wantStringSlice(got, []string{"minimal", "low", "medium", "high", "xhigh", "max"}) {
			t.Fatalf("codex %s efforts = %v, want minimal through max", model.Id, got)
		}
	}
	for _, model := range codexModels {
		if modality := findModality(model.Modalities, "input", "audio"); modality == nil || modality.Support != generated.ProviderModalitySupportUnsupported || modality.Transport != generated.None {
			t.Fatalf("codex %s audio input = %#v, want explicitly unsupported/none", model.Id, modality)
		}
		if modality := findModality(model.Modalities, "input", "video"); modality == nil || modality.Support != generated.ProviderModalitySupportUnsupported || modality.Transport != generated.None {
			t.Fatalf("codex %s video input = %#v, want explicitly unsupported/none", model.Id, modality)
		}
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
	for _, model := range agyModels {
		if got := effortStrings(model.Efforts); len(got) != 0 {
			t.Fatalf("AGY %s efforts = %v, want explicit empty model-encoded effort list", model.Id, got)
		}
	}
	if modality := findModality(agyModels[0].Modalities, "input", "audio"); modality == nil || modality.Support != generated.ProviderModalitySupportSupported || modality.Transport != generated.FilePath {
		t.Fatalf("AGY audio input = %#v, want supported/file_path", modality)
	}
	if modality := findModality(agyModels[0].Modalities, "input", "video"); modality == nil || modality.Support != generated.ProviderModalitySupportSupported || modality.Transport != generated.FilePath {
		t.Fatalf("AGY video input = %#v, want supported/file_path", modality)
	}
	agyLimits := derefSlice(agy.KnownLimits)
	if len(agyLimits) != 3 || agyLimits[0].Name != "add_dir_workspace" || agyLimits[1].Name != "effort_selection" || agyLimits[2].Name != "print_timeout" {
		t.Fatalf("AGY known limits = %#v, want stable name order", agyLimits)
	}

	claude, ok := byID["claude"]
	if !ok {
		t.Fatal("claude is missing from the catalog")
	}
	claudeModels := derefSlice(claude.Models)
	wantClaudeModels := []string{"claude-opus-4-6-thinking", "claude-sonnet-4-20250514", "claude-sonnet-5"}
	if len(claudeModels) != len(wantClaudeModels) {
		t.Fatalf("claude models = %#v, want exact IDs %v", claudeModels, wantClaudeModels)
	}
	for index, model := range claudeModels {
		if model.Id != wantClaudeModels[index] {
			t.Fatalf("claude model[%d] = %q, want %q", index, model.Id, wantClaudeModels[index])
		}
		if got := effortStrings(model.Efforts); !wantStringSlice(got, []string{"low", "medium", "high", "xhigh", "max"}) {
			t.Fatalf("claude %s efforts = %v, want low through max", model.Id, got)
		}
		for _, modalityName := range []string{"audio", "video"} {
			if modality := findModality(model.Modalities, "input", modalityName); modality == nil || modality.Support != generated.ProviderModalitySupportUnsupported || modality.Transport != generated.None {
				t.Fatalf("claude %s %s input = %#v, want explicitly unsupported/none", model.Id, modalityName, modality)
			}
		}
		if modality := findModality(model.Modalities, "input", "text"); modality == nil || modality.Support != generated.ProviderModalitySupportSupported || modality.Transport != generated.Inline {
			t.Fatalf("claude %s text input = %#v, want supported/inline", model.Id, modality)
		}
	}
}

func TestCatalogPublishesEvidencedNativeCapabilityFacts(t *testing.T) {
	t.Parallel()

	catalog, err := modelproviders.Catalog()
	if err != nil {
		t.Fatalf("Catalog() error = %v", err)
	}

	observedSupport := make(map[string]bool)
	for _, provider := range catalog.Providers {
		if provider.Harness == nil || provider.Harness.Kind != generated.NativeCli {
			t.Fatalf("provider %q harness = %#v, want native_cli", provider.Id, provider.Harness)
		}
		if provider.ModelCatalogPosture == nil || *provider.ModelCatalogPosture != generated.ProviderModelCatalogPostureExact {
			t.Fatalf("provider %q model catalog posture = %#v, want exact", provider.Id, provider.ModelCatalogPosture)
		}

		evidence := make(map[string]generated.ProviderCapabilityEvidence)
		for _, record := range derefSlice(provider.Evidence) {
			if _, exists := evidence[record.Id]; exists {
				t.Fatalf("provider %q has duplicate evidence ID %q", provider.Id, record.Id)
			}
			evidence[record.Id] = record
		}
		if len(evidence) == 0 {
			t.Fatalf("provider %q publishes no capability evidence", provider.Id)
		}

		knownFacts := map[string]bool{"model_catalog": true}
		routes := derefSlice(provider.HarnessRoutes)
		if len(routes) != 8 {
			t.Fatalf("provider %q harness route count = %d, want all eight direction/modality facts", provider.Id, len(routes))
		}
		for _, route := range routes {
			factRef := fmt.Sprintf("harness/%s/%s", route.Direction, route.Modality)
			knownFacts[factRef] = true
			assertCapabilityEvidence(t, provider.Id, factRef, string(route.Support), route.Condition, route.EvidenceRefs, evidence, observedSupport)
		}

		for _, model := range derefSlice(provider.Models) {
			modalities := model.Modalities
			if len(modalities) != 8 {
				t.Fatalf("provider %q model %q modality count = %d, want all eight direction/modality facts", provider.Id, model.Id, len(modalities))
			}
			for _, modality := range modalities {
				factRef := fmt.Sprintf("model/%s/%s/%s", model.Id, modality.Direction, modality.Modality)
				knownFacts[factRef] = true
				assertCapabilityEvidence(t, provider.Id, factRef, string(modality.Support), modality.Condition, modality.EvidenceRefs, evidence, observedSupport)
			}
		}

		for _, tool := range derefSlice(provider.Tools) {
			factRef := fmt.Sprintf("tool/%s", tool.Name)
			knownFacts[factRef] = true
			assertCapabilityEvidence(t, provider.Id, factRef, string(tool.Support), tool.Condition, tool.EvidenceRefs, evidence, observedSupport)
			if tool.Availability == nil {
				t.Fatalf("provider %q tool %q has no availability posture", provider.Id, tool.Name)
			}
			switch *tool.Availability {
			case generated.ProviderToolAvailabilityBuiltIn:
				if tool.DefaultEnabled == nil || !*tool.DefaultEnabled {
					t.Fatalf("provider %q built-in tool %q defaultEnabled = %#v, want true", provider.Id, tool.Name, tool.DefaultEnabled)
				}
			case generated.ProviderToolAvailabilityOptional:
				if tool.DefaultEnabled != nil {
					t.Fatalf("provider %q optional tool %q defaultEnabled = %#v, want null", provider.Id, tool.Name, tool.DefaultEnabled)
				}
			}
			for _, output := range derefSlice(tool.OutputModalities) {
				outputFactRef := fmt.Sprintf("tool/%s/output/%s", tool.Name, output.Modality)
				knownFacts[outputFactRef] = true
				assertCapabilityEvidence(t, provider.Id, outputFactRef, string(output.Support), output.Condition, output.EvidenceRefs, evidence, observedSupport)
			}
		}

		for _, record := range evidence {
			if len(derefSlice(record.FactRefs)) == 0 {
				t.Fatalf("provider %q evidence %q has no fact references", provider.Id, record.Id)
			}
			for _, factRef := range derefSlice(record.FactRefs) {
				if !knownFacts[factRef] {
					t.Fatalf("provider %q evidence %q references unknown fact %q", provider.Id, record.Id, factRef)
				}
			}
		}
	}

	for _, support := range []string{"supported", "unsupported", "conditional", "unknown"} {
		if !observedSupport[support] {
			t.Fatalf("catalog did not publish a %q native capability fact", support)
		}
	}

	byID := make(map[string]generated.ProviderManifest, len(catalog.Providers))
	for _, provider := range catalog.Providers {
		byID[provider.Id] = provider
	}

	agy := byID["antigravity"]
	if route := findModality(derefSlice(agy.HarnessRoutes), "input", "image"); route == nil || route.Support != generated.ProviderModalitySupportConditional || route.Condition == nil {
		t.Fatalf("AGY image harness route = %#v, want conditional route with a condition", route)
	}
	if route := findModality(derefSlice(agy.HarnessRoutes), "output", "image"); route == nil || route.Support != generated.ProviderModalitySupportUnknown || route.Transport != generated.None {
		t.Fatalf("AGY image output route = %#v, want unknown/none", route)
	}
	if tool := findTool(derefSlice(agy.Tools), "image_generation"); tool == nil || len(derefSlice(tool.OutputModalities)) != 1 || derefSlice(tool.OutputModalities)[0].Support != generated.ProviderCapabilitySupportSupported || derefSlice(tool.OutputModalities)[0].Transport != generated.ToolMediated {
		t.Fatalf("AGY image-generation tool output = %#v, want supported/tool_mediated", tool)
	}
	for _, model := range derefSlice(agy.Models) {
		if modality := findModality(model.Modalities, "output", "image"); modality == nil || modality.Support != generated.ProviderModalitySupportUnsupported || modality.Transport != generated.None {
			t.Fatalf("AGY %s direct image output = %#v, want explicitly unsupported/none", model.Id, modality)
		}
	}

	codex := byID["codex"]
	for _, model := range derefSlice(codex.Models) {
		if modality := findModality(model.Modalities, "output", "image"); modality == nil || modality.Support != generated.ProviderModalitySupportUnsupported || modality.Transport != generated.None {
			t.Fatalf("Codex %s direct image output = %#v, want explicitly unsupported/none", model.Id, modality)
		}
	}
}

func assertCapabilityEvidence(t *testing.T, providerID, factRef, support string, condition *string, refs *[]string, evidence map[string]generated.ProviderCapabilityEvidence, observedSupport map[string]bool) {
	t.Helper()
	observedSupport[support] = true
	if support == "conditional" && (condition == nil || *condition == "") {
		t.Fatalf("provider %q fact %q is conditional without a condition", providerID, factRef)
	}
	if support == "unknown" && len(derefSlice(refs)) != 0 {
		t.Fatalf("provider %q fact %q is unknown but has evidence references %v", providerID, factRef, derefSlice(refs))
	}
	if support != "unknown" && len(derefSlice(refs)) == 0 {
		t.Fatalf("provider %q fact %q has support %q but no evidence references", providerID, factRef, support)
	}
	for _, evidenceID := range derefSlice(refs) {
		if _, ok := evidence[evidenceID]; !ok {
			t.Fatalf("provider %q fact %q references missing evidence %q", providerID, factRef, evidenceID)
		}
	}
}

func findTool(values []generated.ProviderTool, name string) *generated.ProviderTool {
	for index := range values {
		if values[index].Name == name {
			return &values[index]
		}
	}
	return nil
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
	firstModels[0].Efforts = append(firstModels[0].Efforts, "mutated-effort")
	firstLimits := derefSlice(firstCatalog.Providers[0].KnownLimits)
	for index := range firstLimits {
		if firstLimits[index].Default != nil {
			*firstLimits[index].Default = 999
		}
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
	if thirdModels[0].Id == "mutated-model" || containsEffort(thirdModels[0].Efforts, "mutated-effort") {
		t.Fatal("mutating parsed model facts affected a later caller")
	}
	thirdLimits := derefSlice(thirdCatalog.Providers[0].KnownLimits)
	for _, limit := range thirdLimits {
		if limit.Default != nil && *limit.Default != 300 {
			t.Fatal("mutating parsed limit facts affected a later caller")
		}
	}
}

func containsEffort(values []generated.ProviderEffort, want string) bool {
	for _, value := range values {
		if string(value) == want {
			return true
		}
	}
	return false
}
