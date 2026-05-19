package interfaces

import (
	"reflect"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
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

func TestGeneratedSafeWorkDiagnostics_ClonesMapsAndPreservesNil(t *testing.T) {
	diagnostics := &SafeWorkDiagnostics{
		RenderedPrompt: &SafeRenderedPromptDiagnostic{
			Variables: map[string]string{"prompt_source": "factory"},
		},
		Provider: &SafeProviderDiagnostic{
			RequestMetadata:  map[string]string{"session_id": "req-1"},
			ResponseMetadata: map[string]string{"retry_count": "0"},
		},
	}

	generated := GeneratedSafeWorkDiagnostics(diagnostics)
	(*generated.RenderedPrompt.Variables)["prompt_source"] = "mutated"
	(*generated.Provider.RequestMetadata)["session_id"] = "mutated"
	(*generated.Provider.ResponseMetadata)["retry_count"] = "1"

	if diagnostics.RenderedPrompt.Variables["prompt_source"] != "factory" {
		t.Fatalf("safe rendered prompt variables mutated = %#v", diagnostics.RenderedPrompt.Variables)
	}
	if diagnostics.Provider.RequestMetadata["session_id"] != "req-1" {
		t.Fatalf("safe request metadata mutated = %#v", diagnostics.Provider.RequestMetadata)
	}
	if diagnostics.Provider.ResponseMetadata["retry_count"] != "0" {
		t.Fatalf("safe response metadata mutated = %#v", diagnostics.Provider.ResponseMetadata)
	}

	if got := GeneratedSafeWorkDiagnostics(&SafeWorkDiagnostics{
		RenderedPrompt: &SafeRenderedPromptDiagnostic{Variables: nil},
		Provider: &SafeProviderDiagnostic{
			RequestMetadata:  nil,
			ResponseMetadata: map[string]string{},
		},
	}); got == nil {
		t.Fatal("GeneratedSafeWorkDiagnostics returned nil, want non-nil diagnostics shell")
	} else {
		assertNilStringMapPtr(t, got.RenderedPrompt.Variables, "rendered prompt variables")
		assertNilStringMapPtr(t, got.Provider.RequestMetadata, "request metadata")
		assertNilStringMapPtr(t, got.Provider.ResponseMetadata, "response metadata")
	}
}

func TestSafeWorkDiagnosticsFromGenerated_ClonesMapsAndPreservesNil(t *testing.T) {
	diagnostics := &factoryapi.SafeWorkDiagnostics{
		RenderedPrompt: &factoryapi.RenderedPromptDiagnostic{
			Variables: &factoryapi.StringMap{"prompt_source": "factory"},
		},
		Provider: &factoryapi.ProviderDiagnostic{
			RequestMetadata:  &factoryapi.StringMap{"session_id": "req-1"},
			ResponseMetadata: &factoryapi.StringMap{"retry_count": "0"},
		},
	}

	got := SafeWorkDiagnosticsFromGenerated(diagnostics)
	(*diagnostics.RenderedPrompt.Variables)["prompt_source"] = "mutated"
	(*diagnostics.Provider.RequestMetadata)["session_id"] = "mutated"
	(*diagnostics.Provider.ResponseMetadata)["retry_count"] = "1"

	if got.RenderedPrompt.Variables["prompt_source"] != "factory" {
		t.Fatalf("rendered prompt variables = %#v, want detached copy", got.RenderedPrompt.Variables)
	}
	if got.Provider.RequestMetadata["session_id"] != "req-1" {
		t.Fatalf("request metadata = %#v, want detached copy", got.Provider.RequestMetadata)
	}
	if got.Provider.ResponseMetadata["retry_count"] != "0" {
		t.Fatalf("response metadata = %#v, want detached copy", got.Provider.ResponseMetadata)
	}

	if got := SafeWorkDiagnosticsFromGenerated(&factoryapi.SafeWorkDiagnostics{
		RenderedPrompt: &factoryapi.RenderedPromptDiagnostic{Variables: nil},
		Provider: &factoryapi.ProviderDiagnostic{
			RequestMetadata:  nil,
			ResponseMetadata: &factoryapi.StringMap{},
		},
	}); got == nil {
		t.Fatal("SafeWorkDiagnosticsFromGenerated returned nil, want non-nil diagnostics shell")
	} else {
		if got.RenderedPrompt.Variables != nil {
			t.Fatalf("rendered prompt variables = %#v, want nil", got.RenderedPrompt.Variables)
		}
		if got.Provider.RequestMetadata != nil {
			t.Fatalf("request metadata = %#v, want nil", got.Provider.RequestMetadata)
		}
		if got.Provider.ResponseMetadata != nil {
			t.Fatalf("response metadata = %#v, want nil", got.Provider.ResponseMetadata)
		}
	}
}

func assertNilStringMapPtr(t *testing.T, got *factoryapi.StringMap, field string) {
	t.Helper()
	if got != nil {
		t.Fatalf("%s = %#v, want nil", field, got)
	}
}
