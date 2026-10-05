package catalog_test

import (
	"strings"
	"testing"

	mcpfactorycatalog "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/mcp/catalog"
)

func TestVerifyCatalogToolIdentities_PassesForAuthoredCatalogShape(t *testing.T) {
	discovered := []mcpfactorycatalog.CatalogToolIdentity{{ID: "mcp.tool.you.example.first", Name: "you.example.first"}, {ID: "mcp.tool.you.example.second", Name: "you.example.second"}}
	catalog := make([]mcpfactorycatalog.CatalogToolIdentity, 0, len(discovered))
	for _, tool := range discovered {
		catalog = append(catalog, mcpfactorycatalog.CatalogToolIdentity{
			ID:   mcpfactorycatalog.CatalogToolIDForName(tool.Name),
			Name: tool.Name,
		})
	}
	if err := mcpfactorycatalog.VerifyCatalogToolIdentities(catalog, discovered); err != nil {
		t.Fatalf("VerifyCatalogToolIdentities() error = %v", err)
	}
}

func TestVerifyCatalogToolIdentities_FailsWhenDiscoveredToolMissing(t *testing.T) {
	discovered := []mcpfactorycatalog.CatalogToolIdentity{{ID: "mcp.tool.you.example.first", Name: "you.example.first"}, {ID: "mcp.tool.you.example.second", Name: "you.example.second"}}
	catalog := make([]mcpfactorycatalog.CatalogToolIdentity, 0, len(discovered)-1)
	for _, tool := range discovered[1:] {
		catalog = append(catalog, mcpfactorycatalog.CatalogToolIdentity{
			ID:   mcpfactorycatalog.CatalogToolIDForName(tool.Name),
			Name: tool.Name,
		})
	}
	err := mcpfactorycatalog.VerifyCatalogToolIdentities(catalog, discovered)
	if err == nil {
		t.Fatal("VerifyCatalogToolIdentities() error = nil, want missing-tool failure")
	}
	if !strings.Contains(err.Error(), discovered[0].Name) {
		t.Fatalf("VerifyCatalogToolIdentities() error = %v, want missing tool %q", err, discovered[0].Name)
	}
}

func TestVerifyCatalogToolIdentities_FailsWhenCatalogContainsExtraTool(t *testing.T) {
	discovered := []mcpfactorycatalog.CatalogToolIdentity{{ID: "mcp.tool.you.example.first", Name: "you.example.first"}, {ID: "mcp.tool.you.example.second", Name: "you.example.second"}}
	catalog := make([]mcpfactorycatalog.CatalogToolIdentity, 0, len(discovered)+1)
	for _, tool := range discovered {
		catalog = append(catalog, mcpfactorycatalog.CatalogToolIdentity{
			ID:   mcpfactorycatalog.CatalogToolIDForName(tool.Name),
			Name: tool.Name,
		})
	}
	catalog = append(catalog, mcpfactorycatalog.CatalogToolIdentity{
		ID:   "mcp.tool.you.factory_session.extra_probe",
		Name: "you.factory_session.extra_probe",
	})
	err := mcpfactorycatalog.VerifyCatalogToolIdentities(catalog, discovered)
	if err == nil {
		t.Fatal("VerifyCatalogToolIdentities() error = nil, want extra-tool failure")
	}
	if !strings.Contains(err.Error(), "you.factory_session.extra_probe") {
		t.Fatalf("VerifyCatalogToolIdentities() error = %v, want extra tool", err)
	}
}

func TestVerifyCatalogToolIdentities_FailsWhenPublicNameDuplicated(t *testing.T) {
	discovered := []mcpfactorycatalog.CatalogToolIdentity{{ID: "mcp.tool.you.example.first", Name: "you.example.first"}, {ID: "mcp.tool.you.example.second", Name: "you.example.second"}}
	catalog := []mcpfactorycatalog.CatalogToolIdentity{
		{
			ID:   mcpfactorycatalog.CatalogToolIDForName(discovered[0].Name),
			Name: discovered[0].Name,
		},
		{
			ID:   "mcp.tool.you.factory_session.duplicate_probe",
			Name: discovered[0].Name,
		},
	}
	err := mcpfactorycatalog.VerifyCatalogToolIdentities(catalog, discovered[:1])
	if err == nil {
		t.Fatal("VerifyCatalogToolIdentities() error = nil, want duplicate-name failure")
	}
	if !strings.Contains(err.Error(), "duplicate catalog public name") {
		t.Fatalf("VerifyCatalogToolIdentities() error = %v, want duplicate public name", err)
	}
}

func TestVerifyCatalogToolIdentities_FailsWhenStableIDMismatchesName(t *testing.T) {
	discovered := []mcpfactorycatalog.CatalogToolIdentity{{ID: "mcp.tool.you.example.first", Name: "you.example.first"}, {ID: "mcp.tool.you.example.second", Name: "you.example.second"}}
	catalog := []mcpfactorycatalog.CatalogToolIdentity{{
		ID:   "mcp.tool.you.factory_session.wrong_id",
		Name: discovered[0].Name,
	}}
	err := mcpfactorycatalog.VerifyCatalogToolIdentities(catalog, discovered[:1])
	if err == nil {
		t.Fatal("VerifyCatalogToolIdentities() error = nil, want stable-ID mismatch failure")
	}
	if !strings.Contains(err.Error(), "want") {
		t.Fatalf("VerifyCatalogToolIdentities() error = %v, want stable ID mismatch", err)
	}
}
