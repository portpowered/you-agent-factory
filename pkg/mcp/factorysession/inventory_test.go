package factorysession_test

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	mcpfactorysession "github.com/portpowered/infinite-you/pkg/mcp/factorysession"
)

func TestProjectToolInventory_BuildsDocumentShape(t *testing.T) {
	inventory, err := mcpfactorysession.ProjectToolInventory()
	if err != nil {
		t.Fatalf("ProjectToolInventory() error = %v", err)
	}
	if inventory.FormatVersion != mcpfactorysession.ToolInventoryFormatVersion {
		t.Fatalf("formatVersion = %q, want %q", inventory.FormatVersion, mcpfactorysession.ToolInventoryFormatVersion)
	}
	if inventory.ProtocolVersion != mcpfactorysession.ToolInventoryProtocolVersion {
		t.Fatalf("protocolVersion = %q, want %q", inventory.ProtocolVersion, mcpfactorysession.ToolInventoryProtocolVersion)
	}
	if len(inventory.Tools) != len(mcpfactorysession.DiscoverTools()) {
		t.Fatalf("tool count = %d, want %d", len(inventory.Tools), len(mcpfactorysession.DiscoverTools()))
	}
}

func TestProjectToolInventory_ToolsSortedByCanonicalName(t *testing.T) {
	inventory, err := mcpfactorysession.ProjectToolInventory()
	if err != nil {
		t.Fatalf("ProjectToolInventory() error = %v", err)
	}
	names := make([]string, len(inventory.Tools))
	for i, tool := range inventory.Tools {
		names[i] = tool.Name
	}
	sorted := slices.Clone(names)
	slices.Sort(sorted)
	if !slices.Equal(names, sorted) {
		t.Fatalf("tool names = %#v, want sorted %#v", names, sorted)
	}
}

func TestProjectToolInventory_EachToolHasIdentityFields(t *testing.T) {
	inventory, err := mcpfactorysession.ProjectToolInventory()
	if err != nil {
		t.Fatalf("ProjectToolInventory() error = %v", err)
	}
	for _, tool := range inventory.Tools {
		if strings.TrimSpace(tool.IDCandidate) == "" {
			t.Fatalf("tool %q missing idCandidate", tool.Name)
		}
		if strings.TrimSpace(tool.Name) == "" {
			t.Fatal("tool name is required")
		}
		if strings.TrimSpace(tool.Description) == "" {
			t.Fatalf("tool %q description is required", tool.Name)
		}
		if len(tool.InputSchema) == 0 {
			t.Fatalf("tool %q input schema is required", tool.Name)
		}
		if schemaType, _ := tool.InputSchema["type"].(string); schemaType != "object" {
			t.Fatalf("tool %q input schema type = %q, want object", tool.Name, schemaType)
		}
	}
}

func TestProjectToolInventory_DerivesStableIDCandidates(t *testing.T) {
	inventory, err := mcpfactorysession.ProjectToolInventory()
	if err != nil {
		t.Fatalf("ProjectToolInventory() error = %v", err)
	}
	byName := inventoryToolsByName(t, inventory)
	if got := byName[mcpfactorysession.ToolGetSession].IDCandidate; got != "factory-session.get" {
		t.Fatalf("you.factory_session.get idCandidate = %q, want factory-session.get", got)
	}
	if got := byName[mcpfactorysession.ToolValidateSource].IDCandidate; got != "factory-session.validate-source" {
		t.Fatalf("you.factory_session.validate_source idCandidate = %q, want factory-session.validate-source", got)
	}
}

func TestProjectToolInventory_ExcludesCompatibilityAliases(t *testing.T) {
	inventory, err := mcpfactorysession.ProjectToolInventory()
	if err != nil {
		t.Fatalf("ProjectToolInventory() error = %v", err)
	}
	aliasNames := make(map[string]struct{}, len(mcpfactorysession.DiscoverCompatibilityAliases()))
	for _, alias := range mcpfactorysession.DiscoverCompatibilityAliases() {
		aliasNames[alias.Name] = struct{}{}
	}
	for _, tool := range inventory.Tools {
		if _, ok := aliasNames[tool.Name]; ok {
			t.Fatalf("compatibility alias %q must not appear in canonical inventory", tool.Name)
		}
	}
}

func TestProjectToolInventory_DoesNotAdvertiseResultContracts(t *testing.T) {
	encoded, err := mcpfactorysession.MarshalToolInventoryJSON(mustProjectToolInventory(t))
	if err != nil {
		t.Fatalf("MarshalToolInventoryJSON() error = %v", err)
	}
	payload := string(encoded)
	for _, forbidden := range []string{
		"outputSchema",
		"structuredContent",
		"successStableFields",
		"errorStableFields",
		"\"image\"",
		"\"audio\"",
		"\"resources\"",
		"\"task\"",
	} {
		if strings.Contains(payload, forbidden) {
			t.Fatalf("inventory advertises unsupported field %q", forbidden)
		}
	}
}

func TestProjectToolInventory_CanonicalizesNestedInputSchemaKeys(t *testing.T) {
	inventory, err := mcpfactorysession.ProjectToolInventory()
	if err != nil {
		t.Fatalf("ProjectToolInventory() error = %v", err)
	}
	byName := inventoryToolsByName(t, inventory)
	startSync := byName[mcpfactorysession.ToolStartSync]
	properties, ok := startSync.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatal("start_sync input schema properties missing")
	}
	keys := schemaObjectKeys(properties)
	if !slices.IsSorted(keys) {
		t.Fatalf("start_sync properties keys = %#v, want sorted", keys)
	}
}

func TestProjectToolInventory_DoesNotMutateDiscoverySchemas(t *testing.T) {
	before := cloneToolDefinitions(t, mcpfactorysession.DiscoverTools())
	if _, err := mcpfactorysession.ProjectToolInventory(); err != nil {
		t.Fatalf("ProjectToolInventory() error = %v", err)
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
			t.Fatalf("tool %q input schema mutated by inventory projection", before[i].Name)
		}
	}
}

func TestProjectToolInventory_RepeatExtractionIsByteIdentical(t *testing.T) {
	first, err := mcpfactorysession.MarshalToolInventoryJSON(mustProjectToolInventory(t))
	if err != nil {
		t.Fatalf("first MarshalToolInventoryJSON() error = %v", err)
	}
	second, err := mcpfactorysession.MarshalToolInventoryJSON(mustProjectToolInventory(t))
	if err != nil {
		t.Fatalf("second MarshalToolInventoryJSON() error = %v", err)
	}
	if string(first) != string(second) {
		t.Fatalf("repeat extraction differs:\nfirst=%s\nsecond=%s", first, second)
	}
}

func TestProjectToolInventory_MatchesDiscoverToolsIdentityFields(t *testing.T) {
	inventory, err := mcpfactorysession.ProjectToolInventory()
	if err != nil {
		t.Fatalf("ProjectToolInventory() error = %v", err)
	}
	byName := inventoryToolsByName(t, inventory)
	for _, discovered := range mcpfactorysession.DiscoverTools() {
		entry, ok := byName[discovered.Name]
		if !ok {
			t.Fatalf("inventory missing discovered tool %q", discovered.Name)
		}
		if entry.Description != discovered.Description {
			t.Fatalf("tool %q description = %q, want %q", discovered.Name, entry.Description, discovered.Description)
		}
		canonicalSchema, err := json.Marshal(entry.InputSchema)
		if err != nil {
			t.Fatalf("marshal inventory schema for %q: %v", discovered.Name, err)
		}
		sourceSchema, err := json.Marshal(discovered.InputSchema)
		if err != nil {
			t.Fatalf("marshal discovered schema for %q: %v", discovered.Name, err)
		}
		if string(canonicalSchema) != string(sourceSchema) {
			t.Fatalf("tool %q input schema differs after canonicalization:\ninventory=%s\ndiscovered=%s", discovered.Name, canonicalSchema, sourceSchema)
		}
	}
}

func TestProjectToolInventory_HandlerRegisteredForCanonicalTools(t *testing.T) {
	inventory, err := mcpfactorysession.ProjectToolInventory()
	if err != nil {
		t.Fatalf("ProjectToolInventory() error = %v", err)
	}
	for _, tool := range inventory.Tools {
		if !tool.HandlerRegistered {
			t.Fatalf("tool %q handlerRegistered = false, want true", tool.Name)
		}
		if !mcpfactorysession.IsCanonicalToolHandlerRegistered(tool.Name) {
			t.Fatalf("tool %q is not registered in the canonical handler registry", tool.Name)
		}
	}
}

func mustProjectToolInventory(t *testing.T) mcpfactorysession.ToolInventory {
	t.Helper()
	inventory, err := mcpfactorysession.ProjectToolInventory()
	if err != nil {
		t.Fatalf("ProjectToolInventory() error = %v", err)
	}
	return inventory
}

func inventoryToolsByName(t *testing.T, inventory mcpfactorysession.ToolInventory) map[string]mcpfactorysession.ToolInventoryEntry {
	t.Helper()
	byName := make(map[string]mcpfactorysession.ToolInventoryEntry, len(inventory.Tools))
	for _, tool := range inventory.Tools {
		byName[tool.Name] = tool
	}
	return byName
}

func schemaObjectKeys(properties map[string]any) []string {
	keys := make([]string, 0, len(properties))
	for key := range properties {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func cloneToolDefinitions(t *testing.T, tools []mcpfactorysession.ToolDefinition) []mcpfactorysession.ToolDefinition {
	t.Helper()
	encoded, err := json.Marshal(tools)
	if err != nil {
		t.Fatalf("marshal tool definitions: %v", err)
	}
	var cloned []mcpfactorysession.ToolDefinition
	if err := json.Unmarshal(encoded, &cloned); err != nil {
		t.Fatalf("unmarshal tool definitions: %v", err)
	}
	return cloned
}
