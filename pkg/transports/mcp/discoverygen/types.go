package discoverygen

// DiscoveryMetadata is the generated MCP tools/list discovery projection.
type DiscoveryMetadata struct {
	FormatVersion string                         `json:"formatVersion"`
	Tools         map[string]DiscoveryToolRecord `json:"tools"`
}

// DiscoveryToolRecord is one canonical MCP tool discovery surface.
type DiscoveryToolRecord struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}
