package discoverygen

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/internal/contractjoiner"
	mcpfactorysession "github.com/portpowered/infinite-you/pkg/transports/mcp/factorysession"
	mcpfactorycatalog "github.com/portpowered/infinite-you/pkg/transports/mcp/factorysession/catalog"
)

var discoveryForbiddenRecordKeys = []string{
	"outputSchema",
	"structuredContent",
	"image",
	"audio",
	"resource",
}

var discoveryForbiddenContentTypes = []string{"image", "audio", "resource"}

// VerifyDiscoveryAliasExclusion rejects compatibility workflow-named tools from
// generated primary discovery metadata.
func VerifyDiscoveryAliasExclusion(metadata DiscoveryMetadata) error {
	for id, tool := range metadata.Tools {
		if strings.HasPrefix(tool.Name, "you.workflow.") {
			return fmt.Errorf("compatibility alias %q must not appear in generated discovery metadata", tool.Name)
		}
		if strings.HasPrefix(id, "mcp.tool.you.workflow.") || strings.HasPrefix(tool.ID, "mcp.tool.you.workflow.") {
			return fmt.Errorf("compatibility alias tool id %q must not appear in generated discovery metadata", id)
		}
	}
	return nil
}

// VerifyDiscoveryModalityPolicy ensures generated discovery metadata advertises
// only text-oriented tools/list surfaces without unsupported MCP modalities.
func VerifyDiscoveryModalityPolicy(metadata DiscoveryMetadata) error {
	for id, tool := range metadata.Tools {
		if err := verifyDiscoveryInputSchemaModality(tool.InputSchema); err != nil {
			return fmt.Errorf("discovery tool %q inputSchema: %w", tool.Name, err)
		}
		if tool.ID != id {
			return fmt.Errorf("discovery tool map key %q does not match record id %q", id, tool.ID)
		}
	}
	return nil
}

// VerifyDiscoveryByteStability ensures repeated canonical serialization is identical.
func VerifyDiscoveryByteStability(value any) error {
	first, err := contractjoiner.MarshalCanonicalJSON(value)
	if err != nil {
		return err
	}
	second, err := contractjoiner.MarshalCanonicalJSON(value)
	if err != nil {
		return err
	}
	if !bytes.Equal(first, second) {
		return fmt.Errorf("discovery canonical serialization is not byte-stable across repeated projection")
	}
	return nil
}

// VerifyDiscoveryToolIdentityCompleteness ensures generated discovery metadata
// covers every discovered canonical tool exactly once by stable ID and name.
func VerifyDiscoveryToolIdentityCompleteness(metadata DiscoveryMetadata, discovered []mcpfactorysession.ToolDefinition) error {
	identities := make([]mcpfactorycatalog.CatalogToolIdentity, 0, len(metadata.Tools))
	for _, tool := range metadata.Tools {
		identities = append(identities, mcpfactorycatalog.CatalogToolIdentity{
			ID:   tool.ID,
			Name: tool.Name,
		})
	}
	return mcpfactorycatalog.VerifyCatalogToolIdentityCompleteness(identities, discovered)
}

func verifyDiscoveryInputSchemaModality(schema map[string]any) error {
	if schema == nil {
		return nil
	}
	for _, forbidden := range discoveryForbiddenRecordKeys {
		if _, ok := schema[forbidden]; ok {
			return fmt.Errorf("must not include %q", forbidden)
		}
	}
	if err := verifyDiscoveryContentTypes(schema["content"]); err != nil {
		return err
	}
	if err := verifyDiscoveryProperties(schema["properties"]); err != nil {
		return err
	}
	return verifyDiscoveryItems(schema["items"])
}

func verifyDiscoveryContentTypes(value any) error {
	content, ok := value.([]any)
	if !ok {
		return nil
	}
	for index, raw := range content {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		contentType, _ := item["type"].(string)
		for _, forbidden := range discoveryForbiddenContentTypes {
			if contentType == forbidden {
				return fmt.Errorf("content[%d].type = %q is unsupported", index, contentType)
			}
		}
	}
	return nil
}

func verifyDiscoveryProperties(value any) error {
	properties, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	for propertyName, propertySchema := range properties {
		propertyMap, ok := propertySchema.(map[string]any)
		if !ok {
			continue
		}
		if err := verifyDiscoveryInputSchemaModality(propertyMap); err != nil {
			return fmt.Errorf("properties.%s: %w", propertyName, err)
		}
	}
	return nil
}

func verifyDiscoveryItems(value any) error {
	switch typed := value.(type) {
	case map[string]any:
		if err := verifyDiscoveryInputSchemaModality(typed); err != nil {
			return fmt.Errorf("items: %w", err)
		}
	case []any:
		for index, item := range typed {
			itemMap, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if err := verifyDiscoveryInputSchemaModality(itemMap); err != nil {
				return fmt.Errorf("items[%d]: %w", index, err)
			}
		}
	}
	return nil
}
