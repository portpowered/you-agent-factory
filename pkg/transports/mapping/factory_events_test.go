package apisurface

import (
	"encoding/json"
	"strings"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestFactoryEventsToAPIRejectsMalformedCanonicalPayload(t *testing.T) {
	_, err := FactoryEventsToAPI([]interfaces.FactoryEvent{{
		Id:            "factory-event/run-finished",
		Payload:       json.RawMessage(`{"state":`),
		SchemaVersion: interfaces.FactoryEventSchemaVersionV1,
		Type:          interfaces.FactoryEventTypeRunResponse,
	}})
	if err == nil || !strings.Contains(err.Error(), "factory-event/run-finished") {
		t.Fatalf("malformed canonical event mapping error = %v, want event identity", err)
	}
}
