package acceptance

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type directWorkerSessionCLIResult struct {
	RequestID                string `json:"requestId"`
	WorkerSessionID          string `json:"workerSessionId"`
	SourceWorkerSessionID    string `json:"sourceWorkerSessionId"`
	SuccessorWorkerSessionID string `json:"successorWorkerSessionId"`
	Accepted                 bool   `json:"accepted"`
	State                    string `json:"state"`
	Output                   string `json:"output"`
}

func TestDirectWorkerSessionInvokeContinueLocalPreservesSessionAndLineage(t *testing.T) {
	if testing.Short() {
		t.Skip("root-built direct Worker Session invoke/continue functional flow")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	runner := testutil.NewProviderCommandRunner(
		platformprocess.CommandResult{Stdout: directCodexSessionOutput("local-source-thread", "initial direct output COMPLETE")},
		platformprocess.CommandResult{Stdout: directCodexSessionOutput("local-source-thread", "continued direct output COMPLETE")},
	)
	process := support.BuildProcess(t, serviceedges.Edges{ProviderCommandRunner: runner})
	support.CleanupProcess(t, process)

	homeDir := t.TempDir()
	workingDirectory := t.TempDir()
	env := append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)

	invoke := support.FakeInputs(ctx, []string{
		"you", "--json", "worker-sessions", "invoke",
		"--request-id", "local-invoke-request",
		"--worker-session-id", "local-source-session",
		"--dispatch-id", "local-source-dispatch",
		"--workstation", "direct",
		"--worker-type", "direct-worker",
		"--runner", "codex",
		"--provider", "codex",
		"--model", "functional-model",
		"--user-message", "initial direct prompt",
	})
	invoke.Input.Env = env
	invoke.Input.WorkingDirectory = workingDirectory
	if err := process.Execute(invoke.Input); err != nil {
		t.Fatalf("local invoke: %v\nrequests:%#v\nstdout:\n%s\nstderr:\n%s", err, runner.Requests(), invoke.Stdout(), invoke.Stderr())
	}
	var invoked directWorkerSessionCLIResult
	decodeDirectWorkerSessionResult(t, invoke.Stdout(), &invoked)
	if !invoked.Accepted || invoked.RequestID != "local-invoke-request" ||
		invoked.WorkerSessionID != "local-source-session" || invoked.State != "COMPLETED" {
		t.Fatalf("local invoke result = %#v, want accepted completed source", invoked)
	}
	if !strings.Contains(invoked.Output, "initial direct output COMPLETE") {
		t.Fatalf("local invoke output = %q, want provider output\nstdout:\n%s\nstderr:\n%s", invoked.Output, invoke.Stdout(), invoke.Stderr())
	}

	cont := support.FakeInputs(ctx, []string{
		"you", "--json", "worker-sessions", "continue", "local-source-session",
		"--request-id", "local-continue-request",
		"--successor-worker-session-id", "local-successor-session",
		"--user-message", "continued direct prompt",
	})
	cont.Input.Env = env
	cont.Input.WorkingDirectory = workingDirectory
	if err := process.Execute(cont.Input); err != nil {
		t.Fatalf("local continuation: %v\nrequests:%#v\nstdout:\n%s\nstderr:\n%s", err, runner.Requests(), cont.Stdout(), cont.Stderr())
	}
	var continued directWorkerSessionCLIResult
	decodeDirectWorkerSessionResult(t, cont.Stdout(), &continued)
	if !continued.Accepted || continued.RequestID != "local-continue-request" ||
		continued.SourceWorkerSessionID != "local-source-session" ||
		continued.SuccessorWorkerSessionID != "local-successor-session" || continued.State != "COMPLETED" {
		t.Fatalf("local continuation result = %#v, want accepted successor lineage", continued)
	}
	if !strings.Contains(continued.Output, "continued direct output COMPLETE") {
		t.Fatalf("local continuation output = %q, want provider output", continued.Output)
	}

	requests := runner.Requests()
	if len(requests) != 2 {
		t.Fatalf("provider command requests = %d, want initial and continuation only", len(requests))
	}
	if strings.Contains(strings.Join(requests[0].Args, " "), "resume") {
		t.Fatalf("initial provider command unexpectedly resumed a session: %#v", requests[0].Args)
	}
	continuationArgs := strings.Join(requests[1].Args, " ")
	if !strings.Contains(continuationArgs, "resume") || !strings.Contains(continuationArgs, "local-source-thread") {
		t.Fatalf("continuation provider command = %#v, want resume local-source-thread", requests[1].Args)
	}
}

func decodeDirectWorkerSessionResult(t *testing.T, stdout string, result any) {
	t.Helper()
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), result); err != nil {
		t.Fatalf("decode direct Worker Session result: %v\nstdout:\n%s", err, stdout)
	}
}

func directCodexSessionOutput(sessionID, content string) []byte {
	thread, _ := json.Marshal(map[string]any{
		"type":      "thread.started",
		"thread_id": sessionID,
	})
	item, _ := json.Marshal(map[string]any{
		"type": "item.completed",
		"item": map[string]any{
			"id":   sessionID + "-message",
			"type": "agent_message",
			"text": content,
		},
	})
	completed := []byte(`{"type":"turn.completed","usage":{"input_tokens":1,"output_tokens":1}}`)
	return append(append(append(thread, '\n'), item...), append([]byte{'\n'}, completed...)...)
}
