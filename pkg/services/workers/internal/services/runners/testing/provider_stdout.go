package mockworker

import (
	"encoding/json"
	"strings"

	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/process"
)

func mockAcceptStdout(command string, text string) string {
	switch strings.TrimSpace(command) {
	case "codex":
		return mockCodexAcceptStdout(text)
	case "claude":
		return mockClaudeAcceptStdout(text)
	default:
		return text
	}
}

func mockCodexAcceptStdout(text string) string {
	payload, err := json.Marshal(map[string]any{
		"type": "item.completed",
		"item": map[string]any{
			"id":   "message-final",
			"type": "agent_message",
			"text": text,
		},
	})
	if err != nil {
		return text
	}
	return string(payload) + "\n"
}

func mockClaudeAcceptStdout(text string) string {
	records := []string{
		mustMarshalJSON(map[string]any{
			"type":       "system",
			"subtype":    "init",
			"session_id": "mock-claude-session",
		}),
		mustMarshalJSON(map[string]any{
			"type":       "result",
			"subtype":    "success",
			"is_error":   false,
			"result":     text,
			"session_id": "mock-claude-session",
		}),
	}
	return strings.Join(records, "\n") + "\n"
}

func mockRejectResult(command string, cfg *MockWorkerRejectConfig) workerprocess.CommandResult {
	result := rejectResult(cfg)
	switch strings.TrimSpace(command) {
	case "codex":
		result.Stdout = []byte(mockCodexRejectStdout())
		result.Stderr = nil
	case "claude":
		result.Stdout = []byte(mockClaudeRejectStdout())
		result.Stderr = nil
	}
	return result
}

func mockCodexRejectStdout() string {
	payload, err := json.Marshal(map[string]any{
		"type": "turn.failed",
		"error": map[string]any{
			"message": "mock worker rejected the dispatch",
		},
	})
	if err != nil {
		panic(err)
	}
	return string(payload) + "\n"
}

func mockClaudeRejectStdout() string {
	records := []string{
		mustMarshalJSON(map[string]any{
			"type":       "system",
			"subtype":    "init",
			"session_id": "mock-claude-session",
		}),
		mustMarshalJSON(map[string]any{
			"type":       "result",
			"subtype":    "error",
			"is_error":   true,
			"result":     "mock worker rejected the dispatch",
			"session_id": "mock-claude-session",
		}),
	}
	return strings.Join(records, "\n") + "\n"
}

func mustMarshalJSON(value any) string {
	payload, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(payload)
}
