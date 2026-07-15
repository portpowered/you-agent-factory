package contracts_test

import (
	"testing"

	mcpfactorycatalog "github.com/portpowered/infinite-you/pkg/transports/mcp/factorysession/catalog"
	mcpfactorysession "github.com/portpowered/infinite-you/pkg/transports/mcp/factorysession"
)

func TestMCPToolCatalogIdentityCompleteness_AuthoredCatalogMatchesDiscoverTools(t *testing.T) {
	document := readJSON(t, "mcp/tools.json")
	identities, err := mcpfactorycatalog.CatalogToolIdentitiesFromCatalogDocument(document)
	if err != nil {
		t.Fatalf("CatalogToolIdentitiesFromCatalogDocument() error = %v", err)
	}
	if err := mcpfactorycatalog.VerifyCatalogToolIdentityCompleteness(identities, mcpfactorysession.DiscoverTools()); err != nil {
		t.Fatalf("VerifyCatalogToolIdentityCompleteness() error = %v", err)
	}
}
