package adapter_test

import (
	"os"
	"strings"
	"testing"
)

func TestAdapterKernelSourcesExcludeProviderNativeEventGrammar(t *testing.T) {
	t.Parallel()

	forbidden := []string{
		"thread.started",
		"item.completed",
		"response.output_text",
		"stream_event",
		"message_start",
		"content_block_delta",
		"assistant",
		"tool_call",
		"turn.started",
		"quasar:",
	}
	for _, file := range []string{"registry.go", "orchestration.go", "contract.go"} {
		t.Run(file, func(t *testing.T) {
			assertSourceExcludesProviderNativeGrammar(t, file, forbidden)
		})
	}
}

func TestSharedOrchestrationSourcesExcludeProviderNativeEventGrammar(t *testing.T) {
	t.Parallel()

	forbidden := []string{
		"thread.started",
		"item.completed",
		"response.output_text",
		"stream_event",
		"message_start",
		"content_block_delta",
		"tool_call",
		"turn.started",
		"turn.failed",
		"readToolCall",
		"stream-json",
	}
	for _, file := range []string{
		"../inference_progress.go",
		"../inference_provider.go",
		"../provider_errors.go",
	} {
		t.Run(file, func(t *testing.T) {
			assertSourceExcludesProviderNativeGrammar(t, file, forbidden)
		})
	}
}

func assertSourceExcludesProviderNativeGrammar(t *testing.T, file string, forbidden []string) {
	t.Helper()

	body, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	source := string(body)
	for _, pattern := range forbidden {
		if strings.Contains(source, pattern) {
			t.Fatalf("%s must not embed provider-native grammar %q", file, pattern)
		}
	}
}
