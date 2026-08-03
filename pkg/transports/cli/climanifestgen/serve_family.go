package climanifestgen

import (
	"fmt"
	"slices"
)

// ServeFamilyCommandIDs are canonical command IDs emitted from commands.json
// for the you serve family that hosts the composed process ACP server.
var ServeFamilyCommandIDs = []string{
	"you.serve",
	"you.serve.acp",
}

func AssertServeFamilyCommandID(id string) error {
	if slices.Contains(ServeFamilyCommandIDs, id) {
		return nil
	}
	return fmt.Errorf("command id %q is outside the canonical serve family %v", id, ServeFamilyCommandIDs)
}
