package mockworker

import (
	"encoding/json"
	"strings"

	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/process"
)

func mockAcceptStdout(command string, text string, usage ...*MockWorkerUsageConfig) string {
	switch strings.TrimSpace(command) {
	case "codex":
		if len(usage) > 0 && usage[0] != nil {
			return mockCodexAcceptStdoutWithSession(text)
		}
		return mockCodexAcceptStdout(text)
	case "claude":
		return mockClaudeAcceptStdout(text)
	default:
		return text
	}
}

func mockCodexAcceptStdout(text string) string {
	return mockCodexAcceptStdoutRecords(text, false)
}

func mockCodexAcceptStdoutWithSession(text string) string {
	return mockCodexAcceptStdoutRecords(text, true)
}

func mockCodexAcceptStdoutRecords(text string, withSession bool) string {
	records := make([]string, 0, 2)
	if withSession {
		records = append(records, mustMarshalJSON(map[string]any{
			"type":      "thread.started",
			"thread_id": "mock-codex-session",
		}))
	}
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
	records = append(records, string(payload))
	return strings.Join(records, "\n") + "\n"
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

func mockRejectResult(command string, cfg *MockWorkerRejectConfig, usage ...*MockWorkerUsageConfig) workerprocess.CommandResult {
	result := rejectResult(cfg)
	switch strings.TrimSpace(command) {
	case "codex":
		withSession := len(usage) > 0 && usage[0] != nil
		result.Stdout = []byte(mockCodexRejectStdoutWithSession(withSession))
		result.Stderr = nil
	case "claude":
		result.Stdout = []byte(mockClaudeRejectStdout())
		result.Stderr = nil
	}
	return result
}

func mockCodexRejectStdout() string {
	return mockCodexRejectStdoutWithSession(false)
}

func mockCodexRejectStdoutWithSession(withSession bool) string {
	records := make([]string, 0, 2)
	if withSession {
		records = append(records, mustMarshalJSON(map[string]any{
			"type":      "thread.started",
			"thread_id": "mock-codex-session",
		}))
	}
	payload, err := json.Marshal(map[string]any{
		"type": "turn.failed",
		"error": map[string]any{
			"message": "mock worker rejected the dispatch",
		},
	})
	if err != nil {
		panic(err)
	}
	records = append(records, string(payload))
	return strings.Join(records, "\n") + "\n"
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
