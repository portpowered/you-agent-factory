package diagnostics

import (
	"encoding/json"
	"reflect"
	"testing"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
)

func TestSafeWorkDiagnosticsAllowlistAndCloneIsolation(t *testing.T) {
	t.Parallel()
	safe := SafeWorkDiagnosticsFromWorkDiagnostics(&workerexecution.WorkDiagnostics{
		Provider:       &workerexecution.ProviderDiagnostic{RequestMetadata: map[string]string{"request_id": "req-1", "authorization": "secret"}},
		RenderedPrompt: &workerexecution.RenderedPromptDiagnostic{Variables: map[string]string{"trace_id": "trace-1", "api_key": "secret"}},
	})
	if safe.Provider.RequestMetadata["request_id"] != "req-1" {
		t.Fatal("safe request metadata was dropped")
	}
	if _, ok := safe.Provider.RequestMetadata["authorization"]; ok {
		t.Fatal("secret provider metadata leaked")
	}
	if _, ok := safe.RenderedPrompt.Variables["api_key"]; ok {
		t.Fatal("secret prompt variable leaked")
	}
	clone := CloneSafeWorkDiagnostics(safe)
	clone.Provider.RequestMetadata["request_id"] = "changed"
	if safe.Provider.RequestMetadata["request_id"] != "req-1" {
		t.Fatal("safe diagnostics clone was not detached")
	}
}

func TestWorkDiagnosticsFromSafeEventPayloadDecodesCamelCaseWireShape(t *testing.T) {
	t.Parallel()
	diagnostics, err := WorkDiagnosticsFromSafeEventPayload(json.RawMessage(`{
		"renderedPrompt":{"systemPromptHash":"system-hash","variables":{"request_id":"request-1"}},
		"provider":{"provider":"codex","requestMetadata":{"request_id":"request-1"}},
		"invocation":{"signatureHash":"signature-hash","parameters":[{"name":"prompt","sourceKinds":["text"],"valueCount":1,"redacted":true}]}
	}`))
	if err != nil {
		t.Fatalf("WorkDiagnosticsFromSafeEventPayload: %v", err)
	}
	if diagnostics.RenderedPrompt.SystemPromptHash != "system-hash" || diagnostics.RenderedPrompt.Variables["request_id"] != "request-1" {
		t.Fatalf("rendered prompt = %#v, want camel-case event fields", diagnostics.RenderedPrompt)
	}
	if diagnostics.Provider.Provider != "codex" || diagnostics.Provider.RequestMetadata["request_id"] != "request-1" {
		t.Fatalf("provider = %#v, want safe provider metadata", diagnostics.Provider)
	}
	if diagnostics.Invocation.SignatureHash != "signature-hash" || diagnostics.Invocation.Parameters[0].ValueCount != 1 {
		t.Fatalf("invocation = %#v, want normalized invocation fields", diagnostics.Invocation)
	}
}

func TestWorkDiagnosticsFromSafeEventPayloadRejectsMalformedJSON(t *testing.T) {
	t.Parallel()
	if _, err := WorkDiagnosticsFromSafeEventPayload(json.RawMessage(`{"provider":`)); err == nil {
		t.Fatal("WorkDiagnosticsFromSafeEventPayload error = nil, want malformed JSON error")
	}
}

func TestSafeWorkDiagnosticsEventPayloadPreservesPublicFieldNamesAndMetadataKeys(t *testing.T) {
	diagnostics := &SafeWorkDiagnostics{
		RenderedPrompt: &SafeRenderedPromptDiagnostic{
			SystemPromptHash: "system-hash",
			Variables:        map[string]string{"caller_key": "value"},
		},
		Provider: &SafeProviderDiagnostic{
			RequestMetadata: map[string]string{"provider_key": "value"},
		},
	}

	payload, err := SafeWorkDiagnosticsEventPayload(diagnostics)
	if err != nil {
		t.Fatalf("SafeWorkDiagnosticsEventPayload: %v", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(payload, &fields); err != nil {
		t.Fatalf("decode event payload: %v", err)
	}
	rendered := fields["renderedPrompt"].(map[string]any)
	if rendered["systemPromptHash"] != "system-hash" {
		t.Fatalf("renderedPrompt.systemPromptHash = %#v", rendered["systemPromptHash"])
	}
	if rendered["variables"].(map[string]any)["caller_key"] != "value" {
		t.Fatalf("variables = %#v, want caller-owned key", rendered["variables"])
	}
	provider := fields["provider"].(map[string]any)
	if provider["requestMetadata"].(map[string]any)["provider_key"] != "value" {
		t.Fatalf("requestMetadata = %#v, want caller-owned key", provider["requestMetadata"])
	}

	decoded, err := SafeWorkDiagnosticsFromEventPayload(payload)
	if err != nil {
		t.Fatalf("SafeWorkDiagnosticsFromEventPayload: %v", err)
	}
	if !reflect.DeepEqual(decoded, diagnostics) {
		t.Fatalf("round trip = %#v, want %#v", decoded, diagnostics)
	}
}
