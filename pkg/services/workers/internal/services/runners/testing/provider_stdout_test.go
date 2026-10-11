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

func TestMockAcceptStdout_CodexUsageAddsSessionWithoutNativeUsageRecord(t *testing.T) {
	zero := int64(0)
	stdout := mockAcceptStdout("codex", "mock worker accepted", &MockWorkerUsageConfig{
		Provider: "codex", Model: "gpt-5", InputTokens: &zero,
	})
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) != 2 {
		t.Fatalf("line count = %d, want session and message records", len(lines))
	}
	var session, message map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &session); err != nil {
		t.Fatalf("session line is not valid JSON: %v", err)
	}
	if err := json.Unmarshal([]byte(lines[1]), &message); err != nil {
		t.Fatalf("message line is not valid JSON: %v", err)
	}
	if session["type"] != "thread.started" || session["thread_id"] != "mock-codex-session" {
		t.Fatalf("session record = %#v, want mock Codex session", session)
	}
	if message["type"] != "item.completed" {
		t.Fatalf("message record = %#v, want item.completed", message)
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

func TestMockRejectStdout_CodexUsageAddsSessionWithoutNativeUsageRecord(t *testing.T) {
	stdout := mockCodexRejectStdoutWithSession(true)
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) != 2 {
		t.Fatalf("line count = %d, want session and failure records", len(lines))
	}
	var session, failure map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &session); err != nil {
		t.Fatalf("session line is not valid JSON: %v", err)
	}
	if err := json.Unmarshal([]byte(lines[1]), &failure); err != nil {
		t.Fatalf("failure line is not valid JSON: %v", err)
	}
	if session["type"] != "thread.started" || failure["type"] != "turn.failed" {
		t.Fatalf("records = %#v / %#v, want thread.started / turn.failed", session, failure)
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

func TestMockRejectResultUsesProviderOutputShapes(t *testing.T) {
	for _, command := range []string{"codex", "claude"} {
		result := mockRejectResult(command, nil)
		if len(result.Stdout) == 0 {
			t.Fatalf("%s reject stdout is empty, want provider-shaped output", command)
		}
		if result.Stderr != nil {
			t.Fatalf("%s reject stderr = %q, want provider output to own stderr", command, result.Stderr)
		}
		if result.ExitCode != 1 {
			t.Fatalf("%s reject exit code = %d, want 1", command, result.ExitCode)
		}
	}
}

func TestMockRejectResultPreservesDeclaredStreams(t *testing.T) {
	for _, command := range []string{"codex", "claude", "plain"} {
		for _, streams := range []struct{ name, stdout, stderr string }{
			{"stdout", "ordinary PRIVATE\n世界", ""},
			{"stderr", "", "ordinary stderr\n世界"},
			{"both", "same text\n世界", "same text\n世界"},
			{"empty", "", ""},
		} {
			t.Run(command+"/"+streams.name, func(t *testing.T) {
				t.Parallel()
				exit := 42
				result := mockRejectResult(command, &MockWorkerRejectConfig{Stdout: streams.stdout, Stderr: streams.stderr, ExitCode: &exit})
				if result.ExitCode != exit || string(result.Stderr) != streams.stderr {
					t.Fatalf("exit/stderr = %d/%q, want %d/%q", result.ExitCode, result.Stderr, exit, streams.stderr)
				}
				if command == "plain" {
					if string(result.Stdout) != streams.stdout {
						t.Fatalf("plain stdout = %q", result.Stdout)
					}
					return
				}
				var texts []string
				for _, line := range strings.Split(strings.TrimSpace(string(result.Stdout)), "\n") {
					var record map[string]any
					if err := json.Unmarshal([]byte(line), &record); err != nil {
						t.Fatal(err)
					}
					if record["type"] == "item.updated" {
						texts = append(texts, record["item"].(map[string]any)["text"].(string))
					}
					if record["type"] == "stream_event" {
						block := record["event"].(map[string]any)["content_block"].(map[string]any)
						texts = append(texts, block["text"].(string))
					}
					if record["type"] == "item.completed" || record["is_error"] == false {
						t.Fatalf("rejection supplied a successful result: %s", line)
					}
				}
				if streams.stdout == "" && len(texts) != 0 || streams.stdout != "" && (len(texts) != 1 || texts[0] != streams.stdout) {
					t.Fatalf("declared stdout = %#v, want one %q when nonempty", texts, streams.stdout)
				}
			})
		}
	}
}
