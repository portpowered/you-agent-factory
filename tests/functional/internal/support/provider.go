package support

import (
	"encoding/json"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

var providerErrorCommandResults = map[string]platformprocess.CommandResult{
	"claude_authentication_error": {
		ExitCode: 1,
		Stderr: []byte(
			`API Error: 401 {"type":"error","error":{"type":"authentication_error","message":"invalid api key"}}`,
		),
	},
	"claude_internal_server_api_error": {
		ExitCode: 1,
		Stderr: []byte(
			`API Error: 500 {"type":"error","error":{"type":"api_error","message":"Internal server error"}}`,
		),
	},
	"claude_rate_limit_error": {
		ExitCode: 1,
		Stderr: []byte(
			`API Error: 429 {"type":"error","error":{"type":"rate_limit_error","message":"rate limit exceeded"}}`,
		),
	},
}

func providerErrorCommandResult(t *testing.T, name string) platformprocess.CommandResult {
	t.Helper()

	entry, ok := providerErrorCommandResults[name]
	if !ok {
		t.Fatalf("provider error command result %q not found", name)
	}
	return entry
}

func ProviderErrorCommandResult(t *testing.T, name string) platformprocess.CommandResult {
	t.Helper()
	return providerErrorCommandResult(t, name)
}

func RepeatedProviderErrorCommandResults(t *testing.T, name string, count int) []platformprocess.CommandResult {
	t.Helper()
	entry := providerErrorCommandResult(t, name)
	results := make([]platformprocess.CommandResult, count)
	for i := range results {
		results[i] = entry
	}
	return results
}

func AcceptedProviderResponse() workerexecution.InferenceResponse {
	return workerexecution.InferenceResponse{Content: "COMPLETE"}
}

func RejectedProviderResponse(content string) workerexecution.InferenceResponse {
	return workerexecution.InferenceResponse{Content: content}
}

func CursorProviderSuccessStdout(result string) []byte {
	if result == "" {
		result = "Done. COMPLETE"
	}
	systemPayload := map[string]any{
		"type":       "system",
		"subtype":    "init",
		"session_id": "cursor-functional-test-session",
	}
	resultPayload := map[string]any{
		"type":       "result",
		"subtype":    "success",
		"is_error":   false,
		"result":     result,
		"session_id": "cursor-functional-test-session",
	}
	systemEncoded, err := json.Marshal(systemPayload)
	if err != nil {
		panic(err)
	}
	resultEncoded, err := json.Marshal(resultPayload)
	if err != nil {
		panic(err)
	}
	return append(append(systemEncoded, '\n'), resultEncoded...)
}

func CodexSuccessStdout(result string) []byte {
	if result == "" {
		result = "Done. COMPLETE"
	}
	item, err := json.Marshal(map[string]any{
		"type": "item.completed",
		"item": map[string]any{
			"id":   "codex-functional-message",
			"type": "agent_message",
			"text": result,
		},
	})
	if err != nil {
		panic(err)
	}
	turnCompleted, err := json.Marshal(map[string]any{
		"type":  "turn.completed",
		"usage": map[string]any{"input_tokens": 1, "output_tokens": 1},
	})
	if err != nil {
		panic(err)
	}
	return []byte(
		`{"type":"turn.started"}` + "\n" +
			string(item) + "\n" +
			string(turnCompleted) + "\n",
	)
}

func ClaudeSuccessStdout(result string) []byte {
	if result == "" {
		result = "Done. COMPLETE"
	}
	payload := map[string]any{
		"type":       "result",
		"subtype":    "success",
		"is_error":   false,
		"result":     result,
		"session_id": "claude-functional-test-session",
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}
	return append(encoded, '\n')
}

func AcceptedCommandResults(count int) []platformprocess.CommandResult {
	results := make([]platformprocess.CommandResult, count)
	for i := range results {
		results[i] = platformprocess.CommandResult{Stdout: CodexSuccessStdout("Done. COMPLETE")}
	}
	return results
}

func ProviderCallsForWorker(provider *testutil.MockProvider, workerType string) []workerexecution.ProviderInferenceRequest {
	var calls []workerexecution.ProviderInferenceRequest
	for _, call := range provider.Calls() {
		if call.WorkerType == workerType {
			calls = append(calls, call)
		}
	}
	return calls
}
