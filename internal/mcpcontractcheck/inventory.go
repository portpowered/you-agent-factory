package mcpcontractcheck

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/portpowered/infinite-you/pkg/platform/generatedartifacts"
	factorymcp "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/mcp"
	workermcp "github.com/portpowered/infinite-you/pkg/services/worker_sessions/transports/mcp"
	generated "github.com/portpowered/infinite-you/pkg/transports/mcp/generated"
)

// GenerateInventoryArtifacts projects the complete public tool union and its
// transport policy. Service-owned compatibility projections remain unchanged.
func GenerateInventoryArtifacts() ([]generatedartifacts.Artifact, error) {
	inputs := Inputs{}
	for _, tool := range factorymcp.DiscoverTools() {
		inputs.Discovery = append(inputs.Discovery, ToolRecord{ID: "mcp.tool." + tool.Name, Name: tool.Name, Description: tool.Description, InputSchema: tool.InputSchema})
	}
	for _, tool := range generated.PrimaryDiscovery() {
		if !strings.HasPrefix(tool.Name, "you.worker_session.") {
			continue
		}
		var schema any
		if err := json.Unmarshal(tool.InputSchema, &schema); err != nil {
			return nil, err
		}
		for _, owned := range workermcp.DiscoverTools() {
			if owned.Name == tool.Name {
				schema = owned.InputSchema
			}
		}
		inputs.Discovery = append(inputs.Discovery, ToolRecord{ID: tool.ID, Name: tool.Name, Description: tool.Description, InputSchema: schema})
	}
	for _, binding := range factorymcp.ProjectCanonicalToolHandlerBindings() {
		inputs.Registry = append(inputs.Registry, HandlerBinding(binding))
	}
	for _, binding := range workermcp.ProjectCanonicalToolHandlerBindings() {
		inputs.Registry = append(inputs.Registry, HandlerBinding(binding))
	}
	inventory, err := ProjectToolInventory(inputs.Discovery, inputs.Registry)
	if err != nil {
		return nil, err
	}
	tools, err := factorymcp.MarshalToolInventoryJSON(inventory)
	if err != nil {
		return nil, err
	}
	policy, err := projectResultPolicy()
	if err != nil {
		return nil, err
	}
	results, err := factorymcp.MarshalResultPolicyInventoryJSON(policy)
	if err != nil {
		return nil, err
	}
	return []generatedartifacts.Artifact{
		{Path: factorymcp.ToolInventoryBaselineRelativePath, Payload: tools},
		{Path: factorymcp.ResultPolicyInventoryBaselineRelativePath, Payload: results},
	}, nil
}

// ProjectToolInventory verifies exact discovery/handler coverage before
// serializing service-neutral tool identities. Duplicate and surplus bindings
// fail even when every discovered name has some handler.
func ProjectToolInventory(discovery []ToolRecord, bindings []HandlerBinding) (factorymcp.ToolInventory, error) {
	tools, diagnostics := indexTools(discovery, "discovery")
	registry, bindingDiagnostics := indexBindings(bindings)
	diagnostics = append(diagnostics, bindingDiagnostics...)
	if len(diagnostics) != 0 {
		return factorymcp.ToolInventory{}, fmt.Errorf("invalid inventory registry: %s", diagnostics[0].Message)
	}
	if len(tools) != len(registry) {
		return factorymcp.ToolInventory{}, fmt.Errorf("discovery and handler inventory counts differ: %d != %d", len(tools), len(registry))
	}
	inventory := factorymcp.ToolInventory{FormatVersion: factorymcp.ToolInventoryFormatVersion, ProtocolVersion: factorymcp.ToolInventoryProtocolVersion}
	for id, tool := range tools {
		binding, ok := registry[id]
		if !ok || binding.HandlerID != strings.Replace(id, "mcp.tool.", "mcp.handler.", 1) {
			return factorymcp.ToolInventory{}, fmt.Errorf("discovered canonical tool %q has no matching registered handler", tool.Name)
		}
		schema, ok := tool.InputSchema.(map[string]any)
		if !ok {
			return factorymcp.ToolInventory{}, fmt.Errorf("tool %q input schema must be an object", tool.Name)
		}
		canonical, err := factorymcp.CanonicalizeInputSchema(schema)
		if err != nil {
			return factorymcp.ToolInventory{}, err
		}
		inventory.Tools = append(inventory.Tools, factorymcp.ToolInventoryEntry{
			IDCandidate: strings.ReplaceAll(strings.TrimPrefix(tool.Name, "you."), "_", "-"),
			Name:        tool.Name, Description: tool.Description, InputSchema: canonical, HandlerRegistered: true,
		})
	}
	slices.SortFunc(inventory.Tools, func(a, b factorymcp.ToolInventoryEntry) int { return strings.Compare(a.Name, b.Name) })
	return inventory, nil
}

func projectResultPolicy() (factorymcp.ResultPolicyInventory, error) {
	policy, err := factorymcp.ProjectResultPolicyInventory()
	if err != nil {
		return policy, err
	}
	for _, fixture := range []struct{ name, tool, response string }{
		{"worker_list_empty", workermcp.ToolList, `{"result":{"sessions":[],"paginationContext":{"maxResults":50}}}`},
		{"worker_read_events_empty", workermcp.ToolRead, `{"result":{"session":{"workerSessionId":"fixture-worker"},"events":{"events":[],"truncated":false}}}`},
		{"worker_control_noop", workermcp.ToolControl, `{"result":{"workerSessionId":"fixture-worker","dispatchId":"fixture-attempt","action":"CANCEL","outcome":"NOOP","state":"CANCELED"}}`},
	} {
		response := json.RawMessage(fixture.response)
		encoded, err := factorymcp.MarshalSuccessCallToolResultJSON(response)
		if err != nil {
			return policy, err
		}
		policy.Fixtures = append(policy.Fixtures, factorymcp.ResultPolicyFixture{Name: fixture.name, Description: "Representative Worker Session success transport encoding.", ToolName: fixture.tool, ToolResponse: response, CallToolResult: encoded})
	}
	response := json.RawMessage(`{"error":{"code":"worker_session.not_found","message":"Selected host rejected the Worker Session request","retryable":false,"workerSessionId":"fixture-worker","details":{"status":404}}}`)
	encoded, err := factorymcp.MarshalDomainErrorCallToolResultJSON(response)
	if err != nil {
		return policy, err
	}
	policy.DomainErrorFixtures = append(policy.DomainErrorFixtures, factorymcp.DomainErrorFixture{
		Name: "worker_read_not_found", Description: "Worker Session typed error identity is preserved in structuredContent.", ToolName: workermcp.ToolRead,
		ToolArguments: json.RawMessage(`{"workerSessionId":"fixture-worker"}`), ToolResponse: response, CallToolResult: encoded,
	})
	return policy, factorymcp.VerifyResultPolicyInventory(policy)
}

func inventoryDiagnostics(root string) ([]Diagnostic, error) {
	artifacts, err := GenerateInventoryArtifacts()
	if err != nil {
		return nil, err
	}
	var diagnostics []Diagnostic
	for _, artifact := range artifacts {
		payload, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(artifact.Path)))
		if err != nil {
			return nil, err
		}
		if !bytes.Equal(bytes.TrimSpace(payload), artifact.Payload) {
			diagnostics = append(diagnostics, Diagnostic{Code: "mcp.inventory.drift", Surface: artifact.Path, Message: "reviewed inventory differs from the complete registry; regenerate with go run ./cmd/mcptoolinventorygen"})
		}
	}
	return diagnostics, nil
}
