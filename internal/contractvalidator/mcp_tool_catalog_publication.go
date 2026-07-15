package contractvalidator

import (
	mcpfactorysession "github.com/portpowered/infinite-you/pkg/transports/mcp/factorysession"
)

// MCPToolCatalogPublicationDiagnostics applies authored-catalog publication guards
// for alias exclusion, text-only modality policy, byte-stable serialization, and
// staging-boundary separation from packages/api/generated/mcp/tools.json.
func MCPToolCatalogPublicationDiagnostics(document string, value any) []Diagnostic {
	if document != authoredMCPToolCatalogPath {
		return nil
	}
	if err := mcpfactorysession.VerifyCatalogAliasExclusion(value); err != nil {
		return []Diagnostic{newDiagnostic("catalog.publication.alias", "/tools", err.Error(), document)}
	}
	if err := mcpfactorysession.VerifyCatalogModalityPolicy(value); err != nil {
		return []Diagnostic{newDiagnostic("catalog.publication.modality", "/tools", err.Error(), document)}
	}
	if err := mcpfactorysession.VerifyAuthoredCatalogStagingBoundary(value); err != nil {
		return []Diagnostic{newDiagnostic("catalog.publication.staging_boundary", "/", err.Error(), document)}
	}
	if err := mcpfactorysession.VerifyCatalogByteStability(value); err != nil {
		return []Diagnostic{newDiagnostic("catalog.publication.byte_stability", "/", err.Error(), document)}
	}
	return nil
}

func mcpToolCatalogPublicationDiagnostics(document string, value any) []Diagnostic {
	return MCPToolCatalogPublicationDiagnostics(document, value)
}
