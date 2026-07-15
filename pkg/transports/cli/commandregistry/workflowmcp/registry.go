// Package workflowmcp binds the canonical MCP serve command and the separately
// classified workflow compatibility commands to handwritten Cobra handlers.
package workflowmcp

import (
	"fmt"
	"sort"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestgen"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
)

// Handlers carries handwritten handlers for the runnable commands in both
// classification-isolated families.
type Handlers struct {
	MCPServe         commandregistry.RunE
	WorkflowPreview  commandregistry.RunE
	WorkflowValidate commandregistry.RunE
}

// Registries keeps canonical and compatibility handlers isolated so a workflow
// compatibility ID cannot be attached through the canonical MCP registry.
type Registries struct {
	MCP                   *commandregistry.Registry
	WorkflowCompatibility *commandregistry.Registry
}

// NewRegistries registers handwritten handlers by stable command ID and verifies
// complete runnable coverage against both generated manifest classifications.
func NewRegistries(handlers Handlers) (Registries, error) {
	bindings := []struct {
		commandID string
		handler   commandregistry.RunE
	}{
		{commandID: "you.mcp.serve", handler: handlers.MCPServe},
		{commandID: "you.workflow.preview", handler: handlers.WorkflowPreview},
		{commandID: "you.workflow.validate", handler: handlers.WorkflowValidate},
	}
	for _, binding := range bindings {
		if binding.handler == nil {
			return Registries{}, fmt.Errorf("build workflow/MCP handler registries: %s handler is required", binding.commandID)
		}
	}

	mcpRegistry := commandregistry.NewRegistry()
	if err := mcpRegistry.Register("you.mcp.serve", handlers.MCPServe); err != nil {
		return Registries{}, fmt.Errorf("build workflow/MCP handler registries: %w", err)
	}
	workflowRegistry := commandregistry.NewRegistry()
	for _, binding := range bindings[1:] {
		if err := workflowRegistry.Register(binding.commandID, binding.handler); err != nil {
			return Registries{}, fmt.Errorf("build workflow/MCP handler registries: %w", err)
		}
	}

	mcpManifest, err := generated.MCPFamilyManifest()
	if err != nil {
		return Registries{}, fmt.Errorf("build workflow/MCP handler registries: %w", err)
	}
	if err := VerifyMCPRunnableCoverage(mcpManifest, mcpRegistry); err != nil {
		return Registries{}, fmt.Errorf("build workflow/MCP handler registries: %w", err)
	}
	workflowManifest, err := generated.WorkflowCompatibilityFamilyManifest()
	if err != nil {
		return Registries{}, fmt.Errorf("build workflow/MCP handler registries: %w", err)
	}
	if err := VerifyWorkflowCompatibilityRunnableCoverage(workflowManifest, workflowRegistry); err != nil {
		return Registries{}, fmt.Errorf("build workflow/MCP handler registries: %w", err)
	}
	return Registries{MCP: mcpRegistry, WorkflowCompatibility: workflowRegistry}, nil
}

// RunnableMCPCommandIDs returns canonical runnable MCP IDs in stable order.
func RunnableMCPCommandIDs(manifest climanifest.Manifest) ([]string, error) {
	return runnableCommandIDs(manifest, climanifestgen.MCPFamilyCommandIDs, climanifestgen.AssertMCPFamilyCommandID, "canonical MCP")
}

// RunnableWorkflowCompatibilityCommandIDs returns runnable compatibility IDs in
// stable order without adding them to the canonical registry.
func RunnableWorkflowCompatibilityCommandIDs(manifest climanifest.Manifest) ([]string, error) {
	return runnableCommandIDs(manifest, climanifestgen.WorkflowCompatibilityFamilyCommandIDs, climanifestgen.AssertWorkflowCompatibilityFamilyCommandID, "workflow compatibility")
}

// VerifyMCPRunnableCoverage rejects incomplete or classification-mismatched MCP bindings.
func VerifyMCPRunnableCoverage(manifest climanifest.Manifest, registry *commandregistry.Registry) error {
	ids, err := RunnableMCPCommandIDs(manifest)
	if err != nil {
		return err
	}
	return verifyCoverage(ids, registry, "canonical MCP")
}

// VerifyWorkflowCompatibilityRunnableCoverage rejects incomplete or
// classification-mismatched workflow compatibility bindings.
func VerifyWorkflowCompatibilityRunnableCoverage(manifest climanifest.Manifest, registry *commandregistry.Registry) error {
	ids, err := RunnableWorkflowCompatibilityCommandIDs(manifest)
	if err != nil {
		return err
	}
	return verifyCoverage(ids, registry, "workflow compatibility")
}

func runnableCommandIDs(
	manifest climanifest.Manifest,
	familyIDs []string,
	assertFamilyID func(string) error,
	familyLabel string,
) ([]string, error) {
	if len(manifest.Commands) != len(familyIDs) {
		return nil, fmt.Errorf("%s manifest command count = %d, want %d", familyLabel, len(manifest.Commands), len(familyIDs))
	}
	for commandID := range manifest.Commands {
		if err := assertFamilyID(commandID); err != nil {
			return nil, fmt.Errorf("%s classification mismatch for stable command ID %q: %w", familyLabel, commandID, err)
		}
	}

	ids := make([]string, 0, len(familyIDs))
	for _, commandID := range familyIDs {
		record, err := manifest.CommandByID(commandID)
		if err != nil {
			return nil, err
		}
		if record.Runnable {
			ids = append(ids, commandID)
		}
	}
	sort.Strings(ids)
	return ids, nil
}

func verifyCoverage(ids []string, registry *commandregistry.Registry, familyLabel string) error {
	if registry == nil {
		return fmt.Errorf("%s handler registry is required", familyLabel)
	}
	var missing []string
	for _, commandID := range ids {
		if _, err := registry.Lookup(commandID); err != nil {
			missing = append(missing, commandID)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("%s runnable command handlers missing for: %v", familyLabel, missing)
	}
	return nil
}
