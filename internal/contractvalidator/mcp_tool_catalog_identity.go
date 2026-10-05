package contractvalidator

import (
	"strings"

	mcpfactorysession "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/mcp"
	mcpfactorycatalog "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/mcp/catalog"
	workersessionmcp "github.com/portpowered/infinite-you/pkg/services/worker_sessions/transports/mcp"
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
	var discovered []mcpfactorycatalog.CatalogToolIdentity
	for _, tool := range mcpfactorysession.DiscoverTools() {
		discovered = append(discovered, mcpfactorycatalog.CatalogToolIdentity{ID: mcpfactorycatalog.CatalogToolIDForName(tool.Name), Name: tool.Name})
	}
	for _, binding := range workersessionmcp.ProjectCanonicalToolHandlerBindings() {
		discovered = append(discovered, mcpfactorycatalog.CatalogToolIdentity{ID: binding.ToolID, Name: strings.TrimPrefix(binding.ToolID, "mcp.tool.")})
	}
	if err := mcpfactorycatalog.VerifyCatalogToolIdentities(identities, discovered); err != nil {
		return []Diagnostic{newDiagnostic("catalog.identity.incomplete", "/tools", err.Error(), document)}
	}
	return nil
}

func mcpToolCatalogIdentityDiagnostics(document string, value any) []Diagnostic {
	return MCPToolCatalogIdentityDiagnostics(document, value)
}
