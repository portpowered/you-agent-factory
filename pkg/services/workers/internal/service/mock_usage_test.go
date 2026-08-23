package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers/internal/execution"
)

func TestApplyMockWorkerUsageDiagnosticsPreservesOmittedClassesAndIdentity(t *testing.T) {
	oldInput, oldOutput, oldCached := int64(9), int64(8), int64(3)
	result := workers.RunnerExecutionResult{
		Content: "accepted",
		Diagnostics: &workers.WorkDiagnostics{
			Provider: &workers.ProviderDiagnostic{
				Provider: "old-provider", Model: "old-model",
				ResponseMetadata: map[string]string{"old": "value"},
			},
			Metadata: map[string]string{
				workers.ProviderResponseMetadataInputTokens:       "9",
				workers.ProviderResponseMetadataOutputTokens:      "8",
				workers.ProviderResponseMetadataCachedInputTokens: "3",
				"keep": "value",
			},
		},
	}
	usage := &workers.MockWorkerUsageConfig{
		Provider: " codex ", Model: " gpt-5-codex ",
		InputTokens: &oldInput, OutputTokens: &oldOutput,
		ReasoningOutputTokens: &oldCached,
	}

	got := applyMockWorkerUsageDiagnostics(result, usage)
	if got.Content != result.Content || got.Diagnostics == result.Diagnostics {
		t.Fatalf("result = %#v, want content preserved and diagnostics detached", got)
	}
	if got.Diagnostics.Provider.Provider != "codex" || got.Diagnostics.Provider.Model != "gpt-5-codex" {
		t.Fatalf("provider diagnostics = %#v, want trimmed declared identity", got.Diagnostics.Provider)
	}
	metadata := got.Diagnostics.Metadata
	if metadata[workers.ProviderResponseMetadataInputTokens] != "9" ||
		metadata[workers.ProviderResponseMetadataOutputTokens] != "8" ||
		metadata[workers.ProviderResponseMetadataReasoningOutputTokens] != "3" ||
		metadata["keep"] != "value" {
		t.Fatalf("diagnostic metadata = %#v, want declared classes and preserved metadata", metadata)
	}
	if _, ok := metadata[workers.ProviderResponseMetadataCachedInputTokens]; ok {
		t.Fatalf("cached input metadata = %#v, want omitted when declaration is omitted", metadata)
	}
	if result.Diagnostics.Metadata[workers.ProviderResponseMetadataCachedInputTokens] != "3" {
		t.Fatal("applyMockWorkerUsageDiagnostics mutated the original diagnostics")
	}
}

func TestPublishMockWorkerUsageEmitsCanonicalUsageUpdatedFragment(t *testing.T) {
	input, output, reasoning := int64(0), int64(5), int64(0)
	usage := &workers.MockWorkerUsageConfig{
		Provider: "codex", Model: "gpt-5-codex",
		InputTokens: &input, OutputTokens: &output, ReasoningOutputTokens: &reasoning,
	}
	var fragments []workers.ProgressFragment
	ctx := workerexecution.WithProgressPublisher(context.Background(), func(fragment workers.ProgressFragment) {
		fragments = append(fragments, fragment)
	})
	request := workers.ExecuteRequest{Correlation: workers.ExecutionCorrelation{DispatchID: "dispatch-1"}}
	publishMockWorkerUsage(ctx, request, usage)

	if len(fragments) != 1 {
		t.Fatalf("published fragments = %d, want exactly one", len(fragments))
	}
	fragment := fragments[0]
	if fragment.Type != "usage.updated" || fragment.Provider != "codex" || fragment.DispatchID != "dispatch-1" {
		t.Fatalf("fragment = %#v, want canonical usage identity", fragment)
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(fragment.Payload), &payload); err != nil {
		t.Fatalf("usage payload is not valid JSON: %v", err)
	}
	for _, field := range []string{"inputTokens", "outputTokens", "reasoningOutputTokens", "totalTokens", "model"} {
		if _, ok := payload[field]; !ok {
			t.Fatalf("usage payload = %s, missing %q", fragment.Payload, field)
		}
	}
	if _, ok := payload["cachedInputTokens"]; ok {
		t.Fatalf("usage payload = %s, cachedInputTokens should remain omitted", fragment.Payload)
	}
}
