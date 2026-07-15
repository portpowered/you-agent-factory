package contracts_test

import (
	"testing"

	mcpfactorysession "github.com/portpowered/infinite-you/pkg/transports/mcp/factorysession"
)

func TestMCPToolCatalogIdentityCompleteness_AuthoredCatalogMatchesDiscoverTools(t *testing.T) {
	document := readJSON(t, "mcp/tools.json")
	identities, err := mcpfactorysession.CatalogToolIdentitiesFromCatalogDocument(document)
	if err != nil {
		t.Fatalf("CatalogToolIdentitiesFromCatalogDocument() error = %v", err)
	}
	if err := mcpfactorysession.VerifyCatalogToolIdentityCompleteness(identities, mcpfactorysession.DiscoverTools()); err != nil {
		t.Fatalf("VerifyCatalogToolIdentityCompleteness() error = %v", err)
	}
}
