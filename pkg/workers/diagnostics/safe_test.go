package diagnostics

import (
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
