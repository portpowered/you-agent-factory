package acceptance

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	api "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// Native-shaped facts use the command-runner edge on the shared root process;
// this protects captured public values without claiming real provider emission.
func capturedCodexToolOutput() []byte {
	return []byte(`{"type":"thread.started","thread_id":"captured-tools-thread"}
{"type":"item.started","item":{"id":"success","type":"command_execution","command":"private-command","status":"in_progress"}}
{"type":"item.completed","item":{"id":"success","type":"command_execution","aggregated_output":"private-output","exit_code":0,"status":"completed"}}
{"type":"item.completed","item":{"id":"failure","type":"command_execution","aggregated_output":"private-denial","exit_code":1,"status":"failed"}}
{"type":"item.started","item":{"id":"mcp","type":"mcp_tool_call","server":"docs","tool":"lookup","arguments":{"query":"visible"},"status":"in_progress"}}
{"type":"item.completed","item":{"id":"mcp","type":"mcp_tool_call","result":{"value":"visible"},"status":"completed"}}
{"type":"item.started","item":{"id":"collab","type":"collab_tool_call","arguments":{"task":"visible"},"status":"in_progress"}}
{"type":"item.completed","item":{"id":"collab","type":"collab_tool_call","result":{"answer":"visible"},"status":"completed"}}
{"type":"item.completed","item":{"id":"answer","type":"agent_message","text":"captured tools COMPLETE"}}
{"type":"turn.completed","usage":{"input_tokens":5,"output_tokens":2}}
`)
}

func TestCapturedCodexToolPublicFacts(t *testing.T) {
	t.Parallel()
	fixture := ensureInvokeContinuePackageFixture(t)
	scenario := fixture.scenario(t, "captured-codex-tools")
	defer scenario.close(t)
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	id := scenarioScopedID(scenario, "captured-tools")
	path := requesterExecutionPath(t, scenario, id)
	invoke := t7RemoteCLIInputs(scenario, ctx, fixture.baseURL, "invoke", "--execution", path)
	if err := fixture.process.Execute(invoke.Input); err != nil {
		t.Fatal("tool fixture invocation failed")
	}
	awaitContinuationRestartLogs(t, invokeContinueStartedProcess{process: fixture.process, baseURL: fixture.baseURL}, scenario.homeDirectory, scenario.workingDirectory, id, "captured-tools-thread")
	assertCapturedCodexToolReads(t, ctx, fixture, scenario, id)
	if scenario.providerRunner.CallCount() != 1 {
		t.Fatal("captured reads reran provider")
	}
}

func TestCapturedCodexToolFactoryFacts(t *testing.T) {
	t.Parallel()
	fixture := ensureInvokeContinuePackageFixture(t)
	scenario := fixture.scenario(t, "captured-codex-factory")
	defer scenario.close(t)
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	t7WriteFactorySibling(t, scenario.workingDirectory)
	opened := support.OpenFactorySessionAt(t, fixture.baseURL, scenario.workingDirectory)
	defer support.CloseFactorySessionAt(t, fixture.baseURL, opened.Session.Id)
	item := support.SubmitSessionWorkAt(t, fixture.baseURL, opened.Session.Id, api.SubmitWorkRequest{WorkTypeName: "task", Payload: "controlled tool capture"})
	if item.WorkId == nil {
		t.Fatal("submitted Work has no identity")
	}
	support.WaitForSessionTerminalStatus(t, fixture.baseURL, opened.Session.Id, 30*time.Second)
	rows := support.GetJSON[api.ListWorkerSessionsResponse](t, fixture.baseURL+"/factory-sessions/"+opened.Session.Id+"/worker-sessions?workId="+*item.WorkId)
	if len(rows.Sessions) != 1 || rows.Sessions[0].FactorySessionId == nil || *rows.Sessions[0].FactorySessionId != opened.Session.Id {
		t.Fatal("Factory tool capture lost owning session")
	}
	id := rows.Sessions[0].WorkerSessionId
	awaitContinuationRestartLogs(t, invokeContinueStartedProcess{process: fixture.process, baseURL: fixture.baseURL}, scenario.homeDirectory, scenario.workingDirectory, id, "captured-tools-thread")
	assertCapturedCodexToolReads(t, ctx, fixture, scenario, id)
	if scenario.providerRunner.CallCount() != 1 {
		t.Fatal("Factory captured reads reran provider")
	}
}

func assertCapturedCodexToolReads(t *testing.T, ctx context.Context, fixture *invokeContinuePackageFixture, scenario *invokeContinueScenario, id string) {
	t.Helper()
	for _, view := range []string{"logs", "transcript"} {
		read := t7RemoteCLIInputs(scenario, ctx, fixture.baseURL, "read", "--worker-session-id", id, "--view", view)
		if err := fixture.process.Execute(read.Input); err != nil {
			t.Fatal("tool capture read failed")
		}
		status, body := t7HTTP(t, ctx, http.MethodGet, fixture.baseURL+"/worker-sessions/"+id+"/"+view, nil)
		var cli, httpValue any
		if view == "logs" {
			cli, httpValue = &api.WorkerSessionLogPage{}, &api.WorkerSessionLogPage{}
		} else {
			cli, httpValue = &api.WorkerSessionTranscriptResponse{}, &api.WorkerSessionTranscriptResponse{}
		}
		// Compare the public contract values; CLI re-encodes HTTP timestamps.
		if status != http.StatusOK || json.Unmarshal([]byte(read.Stdout()), cli) != nil || json.Unmarshal([]byte(body), httpValue) != nil || !reflect.DeepEqual(cli, httpValue) {
			t.Fatalf("CLI/HTTP tool facts differ for %s (HTTP %d)", view, status)
		}
		mcpBody := capturedCodexMCPRead(t, ctx, fixture, scenario, id, view)
		if json.Unmarshal(mcpBody, httpValue) != nil || !reflect.DeepEqual(cli, httpValue) {
			t.Fatalf("MCP tool facts differ for %s", view)
		}
		for _, private := range []string{"private-command", "private-output", "private-denial"} {
			if strings.Contains(body, private) {
				t.Fatal("sensitive tool summary leaked")
			}
		}
		if view == "logs" {
			var logs api.WorkerSessionLogPage
			if json.Unmarshal([]byte(body), &logs) != nil {
				t.Fatal("invalid captured logs")
			}
			count := 0
			for _, record := range logs.Events {
				if record.Event.Payload["kind"] == "TOOL" {
					count++
				}
			}
			if count != 7 {
				t.Fatalf("emitted tool lifecycle count = %d, want 7", count)
			}
			assertCapturedCodexToolLogs(t, logs)
		} else {
			assertCapturedCodexToolTranscript(t, body)
		}
	}
	assertCapturedCodexToolPages(t, ctx, fixture, scenario, id)
}

func assertCapturedCodexToolPages(t *testing.T, ctx context.Context, fixture *invokeContinuePackageFixture, scenario *invokeContinueScenario, id string) {
	t.Helper()
	var collected []api.WorkerSessionEvent
	endpoint := fixture.baseURL + "/worker-sessions/" + id + "/logs?limit=1"
	for pages := 0; pages < 30; pages++ {
		status, body := t7HTTP(t, ctx, http.MethodGet, endpoint, nil)
		var page api.WorkerSessionLogPage
		if status != http.StatusOK || json.Unmarshal([]byte(body), &page) != nil {
			t.Fatal("one-record capture page unavailable")
		}
		_, repeated := t7HTTP(t, ctx, http.MethodGet, endpoint, nil)
		var repeat api.WorkerSessionLogPage
		if json.Unmarshal([]byte(repeated), &repeat) != nil || !reflect.DeepEqual(page, repeat) {
			t.Fatal("acknowledged cursor did not repeat the same page")
		}
		for _, record := range page.Events {
			if record.Event.Position != int64(len(collected)+1) {
				t.Fatal("paging omitted or repeated a committed position")
			}
			collected = append(collected, record)
		}
		if page.NextToken == nil {
			full := support.GetJSON[api.WorkerSessionLogPage](t, fixture.baseURL+"/worker-sessions/"+id+"/logs")
			if !reflect.DeepEqual(collected, full.Events) || full.CommittedPosition != int64(len(collected)) {
				t.Fatal("paged capture differs from full acknowledged capture")
			}
			return
		}
		endpoint = fixture.baseURL + "/worker-sessions/" + id + "/logs?limit=1&nextToken=" + url.QueryEscape(*page.NextToken)
	}
	t.Fatal("tiny captured fixture did not finish paging")
}

type capturedCodexToolRunner struct {
	*invokeContinueResettableProviderCommandRunner
	started, release, stopped chan struct{}
}

type capturedCodexDenialRunner struct {
	*invokeContinueResettableProviderCommandRunner
	started, release, stopped chan struct{}
}

func newCapturedCodexDenialRunner() *capturedCodexDenialRunner {
	r := &capturedCodexDenialRunner{invokeContinueResettableProviderCommandRunner: newInvokeContinueResettableProviderCommandRunner(platformprocess.CommandResult{Stderr: []byte("controlled approval rejection"), ExitCode: 1})}
	r.reset()
	return r
}

func (r *capturedCodexDenialRunner) reset() {
	r.Reset()
	r.started, r.release, r.stopped = make(chan struct{}), make(chan struct{}), make(chan struct{})
}

func (r *capturedCodexDenialRunner) RunStreaming(ctx context.Context, request platformprocess.CommandRequest, observer platformprocess.OutputChunkObserver) (platformprocess.CommandResult, error) {
	result, err := r.Run(ctx, request)
	if err != nil {
		return result, err
	}
	if observer != nil {
		observer(platformprocess.OutputStreamStderr, result.Stderr)
	}
	close(r.started)
	defer close(r.stopped)
	select {
	case <-ctx.Done():
		return platformprocess.CommandResult{}, ctx.Err()
	case <-r.release:
		return result, nil
	}
}

func TestCapturedCodexToolIdentitylessRejection(t *testing.T) {
	t.Parallel()
	fixture := ensureInvokeContinuePackageFixture(t)
	scenario := fixture.scenario(t, "captured-codex-denial")
	defer scenario.close(t)
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	runner := scenario.providerRunner.(*capturedCodexDenialRunner)
	defer func() {
		select {
		case <-runner.release:
		default:
			close(runner.release)
		}
		t19AwaitSignal(t, ctx, runner.stopped, "denial fixture joined")
	}()
	id := scenarioScopedID(scenario, "identityless-denial")
	path := requesterExecutionPath(t, scenario, id)
	invoke := t7RemoteCLIInputs(scenario, ctx, fixture.baseURL, "invoke", "--async", "--execution", path)
	if err := fixture.process.Execute(invoke.Input); err != nil {
		t.Fatal("denial fixture admission failed")
	}
	t19AwaitSignal(t, ctx, runner.started, "identityless diagnostic emitted")
	// Publication commits asynchronously; wait on the captured public diagnostic.
	_, err := support.WaitForObservation(30*time.Second, func() (string, error) {
		_, body := t7HTTP(t, ctx, http.MethodGet, fixture.baseURL+"/worker-sessions/"+id+"/logs", nil)
		return body, nil
	}, func(body string) bool { return strings.Contains(body, "controlled approval rejection") })
	if err != nil {
		t.Fatal("identityless diagnostic did not commit")
	}
	assertCapturedCodexDenialReads(t, ctx, fixture, scenario, id, false)
	close(runner.release)
	join := t7RemoteCLIInputs(scenario, ctx, fixture.baseURL, "invoke", "--execution", path)
	if err := fixture.process.Execute(join.Input); err == nil {
		t.Fatal("rejected provider reported success")
	}
	assertCapturedCodexDenialReads(t, ctx, fixture, scenario, id, true)
	observation := requesterObservation(t, fixture, scenario, ctx, id)
	if observation.State != "FAILED" || observation.ProviderSession != nil || runner.CallCount() != 1 {
		t.Fatal("identityless rejection acquired provider identity or another execution")
	}
}

func assertCapturedCodexDenialReads(t *testing.T, ctx context.Context, fixture *invokeContinuePackageFixture, scenario *invokeContinueScenario, id string, terminal bool) {
	t.Helper()
	page := support.GetJSON[api.WorkerSessionLogPage](t, fixture.baseURL+"/worker-sessions/"+id+"/logs")
	read := t7RemoteCLIInputs(scenario, ctx, fixture.baseURL, "read", "--worker-session-id", id, "--view", "logs")
	if err := fixture.process.Execute(read.Input); err != nil {
		t.Fatal("diagnostic CLI read failed")
	}
	var cli, mcp api.WorkerSessionLogPage
	if json.Unmarshal([]byte(read.Stdout()), &cli) != nil || json.Unmarshal(capturedCodexMCPRead(t, ctx, fixture, scenario, id, "logs"), &mcp) != nil || !reflect.DeepEqual(page, cli) || !reflect.DeepEqual(page, mcp) {
		t.Fatal("diagnostic logs parity differs")
	}
	count := 0
	for _, record := range page.Events {
		if record.Event.Payload["kind"] == "TOOL" {
			t.Fatal("identityless diagnostic fabricated a tool")
		}
		raw, err := json.Marshal(record.Event.Payload)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), "controlled approval rejection") {
			count++
			if record.Event.Payload["kind"] != "PROGRESS" {
				t.Fatal("identityless rejection lost diagnostic classification")
			}
		}
	}
	if count != 1 {
		t.Fatalf("identityless diagnostic count = %d, want 1", count)
	}
	assertCapturedCodexUnavailableTranscript(t, ctx, fixture, scenario, id, terminal)
}

func assertCapturedCodexUnavailableTranscript(t *testing.T, ctx context.Context, fixture *invokeContinuePackageFixture, scenario *invokeContinueScenario, id string, terminal bool) {
	t.Helper()
	status, body := t7HTTP(t, ctx, http.MethodGet, fixture.baseURL+"/worker-sessions/"+id+"/transcript", nil)
	var failure api.ErrorResponse
	wantStatus, wantCode, wantMCP := http.StatusConflict, "WORKER_SESSION_TRANSCRIPT_ACTIVE", "worker_session.conflict"
	if terminal {
		wantStatus, wantCode, wantMCP = http.StatusInternalServerError, "WORKER_SESSION_TRANSCRIPT_UNAVAILABLE", "worker_session.unavailable"
	}
	if status != wantStatus || json.Unmarshal([]byte(body), &failure) != nil || string(failure.Code) != wantCode {
		t.Fatal("identityless transcript claimed available native facts")
	}
	transcript := t7RemoteCLIInputs(scenario, ctx, fixture.baseURL, "read", "--worker-session-id", id, "--view", "transcript")
	if err := fixture.process.Execute(transcript.Input); err == nil {
		t.Fatal("identityless CLI transcript reported success")
	}
	assertDirectWorkerSessionCLIError(t, transcript, wantCode)
	// MCP retains only allowlisted upstream codes. Active conflict is conveyed
	// by its typed code/status, while terminal unavailability retains its code.
	wantUpstream := ""
	if terminal {
		wantUpstream = wantCode
	}
	var mcpFailure struct {
		Error struct {
			Code    string `json:"code"`
			Details struct {
				UpstreamCode string `json:"upstreamCode"`
			} `json:"details"`
		} `json:"error"`
	}
	if json.Unmarshal(capturedCodexMCPRead(t, ctx, fixture, scenario, id, "transcript", true), &mcpFailure) != nil || mcpFailure.Error.Code != wantMCP || mcpFailure.Error.Details.UpstreamCode != wantUpstream {
		t.Fatal("identityless MCP transcript error meaning differs")
	}
}

func assertCapturedCodexNoReferenceReads(t *testing.T, scenario *invokeContinueScenario, id string, terminal bool) {
	t.Helper()
	fixture, ctx := scenario.fixture, t.Context()
	page := support.GetJSON[api.WorkerSessionLogPage](t, fixture.baseURL+"/worker-sessions/"+id+"/logs")
	if len(page.Events) == 0 {
		t.Fatal("no-reference capture lost its opening")
	}
	for _, record := range page.Events {
		if record.Event.Payload["kind"] == "TOOL" || record.Event.Payload["kind"] == "MESSAGE" {
			t.Fatal("silent provider acquired invented tool or message facts")
		}
	}
	read := t7RemoteCLIInputs(scenario, ctx, fixture.baseURL, "read", "--worker-session-id", id, "--view", "logs")
	if err := fixture.process.Execute(read.Input); err != nil {
		t.Fatal("no-reference CLI logs failed")
	}
	var cli, mcp api.WorkerSessionLogPage
	if json.Unmarshal([]byte(read.Stdout()), &cli) != nil || json.Unmarshal(capturedCodexMCPRead(t, ctx, fixture, scenario, id, "logs"), &mcp) != nil || !reflect.DeepEqual(page, cli) || !reflect.DeepEqual(page, mcp) {
		t.Fatal("no-reference logs parity differs")
	}
	assertCapturedCodexUnavailableTranscript(t, ctx, fixture, scenario, id, terminal)
}

func newCapturedCodexToolRunner() *capturedCodexToolRunner {
	runner := &capturedCodexToolRunner{invokeContinueResettableProviderCommandRunner: newInvokeContinueResettableProviderCommandRunner(platformprocess.CommandResult{Stdout: capturedCodexToolOutput()})}
	runner.reset()
	return runner
}

func (r *capturedCodexToolRunner) reset() {
	r.Reset()
	r.started, r.release, r.stopped = make(chan struct{}), make(chan struct{}), make(chan struct{})
}

func (r *capturedCodexToolRunner) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	return r.RunStreaming(ctx, request, nil)
}

func (r *capturedCodexToolRunner) RunStreaming(ctx context.Context, request platformprocess.CommandRequest, observer platformprocess.OutputChunkObserver) (platformprocess.CommandResult, error) {
	result, err := r.invokeContinueResettableProviderCommandRunner.Run(ctx, request)
	if err != nil {
		return result, err
	}
	split := strings.Index(string(result.Stdout), `{"type":"item.completed","item":{"id":"answer"`)
	if observer != nil {
		observer(platformprocess.OutputStreamStdout, result.Stdout[:split])
	}
	close(r.started)
	defer close(r.stopped)
	select {
	case <-ctx.Done():
		return platformprocess.CommandResult{}, ctx.Err()
	case <-r.release:
		if observer != nil {
			observer(platformprocess.OutputStreamStdout, result.Stdout[split:])
		}
		return result, nil
	}
}

func TestCapturedCodexToolLiveFacts(t *testing.T) {
	t.Parallel()
	fixture := ensureInvokeContinuePackageFixture(t)
	scenario := fixture.scenario(t, "captured-codex-live")
	defer scenario.close(t)
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	runner := scenario.providerRunner.(*capturedCodexToolRunner)
	defer func() {
		select {
		case <-runner.release:
		default:
			close(runner.release)
		}
		t19AwaitSignal(t, ctx, runner.stopped, "tool fixture joined")
	}()
	id := scenarioScopedID(scenario, "live-tools")
	path := requesterExecutionPath(t, scenario, id)
	invoke := t7RemoteCLIInputs(scenario, ctx, fixture.baseURL, "invoke", "--async", "--execution", path)
	if err := fixture.process.Execute(invoke.Input); err != nil {
		t.Fatal("live tool fixture admission failed")
	}
	t19AwaitSignal(t, ctx, runner.started, "tool facts emitted")
	assertCapturedCodexLiveReads(t, ctx, fixture, scenario, id)
	close(runner.release)
	join := t7RemoteCLIInputs(scenario, ctx, fixture.baseURL, "invoke", "--execution", path)
	if err := fixture.process.Execute(join.Input); err != nil {
		t.Fatal("live tool fixture completion failed")
	}
	awaitContinuationRestartLogs(t, invokeContinueStartedProcess{process: fixture.process, baseURL: fixture.baseURL}, scenario.homeDirectory, scenario.workingDirectory, id, "captured-tools-thread")
	assertCapturedCodexToolReads(t, ctx, fixture, scenario, id)
	if runner.CallCount() != 1 {
		t.Fatal("live capture republished or reran provider")
	}
}

func TestCapturedCodexToolFactoryLiveFacts(t *testing.T) {
	t.Parallel()
	fixture := ensureInvokeContinuePackageFixture(t)
	scenario := fixture.scenario(t, "captured-codex-factory-live")
	defer scenario.close(t)
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	runner := scenario.providerRunner.(*capturedCodexToolRunner)
	defer func() {
		select {
		case <-runner.release:
		default:
			close(runner.release)
		}
		t19AwaitSignal(t, ctx, runner.stopped, "Factory tool fixture joined")
	}()
	t7WriteFactorySibling(t, scenario.workingDirectory)
	opened := support.OpenFactorySessionAt(t, fixture.baseURL, scenario.workingDirectory)
	defer support.CloseFactorySessionAt(t, fixture.baseURL, opened.Session.Id)
	item := support.SubmitSessionWorkAt(t, fixture.baseURL, opened.Session.Id, api.SubmitWorkRequest{WorkTypeName: "task", Payload: "controlled live tool capture"})
	if item.WorkId == nil {
		t.Fatal("live submitted Work has no identity")
	}
	t19AwaitSignal(t, ctx, runner.started, "Factory tool facts emitted")
	rows := support.GetJSON[api.ListWorkerSessionsResponse](t, fixture.baseURL+"/factory-sessions/"+opened.Session.Id+"/worker-sessions?workId="+*item.WorkId)
	if len(rows.Sessions) != 1 || rows.Sessions[0].FactorySessionId == nil || *rows.Sessions[0].FactorySessionId != opened.Session.Id {
		t.Fatal("live Factory tool capture lost owning session")
	}
	id := rows.Sessions[0].WorkerSessionId
	assertCapturedCodexLiveReads(t, ctx, fixture, scenario, id)
	close(runner.release)
	support.WaitForSessionTerminalStatus(t, fixture.baseURL, opened.Session.Id, 30*time.Second)
	awaitContinuationRestartLogs(t, invokeContinueStartedProcess{process: fixture.process, baseURL: fixture.baseURL}, scenario.homeDirectory, scenario.workingDirectory, id, "captured-tools-thread")
	assertCapturedCodexToolReads(t, ctx, fixture, scenario, id)
	if runner.CallCount() != 1 {
		t.Fatal("live Factory capture republished or reran provider")
	}
}

func assertCapturedCodexLiveReads(t *testing.T, ctx context.Context, fixture *invokeContinuePackageFixture, scenario *invokeContinueScenario, id string) {
	t.Helper()
	page := awaitCapturedCodexLiveTools(t, ctx, fixture, id)
	assertCapturedCodexToolLogs(t, page)
	read := t7RemoteCLIInputs(scenario, ctx, fixture.baseURL, "read", "--worker-session-id", id, "--view", "logs")
	if err := fixture.process.Execute(read.Input); err != nil {
		t.Fatal("live CLI logs failed")
	}
	var cli, mcp api.WorkerSessionLogPage
	if json.Unmarshal([]byte(read.Stdout()), &cli) != nil || json.Unmarshal(capturedCodexMCPRead(t, ctx, fixture, scenario, id, "logs"), &mcp) != nil || !reflect.DeepEqual(page, cli) || !reflect.DeepEqual(page, mcp) {
		t.Fatal("live tool read parity differs")
	}
	status, _ := t7HTTP(t, ctx, http.MethodGet, fixture.baseURL+"/worker-sessions/"+id+"/transcript", nil)
	if status != http.StatusConflict {
		t.Fatal("active transcript contract changed")
	}
}

func awaitCapturedCodexLiveTools(t *testing.T, ctx context.Context, fixture *invokeContinuePackageFixture, id string) api.WorkerSessionLogPage {
	t.Helper()
	// Capture commits asynchronously; the owned public watermark is the
	// synchronization boundary, rather than the provider callback alone.
	body, err := support.WaitForObservation(30*time.Second, func() (string, error) {
		_, body := t7HTTP(t, ctx, http.MethodGet, fixture.baseURL+"/worker-sessions/"+id+"/logs", nil)
		return body, nil
	}, func(body string) bool {
		var page api.WorkerSessionLogPage
		if json.Unmarshal([]byte(body), &page) != nil {
			return false
		}
		count := 0
		for _, event := range page.Events {
			if event.Event.Payload["kind"] == "TOOL" {
				count++
			}
		}
		return count == 7
	})
	if err != nil {
		t.Fatal("live tools did not commit")
	}
	var page api.WorkerSessionLogPage
	if json.Unmarshal([]byte(body), &page) != nil {
		t.Fatal("invalid live capture")
	}
	return page
}

func assertCapturedCodexToolTranscript(t *testing.T, body string) {
	t.Helper()
	var transcript api.WorkerSessionTranscriptResponse
	if json.Unmarshal([]byte(body), &transcript) != nil || len(transcript.Entries) != 9 {
		t.Fatal("tool transcript lost or duplicated facts")
	}
	names := map[string]string{"success": "command_execution", "failure": "command_execution", "mcp": "mcp_tool_call:docs/lookup", "collab": "collab_tool_call"}
	wantCalls := []string{"success", "success", "failure", "failure", "mcp", "mcp", "collab", "collab"}
	for index, entry := range transcript.Entries {
		if entry.Order != index+1 || entry.Timestamp == nil || (index < len(wantCalls) && (entry.CallId == nil || *entry.CallId != wantCalls[index])) {
			t.Fatal("transcript changed first-observed tool order")
		}
		if entry.Type == "tool_call" {
			if entry.CallId == nil || entry.Name == nil || *entry.Name != names[*entry.CallId] {
				if entry.CallId == nil || entry.Name == nil {
					t.Fatal("native tool identity/name unavailable")
				}
				t.Fatalf("native tool identity/name lost: id=%q name=%q", *entry.CallId, *entry.Name)
			}
			if (*entry.CallId == "mcp" || *entry.CallId == "collab") && (entry.Arguments == nil || !strings.Contains(*entry.Arguments, "visible")) {
				t.Fatal("opening arguments lost on completion")
			}
		}
		if entry.Type == "tool_output" && entry.CallId != nil && (*entry.CallId == "success" || *entry.CallId == "failure") {
			var result map[string]any
			if entry.Output == nil || json.Unmarshal([]byte(*entry.Output), &result) != nil {
				t.Fatal("tool result is not valid JSON")
			}
			code, status := float64(0), "completed"
			if *entry.CallId == "failure" {
				code, status = 1, "failed"
			}
			if result["exit_code"] != code || result["status"] != status || entry.Status == nil || *entry.Status != status {
				t.Fatal("tool outcome lost or misreported")
			}
		}
	}
}

func assertCapturedCodexToolLogs(t *testing.T, logs api.WorkerSessionLogPage) {
	t.Helper()
	wantIDs := []string{"success", "success", "failure", "mcp", "mcp", "collab", "collab"}
	wantStatus := []string{"in_progress", "completed", "failed", "in_progress", "completed", "in_progress", "completed"}
	index := 0
	for position, record := range logs.Events {
		if record.Event.Position != int64(position+1) || record.Event.CapturedAt == nil {
			t.Fatal("capture lost acknowledged order or timestamp")
		}
		if record.Event.Payload["kind"] != "TOOL" {
			continue
		}
		raw, err := json.Marshal(record.Event.Payload)
		var draft workers.Draft
		if err != nil || json.Unmarshal(raw, &draft) != nil {
			t.Fatal("invalid public tool draft")
		}
		var payload workers.ToolPayload
		if json.Unmarshal(draft.Payload, &payload) != nil || index >= len(wantIDs) || payload.ToolCallID != wantIDs[index] || payload.Status != wantStatus[index] {
			t.Fatalf("tool lifecycle order/facts differ at %d", index)
		}
		if index == 1 || index == 2 {
			var result map[string]any
			if json.Unmarshal(payload.ResultSummary, &result) != nil || result["exit_code"] != float64(index-1) || result["status"] != wantStatus[index] {
				t.Fatal("command result lost explicit zero or failure")
			}
		}
		if index >= 3 && strings.Contains(string(payload.ResultSummary), "exit_code") {
			t.Fatal("non-command tool acquired an invented exit code")
		}
		index++
	}
	if index != len(wantIDs) || logs.CommittedPosition != int64(len(logs.Events)) {
		t.Fatal("capture lost or duplicated acknowledged native facts")
	}
}

// This is an in-process public stdio client, not a built CLI or a second
// application graph. Each connection owns its pipes and joins on closure.
func capturedCodexMCPRead(t *testing.T, ctx context.Context, fixture *invokeContinuePackageFixture, scenario *invokeContinueScenario, id, view string, expectedError ...bool) json.RawMessage {
	t.Helper()
	input := support.FakeInputs(ctx, []string{"you", "--server", fixture.baseURL, "server", "mcp", "--project-root", scenario.workingDirectory})
	input.Input.Env, input.Input.WorkingDirectory = scenario.environment(), scenario.workingDirectory
	reader, writer := io.Pipe()
	outputReader, outputWriter := io.Pipe()
	input.Input.Stdin, input.Input.Stdout = reader, outputWriter
	done := make(chan error, 1)
	go func() {
		err := fixture.process.Execute(input.Input)
		_ = outputWriter.CloseWithError(err)
		_ = reader.CloseWithError(err)
		done <- err
	}()
	defer func() {
		_ = writer.Close()
		_ = outputReader.Close()
		if err := <-done; err != nil {
			t.Errorf("captured MCP connection: %v", err)
		}
	}()
	encoder, decoder := json.NewEncoder(writer), json.NewDecoder(outputReader)
	call := func(id int, method string, params any) json.RawMessage {
		if err := encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}); err != nil {
			t.Fatal(err)
		}
		var response struct {
			ID     int             `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  json.RawMessage `json:"error"`
		}
		if err := decoder.Decode(&response); err != nil || response.ID != id || len(response.Error) > 0 {
			t.Fatal("captured MCP protocol response unavailable")
		}
		return response.Result
	}
	call(1, "initialize", map[string]any{"protocolVersion": "2024-11-05", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "captured-functional", "version": "test"}})
	raw := call(2, "tools/call", map[string]any{"name": "you.subagent", "arguments": map[string]any{"action": "READ", "workerSessionId": id, "view": view}})
	var response struct {
		IsError           bool            `json:"isError"`
		StructuredContent json.RawMessage `json:"structuredContent"`
		Content           []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	wantError := len(expectedError) > 0 && expectedError[0]
	if json.Unmarshal(raw, &response) != nil || response.IsError != wantError || len(response.Content) != 1 {
		t.Fatal("captured MCP READ failed")
	}
	if wantError {
		return response.StructuredContent
	}
	var envelope struct {
		Result map[string]json.RawMessage `json:"result"`
	}
	if json.Unmarshal([]byte(response.Content[0].Text), &envelope) != nil || len(envelope.Result[view]) == 0 {
		t.Fatal("captured MCP READ value unavailable")
	}
	return envelope.Result[view]
}
