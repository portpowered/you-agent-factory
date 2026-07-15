package climanifestgen

import (
	"fmt"
	"slices"
)

// MCPFamilyCommandIDs are canonical command IDs emitted from commands.json.
var MCPFamilyCommandIDs = []string{
	"you.mcp",
	"you.mcp.serve",
}

// WorkflowCompatibilityFamilyCommandIDs are compatibility-only command IDs
// emitted from deprecated-commands.json, never from the primary manifest.
var WorkflowCompatibilityFamilyCommandIDs = []string{
	"you.workflow.preview",
	"you.workflow.validate",
}

func AssertMCPFamilyCommandID(id string) error {
	if slices.Contains(MCPFamilyCommandIDs, id) {
		return nil
	}
	return fmt.Errorf("command id %q is outside the canonical MCP family %v", id, MCPFamilyCommandIDs)
}

func AssertWorkflowCompatibilityFamilyCommandID(id string) error {
	if slices.Contains(WorkflowCompatibilityFamilyCommandIDs, id) {
		return nil
	}
	return fmt.Errorf("command id %q is outside the workflow compatibility family %v", id, WorkflowCompatibilityFamilyCommandIDs)
}
