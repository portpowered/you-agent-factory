package diagnostics

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	workerenvdiagnostics "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/envdiagnostics"
)

func TestSafeWorkDiagnosticsRedactsHostSpecificWorkingDirectory(t *testing.T) {
	t.Parallel()
	safe := SafeWorkDiagnosticsFromWorkDiagnostics(&WorkDiagnostics{
		Provider: &ProviderDiagnostic{
			RequestMetadata: map[string]string{
				"working_directory": `C:\Users\alice\factory`,
				"worktree":          "feature-runtime",
			},
		},
	})
	if got := safe.Provider.RequestMetadata["working_directory"]; got != workerenvdiagnostics.MetadataOnlyCommandEnvValue {
		t.Fatalf("working_directory = %q, want metadata-only marker", got)
	}
	if got := safe.Provider.RequestMetadata["worktree"]; got != "feature-runtime" {
		t.Fatalf("worktree = %q, want portable branch name preserved", got)
	}

	relative := SafeWorkDiagnosticsFromWorkDiagnostics(&WorkDiagnostics{
		Provider: &ProviderDiagnostic{
			RequestMetadata: map[string]string{
				"working_directory": "workspace/cursor",
			},
		},
	})
	if got := relative.Provider.RequestMetadata["working_directory"]; got != "workspace/cursor" {
		t.Fatalf("relative working_directory = %q, want preserved portable path", got)
	}

	portableAbsolute := SafeWorkDiagnosticsFromWorkDiagnostics(&WorkDiagnostics{
		Provider: &ProviderDiagnostic{
			RequestMetadata: map[string]string{
				"working_directory": `C:\repo`,
			},
		},
	})
	if got := portableAbsolute.Provider.RequestMetadata["working_directory"]; got != `C:\repo` {
		t.Fatalf("portable absolute working_directory = %q, want preserved fixture path", got)
	}
}

func TestSafeWorkDiagnosticsAllowlistAndCloneIsolation(t *testing.T) {
	t.Parallel()
	safe := SafeWorkDiagnosticsFromWorkDiagnostics(&WorkDiagnostics{
		Provider: &ProviderDiagnostic{RequestMetadata: map[string]string{
			"request_id":        "req-1",
			"authorization":     "secret",
			"working_directory": `C:\Users\operator\workspace`,
			"worktree":          `C:\Users\operator\worktree`,
		}},
		RenderedPrompt: &RenderedPromptDiagnostic{Variables: map[string]string{"trace_id": "trace-1", "api_key": "secret"}},
	})
	if safe.Provider.RequestMetadata["request_id"] != "req-1" {
		t.Fatal("safe request metadata was dropped")
	}
	if _, ok := safe.Provider.RequestMetadata["authorization"]; ok {
		t.Fatal("secret provider metadata leaked")
	}
	if got := safe.Provider.RequestMetadata["working_directory"]; got != workerenvdiagnostics.MetadataOnlyCommandEnvValue {
		t.Fatalf("host working directory = %q, want metadata-only marker", got)
	}
	if got := safe.Provider.RequestMetadata["worktree"]; got != workerenvdiagnostics.MetadataOnlyCommandEnvValue {
		t.Fatalf("host worktree = %q, want metadata-only marker", got)
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

func TestSafeWorkDiagnosticsBoundsFailureMetadataAndKeepsCorrelation(t *testing.T) {
	t.Parallel()
	longValue := strings.Repeat("x", maxSafeMetadataValueRunes+100)
	safe := SafeWorkDiagnosticsFromWorkDiagnostics(&WorkDiagnostics{
		Provider: &ProviderDiagnostic{
			RequestMetadata: map[string]string{"dispatch_id": longValue},
			ResponseMetadata: map[string]string{
				"failure_operation":           "provider_session_ingestion",
				"failure_classification":      "resource_limit",
				"failure_stage":               longValue,
				"inspection_limit_category":   "record",
				"inspection_limit_configured": "1048576",
				"inspection_limit_observed":   "1048577",
				"inspection_limit_line":       "2",
				"raw_rollout":                 longValue,
			},
		},
	})
	if safe == nil || safe.Provider == nil {
		t.Fatalf("safe diagnostics = %#v, want provider diagnostics", safe)
	}
	if got := safe.Provider.RequestMetadata["dispatch_id"]; len([]rune(got)) != maxSafeMetadataValueRunes {
		t.Fatalf("bounded dispatch id length = %d, want %d", len([]rune(got)), maxSafeMetadataValueRunes)
	}
	if safe.Provider.ResponseMetadata["failure_operation"] != "provider_session_ingestion" ||
		safe.Provider.ResponseMetadata["failure_classification"] != "resource_limit" {
		t.Fatalf("response metadata = %#v, want stable failure classification", safe.Provider.ResponseMetadata)
	}
	if safe.Provider.ResponseMetadata["inspection_limit_category"] != "record" ||
		safe.Provider.ResponseMetadata["inspection_limit_configured"] != "1048576" ||
		safe.Provider.ResponseMetadata["inspection_limit_observed"] != "1048577" ||
		safe.Provider.ResponseMetadata["inspection_limit_line"] != "2" {
		t.Fatalf("response metadata = %#v, want bounded inspection limit facts", safe.Provider.ResponseMetadata)
	}
	if _, ok := safe.Provider.ResponseMetadata["failure_stage"]; ok {
		t.Fatal("unrecognized failure stage value was retained")
	}
	if _, ok := safe.Provider.ResponseMetadata["raw_rollout"]; ok {
		t.Fatal("raw rollout metadata leaked through the safe diagnostics allowlist")
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
