package contractvalidator

import (
	mcpfactorysession "github.com/portpowered/infinite-you/pkg/transports/mcp/factorysession"
	mcpfactorycatalog "github.com/portpowered/infinite-you/pkg/transports/mcp/factorysession/catalog"
)

const authoredMCPToolCatalogPath = "contracts/mcp/tools.json"

// MCPToolCatalogIdentityDiagnostics applies authored-catalog identity completeness
// checks against current DiscoverTools output.
func MCPToolCatalogIdentityDiagnostics(document string, value any) []Diagnostic {
	if document != authoredMCPToolCatalogPath {
		return nil
	}
	identities, err := mcpfactorycatalog.CatalogToolIdentitiesFromCatalogDocument(value)
	if err != nil {
		return []Diagnostic{newDiagnostic("catalog.identity.parse", "/tools", err.Error(), document)}
	}
	if err := mcpfactorycatalog.VerifyCatalogToolIdentityCompleteness(identities, mcpfactorysession.DiscoverTools()); err != nil {
		return []Diagnostic{newDiagnostic("catalog.identity.incomplete", "/tools", err.Error(), document)}
	}
	return nil
}

func mcpToolCatalogIdentityDiagnostics(document string, value any) []Diagnostic {
	return MCPToolCatalogIdentityDiagnostics(document, value)
}
