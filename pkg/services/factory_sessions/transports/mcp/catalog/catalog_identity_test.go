package catalog_test

import (
	"strings"
	"testing"

	mcpfactorysession "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/mcp"
	mcpfactorycatalog "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/mcp/catalog"
)

func TestVerifyCatalogToolIdentityCompleteness_PassesForAuthoredCatalogShape(t *testing.T) {
	discovered := mcpfactorysession.DiscoverTools()
	catalog := make([]mcpfactorycatalog.CatalogToolIdentity, 0, len(discovered))
	for _, tool := range discovered {
		catalog = append(catalog, mcpfactorycatalog.CatalogToolIdentity{
			ID:   mcpfactorycatalog.CatalogToolIDForName(tool.Name),
			Name: tool.Name,
		})
	}
	if err := mcpfactorycatalog.VerifyCatalogToolIdentityCompleteness(catalog, discovered); err != nil {
		t.Fatalf("VerifyCatalogToolIdentityCompleteness() error = %v", err)
	}
}

func TestVerifyCatalogToolIdentityCompleteness_FailsWhenDiscoveredToolMissing(t *testing.T) {
	discovered := mcpfactorysession.DiscoverTools()
	catalog := make([]mcpfactorycatalog.CatalogToolIdentity, 0, len(discovered)-1)
	for _, tool := range discovered[1:] {
		catalog = append(catalog, mcpfactorycatalog.CatalogToolIdentity{
			ID:   mcpfactorycatalog.CatalogToolIDForName(tool.Name),
			Name: tool.Name,
		})
	}
	err := mcpfactorycatalog.VerifyCatalogToolIdentityCompleteness(catalog, discovered)
	if err == nil {
		t.Fatal("VerifyCatalogToolIdentityCompleteness() error = nil, want missing-tool failure")
	}
	if !strings.Contains(err.Error(), discovered[0].Name) {
		t.Fatalf("VerifyCatalogToolIdentityCompleteness() error = %v, want missing tool %q", err, discovered[0].Name)
	}
}

func TestVerifyCatalogToolIdentityCompleteness_FailsWhenCatalogContainsExtraTool(t *testing.T) {
	discovered := mcpfactorysession.DiscoverTools()
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
	err := mcpfactorycatalog.VerifyCatalogToolIdentityCompleteness(catalog, discovered)
	if err == nil {
		t.Fatal("VerifyCatalogToolIdentityCompleteness() error = nil, want extra-tool failure")
	}
	if !strings.Contains(err.Error(), "you.factory_session.extra_probe") {
		t.Fatalf("VerifyCatalogToolIdentityCompleteness() error = %v, want extra tool", err)
	}
}

func TestVerifyCatalogToolIdentityCompleteness_FailsWhenPublicNameDuplicated(t *testing.T) {
	discovered := mcpfactorysession.DiscoverTools()
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
	err := mcpfactorycatalog.VerifyCatalogToolIdentityCompleteness(catalog, discovered[:1])
	if err == nil {
		t.Fatal("VerifyCatalogToolIdentityCompleteness() error = nil, want duplicate-name failure")
	}
	if !strings.Contains(err.Error(), "duplicate catalog public name") {
		t.Fatalf("VerifyCatalogToolIdentityCompleteness() error = %v, want duplicate public name", err)
	}
}

func TestVerifyCatalogToolIdentityCompleteness_FailsWhenStableIDMismatchesName(t *testing.T) {
	discovered := mcpfactorysession.DiscoverTools()
	catalog := []mcpfactorycatalog.CatalogToolIdentity{{
		ID:   "mcp.tool.you.factory_session.wrong_id",
		Name: discovered[0].Name,
	}}
	err := mcpfactorycatalog.VerifyCatalogToolIdentityCompleteness(catalog, discovered[:1])
	if err == nil {
		t.Fatal("VerifyCatalogToolIdentityCompleteness() error = nil, want stable-ID mismatch failure")
	}
	if !strings.Contains(err.Error(), "want") {
		t.Fatalf("VerifyCatalogToolIdentityCompleteness() error = %v, want stable ID mismatch", err)
	}
}
