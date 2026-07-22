package catalog_test

import (
	"strings"
	"testing"

	mcpfactorycatalog "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/mcp/catalog"
)

func TestVerifyCatalogAliasExclusion_RejectsWorkflowAliasName(t *testing.T) {
	document := map[string]any{
		"tools": map[string]any{
			"mcp.tool.you.workflow.run": map[string]any{
				"id":   "mcp.tool.you.workflow.run",
				"name": "you.workflow.run",
			},
		},
	}
	err := mcpfactorycatalog.VerifyCatalogAliasExclusion(document)
	if err == nil {
		t.Fatal("VerifyCatalogAliasExclusion() error = nil, want workflow alias rejection")
	}
	if !strings.Contains(err.Error(), "you.workflow.run") {
		t.Fatalf("VerifyCatalogAliasExclusion() error = %v, want workflow alias name", err)
	}
}

func TestVerifyCatalogModalityPolicy_RejectsStructuredContentTransport(t *testing.T) {
	document := map[string]any{
		"tools": map[string]any{
			"mcp.tool.you.factory_session.list": map[string]any{
				"name":       "you.factory_session.list",
				"execution":  map[string]any{"mode": "tools-call"},
				"transports": []any{"stdio-json-rpc"},
				"result": map[string]any{
					"transport": map[string]any{
						"contentTypes":            []any{"text"},
						"textEncoding":            "serialized-json",
						"allowsStructuredContent": true,
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
	err := mcpfactorycatalog.VerifyCatalogModalityPolicy(document)
	if err == nil {
		t.Fatal("VerifyCatalogModalityPolicy() error = nil, want structuredContent rejection")
	}
	if !strings.Contains(err.Error(), "allowsStructuredContent") {
		t.Fatalf("VerifyCatalogModalityPolicy() error = %v, want structuredContent policy failure", err)
	}
}

func TestVerifyCatalogModalityPolicy_RejectsHTTPTransport(t *testing.T) {
	document := map[string]any{
		"tools": map[string]any{
			"mcp.tool.you.factory_session.list": map[string]any{
				"name":       "you.factory_session.list",
				"execution":  map[string]any{"mode": "tools-call"},
				"transports": []any{"http-json-rpc"},
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
	err := mcpfactorycatalog.VerifyCatalogModalityPolicy(document)
	if err == nil {
		t.Fatal("VerifyCatalogModalityPolicy() error = nil, want HTTP transport rejection")
	}
	if !strings.Contains(err.Error(), "http") {
		t.Fatalf("VerifyCatalogModalityPolicy() error = %v, want HTTP transport failure", err)
	}
}

func TestVerifyCatalogModalityPolicy_RejectsImageContentExample(t *testing.T) {
	document := map[string]any{
		"tools": map[string]any{
			"mcp.tool.you.factory_session.list": map[string]any{
				"name":       "you.factory_session.list",
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
								map[string]any{"type": "image", "data": "abc"},
							},
							"isError": false,
						},
					},
				},
			},
		},
	}
	err := mcpfactorycatalog.VerifyCatalogModalityPolicy(document)
	if err == nil {
		t.Fatal("VerifyCatalogModalityPolicy() error = nil, want image content rejection")
	}
	if !strings.Contains(err.Error(), "image") {
		t.Fatalf("VerifyCatalogModalityPolicy() error = %v, want image content failure", err)
	}
}

func TestVerifyCatalogByteStability_RepeatSerializationMatches(t *testing.T) {
	document := map[string]any{
		"formatVersion":   "1.0.0",
		"protocolVersion": "2024-11-05",
		"tools": map[string]any{
			"mcp.tool.you.factory_session.list": map[string]any{
				"id":   "mcp.tool.you.factory_session.list",
				"name": "you.factory_session.list",
			},
		},
	}
	if err := mcpfactorycatalog.VerifyCatalogByteStability(document); err != nil {
		t.Fatalf("VerifyCatalogByteStability() error = %v", err)
	}
}

func TestVerifyAuthoredCatalogStagingBoundary_RejectsInventoryArrayShape(t *testing.T) {
	document := map[string]any{
		"formatVersion":   "1",
		"protocolVersion": "2024-11-05",
		"tools": []any{
			map[string]any{"name": "you.factory_session.list"},
		},
	}
	err := mcpfactorycatalog.VerifyAuthoredCatalogStagingBoundary(document)
	if err == nil {
		t.Fatal("VerifyAuthoredCatalogStagingBoundary() error = nil, want staging inventory rejection")
	}
	if !strings.Contains(err.Error(), "sharedSchemas") && !strings.Contains(err.Error(), "staged inventory") {
		t.Fatalf("VerifyAuthoredCatalogStagingBoundary() error = %v, want authored/staging boundary failure", err)
	}
}
