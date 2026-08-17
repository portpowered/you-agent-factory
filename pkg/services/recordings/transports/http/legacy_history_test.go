package http

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
)

func TestLegacyFactoryEventWriterProjectsProviderSession(t *testing.T) {
	t.Parallel()

	payload, err := json.Marshal(map[string]any{
		"attempt":            1,
		"continuation":       providers.ContinuationRef{Provider: "antigravity", Kind: providers.SessionIDKind, ProviderSessionID: "provider-session-1"},
		"durationMillis":     12,
		"inferenceRequestId": "inference-request-1",
		"outcome":            "SUCCEEDED",
		"response":           "ok",
	})
	if err != nil {
		t.Fatalf("marshal event payload: %v", err)
	}
	event := interfaces.FactoryEvent{
		Id:            "event-inference-response-1",
		Payload:       payload,
		SchemaVersion: interfaces.FactoryEventSchemaVersionV1,
		Type:          interfaces.FactoryEventTypeInferenceResponse,
	}
	recorder := httptest.NewRecorder()

	if err := legacyFactoryEventWriter(recorder)(event); err != nil {
		t.Fatalf("legacyFactoryEventWriter: %v", err)
	}
	body := recorder.Body.String()
	if !strings.Contains(body, `"providerSession":{"provider":"antigravity","kind":"session_id","id":"provider-session-1"}`) {
		t.Fatalf("SSE body = %s, want projected provider session", body)
	}
	if strings.Contains(body, `"continuation"`) {
		t.Fatalf("SSE body = %s, must not expose the worker continuation", body)
	}
}
