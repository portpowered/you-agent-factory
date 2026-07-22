package mcpcontractcheck

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	mcpfactorysession "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/mcp"
	"github.com/portpowered/infinite-you/pkg/transports/mcp/discoverygen"
	mcpgenerated "github.com/portpowered/infinite-you/pkg/transports/mcp/generated"
)

const retainedAliasInventoryPath = "contracts/mcp/deprecated.json"

type retainedAliasInventory struct {
	Records map[string]struct {
		ItemID     string `json:"itemId"`
		PublicName string `json:"publicName"`
		Lifecycle  struct {
			Successor struct {
				TargetItemID string `json:"targetItemId"`
			} `json:"successor"`
		} `json:"lifecycle"`
	} `json:"records"`
}

// Check loads the repository-owned boundary projections and runs the pure
// structural comparison. It is read-only.
func Check(repositoryRoot string) ([]Diagnostic, error) {
	inputs, err := LoadInputs(repositoryRoot)
	if err != nil {
		return nil, err
	}
	return Validate(inputs), nil
}

// LoadInputs projects authored, generated, registry, and alias values into
// explicit checker inputs without making contract data executable.
func LoadInputs(repositoryRoot string) (Inputs, error) {
	resolved, err := discoverygen.LoadResolvedCatalog(repositoryRoot)
	if err != nil {
		return Inputs{}, err
	}
	projected, err := discoverygen.ProjectDiscoveryFromCatalogDocument(resolved)
	if err != nil {
		return Inputs{}, fmt.Errorf("project authored MCP catalog: %w", err)
	}

	root, ok := resolved.(map[string]any)
	if !ok {
		return Inputs{}, fmt.Errorf("resolved MCP catalog is not an object")
	}
	authoredTools, ok := root["tools"].(map[string]any)
	if !ok {
		return Inputs{}, fmt.Errorf("resolved MCP catalog tools is not an object")
	}

	inputs := Inputs{Catalog: make([]ToolRecord, 0, len(projected.Tools))}
	for id, record := range projected.Tools {
		raw, ok := authoredTools[id].(map[string]any)
		if !ok {
			return Inputs{}, fmt.Errorf("authored MCP tool %q is not an object", id)
		}
		handler, ok := raw["handler"].(map[string]any)
		if !ok {
			return Inputs{}, fmt.Errorf("authored MCP tool %q handler is not an object", id)
		}
		handlerID, _ := handler["id"].(string)
		inputs.Catalog = append(inputs.Catalog, ToolRecord{
			ID: id, Name: record.Name, Description: record.Description,
			InputSchema: record.InputSchema, HandlerID: handlerID,
		})
	}

	for _, record := range mcpgenerated.PrimaryDiscovery() {
		var inputSchema any
		if err := json.Unmarshal(record.InputSchema, &inputSchema); err != nil {
			return Inputs{}, fmt.Errorf("decode generated discovery input schema for %q: %w", record.ID, err)
		}
		inputs.Discovery = append(inputs.Discovery, ToolRecord{
			ID: record.ID, Name: record.Name, Description: record.Description, InputSchema: inputSchema,
		})
	}
	for _, binding := range mcpfactorysession.ProjectCanonicalToolHandlerBindings() {
		inputs.Registry = append(inputs.Registry, HandlerBinding(binding))
	}
	inputs.Aliases, err = loadRetainedAliases(repositoryRoot)
	if err != nil {
		return Inputs{}, err
	}
	return inputs, nil
}

func loadRetainedAliases(repositoryRoot string) ([]AliasBinding, error) {
	path := filepath.Join(repositoryRoot, filepath.FromSlash(retainedAliasInventoryPath))
	payload, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read retained MCP alias inventory %s: %w", filepath.ToSlash(path), err)
	}
	var inventory retainedAliasInventory
	if err := json.Unmarshal(payload, &inventory); err != nil {
		return nil, fmt.Errorf("decode retained MCP alias inventory %s: %w", filepath.ToSlash(path), err)
	}
	aliases := make([]AliasBinding, 0, len(inventory.Records))
	for _, record := range inventory.Records {
		aliases = append(aliases, AliasBinding{
			ID: record.ItemID, Name: record.PublicName,
			CanonicalToolID: record.Lifecycle.Successor.TargetItemID,
		})
	}
	return aliases, nil
}
