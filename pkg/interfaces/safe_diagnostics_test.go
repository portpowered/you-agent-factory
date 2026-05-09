package interfaces

import (
	"reflect"
	"testing"
)

func TestWorkDiagnosticsFromSafeWorkDiagnostics_NilSafe(t *testing.T) {
	if got := WorkDiagnosticsFromSafeWorkDiagnostics(nil); got != nil {
		t.Fatalf("WorkDiagnosticsFromSafeWorkDiagnostics(nil) = %#v, want nil", got)
	}
	if got := WorkDiagnosticsFromSafeWorkDiagnostics(&SafeWorkDiagnostics{}); got != nil {
		t.Fatalf("WorkDiagnosticsFromSafeWorkDiagnostics(empty) = %#v, want nil", got)
	}
}

func TestWorkDiagnosticsFromSafeWorkDiagnostics_ClonesMutableMaps(t *testing.T) {
	safe := &SafeWorkDiagnostics{
		RenderedPrompt: &SafeRenderedPromptDiagnostic{
			Variables: map[string]string{"prompt_source": "factory"},
		},
		Provider: &SafeProviderDiagnostic{
			RequestMetadata:  map[string]string{"session_id": "req-1"},
			ResponseMetadata: map[string]string{"retry_count": "0"},
		},
	}

	got := WorkDiagnosticsFromSafeWorkDiagnostics(safe)
	got.RenderedPrompt.Variables["prompt_source"] = "mutated"
	got.Provider.RequestMetadata["session_id"] = "mutated"
	got.Provider.ResponseMetadata["retry_count"] = "1"

	if safe.RenderedPrompt.Variables["prompt_source"] != "factory" {
		t.Fatalf("safe rendered prompt variables mutated = %#v", safe.RenderedPrompt.Variables)
	}
	if safe.Provider.RequestMetadata["session_id"] != "req-1" {
		t.Fatalf("safe request metadata mutated = %#v", safe.Provider.RequestMetadata)
	}
	if safe.Provider.ResponseMetadata["retry_count"] != "0" {
		t.Fatalf("safe response metadata mutated = %#v", safe.Provider.ResponseMetadata)
	}
}

func TestSafeWorkDiagnosticsRoundTrip_PreservesSafeFieldsOnly(t *testing.T) {
	original := &WorkDiagnostics{
		RenderedPrompt: &RenderedPromptDiagnostic{
			SystemPromptHash: "system-hash",
			UserMessageHash:  "user-hash",
			Variables: map[string]string{
				"prompt_source": "factory",
				"request_id":    "req-1",
				"secret":        "drop-me",
			},
		},
		Provider: &ProviderDiagnostic{
			Provider: "openai",
			Model:    "gpt-5.4",
			RequestMetadata: map[string]string{
				"session_id": "sess-1",
				"unsafe":     "drop-me",
			},
			ResponseMetadata: map[string]string{
				"retry_count": "0",
				"raw_body":    "drop-me",
			},
		},
		Command: &CommandDiagnostic{
			Command: "python",
			Stdin:   "raw prompt",
		},
		Panic: &PanicDiagnostic{
			Message: "boom",
			Stack:   "stack",
		},
		Metadata: map[string]string{"arbitrary": "drop-me"},
	}

	safe := SafeWorkDiagnosticsFromWorkDiagnostics(original)
	rehydrated := WorkDiagnosticsFromSafeWorkDiagnostics(safe)

	want := &WorkDiagnostics{
		RenderedPrompt: &RenderedPromptDiagnostic{
			SystemPromptHash: "system-hash",
			UserMessageHash:  "user-hash",
			Variables: map[string]string{
				"prompt_source": "factory",
				"request_id":    "req-1",
			},
		},
		Provider: &ProviderDiagnostic{
			Provider: "openai",
			Model:    "gpt-5.4",
			RequestMetadata: map[string]string{
				"session_id": "sess-1",
			},
			ResponseMetadata: map[string]string{
				"retry_count": "0",
			},
		},
	}

	if !reflect.DeepEqual(rehydrated, want) {
		t.Fatalf("rehydrated diagnostics = %#v, want %#v", rehydrated, want)
	}
	if rehydrated.Command != nil {
		t.Fatalf("rehydrated command diagnostics = %#v, want nil", rehydrated.Command)
	}
	if rehydrated.Panic != nil {
		t.Fatalf("rehydrated panic diagnostics = %#v, want nil", rehydrated.Panic)
	}
	if rehydrated.Metadata != nil {
		t.Fatalf("rehydrated metadata = %#v, want nil", rehydrated.Metadata)
	}
}
