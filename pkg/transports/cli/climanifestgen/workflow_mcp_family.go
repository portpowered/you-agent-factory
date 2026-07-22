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

func AssertMCPFamilyCommandID(id string) error {
	if slices.Contains(MCPFamilyCommandIDs, id) {
		return nil
	}
	return fmt.Errorf("command id %q is outside the canonical MCP family %v", id, MCPFamilyCommandIDs)
}
