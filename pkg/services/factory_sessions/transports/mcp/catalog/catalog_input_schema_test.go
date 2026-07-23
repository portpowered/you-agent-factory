package catalog_test

import (
	"encoding/json"
	"strings"
	"testing"

	mcpfactorysession "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/mcp"
	mcpfactorycatalog "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/mcp/catalog"
)

func TestVerifyCatalogInputSchemaParity_PassesForDiscoverToolsProjection(t *testing.T) {
	discovered := mcpfactorysession.DiscoverTools()
	catalog := make([]mcpfactorycatalog.CatalogInputSchema, 0, len(discovered))
	for _, tool := range discovered {
		catalog = append(catalog, mcpfactorycatalog.CatalogInputSchema{
			Name:   tool.Name,
			Schema: tool.InputSchema,
		})
	}
	if err := mcpfactorycatalog.VerifyCatalogInputSchemaParity(catalog, discovered); err != nil {
		t.Fatalf("VerifyCatalogInputSchemaParity() error = %v", err)
	}
}

func TestVerifyCatalogInputSchemaParity_FailsWhenRequiredFieldMissing(t *testing.T) {
	tool, ok := mcpfactorysession.ToolByName(mcpfactorysession.ToolGetSession)
	if !ok {
		t.Fatal("get tool missing from discovery")
	}
	schema := cloneSchemaMap(t, tool.InputSchema)
	delete(schema, "required")
	catalog := []mcpfactorycatalog.CatalogInputSchema{{
		Name:   tool.Name,
		Schema: schema,
	}}
	err := mcpfactorycatalog.VerifyCatalogInputSchemaParity(catalog, []mcpfactorysession.ToolDefinition{tool})
	if err == nil {
		t.Fatal("VerifyCatalogInputSchemaParity() error = nil, want required-field mismatch")
	}
	if got := err.Error(); !strings.Contains(got, tool.Name) || !strings.Contains(got, "differs from DiscoverTools semantics") {
		t.Fatalf("VerifyCatalogInputSchemaParity() error = %v, want parity failure for %q", err, tool.Name)
	}
}

func TestVerifyCatalogInputSchemaParity_FailsWhenNestedObjectNotClosed(t *testing.T) {
	tool, ok := mcpfactorysession.ToolByName(mcpfactorysession.ToolStartSync)
	if !ok {
		t.Fatal("start_sync tool missing from discovery")
	}
	schema := cloneSchemaMap(t, tool.InputSchema)
	properties := schema["properties"].(map[string]any)
	source := properties["source"].(map[string]any)
	source["additionalProperties"] = true
	catalog := []mcpfactorycatalog.CatalogInputSchema{{
		Name:   tool.Name,
		Schema: schema,
	}}
	err := mcpfactorycatalog.VerifyCatalogInputSchemaParity(catalog, []mcpfactorysession.ToolDefinition{tool})
	if err == nil {
		t.Fatal("VerifyCatalogInputSchemaParity() error = nil, want nested closing mismatch")
	}
	if got := err.Error(); !strings.Contains(got, tool.Name) || !strings.Contains(got, "differs from DiscoverTools semantics") {
		t.Fatalf("VerifyCatalogInputSchemaParity() error = %v, want nested closing failure", err)
	}
}

func TestVerifyCatalogInputSchemaParity_DoesNotMutateDiscoverySchemas(t *testing.T) {
	before := cloneToolDefinitions(t, mcpfactorysession.DiscoverTools())
	catalog := make([]mcpfactorycatalog.CatalogInputSchema, 0, len(before))
	for _, tool := range before {
		catalog = append(catalog, mcpfactorycatalog.CatalogInputSchema{
			Name:   tool.Name,
			Schema: tool.InputSchema,
		})
	}
	if err := mcpfactorycatalog.VerifyCatalogInputSchemaParity(catalog, before); err != nil {
		t.Fatalf("VerifyCatalogInputSchemaParity() error = %v", err)
	}
	after := mcpfactorysession.DiscoverTools()
	if len(before) != len(after) {
		t.Fatalf("discover tool count changed: before=%d after=%d", len(before), len(after))
	}
	for i := range before {
		beforeJSON, err := json.Marshal(before[i].InputSchema)
		if err != nil {
			t.Fatalf("marshal before schema: %v", err)
		}
		afterJSON, err := json.Marshal(after[i].InputSchema)
		if err != nil {
			t.Fatalf("marshal after schema: %v", err)
		}
		if string(beforeJSON) != string(afterJSON) {
			t.Fatalf("tool %q input schema mutated by parity check", before[i].Name)
		}
	}
}

func cloneSchemaMap(t *testing.T, schema map[string]any) map[string]any {
	t.Helper()
	encoded, err := json.Marshal(schema)
	if err != nil {
		t.Fatalf("marshal schema: %v", err)
	}
	var cloned map[string]any
	if err := json.Unmarshal(encoded, &cloned); err != nil {
		t.Fatalf("unmarshal schema: %v", err)
	}
	return cloned
}

func cloneToolDefinitions(t *testing.T, tools []mcpfactorysession.ToolDefinition) []mcpfactorysession.ToolDefinition {
	t.Helper()
	encoded, err := json.Marshal(tools)
	if err != nil {
		t.Fatalf("marshal tools: %v", err)
	}
	var cloned []mcpfactorysession.ToolDefinition
	if err := json.Unmarshal(encoded, &cloned); err != nil {
		t.Fatalf("unmarshal tools: %v", err)
	}
	return cloned
}
