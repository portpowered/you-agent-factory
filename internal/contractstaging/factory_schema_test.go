package contractstaging_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/internal/contractopenapiconverter"
	"github.com/portpowered/infinite-you/internal/contractstaging"
	"github.com/portpowered/infinite-you/internal/testpath"
	"gopkg.in/yaml.v3"
)

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
