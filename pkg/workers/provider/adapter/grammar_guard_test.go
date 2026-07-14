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
		})
	}
}
