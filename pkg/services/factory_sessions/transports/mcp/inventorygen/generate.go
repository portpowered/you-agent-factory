// Package inventorygen composes the reviewed S-11 MCP tool inventory artifact.
package inventorygen

import (
	"fmt"

	"github.com/portpowered/infinite-you/pkg/platform/generatedartifacts"
	factorysession "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/mcp"
)

// Artifact returns the deterministic S-11 artifact projected from the live
// Factory Sessions MCP registry. The projection and marshal operations remain
// owned by the MCP package; this package only composes their output for the
// platform artifact store.
func Artifact() (generatedartifacts.Artifact, error) {
	inventory, err := factorysession.ProjectToolInventory()
	if err != nil {
		return generatedartifacts.Artifact{}, fmt.Errorf("project MCP tool inventory: %w", err)
	}
	return artifactFromInventory(inventory)
}

func artifactFromInventory(inventory factorysession.ToolInventory) (generatedartifacts.Artifact, error) {
	if err := factorysession.VerifyToolInventory(inventory); err != nil {
		return generatedartifacts.Artifact{}, fmt.Errorf("verify MCP tool inventory: %w", err)
	}
	payload, err := factorysession.MarshalToolInventoryJSON(inventory)
	if err != nil {
		return generatedartifacts.Artifact{}, fmt.Errorf("marshal MCP tool inventory: %w", err)
	}
	return generatedartifacts.Artifact{
		Path:    factorysession.ToolInventoryBaselineRelativePath,
		Payload: payload,
	}, nil
}
