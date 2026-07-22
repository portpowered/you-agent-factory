package contractvalidator

import (
	mcpfactorycatalog "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/mcp/catalog"
)

// MCPToolCatalogPublicationDiagnostics applies authored-catalog publication guards
// for alias exclusion, text-only modality policy, byte-stable serialization, and
// staging-boundary separation from packages/api/generated/mcp/tools.json.
func MCPToolCatalogPublicationDiagnostics(document string, value any) []Diagnostic {
	if document != authoredMCPToolCatalogPath {
		return nil
	}
	if err := mcpfactorycatalog.VerifyCatalogAliasExclusion(value); err != nil {
		return []Diagnostic{newDiagnostic("catalog.publication.alias", "/tools", err.Error(), document)}
	}
	if err := mcpfactorycatalog.VerifyCatalogModalityPolicy(value); err != nil {
		return []Diagnostic{newDiagnostic("catalog.publication.modality", "/tools", err.Error(), document)}
	}
	if err := mcpfactorycatalog.VerifyAuthoredCatalogStagingBoundary(value); err != nil {
		return []Diagnostic{newDiagnostic("catalog.publication.staging_boundary", "/", err.Error(), document)}
	}
	if err := mcpfactorycatalog.VerifyCatalogByteStability(value); err != nil {
		return []Diagnostic{newDiagnostic("catalog.publication.byte_stability", "/", err.Error(), document)}
	}
	return nil
}

func mcpToolCatalogPublicationDiagnostics(document string, value any) []Diagnostic {
	return MCPToolCatalogPublicationDiagnostics(document, value)
}
