package contractvalidator_test

import (
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/internal/contractvalidator"
)

func TestMCPToolCatalogPublicationDiagnostics_PassesForAuthoredCatalog(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}
	resolved, diagnostics := contractvalidator.LoadAndResolve(root, "contracts/mcp/tools.json", []string{"contracts/mcp/tools.json"})
	if len(diagnostics) != 0 {
		t.Fatalf("resolve authored catalog diagnostics = %+v", diagnostics)
	}
	publicationDiagnostics := contractvalidator.MCPToolCatalogPublicationDiagnostics("contracts/mcp/tools.json", resolved)
	if len(publicationDiagnostics) != 0 {
		t.Fatalf("MCPToolCatalogPublicationDiagnostics() = %+v, want none", publicationDiagnostics)
	}
}

func TestMCPToolCatalogPublicationDiagnostics_RejectsWorkflowAliasTool(t *testing.T) {
	document := map[string]any{
		"formatVersion":   "1.0.0",
		"protocolVersion": "2024-11-05",
		"sharedSchemas":   map[string]any{},
		"tools": map[string]any{
			"mcp.tool.you.workflow.run": map[string]any{
				"id":   "mcp.tool.you.workflow.run",
				"name": "you.workflow.run",
			},
		},
	}
	diagnostics := contractvalidator.MCPToolCatalogPublicationDiagnostics("contracts/mcp/tools.json", document)
	if len(diagnostics) == 0 {
		t.Fatal("MCPToolCatalogPublicationDiagnostics() = none, want alias rejection")
	}
	if diagnostics[0].Code != "catalog.publication.alias" {
		t.Fatalf("diagnostic code = %q, want catalog.publication.alias", diagnostics[0].Code)
	}
}
