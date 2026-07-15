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
