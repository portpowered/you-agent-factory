package contractvalidator

import (
	"strings"

	mcpfactorysession "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/mcp"
	mcpfactorycatalog "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/mcp/catalog"
)

// MCPToolCatalogInputSchemaDiagnostics applies authored-catalog input-schema parity
// checks against current DiscoverTools semantics.
func MCPToolCatalogInputSchemaDiagnostics(document string, value any) []Diagnostic {
	if document != authoredMCPToolCatalogPath {
		return nil
	}
	schemas, err := mcpfactorycatalog.CatalogInputSchemasFromCatalogDocument(value)
	if err != nil {
		return []Diagnostic{newDiagnostic("catalog.input_schema.parse", "/tools", err.Error(), document)}
	}
	if err := mcpfactorycatalog.VerifyCatalogInputSchemaParity(schemas, mcpfactorysession.DiscoverTools()); err != nil {
		path := "/tools"
		if toolName := catalogInputSchemaParityToolName(err.Error()); toolName != "" {
			path = mcpfactorycatalog.CatalogInputSchemaToolPath(toolName)
		}
		return []Diagnostic{newDiagnostic("catalog.input_schema.parity", path, err.Error(), document)}
	}
	return nil
}

func mcpToolCatalogInputSchemaDiagnostics(document string, value any) []Diagnostic {
	return MCPToolCatalogInputSchemaDiagnostics(document, value)
}

func catalogInputSchemaParityToolName(message string) string {
	const prefix = `catalog input schema for "`
	if !strings.HasPrefix(message, prefix) {
		return ""
	}
	rest := message[len(prefix):]
	end := strings.IndexByte(rest, '"')
	if end < 0 {
		return ""
	}
	return rest[:end]
}
