package discoverygen

import (
	"fmt"
	"strings"
)

// ValidateManifest checks semantic constraints not expressed by the JSON schema.
func ValidateManifest(manifest MCPManifest) error {
	if manifest.FormatVersion != "1.0.0" || manifest.ProtocolVersion != "2024-11-05" {
		return fmt.Errorf("MCP manifest has unsupported format or protocol version")
	}
	ids, names, uris := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, tool := range manifest.Tools {
		if tool.ID == "" || tool.Name == "" || strings.TrimSpace(tool.Description) == "" || tool.Handler == "" || tool.InputSchema["type"] != "object" {
			return fmt.Errorf("MCP manifest tool %q is incomplete", tool.ID)
		}
		if ids[tool.ID] || names[tool.Name] {
			return fmt.Errorf("MCP manifest tool %q or name %q is duplicated", tool.ID, tool.Name)
		}
		ids[tool.ID], names[tool.Name] = true, true
	}
	for _, resource := range manifest.Resources {
		if resource.ID == "" || resource.URI == "" || resource.Name == "" || resource.Description == "" || resource.MIMEType == "" || resource.Handler == "" {
			return fmt.Errorf("MCP manifest resource %q is incomplete", resource.ID)
		}
		if ids[resource.ID] || uris[resource.URI] {
			return fmt.Errorf("MCP manifest resource %q or URI %q is duplicated", resource.ID, resource.URI)
		}
		ids[resource.ID], uris[resource.URI] = true, true
	}
	for _, skill := range manifest.Skills {
		name, _ := skill.Frontmatter["name"].(string)
		description, _ := skill.Frontmatter["description"].(string)
		if skill.ID == "" || skill.URI == "" || name == "" || description == "" || len(skill.ResourceURIs) == 0 {
			return fmt.Errorf("MCP manifest skill %q is incomplete", skill.ID)
		}
		if ids[skill.ID] {
			return fmt.Errorf("MCP manifest skill %q is duplicated", skill.ID)
		}
		ids[skill.ID] = true
		if !uris[skill.URI] {
			return fmt.Errorf("MCP manifest skill %q has undeclared skill URI %q", skill.ID, skill.URI)
		}
		for _, resourceURI := range skill.ResourceURIs {
			if !uris[resourceURI] {
				return fmt.Errorf("MCP manifest skill %q references undeclared resource URI %q", skill.ID, resourceURI)
			}
		}
	}
	return nil
}

// ProjectManifestDiscovery projects tool discovery from the unified MCP manifest.
func ProjectManifestDiscovery(manifest MCPManifest) (DiscoveryMetadata, error) {
	if err := ValidateManifest(manifest); err != nil {
		return DiscoveryMetadata{}, err
	}
	metadata := DiscoveryMetadata{FormatVersion: DiscoveryFormatVersion, Tools: make(map[string]DiscoveryToolRecord, len(manifest.Tools))}
	for _, tool := range manifest.Tools {
		metadata.Tools[tool.ID] = DiscoveryToolRecord{ID: tool.ID, Name: tool.Name, Description: tool.Description, InputSchema: tool.InputSchema}
	}
	return metadata, nil
}
