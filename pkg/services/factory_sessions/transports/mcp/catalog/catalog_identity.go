package catalog

import (
	"fmt"
	"slices"
	"strings"

	mcpfactorysession "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/mcp"
)

const (
	// CatalogToolIDPrefix is the stable ID prefix for authored MCP tool catalog entries.
	CatalogToolIDPrefix = "mcp.tool."
)

// CatalogToolIdentity records one authored catalog tool identity projection.
type CatalogToolIdentity struct {
	ID   string
	Name string
}

// CatalogToolIDForName maps one canonical discovered public name to its stable catalog ID.
func CatalogToolIDForName(name string) string {
	return CatalogToolIDPrefix + name
}

// CatalogToolIdentitiesFromCatalogDocument extracts tool identities from one resolved
// MCP tool-catalog document value.
func CatalogToolIdentitiesFromCatalogDocument(value any) ([]CatalogToolIdentity, error) {
	root, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("catalog document is not an object")
	}
	toolsValue, ok := root["tools"]
	if !ok {
		return nil, fmt.Errorf("catalog document missing tools")
	}
	tools, ok := toolsValue.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("catalog tools is not an object")
	}

	identities := make([]CatalogToolIdentity, 0, len(tools))
	for key, raw := range tools {
		record, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("catalog tool %q is not an object", key)
		}
		id, _ := record["id"].(string)
		name, _ := record["name"].(string)
		if id == "" {
			return nil, fmt.Errorf("catalog tool %q has empty id", key)
		}
		if name == "" {
			return nil, fmt.Errorf("catalog tool %q has empty name", key)
		}
		if id != key {
			return nil, fmt.Errorf("catalog tool key %q does not match record id %q", key, id)
		}
		identities = append(identities, CatalogToolIdentity{ID: id, Name: name})
	}
	slices.SortFunc(identities, func(left, right CatalogToolIdentity) int {
		return strings.Compare(left.Name, right.Name)
	})
	return identities, nil
}

// VerifyCatalogToolIdentityCompleteness ensures every discovered canonical tool occurs
// exactly once in the catalog by stable ID and public name and rejects extras and duplicates.
func VerifyCatalogToolIdentityCompleteness(catalog []CatalogToolIdentity, discovered []mcpfactorysession.ToolDefinition) error {
	byID := make(map[string]string, len(catalog))
	byName := make(map[string]string, len(catalog))
	for _, entry := range catalog {
		if previous, ok := byID[entry.ID]; ok {
			return fmt.Errorf("duplicate catalog stable ID %q for tools %q and %q", entry.ID, previous, entry.Name)
		}
		if previous, ok := byName[entry.Name]; ok {
			return fmt.Errorf("duplicate catalog public name %q for IDs %q and %q", entry.Name, previous, entry.ID)
		}
		wantID := CatalogToolIDForName(entry.Name)
		if entry.ID != wantID {
			return fmt.Errorf("catalog tool %q id = %q, want %q", entry.Name, entry.ID, wantID)
		}
		byID[entry.ID] = entry.Name
		byName[entry.Name] = entry.ID
	}

	discoveredByName := make(map[string]struct{}, len(discovered))
	for _, tool := range discovered {
		discoveredByName[tool.Name] = struct{}{}
		wantID := CatalogToolIDForName(tool.Name)
		catalogName, ok := byID[wantID]
		if !ok {
			return fmt.Errorf("discovered canonical tool %q (stable ID %q) missing from catalog", tool.Name, wantID)
		}
		if catalogName != tool.Name {
			return fmt.Errorf("catalog stable ID %q name = %q, want discovered name %q", wantID, catalogName, tool.Name)
		}
	}

	if len(catalog) != len(discovered) {
		for id, name := range byID {
			if _, ok := discoveredByName[name]; !ok {
				return fmt.Errorf("catalog contains extra tool %q (stable ID %q) not in DiscoverTools", name, id)
			}
		}
		return fmt.Errorf("catalog tool count = %d, want %d discovered canonical tools", len(catalog), len(discovered))
	}

	return nil
}
