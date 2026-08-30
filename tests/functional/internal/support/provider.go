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

// CodexSuccessStdoutWithUsage emits the same sanitized Codex JSONL fixture as
// CodexSuccessStdout while allowing a functional test to prove provider usage
// metadata at the command-runner edge.
func CodexSuccessStdoutWithUsage(result string, inputTokens, outputTokens int64, cachedInputTokens ...int64) []byte {
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
	usage := map[string]any{
		"input_tokens":  inputTokens,
		"output_tokens": outputTokens,
	}
	if len(cachedInputTokens) > 0 {
		usage["cached_input_tokens"] = cachedInputTokens[0]
	}
	turnCompleted, err := json.Marshal(map[string]any{
		"type":  "turn.completed",
		"usage": usage,
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
