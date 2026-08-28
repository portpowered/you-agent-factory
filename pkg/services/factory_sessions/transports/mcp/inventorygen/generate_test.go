package inventorygen

import (
	"bytes"
	"strings"
	"testing"

	factorysession "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/mcp"
)

func TestArtifactUsesProductionToolInventoryProjection(t *testing.T) {
	artifact, err := Artifact()
	if err != nil {
		t.Fatalf("Artifact() error = %v", err)
	}

	inventory, err := factorysession.ProjectToolInventory()
	if err != nil {
		t.Fatalf("ProjectToolInventory() error = %v", err)
	}
	expectedPayload, err := factorysession.MarshalToolInventoryJSON(inventory)
	if err != nil {
		t.Fatalf("MarshalToolInventoryJSON() error = %v", err)
	}
	if artifact.Path != factorysession.ToolInventoryBaselineRelativePath {
		t.Fatalf("artifact path = %q, want %q", artifact.Path, factorysession.ToolInventoryBaselineRelativePath)
	}
	if !bytes.Equal(artifact.Payload, expectedPayload) {
		t.Fatalf("artifact payload differs from production projection")
	}
}

func TestArtifactIsByteStableAcrossRuns(t *testing.T) {
	first, err := Artifact()
	if err != nil {
		t.Fatalf("first Artifact() error = %v", err)
	}
	second, err := Artifact()
	if err != nil {
		t.Fatalf("second Artifact() error = %v", err)
	}
	if first.Path != second.Path {
		t.Fatalf("artifact paths differ: %q and %q", first.Path, second.Path)
	}
	if !bytes.Equal(first.Payload, second.Payload) {
		t.Fatal("artifact payload changed across unchanged production runs")
	}
}

func TestArtifactRejectsUnregisteredProjectedTool(t *testing.T) {
	const unregisteredTool = "you.factory_session.inventorygen_probe"
	inventory, err := factorysession.ProjectToolInventoryFromDiscovered([]factorysession.ToolDefinition{{
		Name:        unregisteredTool,
		Description: "probe tool without handler registration",
		InputSchema: map[string]any{"type": "object"},
	}})
	if err != nil {
		t.Fatalf("ProjectToolInventoryFromDiscovered() error = %v", err)
	}

	artifact, err := artifactFromInventory(inventory)
	if err == nil {
		t.Fatal("artifactFromInventory() error = nil, want handler verification failure")
	}
	if !strings.Contains(err.Error(), unregisteredTool) {
		t.Fatalf("artifactFromInventory() error = %v, want offending tool %q", err, unregisteredTool)
	}
	if artifact.Path != "" || artifact.Payload != nil {
		t.Fatalf("failed artifact = %#v, want empty artifact", artifact)
	}
}
