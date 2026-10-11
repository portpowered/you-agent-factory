package acceptance

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	api "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func newCapturedCodexPartialRunner() *capturedCodexToolRunner {
	output := []byte(`{"type":"thread.started","thread_id":"captured-partial-thread"}
{"type":"item.started","item":{"id":"partial","type":"command_execution","command":"private-command","status":"in_progress"}}
{"type":"item.updated","item":{"id":"partial","type":"command_execution","aggregated_output":"private-output","status":"in_progress"}}
{"type":"item.completed","item":{"id":"answer","type":"agent_message","text":"must never arrive"}}
`)
	r := &capturedCodexToolRunner{invokeContinueResettableProviderCommandRunner: newInvokeContinueResettableProviderCommandRunner(platformprocess.CommandResult{Stdout: output})}
	r.reset()
	return r
}

// FC-13 observes the real capture/public READ boundary on the reusable process.
// The command edge gates completion; stopping joins that exact source before
// terminal reads. The sibling owns another route, attempt and capture.
func TestCapturedCodexToolTerminationPreservesPartialFacts(t *testing.T) {
	t.Parallel()
	fixture := ensureInvokeContinuePackageFixture(t)
	scenario := fixture.scenario(t, "captured-codex-partial")
	defer scenario.close(t)
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	id := scenarioScopedID(scenario, "partial")
	runner := scenario.providerRunner.(*capturedCodexToolRunner)
	invoke := t7RemoteCLIInputs(scenario, ctx, fixture.baseURL, "invoke", "--async", "--execution", requesterExecutionPath(t, scenario, id))
	if err := fixture.process.Execute(invoke.Input); err != nil {
		t.Fatal("partial tool admission failed")
	}
	t.Cleanup(func() { stopNativeContinuationWorker(t, scenario, id, true) })
	t19AwaitSignal(t, ctx, runner.started, "partial tool emitted")
	// The production append is asynchronous to the edge callback. Wait on the
	// public committed watermark, rather than assuming callback means durable.
	before := awaitCapturedCodexPartial(t, ctx, fixture, id)
	assertCapturedCodexPartialReads(t, ctx, fixture, scenario, id, before, false)
	assertCapturedCodexStaleAttempt(t, ctx, fixture, scenario, id, before)
	assertCapturedCodexUnknownReads(t, ctx, fixture, scenario, id+"-unknown")
	peer := fixture.scenario(t, "captured-codex-partial-peer")
	defer peer.close(t)
	peerID := scenarioScopedID(peer, "peer")
	peerInvoke := t7RemoteCLIInputs(peer, ctx, fixture.baseURL, "invoke", "--execution", requesterExecutionPath(t, peer, peerID))
	if err := fixture.process.Execute(peerInvoke.Input); err != nil {
		t.Fatal("partial tool sibling admission failed")
	}
	awaitContinuationRestartLogs(t, invokeContinueStartedProcess{process: fixture.process, baseURL: fixture.baseURL}, peer.homeDirectory, peer.workingDirectory, peerID, "captured-tools-thread")
	peerBefore := support.GetJSON[api.WorkerSessionLogPage](t, fixture.baseURL+"/worker-sessions/"+peerID+"/logs")
	stopNativeContinuationWorker(t, scenario, id, true)
	t19AwaitSignal(t, ctx, runner.stopped, "partial command joined")
	after := support.GetJSON[api.WorkerSessionLogPage](t, fixture.baseURL+"/worker-sessions/"+id+"/logs")
	if len(after.Events) < len(before.Events) || !reflect.DeepEqual(before.Events, after.Events[:len(before.Events)]) {
		t.Fatal("termination changed acknowledged partial prefix")
	}
	assertCapturedCodexPartialReads(t, ctx, fixture, scenario, id, after, true)
	observation := support.GetJSON[api.WorkerSessionObservation](t, fixture.baseURL+"/worker-sessions/"+id)
	if observation.State != "TERMINATED" || observation.EndedAt == nil || observation.DurationMillis == nil || observation.TerminalCause == nil || *observation.TerminalCause != "OPERATOR_TERMINATE" {
		t.Fatalf("partial terminal metadata: state=%s end=%v duration=%v cause=%v", observation.State, observation.EndedAt, observation.DurationMillis, observation.TerminalCause)
	}
	peerAfter := support.GetJSON[api.WorkerSessionLogPage](t, fixture.baseURL+"/worker-sessions/"+peerID+"/logs")
	if !reflect.DeepEqual(peerBefore, peerAfter) || runner.CallCount() != 1 || peer.providerRunner.CallCount() != 1 {
		t.Fatal("partial termination changed sibling capture or reran execution")
	}
}

func awaitCapturedCodexPartial(t *testing.T, ctx context.Context, fixture *invokeContinuePackageFixture, id string) api.WorkerSessionLogPage {
	t.Helper()
	// Durable commit has no edge-fixture callback; poll its public observation.
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		page := support.GetJSON[api.WorkerSessionLogPage](t, fixture.baseURL+"/worker-sessions/"+id+"/logs")
		count := 0
		for _, event := range page.Events {
			if event.Event.Payload["kind"] == "TOOL" {
				count++
			}
		}
		if count == 2 {
			return page
		}
		select {
		case <-ctx.Done():
			t.Fatal("partial tool prefix did not commit")
		case <-ticker.C:
		}
	}
}

func assertCapturedCodexPartialReads(t *testing.T, ctx context.Context, fixture *invokeContinuePackageFixture, scenario *invokeContinueScenario, id string, page api.WorkerSessionLogPage, terminal bool) {
	t.Helper()
	read := t7RemoteCLIInputs(scenario, ctx, fixture.baseURL, "read", "--worker-session-id", id, "--view", "logs")
	if err := fixture.process.Execute(read.Input); err != nil {
		t.Fatal("partial CLI capture read failed")
	}
	var cli, mcp api.WorkerSessionLogPage
	if json.Unmarshal([]byte(read.Stdout()), &cli) != nil || json.Unmarshal(capturedCodexMCPRead(t, ctx, fixture, scenario, id, "logs"), &mcp) != nil || !reflect.DeepEqual(page, cli) || !reflect.DeepEqual(page, mcp) {
		t.Fatal("partial CLI/HTTP/MCP logs disagree")
	}
	assertCapturedCodexPartialLogs(t, page)
	if !terminal {
		assertCapturedCodexUnavailableTranscript(t, ctx, fixture, scenario, id, false)
		return
	}
	status, body := t7HTTP(t, ctx, http.MethodGet, fixture.baseURL+"/worker-sessions/"+id+"/transcript", nil)
	var transcript api.WorkerSessionTranscriptResponse
	if status != http.StatusOK || json.Unmarshal([]byte(body), &transcript) != nil || len(transcript.Entries) != 2 {
		t.Fatal("partial transcript lost emitted call/output")
	}
	for _, entry := range transcript.Entries {
		if entry.CallId == nil || *entry.CallId != "partial" || entry.Status == nil || *entry.Status != "in_progress" {
			t.Fatal("partial transcript invented completion or lost identity")
		}
	}
	if transcript.Entries[0].Type != "tool_call" || transcript.Entries[0].Arguments == nil || transcript.Entries[1].Type != "tool_output" || transcript.Entries[1].Output == nil {
		t.Fatal("partial transcript lost earlier arguments or emitted output")
	}
	if strings.Contains(body, "exit_code") || strings.Contains(body, "private-command") || strings.Contains(body, "private-output") || strings.Contains(body, "must never arrive") {
		t.Fatal("partial transcript invented unavailable facts or leaked private summary")
	}
	assertCapturedCodexPartialTranscriptParity(t, ctx, fixture, scenario, id, transcript)
}

func assertCapturedCodexStaleAttempt(t *testing.T, ctx context.Context, fixture *invokeContinuePackageFixture, scenario *invokeContinueScenario, id string, before api.WorkerSessionLogPage) {
	t.Helper()
	status, body := t7HTTP(t, ctx, http.MethodPost, fixture.baseURL+"/worker-sessions/"+id+"/terminate", map[string]any{"force": true, "requestId": id + "-stale", "expectedAttemptId": "stale"})
	var failure api.ErrorResponse
	if status != http.StatusConflict || json.Unmarshal([]byte(body), &failure) != nil || string(failure.Code) != "WORKER_SESSION_CONTROL_CONFLICT" {
		t.Fatal("stale tool attempt did not return HTTP conflict")
	}
	request := t7RemoteCLIInputs(scenario, ctx, fixture.baseURL, "terminate", id, "--force", "--request-id", id+"-stale", "--expected-attempt-id", "stale")
	if err := fixture.process.Execute(request.Input); err == nil {
		t.Fatal("stale tool attempt was controlled by CLI")
	}
	assertDirectWorkerSessionCLIError(t, request, "WORKER_SESSION_CONTROL_CONFLICT")
	args := map[string]any{"action": "CONTROL", "operation": "KILL", "workerSessionId": id, "requestId": id + "-stale", "expectedAttemptId": "stale"}
	var mcp struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal(capturedCodexMCPCall(t, ctx, fixture, scenario, "", true, args), &mcp) != nil || mcp.Error.Code != "worker_session.conflict" {
		t.Fatal("stale tool attempt MCP conflict differs")
	}
	after := support.GetJSON[api.WorkerSessionLogPage](t, fixture.baseURL+"/worker-sessions/"+id+"/logs")
	if !reflect.DeepEqual(before, after) {
		t.Fatal("stale attempt changed committed tool capture")
	}
	select {
	case <-scenario.providerRunner.(*capturedCodexToolRunner).stopped:
		t.Fatal("stale attempt stopped owned tool execution")
	default:
	}
}

func assertCapturedCodexUnknownReads(t *testing.T, ctx context.Context, fixture *invokeContinuePackageFixture, scenario *invokeContinueScenario, id string) {
	t.Helper()
	for _, view := range []string{"logs", "transcript"} {
		code := "WORKER_SESSION_NOT_FOUND"
		status, body := t7HTTP(t, ctx, http.MethodGet, fixture.baseURL+"/worker-sessions/"+id+"/"+view, nil)
		var failure api.ErrorResponse
		if status != http.StatusNotFound || json.Unmarshal([]byte(body), &failure) != nil || string(failure.Code) != code {
			t.Fatalf("unknown Worker %s status=%d code=%s", view, status, failure.Code)
		}
		read := t7RemoteCLIInputs(scenario, ctx, fixture.baseURL, "read", "--worker-session-id", id, "--view", view)
		if err := fixture.process.Execute(read.Input); err == nil {
			t.Fatal("unknown Worker CLI read succeeded")
		}
		assertDirectWorkerSessionCLIError(t, read, code)
		var mcp struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if json.Unmarshal(capturedCodexMCPRead(t, ctx, fixture, scenario, id, view, true), &mcp) != nil || mcp.Error.Code != "worker_session.not_found" {
			t.Fatal("unknown Worker MCP read meaning differs")
		}
	}
}

func assertCapturedCodexPartialLogs(t *testing.T, page api.WorkerSessionLogPage) {
	t.Helper()
	count := 0
	for i, record := range page.Events {
		if record.Event.Position != int64(i+1) {
			t.Fatal("partial prefix lost committed order")
		}
		if record.Event.Payload["kind"] != "TOOL" {
			continue
		}
		raw, err := json.Marshal(record.Event.Payload)
		var draft workers.Draft
		var payload workers.ToolPayload
		if err != nil || json.Unmarshal(raw, &draft) != nil || json.Unmarshal(draft.Payload, &payload) != nil || payload.ToolCallID != "partial" || payload.ToolName != "command_execution" || payload.Status != "in_progress" {
			t.Fatal("partial tool facts changed")
		}
		if strings.Contains(string(raw), "exit_code") || strings.Contains(string(raw), "private-command") || strings.Contains(string(raw), "private-output") {
			t.Fatal("partial tool invented exit code or leaked sensitive summary")
		}
		count++
	}
	if count != 2 || page.CommittedPosition != int64(len(page.Events)) {
		t.Fatal("partial tool lifecycle lost or duplicated facts")
	}
}

func assertCapturedCodexPartialTranscriptParity(t *testing.T, ctx context.Context, fixture *invokeContinuePackageFixture, scenario *invokeContinueScenario, id string, want api.WorkerSessionTranscriptResponse) {
	t.Helper()
	read := t7RemoteCLIInputs(scenario, ctx, fixture.baseURL, "read", "--worker-session-id", id, "--view", "transcript")
	if err := fixture.process.Execute(read.Input); err != nil {
		t.Fatal("partial CLI transcript failed")
	}
	var cli, mcp api.WorkerSessionTranscriptResponse
	if json.Unmarshal([]byte(read.Stdout()), &cli) != nil || json.Unmarshal(capturedCodexMCPRead(t, ctx, fixture, scenario, id, "transcript"), &mcp) != nil || !reflect.DeepEqual(want, cli) || !reflect.DeepEqual(want, mcp) {
		t.Fatal("partial transcript read parity differs")
	}
}
