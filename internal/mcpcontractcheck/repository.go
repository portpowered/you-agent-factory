package mcpcontractcheck

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	mcpfactorysession "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/mcp"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/mcp/discoverygen"
	workersessionmcp "github.com/portpowered/infinite-you/pkg/services/worker_sessions/transports/mcp"
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
	diagnostics := Validate(inputs)
	inventory, err := inventoryDiagnostics(repositoryRoot)
	if err != nil {
		return nil, err
	}
	return append(diagnostics, inventory...), nil
}

// LoadInputs projects authored, generated, registry, and alias values into
// explicit checker inputs without making contract data executable.
func LoadInputs(repositoryRoot string) (Inputs, error) {
	manifest, err := discoverygen.LoadAuthoredManifest(repositoryRoot)
	if err != nil {
		return Inputs{}, err
	}
	if err := discoverygen.ValidateManifest(manifest); err != nil {
		return Inputs{}, fmt.Errorf("project authored MCP manifest: %w", err)
	}
	inputs := Inputs{Catalog: make([]ToolRecord, 0, len(manifest.Tools))}
	for _, record := range manifest.Tools {
		inputs.Catalog = append(inputs.Catalog, ToolRecord{ID: record.ID, Name: record.Name, Description: record.Description, InputSchema: record.InputSchema, HandlerID: record.Handler})
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

	bindings := mcpfactorysession.ProjectCanonicalToolHandlerBindings()
	for _, binding := range bindings {
		inputs.Registry = append(inputs.Registry, HandlerBinding(binding))
	}
	for _, binding := range workersessionmcp.ProjectCanonicalToolHandlerBindings() {
		inputs.Registry = append(inputs.Registry, HandlerBinding(binding))
	}
	for _, resource := range manifest.Resources {
		inputs.Resources = append(inputs.Resources, ResourceRecord{ID: resource.ID, URI: resource.URI, Name: resource.Name, Description: resource.Description, MIMEType: resource.MIMEType, Handler: resource.Handler})
	}
	for _, resource := range mcpgenerated.PrimaryResources() {
		inputs.GeneratedResources = append(inputs.GeneratedResources, ResourceRecord{ID: resource.ID, URI: resource.URI, Name: resource.Name, Description: resource.Description, MIMEType: resource.MIMEType, Handler: resource.Handler})
	}
	for _, skill := range manifest.Skills {
		inputs.Skills = append(inputs.Skills, SkillRecord{ID: skill.ID, URI: skill.URI, Frontmatter: skill.Frontmatter, ResourceURIs: skill.ResourceURIs})
	}
	for _, skill := range mcpgenerated.PrimarySkills() {
		inputs.GeneratedSkills = append(inputs.GeneratedSkills, SkillRecord{ID: skill.ID, URI: skill.URI, Frontmatter: skill.Frontmatter, ResourceURIs: skill.ResourceURIs})
	}

	// Keep the broader service adapter catalog parity checked independently from
	// the public MCP surface. These tools remain internal adapter contracts.
	legacyValue, err := discoverygen.LoadResolvedCatalog(repositoryRoot)
	if err != nil {
		return Inputs{}, err
	}
	legacyProjection, err := discoverygen.ProjectDiscoveryFromCatalogDocument(legacyValue)
	if err != nil {
		return Inputs{}, fmt.Errorf("project legacy MCP adapter catalog: %w", err)
	}
	legacyRoot, ok := legacyValue.(map[string]any)
	if !ok {
		return Inputs{}, fmt.Errorf("legacy MCP catalog is not an object")
	}
	legacyTools, ok := legacyRoot["tools"].(map[string]any)
	if !ok {
		return Inputs{}, fmt.Errorf("legacy MCP catalog tools is not an object")
	}
	for id, record := range legacyProjection.Tools {
		if strings.HasPrefix(id, "mcp.tool.you.worker_session.") {
			continue
		}
		raw, ok := legacyTools[id].(map[string]any)
		if !ok {
			return Inputs{}, fmt.Errorf("legacy MCP tool %q is not an object", id)
		}
		handler, ok := raw["handler"].(map[string]any)
		if !ok {
			return Inputs{}, fmt.Errorf("legacy MCP tool %q handler is not an object", id)
		}
		handlerID, _ := handler["id"].(string)
		inputs.LegacyCatalog = append(inputs.LegacyCatalog, ToolRecord{ID: id, Name: record.Name, Description: record.Description, InputSchema: record.InputSchema, HandlerID: handlerID})
	}
	for _, record := range mcpgenerated.LegacyDiscovery() {
		var inputSchema any
		if err := json.Unmarshal(record.InputSchema, &inputSchema); err != nil {
			return Inputs{}, fmt.Errorf("decode legacy generated schema for %q: %w", record.ID, err)
		}
		inputs.LegacyDiscovery = append(inputs.LegacyDiscovery, ToolRecord{ID: record.ID, Name: record.Name, Description: record.Description, InputSchema: inputSchema})
	}
	for _, binding := range bindings {
		inputs.LegacyRegistry = append(inputs.LegacyRegistry, HandlerBinding(binding))
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
