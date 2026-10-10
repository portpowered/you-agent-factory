package acceptance

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factorysessionmcp "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/mcp"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/internal/functionalevidence"
)

func appendRequesterMCPScenarios(rootDir string, setup *invokeContinueScenarioSetup) error {
	parent := &t7GatedProviderRunner{}
	parent.reset()
	if err := appendInvokeContinueScenario(rootDir, &setup.scenarios, &setup.routes, "requester-mcp-parent", parent, parent, nil, nil, nil, parent.reset); err != nil {
		return err
	}
	result := platformprocess.CommandResult{Stdout: directCodexSessionOutput("mcp-child-thread", "selected host MCP output COMPLETE")}
	child := newInvokeContinueResettableProviderCommandRunner(result, result, result, result)
	return appendInvokeContinueScenario(rootDir, &setup.scenarios, &setup.routes, "requester-mcp-child", child, child, nil, nil, nil, child.Reset)
}

// MCP parity crosses stdio through public Process.Execute and the selected
// shared BuildProcess host. Providers are controlled only at the command edge.
func TestRequesterMCPSelectedHostDefaultsAndRefusals(t *testing.T) {
	t.Parallel()
	fixture := ensureInvokeContinuePackageFixture(t)
	parent, child := fixture.scenario(t, "requester-mcp-parent"), fixture.scenario(t, "requester-mcp-child")
	defer parent.close(t)
	defer child.close(t)
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	runner := parent.providerRunner.(*t7GatedProviderRunner)
	defer t7ReleaseAndJoin(t, ctx, runner)()
	parentID := scenarioScopedID(parent, "mcp-parent")
	start := t7RemoteCLIInputs(parent, ctx, fixture.baseURL, "invoke", "--execution", requesterExecutionPath(t, parent, parentID), "--async")
	if err := fixture.process.Execute(start.Input); err != nil {
		t.Fatal(err)
	}
	t19AwaitSignal(t, ctx, runner.started, "MCP caller running")
	token := requesterSourceToken(t, runner, parentID)
	for _, explicit := range []bool{true, false} {
		result := executeRequesterMCP(t, fixture, child, ctx, "", "", explicit)
		if result.Error != nil || result.Result == nil || result.Result.Text != "selected host MCP output COMPLETE" {
			t.Fatalf("selected-host RUN failed: %#v", result.Error)
		}
		assertRequesterMCPUnattributedChild(t, fixture, child, ctx, result.Result.SessionID)
	}
	assertRequesterMCPRunningCalls(t, fixture, child, parent, ctx, parentID, token)
	before := child.providerRunner.CallCount()
	refused := executeRequesterMCP(t, fixture, child, ctx, parentID, strings.Repeat("A", 43), true)
	if refused.Error == nil || refused.Error.Code != "WORKER_SESSION_CALLER_INVALID" || child.providerRunner.CallCount() != before {
		t.Fatalf("invalid caller was not refused: %#v", refused.Error)
	}
	assertRequesterMCPParentActive(t, fixture, parent, ctx, parentID)
	if err := fixture.process.Execute(t7RemoteCLIInputs(parent, ctx, fixture.baseURL, "cancel", parentID).Input); err != nil {
		t.Fatal(err)
	}
	t19AwaitSignal(t, ctx, runner.stopped, "MCP caller ended")
	refused = executeRequesterMCP(t, fixture, child, ctx, parentID, token, false)
	if refused.Error == nil || refused.Error.Code != "WORKER_SESSION_CALLER_INVALID" || child.providerRunner.CallCount() != before {
		t.Fatalf("ended caller was not refused: %#v", refused.Error)
	}
	functionalevidence.Covers(t, "cli/you.server.mcp", "mcp/mcp.tool.you.subagent", "rest/openFactorySession", "rest/invokeFactorySessionBySessionId", "rest/terminateFactorySession", "rest/closeFactorySession")
}

func assertRequesterMCPRunningCalls(t *testing.T, fixture *invokeContinuePackageFixture, child, parent *invokeContinueScenario, ctx context.Context, parentID, token string) {
	t.Helper()
	for _, explicit := range []bool{true, false} {
		result := executeRequesterMCP(t, fixture, child, ctx, parentID, token, explicit)
		if result.Error != nil || result.Result == nil || result.Result.Text != "selected host MCP output COMPLETE" {
			t.Fatalf("running-caller selected-host RUN failed: %#v", result.Error)
		}
		assertRequesterMCPChild(t, fixture, child, ctx, result.Result.SessionID, parentID, token)
		assertRequesterMCPParentActive(t, fixture, parent, ctx, parentID)
	}
}

func assertRequesterMCPParentActive(t *testing.T, fixture *invokeContinuePackageFixture, parent *invokeContinueScenario, ctx context.Context, parentID string) {
	t.Helper()
	if requesterObservation(t, fixture, parent, ctx, parentID).State != "RUNNING" || parent.providerRunner.CallCount() != 1 {
		t.Fatal("MCP child cleanup or caller refusal changed the active parent")
	}
}

func executeRequesterMCP(t *testing.T, fixture *invokeContinuePackageFixture, child *invokeContinueScenario, ctx context.Context, callerID, token string, explicit bool) factorysessionmcp.ToolResponse[factorysessionmcp.SubagentResult] {
	t.Helper()
	input := support.FakeInputs(ctx, []string{"you", "--server", fixture.baseURL, "server", "mcp", "--project-root", child.workingDirectory})
	input.Input.Env = child.environment()
	if callerID != "" || token != "" {
		input.Input.Env = append(input.Input.Env, "YOU_WORKER_SESSION_ID="+callerID, "YOU_WORKER_SESSION_TOKEN="+token)
	}
	input.Input.WorkingDirectory = child.workingDirectory
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
			t.Errorf("MCP process: %v", err)
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
			t.Fatal("MCP protocol response unavailable")
		}
		return response.Result
	}
	call(1, "initialize", map[string]any{"protocolVersion": "2024-11-05", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "requester-functional", "version": "test"}})
	args := map[string]any{"prompt": "MCP selected host prompt", "provider": "codex", "model": "gpt-5-codex", "timeoutMillis": 30000}
	if explicit {
		args["action"] = "RUN"
	}
	raw := call(2, "tools/call", map[string]any{"name": "you.subagent", "arguments": args})
	if token != "" && (strings.Contains(string(raw), token) || strings.Contains(input.Stderr(), token)) {
		t.Fatal("MCP exposed caller token")
	}
	return decodeRequesterMCPResult(t, raw)
}

func assertRequesterMCPUnattributedChild(t *testing.T, fixture *invokeContinuePackageFixture, child *invokeContinueScenario, ctx context.Context, sessionID string) {
	t.Helper()
	requests := child.providerRunner.Requests()
	command := requests[len(requests)-1]
	environment := requesterEnvironment(command.Env)
	if environment["YOU_MESSAGE_TARGET"] != "" || environment["YOU_FACTORY_SESSION_ID"] != sessionID || environment["YOU_SERVER"] != fixture.baseURL || environment["YOU_WORKER_SESSION_TOKEN"] == "" || command.WorkDir != child.workingDirectory {
		t.Fatal("unattributed MCP child lost own identity, selected host or working root")
	}
	observation := requesterObservation(t, fixture, child, ctx, environment["YOU_WORKER_SESSION_ID"])
	if observation.Requester != nil || observation.Correlation == nil || observation.Correlation.FactorySessionId == nil || *observation.Correlation.FactorySessionId != sessionID || observation.State != "COMPLETED" {
		t.Fatal("unattributed MCP runner and observation disagree")
	}
	status, _ := t7HTTP(t, ctx, http.MethodGet, fixture.baseURL+"/factory-sessions/"+sessionID, nil)
	if status != http.StatusNotFound {
		t.Fatal("RUN returned before owner retirement")
	}
}

func decodeRequesterMCPResult(t *testing.T, raw json.RawMessage) factorysessionmcp.ToolResponse[factorysessionmcp.SubagentResult] {
	t.Helper()
	var response struct {
		IsError    bool            `json:"isError"`
		Structured json.RawMessage `json:"structuredContent"`
		Content    []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if json.Unmarshal(raw, &response) != nil {
		t.Fatal("MCP result decode failed")
	}
	var result factorysessionmcp.ToolResponse[factorysessionmcp.SubagentResult]
	encoded := response.Structured
	if !response.IsError && len(response.Content) > 0 {
		encoded = []byte(response.Content[0].Text)
	}
	if json.Unmarshal(encoded, &result) != nil {
		t.Fatal("MCP typed result unavailable")
	}
	return result
}

func assertRequesterMCPChild(t *testing.T, fixture *invokeContinuePackageFixture, child *invokeContinueScenario, ctx context.Context, sessionID, parentID, token string) {
	t.Helper()
	requests := child.providerRunner.Requests()
	command := requests[len(requests)-1]
	environment := requesterEnvironment(command.Env)
	if environment["YOU_MESSAGE_TARGET"] != parentID || environment["YOU_FACTORY_SESSION_ID"] != sessionID || environment["YOU_SERVER"] != fixture.baseURL || environment["YOU_WORKER_SESSION_TOKEN"] == token || len(environment["YOU_WORKER_SESSION_TOKEN"]) != 43 || command.WorkDir != child.workingDirectory {
		t.Fatalf("MCP child agreement target=%t factory=%t host=%t fresh=%t root=%t", environment["YOU_MESSAGE_TARGET"] == parentID, environment["YOU_FACTORY_SESSION_ID"] == sessionID, environment["YOU_SERVER"] == fixture.baseURL, environment["YOU_WORKER_SESSION_TOKEN"] != token, command.WorkDir == child.workingDirectory)
	}
	observation := requesterObservation(t, fixture, child, ctx, environment["YOU_WORKER_SESSION_ID"])
	if observation.Requester == nil || observation.Requester.WorkerSessionId != parentID || observation.Correlation == nil || observation.Correlation.FactorySessionId == nil || *observation.Correlation.FactorySessionId != sessionID {
		t.Fatal("MCP runner and observation disagree")
	}
	assertRequesterMCPMetadata(t, observation.Labels, observation.Correlation.WorkId, environment["YOU_WORK_ID"], parentID)
	status, _ := t7HTTP(t, ctx, http.MethodGet, fixture.baseURL+"/factory-sessions/"+sessionID, nil)
	if status != http.StatusNotFound {
		t.Fatal("RUN returned before owner retirement")
	}
	if requesterObservation(t, fixture, child, ctx, environment["YOU_WORKER_SESSION_ID"]).State != "COMPLETED" {
		t.Fatal("MCP child did not complete")
	}
}

func assertRequesterMCPMetadata(t *testing.T, labels *[]string, workID *string, executionWorkID, parentID string) {
	t.Helper()
	if labels == nil || !slices.Contains(*labels, "parent:"+parentID) || workID == nil || *workID == "" || *workID != executionWorkID {
		t.Fatal("MCP child lost its parent label or actual Work correlation")
	}
	// Packaged invocation creates actual Work tags. These belong to the child;
	// caller tag inheritance is guarded by Worker Sessions admission units.
}
