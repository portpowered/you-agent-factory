package projections_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	factoryeventprojection "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryeventprojection"

	"github.com/portpowered/infinite-you/pkg/factory/projections"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestReconstructFactoryWorldState_JavaScriptCheckpointRefEventProjectsArtifactMetadata(t *testing.T) {
	timestamp := time.Date(2026, 6, 8, 16, 0, 0, 0, time.UTC)
	hash := "sha256:checkpoint-body"
	size := int64(128)
	payload := factoryapi.JavaScriptCheckpointRefEventPayload{
		CheckpointId: "ckpt-1",
		Label:        stringPointer("after-plan"),
		Summary:      stringPointer("Completed planning phase"),
		Timestamp:    &timestamp,
		ArtifactRef: factoryapi.FactoryArtifactRef{
			Id:          "artifact-ckpt-1",
			Kind:        factoryapi.FactoryArtifactKindCHECKPOINT,
			Visibility:  factoryapi.FactoryArtifactVisibilityINTERNALCHECKPOINT,
			ContentHash: &hash,
			SizeBytes:   &size,
		},
	}
	var eventPayload factoryapi.FactoryEvent_Payload
	if err := eventPayload.FromJavaScriptCheckpointRefEventPayload(payload); err != nil {
		t.Fatalf("marshal checkpoint event payload: %v", err)
	}
	events := []factoryapi.FactoryEvent{{
		SchemaVersion: factoryapi.AgentFactoryEventV1,
		Id:            "evt-checkpoint-1",
		Type:          factoryapi.FactoryEventTypeJavaScriptCheckpointRef,
		Context: factoryapi.FactoryEventContext{
			Tick:      1,
			Sequence:  1,
			EventTime: timestamp,
		},
		Payload: eventPayload,
	}}

	worldState, err := factoryeventprojection.ReconstructFactoryWorldState(events, 1)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}
	if len(worldState.JavaScriptCheckpoints) != 1 {
		t.Fatalf("javascript checkpoints = %#v, want one ref", worldState.JavaScriptCheckpoints)
	}
	checkpoint := worldState.JavaScriptCheckpoints[0]
	if checkpoint.ArtifactRef == nil || checkpoint.ArtifactRef.Visibility != "INTERNAL_CHECKPOINT" {
		t.Fatalf("checkpoint artifact ref = %#v", checkpoint.ArtifactRef)
	}

	view := projections.BuildFactoryWorldView(worldState)
	if view.Runtime.JavaScript == nil || len(view.Runtime.JavaScript.Checkpoints) != 1 {
		t.Fatalf("world view javascript projection = %#v", view.Runtime.JavaScript)
	}
	encoded, err := json.Marshal(view)
	if err != nil {
		t.Fatalf("marshal world view: %v", err)
	}
	body := string(encoded)
	for _, forbidden := range []string{"rawBody", "storagePath", "vmState"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("world view leaked %q: %s", forbidden, body)
		}
	}
}

func stringPointer(value string) *string {
	return &value
}
