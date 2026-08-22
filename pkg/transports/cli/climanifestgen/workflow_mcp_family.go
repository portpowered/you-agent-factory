package climanifestgen

import (
	"fmt"
	"slices"
)

// MCPFamilyCommandIDs are canonical command IDs emitted from commands.json
// for the MCP child of the shared you server family.
var MCPFamilyCommandIDs = []string{
	"you.server",
	"you.server.mcp",
}

func AssertMCPFamilyCommandID(id string) error {
	if slices.Contains(MCPFamilyCommandIDs, id) {
		return nil
	}
	return fmt.Errorf("command id %q is outside the canonical MCP family %v", id, MCPFamilyCommandIDs)
}
