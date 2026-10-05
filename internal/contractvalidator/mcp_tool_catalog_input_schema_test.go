package contractvalidator_test

import (
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/internal/contractvalidator"
)

func TestMCPToolCatalogInputSchemaDiagnostics_AuthoredCatalogPasses(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}
	resolved, diagnostics := contractvalidator.LoadAndResolve(root, "contracts/mcp/tools.json", []string{"contracts/mcp/tools.json"})
	if len(diagnostics) != 0 {
		t.Fatalf("resolve authored catalog diagnostics = %+v", diagnostics)
	}
	got := contractvalidator.MCPToolCatalogInputSchemaDiagnostics("contracts/mcp/tools.json", resolved)
	if len(got) != 0 {
		t.Fatalf("MCPToolCatalogInputSchemaDiagnostics() = %+v, want none", got)
	}
}

func TestMCPToolCatalogInputSchemaDiagnostics_SkipsNonAuthoredCatalog(t *testing.T) {
	got := contractvalidator.MCPToolCatalogInputSchemaDiagnostics(
		"contracts/testdata/mcp/valid-minimal.json",
		map[string]any{"tools": map[string]any{}},
	)
	if len(got) != 0 {
		t.Fatalf("MCPToolCatalogInputSchemaDiagnostics() = %+v, want skip", got)
	}
}

func TestWorkerSessionInputSchemaDriftIsRejected(t *testing.T) {
	t.Parallel()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	resolved, diagnostics := contractvalidator.LoadAndResolve(root, "contracts/mcp/tools.json", []string{"contracts/mcp/tools.json"})
	if len(diagnostics) != 0 {
		t.Fatalf("resolve catalog: %v", diagnostics)
	}
	tools := resolved.(map[string]any)["tools"].(map[string]any)
	read := tools["mcp.tool.you.worker_session.read"].(map[string]any)
	schema := read["input"].(map[string]any)["schema"].(map[string]any)
	schema["additionalProperties"] = true
	got := contractvalidator.MCPToolCatalogInputSchemaDiagnostics("contracts/mcp/tools.json", resolved)
	if len(got) != 1 || got[0].Code != "catalog.input_schema.parity" {
		t.Fatalf("Worker Session schema drift was accepted: %v", got)
	}
}
