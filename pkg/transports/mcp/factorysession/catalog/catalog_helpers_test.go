package catalog_test

import (
	"strings"
	"testing"

	mcpfactorysession "github.com/portpowered/infinite-you/pkg/transports/mcp/factorysession"
	mcpfactorycatalog "github.com/portpowered/infinite-you/pkg/transports/mcp/factorysession/catalog"
)

func validCatalogToolRecord(name string) map[string]any {
	return map[string]any{
		"id":   mcpfactorycatalog.CatalogToolIDForName(name),
		"name": name,
		"input": map[string]any{
			"schema": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
			},
		},
	}
}

func TestCatalogToolIdentitiesFromCatalogDocument_ExtractsSortedIdentities(t *testing.T) {
	document := map[string]any{
		"tools": map[string]any{
			mcpfactorycatalog.CatalogToolIDForName(mcpfactorysession.ToolGetSession):   validCatalogToolRecord(mcpfactorysession.ToolGetSession),
			mcpfactorycatalog.CatalogToolIDForName(mcpfactorysession.ToolListSessions): validCatalogToolRecord(mcpfactorysession.ToolListSessions),
		},
	}
	identities, err := mcpfactorycatalog.CatalogToolIdentitiesFromCatalogDocument(document)
	if err != nil {
		t.Fatalf("CatalogToolIdentitiesFromCatalogDocument() error = %v", err)
	}
	if len(identities) != 2 {
		t.Fatalf("identity count = %d, want 2", len(identities))
	}
	if identities[0].Name != mcpfactorysession.ToolGetSession || identities[1].Name != mcpfactorysession.ToolListSessions {
		t.Fatalf("identities = %#v, want sorted by public name", identities)
	}
}

func TestCatalogToolIdentitiesFromCatalogDocument_RejectsMalformedDocument(t *testing.T) {
	_, err := mcpfactorycatalog.CatalogToolIdentitiesFromCatalogDocument("not-an-object")
	if err == nil || !strings.Contains(err.Error(), "not an object") {
		t.Fatalf("CatalogToolIdentitiesFromCatalogDocument() error = %v, want object failure", err)
	}

	_, err = mcpfactorycatalog.CatalogToolIdentitiesFromCatalogDocument(map[string]any{})
	if err == nil || !strings.Contains(err.Error(), "missing tools") {
		t.Fatalf("CatalogToolIdentitiesFromCatalogDocument() error = %v, want missing tools", err)
	}

	document := map[string]any{
		"tools": map[string]any{
			"mcp.tool.you.factory_session.list": "not-an-object",
		},
	}
	_, err = mcpfactorycatalog.CatalogToolIdentitiesFromCatalogDocument(document)
	if err == nil || !strings.Contains(err.Error(), "not an object") {
		t.Fatalf("CatalogToolIdentitiesFromCatalogDocument() error = %v, want tool object failure", err)
	}

	document = map[string]any{
		"tools": map[string]any{
			"mcp.tool.you.factory_session.list": map[string]any{
				"id":   "mcp.tool.you.factory_session.other",
				"name": mcpfactorysession.ToolListSessions,
			},
		},
	}
	_, err = mcpfactorycatalog.CatalogToolIdentitiesFromCatalogDocument(document)
	if err == nil || !strings.Contains(err.Error(), "does not match record id") {
		t.Fatalf("CatalogToolIdentitiesFromCatalogDocument() error = %v, want key/id mismatch", err)
	}
}

func TestCatalogInputSchemasFromCatalogDocument_ExtractsInputSchemas(t *testing.T) {
	document := map[string]any{
		"tools": map[string]any{
			mcpfactorycatalog.CatalogToolIDForName(mcpfactorysession.ToolListSessions): validCatalogToolRecord(mcpfactorysession.ToolListSessions),
		},
	}
	schemas, err := mcpfactorycatalog.CatalogInputSchemasFromCatalogDocument(document)
	if err != nil {
		t.Fatalf("CatalogInputSchemasFromCatalogDocument() error = %v", err)
	}
	if len(schemas) != 1 || schemas[0].Name != mcpfactorysession.ToolListSessions {
		t.Fatalf("schemas = %#v, want one list schema", schemas)
	}
}

func TestCatalogInputSchemasFromCatalogDocument_RejectsMalformedInput(t *testing.T) {
	document := map[string]any{
		"tools": map[string]any{
			mcpfactorycatalog.CatalogToolIDForName(mcpfactorysession.ToolListSessions): map[string]any{
				"name":  mcpfactorysession.ToolListSessions,
				"input": "not-an-object",
			},
		},
	}
	_, err := mcpfactorycatalog.CatalogInputSchemasFromCatalogDocument(document)
	if err == nil || !strings.Contains(err.Error(), "input is not an object") {
		t.Fatalf("CatalogInputSchemasFromCatalogDocument() error = %v, want input object failure", err)
	}

	document = map[string]any{
		"tools": map[string]any{
			mcpfactorycatalog.CatalogToolIDForName(mcpfactorysession.ToolListSessions): map[string]any{
				"name": mcpfactorysession.ToolListSessions,
				"input": map[string]any{
					"schema": "not-an-object",
				},
			},
		},
	}
	_, err = mcpfactorycatalog.CatalogInputSchemasFromCatalogDocument(document)
	if err == nil || !strings.Contains(err.Error(), "input.schema is not an object") {
		t.Fatalf("CatalogInputSchemasFromCatalogDocument() error = %v, want schema object failure", err)
	}
}

func TestCatalogInputSchemaToolPath_EscapesJSONPointerTokens(t *testing.T) {
	path := mcpfactorycatalog.CatalogInputSchemaToolPath(mcpfactorysession.ToolListSessions)
	want := "/tools/mcp.tool.you.factory_session.list/input/schema"
	if path != want {
		t.Fatalf("CatalogInputSchemaToolPath() = %q, want %q", path, want)
	}
}

func TestVerifyCatalogToolIdentityCompleteness_FailsWhenStableIDDuplicated(t *testing.T) {
	discovered := mcpfactorysession.DiscoverTools()
	catalog := []mcpfactorycatalog.CatalogToolIdentity{
		{
			ID:   mcpfactorycatalog.CatalogToolIDForName(discovered[0].Name),
			Name: discovered[0].Name,
		},
		{
			ID:   mcpfactorycatalog.CatalogToolIDForName(discovered[0].Name),
			Name: discovered[1].Name,
		},
	}
	err := mcpfactorycatalog.VerifyCatalogToolIdentityCompleteness(catalog, discovered[:2])
	if err == nil || !strings.Contains(err.Error(), "duplicate catalog stable ID") {
		t.Fatalf("VerifyCatalogToolIdentityCompleteness() error = %v, want duplicate stable ID", err)
	}
}

func TestVerifyCatalogInputSchemaParity_FailsWhenCatalogMissingDiscoveredTool(t *testing.T) {
	tool, ok := mcpfactorysession.ToolByName(mcpfactorysession.ToolListSessions)
	if !ok {
		t.Fatal("list tool missing from discovery")
	}
	err := mcpfactorycatalog.VerifyCatalogInputSchemaParity(nil, []mcpfactorysession.ToolDefinition{tool})
	if err == nil || !strings.Contains(err.Error(), "missing input schema") {
		t.Fatalf("VerifyCatalogInputSchemaParity() error = %v, want missing schema failure", err)
	}
}

func TestVerifyCatalogInputSchemaParity_FailsWhenCatalogContainsExtraSchema(t *testing.T) {
	tool, ok := mcpfactorysession.ToolByName(mcpfactorysession.ToolListSessions)
	if !ok {
		t.Fatal("list tool missing from discovery")
	}
	catalog := []mcpfactorycatalog.CatalogInputSchema{
		{Name: tool.Name, Schema: tool.InputSchema},
		{Name: "you.factory_session.extra_probe", Schema: tool.InputSchema},
	}
	err := mcpfactorycatalog.VerifyCatalogInputSchemaParity(catalog, []mcpfactorysession.ToolDefinition{tool})
	if err == nil || !strings.Contains(err.Error(), "extra input schema") {
		t.Fatalf("VerifyCatalogInputSchemaParity() error = %v, want extra schema failure", err)
	}
}

func TestVerifyAuthoredCatalogStagingBoundary_AcceptsAuthoredCatalogShape(t *testing.T) {
	document := map[string]any{
		"formatVersion": mcpfactorycatalog.AuthoredCatalogFormatVersion,
		"sharedSchemas": map[string]any{},
		"tools": map[string]any{
			mcpfactorycatalog.CatalogToolIDForName(mcpfactorysession.ToolListSessions): map[string]any{
				"id":   mcpfactorycatalog.CatalogToolIDForName(mcpfactorysession.ToolListSessions),
				"name": mcpfactorysession.ToolListSessions,
			},
		},
	}
	if err := mcpfactorycatalog.VerifyAuthoredCatalogStagingBoundary(document); err != nil {
		t.Fatalf("VerifyAuthoredCatalogStagingBoundary() error = %v", err)
	}
}

func TestVerifyCatalogAliasExclusion_RejectsWorkflowAliasKeyPrefix(t *testing.T) {
	document := map[string]any{
		"tools": map[string]any{
			"mcp.tool.you.workflow.run": map[string]any{
				"id":   "mcp.tool.you.factory_session.list",
				"name": mcpfactorysession.ToolListSessions,
			},
		},
	}
	err := mcpfactorycatalog.VerifyCatalogAliasExclusion(document)
	if err == nil || !strings.Contains(err.Error(), "mcp.tool.you.workflow.run") {
		t.Fatalf("VerifyCatalogAliasExclusion() error = %v, want workflow key rejection", err)
	}
}

func TestVerifyCatalogModalityPolicy_AcceptsTextOnlyStdioCatalogShape(t *testing.T) {
	document := map[string]any{
		"tools": map[string]any{
			mcpfactorycatalog.CatalogToolIDForName(mcpfactorysession.ToolListSessions): map[string]any{
				"name":       mcpfactorysession.ToolListSessions,
				"execution":  map[string]any{"mode": "tools-call"},
				"transports": []any{"stdio-json-rpc"},
				"result": map[string]any{
					"transport": map[string]any{
						"contentTypes":            []any{"text"},
						"textEncoding":            "serialized-json",
						"allowsStructuredContent": false,
						"allowsOutputSchema":      false,
					},
					"examples": []any{
						map[string]any{
							"content": []any{
								map[string]any{"type": "text", "text": "{}"},
							},
							"isError": false,
						},
					},
				},
			},
		},
	}
	if err := mcpfactorycatalog.VerifyCatalogModalityPolicy(document); err != nil {
		t.Fatalf("VerifyCatalogModalityPolicy() error = %v", err)
	}
}

func TestMarshalCatalogDocumentJSON_UsesStableTrailingNewline(t *testing.T) {
	payload, err := mcpfactorycatalog.MarshalCatalogDocumentJSON(map[string]any{"formatVersion": "1.0.0"})
	if err != nil {
		t.Fatalf("MarshalCatalogDocumentJSON() error = %v", err)
	}
	if len(payload) == 0 || payload[len(payload)-1] != '\n' {
		t.Fatalf("MarshalCatalogDocumentJSON() payload = %q, want trailing newline", payload)
	}
}

func TestVerifyCatalogAliasExclusion_AcceptsCanonicalCatalogTools(t *testing.T) {
	document := map[string]any{
		"tools": map[string]any{
			mcpfactorycatalog.CatalogToolIDForName(mcpfactorysession.ToolListSessions): map[string]any{
				"id":   mcpfactorycatalog.CatalogToolIDForName(mcpfactorysession.ToolListSessions),
				"name": mcpfactorysession.ToolListSessions,
			},
		},
	}
	if err := mcpfactorycatalog.VerifyCatalogAliasExclusion(document); err != nil {
		t.Fatalf("VerifyCatalogAliasExclusion() error = %v", err)
	}
}

func TestVerifyCatalogAliasExclusion_RejectsWorkflowAliasIDField(t *testing.T) {
	document := map[string]any{
		"tools": map[string]any{
			mcpfactorycatalog.CatalogToolIDForName(mcpfactorysession.ToolListSessions): map[string]any{
				"id":   "mcp.tool.you.workflow.run",
				"name": mcpfactorysession.ToolListSessions,
			},
		},
	}
	err := mcpfactorycatalog.VerifyCatalogAliasExclusion(document)
	if err == nil || !strings.Contains(err.Error(), "mcp.tool.you.workflow.run") {
		t.Fatalf("VerifyCatalogAliasExclusion() error = %v, want workflow id rejection", err)
	}
}

func TestVerifyAuthoredCatalogStagingBoundary_RejectsWrongFormatVersion(t *testing.T) {
	document := map[string]any{
		"formatVersion": "9.9.9",
		"sharedSchemas": map[string]any{},
		"tools":         map[string]any{},
	}
	err := mcpfactorycatalog.VerifyAuthoredCatalogStagingBoundary(document)
	if err == nil || !strings.Contains(err.Error(), "want authored catalog") {
		t.Fatalf("VerifyAuthoredCatalogStagingBoundary() error = %v, want formatVersion failure", err)
	}
}

func TestVerifyCatalogModalityPolicy_RejectsStructuredContentExampleField(t *testing.T) {
	document := map[string]any{
		"tools": map[string]any{
			mcpfactorycatalog.CatalogToolIDForName(mcpfactorysession.ToolListSessions): map[string]any{
				"name":       mcpfactorysession.ToolListSessions,
				"execution":  map[string]any{"mode": "tools-call"},
				"transports": []string{"stdio-json-rpc"},
				"result": map[string]any{
					"transport": map[string]any{
						"contentTypes":            []string{"text"},
						"textEncoding":            "serialized-json",
						"allowsStructuredContent": false,
						"allowsOutputSchema":      false,
					},
					"examples": []any{
						map[string]any{
							"structuredContent": map[string]any{"ok": true},
							"content": []any{
								map[string]any{"type": "text", "text": "{}"},
							},
							"isError": false,
						},
					},
				},
			},
		},
	}
	err := mcpfactorycatalog.VerifyCatalogModalityPolicy(document)
	if err == nil || !strings.Contains(err.Error(), "structuredContent") {
		t.Fatalf("VerifyCatalogModalityPolicy() error = %v, want structuredContent example rejection", err)
	}
}

func TestVerifyCatalogModalityPolicy_RejectsOutputSchemaExampleField(t *testing.T) {
	document := map[string]any{
		"tools": map[string]any{
			mcpfactorycatalog.CatalogToolIDForName(mcpfactorysession.ToolListSessions): map[string]any{
				"name":       mcpfactorysession.ToolListSessions,
				"execution":  map[string]any{"mode": "tools-call"},
				"transports": []string{"stdio-json-rpc"},
				"result": map[string]any{
					"transport": map[string]any{
						"contentTypes":            []string{"text"},
						"textEncoding":            "serialized-json",
						"allowsStructuredContent": false,
						"allowsOutputSchema":      false,
					},
					"examples": []any{
						map[string]any{
							"outputSchema": map[string]any{"type": "object"},
							"content": []any{
								map[string]any{"type": "text", "text": "{}"},
							},
							"isError": false,
						},
					},
				},
			},
		},
	}
	err := mcpfactorycatalog.VerifyCatalogModalityPolicy(document)
	if err == nil || !strings.Contains(err.Error(), "outputSchema") {
		t.Fatalf("VerifyCatalogModalityPolicy() error = %v, want outputSchema example rejection", err)
	}
}
