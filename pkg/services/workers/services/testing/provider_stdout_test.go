package mockworker

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMockAcceptStdout_CodexEmitsJSONL(t *testing.T) {
	stdout := mockAcceptStdout("codex", "mock worker accepted\nSTOP")
	if !strings.HasSuffix(stdout, "\n") {
		t.Fatalf("stdout should end with newline: %q", stdout)
	}
	line := strings.TrimSpace(stdout)
	var payload map[string]any
	if err := json.Unmarshal([]byte(line), &payload); err != nil {
		t.Fatalf("stdout is not valid JSON: %v (%q)", err, stdout)
	}
	if payload["type"] != "item.completed" {
		t.Fatalf("type = %v, want item.completed", payload["type"])
	}
}

func TestMockAcceptStdout_ClaudeEmitsStreamJSON(t *testing.T) {
	stdout := mockAcceptStdout("claude", "mock worker accepted")
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) != 2 {
		t.Fatalf("line count = %d, want 2", len(lines))
	}
	for _, line := range lines {
		var payload map[string]any
		if err := json.Unmarshal([]byte(line), &payload); err != nil {
			t.Fatalf("line is not valid JSON: %v (%q)", err, line)
		}
	}
}

func TestMockAcceptStdout_DefaultPreservesPlainText(t *testing.T) {
	want := "mock worker accepted"
	if got := mockAcceptStdout("", want); got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestMockRejectStdout_CodexEmitsTurnFailedJSONL(t *testing.T) {
	stdout := mockCodexRejectStdout()
	var payload map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &payload); err != nil {
		t.Fatalf("stdout is not valid JSON: %v (%q)", err, stdout)
	}
	if payload["type"] != "turn.failed" {
		t.Fatalf("type = %v, want turn.failed", payload["type"])
	}
}

func TestMockRejectStdout_ClaudeEmitsErrorResult(t *testing.T) {
	stdout := mockClaudeRejectStdout()
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) != 2 {
		t.Fatalf("line count = %d, want 2", len(lines))
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(lines[1]), &result); err != nil {
		t.Fatalf("result line is not valid JSON: %v", err)
	}
	if result["is_error"] != true {
		t.Fatalf("is_error = %v, want true", result["is_error"])
	}
}
