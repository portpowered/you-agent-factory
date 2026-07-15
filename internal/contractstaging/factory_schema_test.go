package contractstaging_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/contractopenapiconverter"
	"github.com/portpowered/infinite-you/internal/contractstaging"
	"github.com/portpowered/infinite-you/internal/testpath"
	"gopkg.in/yaml.v3"
)

func TestFactorySchemaGenerationUsesConverterPath(t *testing.T) {
	repositoryRoot := testpath.MustRepoPathFromCaller(t, 0)
	factory, components := loadFactoryGraph(t, repositoryRoot)

	converted, diagnostics := contractopenapiconverter.ConvertFailClosedSchema(factory, components)
	if len(diagnostics) != 0 {
		t.Fatalf("ConvertFailClosedSchema() diagnostics = %#v, want none", diagnostics)
	}
	if converted == nil {
		t.Fatal("ConvertFailClosedSchema() = nil, want converted schema")
	}

	artifacts, err := contractstaging.Artifacts(repositoryRoot)
	if err != nil {
		t.Fatalf("Artifacts() error = %v", err)
	}
	payload := artifacts[contractstaging.FactorySchemaAuthoredPath]
	var document map[string]any
	if err := json.Unmarshal(payload, &document); err != nil {
		t.Fatalf("decode authored factory schema: %v", err)
	}
	for _, key := range []string{"$schema", "$id", "title"} {
		if document[key] == nil || document[key] == "" {
			t.Fatalf("authored factory schema missing %s metadata", key)
		}
	}
	if document["$schema"] != "https://json-schema.org/draft/2020-12/schema" {
		t.Fatalf("$schema = %#v, want Draft 2020-12", document["$schema"])
	}
	if document["$id"] != "https://schemas.portpowered.com/you/config/factory.schema.json" {
		t.Fatalf("$id = %#v, want factory config schema id", document["$id"])
	}
	if document["title"] != "You Factory configuration" {
		t.Fatalf("title = %#v, want You Factory configuration", document["title"])
	}
	if containsNullableKeyword(document) {
		t.Fatal("authored factory schema still contains legacy nullable keyword; want converter type unions")
	}
	if !containsTypeNullUnion(document) {
		t.Fatal("authored factory schema missing converter-backed nullable type unions")
	}
	if !bytes.Equal(payload, artifacts["packages/api/generated/schemas/factory.schema.json"]) {
		t.Fatal("authored and staged factory schema bytes differ")
	}
}

func TestFactorySchemaPreservesOrchestratorTopology(t *testing.T) {
	repositoryRoot := testpath.MustRepoPathFromCaller(t, 0)
	artifacts, err := contractstaging.Artifacts(repositoryRoot)
	if err != nil {
		t.Fatalf("Artifacts() error = %v", err)
	}
	payload := artifacts[contractstaging.FactorySchemaAuthoredPath]
	text := string(payload)
	for _, needle := range []string{
		"FactoryOrchestratorJavaScriptConfig",
		"argsSchema",
		"FactoryOrchestratorJavaScriptInlineSource",
		`"orchestrator"`,
		"#/$defs/",
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("authored factory schema missing topology marker %q", needle)
		}
	}
}

func containsNullableKeyword(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		if _, ok := typed["nullable"]; ok {
			return true
		}
		for _, child := range typed {
			if containsNullableKeyword(child) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if containsNullableKeyword(child) {
				return true
			}
		}
	}
	return false
}

func containsTypeNullUnion(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		if typeValue, ok := typed["type"]; ok {
			switch array := typeValue.(type) {
			case []any:
				for _, item := range array {
					if item == "null" {
						return true
					}
				}
			}
		}
		for _, child := range typed {
			if containsTypeNullUnion(child) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if containsTypeNullUnion(child) {
				return true
			}
		}
	}
	return false
}

func TestFactorySchemaConverterHasNoUnsupportedReferenceDiagnostics(t *testing.T) {
	repositoryRoot := testpath.MustRepoPathFromCaller(t, 0)
	factory, components := loadFactoryGraph(t, repositoryRoot)

	collected := contractstaging.CollectFactorySchemaConverterDiagnosticsForTest(factory, components)
	for _, diagnostic := range collected {
		if diagnostic.Code == "openapi.convert.unsupported_reference" {
			t.Fatalf("unsupported_reference diagnostic remains: %#v", diagnostic)
		}
	}
}

func TestFactorySchemaB16GapRecordCoversCanonicalFactoryGraph(t *testing.T) {
	repositoryRoot := testpath.MustRepoPathFromCaller(t, 0)
	factory, components := loadFactoryGraph(t, repositoryRoot)

	collected := contractstaging.CollectFactorySchemaConverterDiagnosticsForTest(factory, components)

	recordPath := filepath.Join(repositoryRoot, "docs", "internal", "contract", "factory-schema-b16-gaps.json")
	recordPayload, err := os.ReadFile(recordPath)
	if err != nil {
		t.Fatalf("read gap record: %v", err)
	}
	var record struct {
		BlockingCategories []struct {
			Code          string `json:"code"`
			InstanceCount int    `json:"instanceCount"`
		} `json:"blockingCategories"`
	}
	if err := json.Unmarshal(recordPayload, &record); err != nil {
		t.Fatalf("decode gap record: %v", err)
	}

	counts := map[string]int{}
	for _, diagnostic := range collected {
		matches, err := contractstaging.FactorySchemaDiagnosticMatchesGapRecordForTest(repositoryRoot, diagnostic)
		if err != nil {
			t.Fatalf("FactorySchemaDiagnosticMatchesGapRecordForTest() error = %v", err)
		}
		if !matches {
			t.Fatalf("diagnostic %#v is not covered by the B16 gap record", diagnostic)
		}
		counts[diagnostic.Code]++
	}

	for _, category := range record.BlockingCategories {
		if counts[category.Code] != category.InstanceCount {
			t.Fatalf("gap record %s instanceCount = %d, collected %d", category.Code, category.InstanceCount, counts[category.Code])
		}
	}
	if len(collected) == 0 {
		return
	}

	factoryCopy := contractstaging.DeepCopyValueForTest(factory).(map[string]any)
	componentsCopy := contractstaging.DeepCopyValueForTest(components).(map[string]any)
	for _, diagnostic := range collected {
		if !contractstaging.StripFactorySchemaConverterGapForTest(factoryCopy, componentsCopy, diagnostic) {
			t.Fatalf("could not strip diagnostic %#v from copied Factory graph", diagnostic)
		}
	}
	_, diagnostics := contractopenapiconverter.ConvertFailClosedSchema(factoryCopy, componentsCopy)
	if len(diagnostics) != 0 {
		t.Fatalf("ConvertFailClosedSchema() after stripping recorded gaps = %#v, want success", diagnostics)
	}
}

func loadFactoryGraph(t *testing.T, repositoryRoot string) (map[string]any, map[string]any) {
	t.Helper()
	path := filepath.Join(repositoryRoot, "api", "openapi.yaml")
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read openapi: %v", err)
	}
	var document map[string]any
	if err := yaml.Unmarshal(payload, &document); err != nil {
		t.Fatalf("decode openapi: %v", err)
	}
	componentsValue, ok := document["components"].(map[string]any)
	if !ok {
		t.Fatal("missing components")
	}
	components, ok := componentsValue["schemas"].(map[string]any)
	if !ok {
		t.Fatal("missing components.schemas")
	}
	factory, ok := components["Factory"].(map[string]any)
	if !ok {
		t.Fatal("missing Factory schema")
	}
	return factory, components
}
