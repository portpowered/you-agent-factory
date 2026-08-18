package discoverygen

import (
	"fmt"
	"strings"

	mcpfactorycatalog "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/mcp/catalog"
)

const (
	// DiscoveryFormatVersion is the reviewed MCP discovery metadata document version.
	DiscoveryFormatVersion = "1.0.0"
)

// ProjectDiscoveryFromCatalogDocument projects canonical MCP discovery metadata
// from one resolved authored catalog document value.
func ProjectDiscoveryFromCatalogDocument(value any) (DiscoveryMetadata, error) {
	if err := mcpfactorycatalog.VerifyCatalogAliasExclusion(value); err != nil {
		return DiscoveryMetadata{}, fmt.Errorf("catalog alias exclusion: %w", err)
	}

	root, ok := value.(map[string]any)
	if !ok {
		return DiscoveryMetadata{}, fmt.Errorf("catalog document is not an object")
	}
	toolsValue, ok := root["tools"]
	if !ok {
		return DiscoveryMetadata{}, fmt.Errorf("catalog document missing tools")
	}
	tools, ok := toolsValue.(map[string]any)
	if !ok {
		return DiscoveryMetadata{}, fmt.Errorf("catalog tools is not an object")
	}

	records := make(map[string]DiscoveryToolRecord, len(tools))
	for key, raw := range tools {
		projected, err := projectDiscoveryTool(key, raw)
		if err != nil {
			return DiscoveryMetadata{}, err
		}
		records[key] = projected
	}

	metadata := DiscoveryMetadata{
		FormatVersion: DiscoveryFormatVersion,
		Tools:         records,
	}
	if err := VerifyDiscoveryAliasExclusion(metadata); err != nil {
		return DiscoveryMetadata{}, err
	}
	if err := VerifyDiscoveryModalityPolicy(metadata); err != nil {
		return DiscoveryMetadata{}, err
	}
	return metadata, nil
}

func projectDiscoveryTool(key string, raw any) (DiscoveryToolRecord, error) {
	record, ok := raw.(map[string]any)
	if !ok {
		return DiscoveryToolRecord{}, fmt.Errorf("catalog tool %q is not an object", key)
	}
	id, _ := record["id"].(string)
	name, _ := record["name"].(string)
	if id == "" {
		return DiscoveryToolRecord{}, fmt.Errorf("catalog tool %q has empty id", key)
	}
	if name == "" {
		return DiscoveryToolRecord{}, fmt.Errorf("catalog tool %q has empty name", key)
	}
	if id != key {
		return DiscoveryToolRecord{}, fmt.Errorf("catalog tool key %q does not match record id %q", key, id)
	}
	if !strings.HasPrefix(id, mcpfactorycatalog.CatalogToolIDPrefix) {
		return DiscoveryToolRecord{}, fmt.Errorf("catalog tool %q id %q is not a canonical tool id", name, id)
	}

	description, err := catalogToolDescription(record)
	if err != nil {
		return DiscoveryToolRecord{}, fmt.Errorf("catalog tool %q description: %w", name, err)
	}

	inputValue, ok := record["input"].(map[string]any)
	if !ok {
		return DiscoveryToolRecord{}, fmt.Errorf("catalog tool %q input is not an object", name)
	}
	schemaValue, ok := inputValue["schema"].(map[string]any)
	if !ok {
		return DiscoveryToolRecord{}, fmt.Errorf("catalog tool %q input.schema is not an object", name)
	}
	inputSchema, err := mcpfactorycatalog.PrepareCatalogInputSchemaForParity(schemaValue)
	if err != nil {
		return DiscoveryToolRecord{}, fmt.Errorf("catalog tool %q input schema: %w", name, err)
	}

	return DiscoveryToolRecord{
		ID:          id,
		Name:        name,
		Description: description,
		InputSchema: inputSchema,
	}, nil
}
