package discoverygen

// DiscoveryMetadata is the generated MCP tools/list discovery projection.
type DiscoveryMetadata struct {
	FormatVersion string                         `json:"formatVersion"`
	Tools         map[string]DiscoveryToolRecord `json:"tools"`
}

// DiscoveryToolRecord is one canonical MCP tool discovery surface.
type DiscoveryToolRecord struct {
	Annotations map[string]any `json:"annotations,omitempty"`
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// MCPManifest is the reviewed MCP surface consumed by both publication and runtime generation.
type MCPManifest struct {
	FormatVersion   string             `json:"formatVersion"`
	ProtocolVersion string             `json:"protocolVersion"`
	Tools           []ManifestTool     `json:"tools"`
	Resources       []ManifestResource `json:"resources"`
	Skills          []ManifestSkill    `json:"skills"`
}

type ManifestTool struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
	Handler     string         `json:"handler"`
}

type ManifestResource struct {
	ID          string `json:"id"`
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description"`
	MIMEType    string `json:"mimeType"`
	Handler     string `json:"handler"`
}

type ManifestSkill struct {
	ID           string         `json:"id"`
	URI          string         `json:"uri"`
	Frontmatter  map[string]any `json:"frontmatter"`
	ResourceURIs []string       `json:"resourceURIs"`
}
