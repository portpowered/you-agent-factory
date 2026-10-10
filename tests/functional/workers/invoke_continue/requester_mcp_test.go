package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factorysessionmcp "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/mcp"
	api "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/internal/functionalevidence"
)

func appendRequesterMCPScenarios(rootDir string, setup *invokeContinueScenarioSetup) error {
	durableRoutes := make(map[string]platformprocess.CommandRunner)
	for _, mode := range []string{"live", "local", "async", "sync"} {
		parent := &t7GatedProviderRunner{}
		parent.reset()
		if err := appendInvokeContinueScenario(rootDir, &setup.scenarios, &setup.routes, "requester-factory-parent-"+mode, parent, parent, nil, nil, nil, parent.reset); err != nil {
			return err
		}
		result := platformprocess.CommandResult{Stdout: directCodexSessionOutput("factory-caller-child-thread", "Factory caller output COMPLETE")}
		child := newInvokeContinueResettableProviderCommandRunner(result)
		if err := appendInvokeContinueScenario(rootDir, &setup.scenarios, &setup.routes, "requester-factory-child-"+mode, child, child, nil, nil, nil, child.Reset); err != nil {
			return err
		}
		if mode == "async" || mode == "sync" {
			durableRoutes["Factory caller "+mode+" prompt"] = child
		}
	}
	setup.routes = append(setup.routes, invokeContinueStaticCommandRouteEntry{workingDirectory: filepath.Join(rootDir, "host-factory"), runner: requesterDurableCommandRoute(durableRoutes)})
	parent := &t7GatedProviderRunner{}
	parent.reset()
	if err := appendInvokeContinueScenario(rootDir, &setup.scenarios, &setup.routes, "requester-mcp-parent", parent, parent, nil, nil, nil, parent.reset); err != nil {
		return err
	}
	result := platformprocess.CommandResult{Stdout: directCodexSessionOutput("mcp-child-thread", "selected host MCP output COMPLETE")}
	child := newInvokeContinueResettableProviderCommandRunner(result, result, result, result)
	if err := appendInvokeContinueScenario(rootDir, &setup.scenarios, &setup.routes, "requester-mcp-child", child, child, nil, nil, nil, child.Reset); err != nil {
		return err
	}
	for _, mode := range []string{"auth", "timeout"} {
		parent := &t7GatedProviderRunner{}
		parent.reset()
		if err := appendInvokeContinueScenario(rootDir, &setup.scenarios, &setup.routes, "requester-mcp-failure-parent-"+mode, parent, parent, nil, nil, nil, parent.reset); err != nil {
			return err
		}
	}
	auth := newInvokeContinueResettableProviderCommandRunner(platformprocess.CommandResult{Stderr: []byte("401 Unauthorized: controlled MCP authentication failure"), ExitCode: 1})
	if err := appendInvokeContinueScenario(rootDir, &setup.scenarios, &setup.routes, "requester-mcp-failure-auth", auth, auth, nil, nil, nil, auth.Reset); err != nil {
		return err
	}
	timeout := &t7GatedProviderRunner{}
	timeout.reset()
	return appendInvokeContinueScenario(rootDir, &setup.scenarios, &setup.routes, "requester-mcp-failure-timeout", timeout, timeout, nil, nil, nil, timeout.reset)
}

// HTTP durable starts use the host's working root. Their unique public prompts
// select immutable external-command routes without sharing scenario ledgers.
type requesterDurableCommandRoute map[string]platformprocess.CommandRunner

func (route requesterDurableCommandRoute) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	for prompt, runner := range route {
		if strings.Contains(string(request.Stdin), prompt) {
			return runner.Run(ctx, request)
		}
	}
	return platformprocess.CommandResult{}, errors.New("unrecognized requester durable command")
}

// CLI live/local invocation and HTTP durable starts have different owners.
// Each cell observes its own native command and public Worker Session metadata.
func TestRequesterFactoryCallerVariants(t *testing.T) {
	t.Parallel()
	functionalevidence.Covers(t, "cli/you.run", "rest/startDurableFactorySessionAsync", "rest/startDurableFactorySessionSync")
	fixture := ensureInvokeContinuePackageFixture(t)
	for _, mode := range []string{"live", "local", "async", "sync"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			parent := fixture.scenario(t, "requester-factory-parent-"+mode)
			child := fixture.scenario(t, "requester-factory-child-"+mode)
			defer parent.close(t)
			defer child.close(t)
			ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
			defer cancel()
			runner := parent.providerRunner.(*t7GatedProviderRunner)
			defer t7ReleaseAndJoin(t, ctx, runner)()
			parentID := scenarioScopedID(parent, "Factory-caller")
			start := t7RemoteCLIInputs(parent, ctx, fixture.baseURL, "invoke", "--execution", requesterExecutionPath(t, parent, parentID), "--async")
			if err := fixture.process.Execute(start.Input); err != nil {
				t.Fatal(err)
			}
			t19AwaitSignal(t, ctx, runner.started, "Factory caller running")
			token := requesterSourceToken(t, runner, parentID)
			invokeRequesterFactory(t, fixture, child, ctx, mode, parentID, token)
			assertRequesterFactoryInvocationChild(t, fixture, child, ctx, parentID, token)
			assertRequesterMCPParentActive(t, fixture, parent, ctx, parentID)
		})
	}
}

func invokeRequesterFactory(t *testing.T, fixture *invokeContinuePackageFixture, child *invokeContinueScenario, ctx context.Context, mode, parentID, token string) {
	t.Helper()
	if mode == "async" || mode == "sync" {
		invokeRequesterDurableFactory(t, fixture, child, ctx, mode, parentID, token)
		return
	}
	factoryPath := filepath.Join(child.workingDirectory, "factory.json")
	writeInvokeContinueJSON(t, factoryPath, map[string]any{
		"name": "requester-variant-factory", "orchestrator": map[string]any{"kind": "JAVASCRIPT", "javascript": map[string]any{"inlineSource": map[string]any{"encoding": "utf-8", "inline": `return (async function () { const result = await agent.run({prompt: "Factory caller prompt", executorProvider: "codex", modelProvider: "codex"}); return "Factory caller output COMPLETE"; })();`}}},
	})
	args := []string{"you", "--json", "--session", uuid.NewString(), "run", "--factory", factoryPath, "--no-record", "--output", "primary", "Factory caller prompt"}
	if mode == "live" {
		opened := support.OpenFactorySessionAt(t, fixture.baseURL, child.workingDirectory)
		defer support.CloseFactorySessionAt(t, fixture.baseURL, opened.Session.Id)
		args = []string{"you", "--json", "--remote", "--server", fixture.baseURL, "--session", opened.Session.Id, "run", "--factory", factoryPath, "--no-record", "--output", "primary", "Factory caller prompt"}
	}
	input := support.FakeInputs(ctx, args)
	input.Input.Env = append(child.environment(), "YOU_SERVER="+fixture.baseURL, "YOU_WORKER_SESSION_ID="+parentID, "YOU_WORKER_SESSION_TOKEN="+token)
	input.Input.WorkingDirectory = child.workingDirectory
	if err := fixture.process.Execute(input.Input); err != nil {
		t.Fatalf("%s Factory caller: %v: %s", mode, err, input.Stderr())
	}
	if !strings.Contains(input.Stdout(), "Factory caller output COMPLETE") || strings.Contains(input.Stdout()+input.Stderr(), token) {
		t.Fatal("Factory caller result missing or credential disclosed")
	}
}

func invokeRequesterDurableFactory(t *testing.T, fixture *invokeContinuePackageFixture, child *invokeContinueScenario, ctx context.Context, mode, parentID, token string) {
	t.Helper()
	source := `return (async function () { await agent.run({prompt: "Factory caller ` + mode + ` prompt", executorProvider: "codex", modelProvider: "codex"}); return "Factory caller output COMPLETE"; })();`
	status, body := requesterFactoryHTTP(t, ctx, fixture.baseURL+"/factory-sessions/"+mode, map[string]any{
		"requestId": scenarioScopedID(child, "durable-caller"),
		"source":    map[string]any{"kind": "INLINE_WORKFLOW", "inlineWorkflow": map[string]any{"inlineSource": map[string]any{"encoding": "utf-8", "inline": source}}},
	}, parentID, token)
	if status != http.StatusOK && status != http.StatusAccepted {
		t.Fatalf("durable %s caller invocation: %d %s", mode, status, body)
	}
	var response struct {
		SessionID string `json:"sessionId"`
	}
	if json.Unmarshal([]byte(body), &response) != nil || response.SessionID == "" {
		t.Fatal("durable caller start lost Factory Session identity")
	}
	defer support.TerminateFactorySessionAt(t, fixture.baseURL, response.SessionID)
	// Durable workflows expose completion through results; Petri Work-category
	// counts on /status cannot establish this workflow's terminal result.
	result, err := support.WaitForObservation(30*time.Second, func() (api.FactorySessionResult, error) {
		return support.GetJSON[api.FactorySessionResult](t, fixture.baseURL+"/factory-sessions/"+response.SessionID+"/results"), nil
	}, func(result api.FactorySessionResult) bool {
		return result.SessionStatus != nil && (*result.SessionStatus == api.FactorySessionDurableLifecycleStatusSucceeded || *result.SessionStatus == api.FactorySessionDurableLifecycleStatusFailed)
	})
	if err != nil || result.ResultStatus != api.FactorySessionResultStatusFinal {
		t.Fatalf("durable caller terminal result: %v %#v", err, result)
	}
	encoded, err := json.Marshal(result)
	if err != nil || !strings.Contains(string(encoded), "Factory caller output COMPLETE") {
		t.Fatal("durable caller lost ordinary result")
	}
	assertRequesterTokenAbsent(t, token, string(encoded))
	assertRequesterTokenAbsent(t, token, body)
}

func assertRequesterFactoryInvocationChild(t *testing.T, fixture *invokeContinuePackageFixture, child *invokeContinueScenario, ctx context.Context, parentID, token string) {
	t.Helper()
	requests := child.providerRunner.Requests()
	if len(requests) != 1 {
		t.Fatal("Factory invocation did not launch exactly one child")
	}
	environment := requesterEnvironment(requests[0].Env)
	if environment["YOU_MESSAGE_TARGET"] != parentID || environment["YOU_WORKER_SESSION_TOKEN"] == token || len(environment["YOU_WORKER_SESSION_TOKEN"]) != 43 {
		t.Fatal("Factory child lost requester or fresh own identity")
	}
	show := t7RemoteCLIInputs(child, ctx, fixture.baseURL, "show", "--session", environment["YOU_FACTORY_SESSION_ID"], "--worker-session-id", environment["YOU_WORKER_SESSION_ID"])
	if err := fixture.process.Execute(show.Input); err != nil {
		t.Fatal(err)
	}
	var observation api.WorkerSessionObservation
	decodeDirectWorkerSessionResult(t, show.Stdout(), &observation)
	assertRequesterTokenAbsent(t, token, show.Stdout()+show.Stderr())
	assertRequesterTokenAbsent(t, environment["YOU_WORKER_SESSION_TOKEN"], show.Stdout()+show.Stderr())
	if observation.Requester == nil || observation.Requester.WorkerSessionId != parentID || observation.Correlation == nil || observation.Correlation.FactorySessionId == nil || *observation.Correlation.FactorySessionId != environment["YOU_FACTORY_SESSION_ID"] {
		t.Fatal("Factory runner and public metadata disagree")
	}
	assertRequesterSuccessorEnvironment(t, environment, observation.WorkerSessionId, parentID, token)
	assertRequesterFactoryListed(t, fixture, child, ctx, environment["YOU_WORK_ID"], environment["YOU_FACTORY_SESSION_ID"], observation)
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
	assertRequesterMCPPartialCredentials(t, fixture, child, ctx, parentID, token)
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

// Partial execution credentials are rejected before the MCP server initializes;
// there can be no protocol result or selected-host admission in that case.
func assertRequesterMCPPartialCredentials(t *testing.T, fixture *invokeContinuePackageFixture, child *invokeContinueScenario, ctx context.Context, callerID, token string) {
	t.Helper()
	before := child.providerRunner.CallCount()
	for _, credentials := range []struct{ id, token string }{{callerID, ""}, {"", token}} {
		input := support.FakeInputs(ctx, []string{"you", "--server", fixture.baseURL, "server", "mcp", "--project-root", child.workingDirectory})
		input.Input.WorkingDirectory = child.workingDirectory
		input.Input.Env = append(child.environment(), "YOU_WORKER_SESSION_ID="+credentials.id, "YOU_WORKER_SESSION_TOKEN="+credentials.token)
		err := fixture.process.Execute(input.Input)
		if err == nil || !strings.Contains(err.Error(), "WORKER_SESSION_CALLER_INVALID") || child.providerRunner.CallCount() != before {
			t.Fatal("partial MCP credential did not refuse startup before provider launch")
		}
		assertRequesterTokenAbsent(t, token, err.Error()+input.Stdout()+input.Stderr())
	}
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
	return executeRequesterMCPTimeout(t, fixture, child, ctx, callerID, token, explicit, 30000)
}

func executeRequesterMCPTimeout(t *testing.T, fixture *invokeContinuePackageFixture, child *invokeContinueScenario, ctx context.Context, callerID, token string, explicit bool, timeoutMillis int) factorysessionmcp.ToolResponse[factorysessionmcp.SubagentResult] {
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
	args := map[string]any{"prompt": "MCP selected host prompt", "provider": "codex", "model": "gpt-5-codex", "timeoutMillis": timeoutMillis}
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

// These parallel cells cross the MCP protocol and the selected live host. The
// timeout is the customer operation deadline; the command remains gated until
// that deadline cancels it, rather than sleeping to simulate provider latency.
func TestRequesterMCPSelectedHostFailures(t *testing.T) {
	t.Parallel()
	t.Cleanup(func() {
		if !t.Failed() {
			functionalevidence.Covers(t, "cli/you.server.mcp", "mcp/mcp.tool.you.subagent", "rest/openFactorySession", "rest/invokeFactorySessionBySessionId", "rest/terminateFactorySession", "rest/closeFactorySession")
		}
	})
	fixture := ensureInvokeContinuePackageFixture(t)
	for _, mode := range []string{"auth", "timeout"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			parent := fixture.scenario(t, "requester-mcp-failure-parent-"+mode)
			child := fixture.scenario(t, "requester-mcp-failure-"+mode)
			defer parent.close(t)
			defer child.close(t)
			ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
			defer cancel()
			runner := parent.providerRunner.(*t7GatedProviderRunner)
			defer t7ReleaseAndJoin(t, ctx, runner)()
			parentID := scenarioScopedID(parent, "mcp-failure-parent")
			start := t7RemoteCLIInputs(parent, ctx, fixture.baseURL, "invoke", "--execution", requesterExecutionPath(t, parent, parentID), "--async")
			if err := fixture.process.Execute(start.Input); err != nil {
				t.Fatal(err)
			}
			t19AwaitSignal(t, ctx, runner.started, "failure caller running")
			token := requesterSourceToken(t, runner, parentID)
			timeoutMillis := 30000
			if mode == "timeout" {
				timeoutMillis = 3000
				defer t7ReleaseAndJoin(t, ctx, child.providerRunner.(*t7GatedProviderRunner))()
			}
			response := executeRequesterMCPTimeout(t, fixture, child, ctx, parentID, token, true, timeoutMillis)
			assertRequesterMCPFailure(t, fixture, child, ctx, mode, parentID, token, response)
			assertRequesterMCPParentActive(t, fixture, parent, ctx, parentID)
		})
	}
}

func assertRequesterMCPFailure(t *testing.T, fixture *invokeContinuePackageFixture, child *invokeContinueScenario, ctx context.Context, mode, parentID, token string, response factorysessionmcp.ToolResponse[factorysessionmcp.SubagentResult]) {
	t.Helper()
	wantState := assertRequesterMCPFailureOutcome(t, mode, response)
	env := assertRequesterMCPFailureRunner(t, fixture, child, ctx, wantState, parentID, token, response.Error.SessionID)
	status, _ := t7HTTP(t, ctx, http.MethodGet, fixture.baseURL+"/factory-sessions/"+response.Error.SessionID, nil)
	if status != http.StatusNotFound {
		t.Fatal("MCP failure reported cleanup before exact child retirement")
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	assertRequesterTokenAbsent(t, token, string(encoded))
	assertRequesterTokenAbsent(t, env["YOU_WORKER_SESSION_TOKEN"], string(encoded))
	if strings.Contains(string(encoded), "controlled MCP authentication failure") {
		t.Fatal("MCP failure disclosed provider diagnostics")
	}
	assertRequesterRefusal(t, fixture, child, ctx, "failed-mcp-child", env["YOU_WORKER_SESSION_ID"], env["YOU_WORKER_SESSION_TOKEN"])
}

func assertRequesterMCPFailureOutcome(t *testing.T, mode string, response factorysessionmcp.ToolResponse[factorysessionmcp.SubagentResult]) string {
	t.Helper()
	wantCode, wantState := "factory_session.subagent.provider_auth_failure", "FAILED"
	if mode == "timeout" {
		wantCode, wantState = "factory_session.subagent.timed_out", "CANCELED"
	}
	if response.Result != nil || response.Error == nil || response.Error.Code != wantCode || response.Error.Details["sessionClosed"] != true || response.Error.SessionID == "" {
		t.Fatalf("%s MCP failure classification/retirement missing: %#v", mode, response.Error)
	}
	if mode == "auth" && (response.Error.Retryable || response.Error.Details["failureReason"] != string(api.WorkFailureTypeAuthFailure)) {
		t.Fatal("terminal authentication failure lost nonretryable owner classification")
	}
	if mode == "timeout" {
		assertRequesterMCPTimeoutProgress(t, response.Error.Details)
	}
	return wantState
}

func assertRequesterMCPTimeoutProgress(t *testing.T, details map[string]any) {
	t.Helper()
	progress, ok := details["progress"].(map[string]any)
	if !ok || progress["available"] != true {
		t.Fatal("timeout lost its public progress snapshot before retirement")
	}
	activity, ok := progress["lastObservedProviderActivity"].(map[string]any)
	if !ok || activity["kind"] == "" || activity["observedAt"] == nil {
		t.Fatal("timeout lost retained provider activity before retirement")
	}
}

func assertRequesterMCPFailureRunner(t *testing.T, fixture *invokeContinuePackageFixture, child *invokeContinueScenario, ctx context.Context, wantState, parentID, token, sessionID string) map[string]string {
	t.Helper()
	requests := child.providerRunner.Requests()
	if len(requests) != 1 {
		t.Fatal("MCP failure did not own exactly one provider command")
	}
	env := requesterEnvironment(requests[0].Env)
	if env["YOU_MESSAGE_TARGET"] != parentID || env["YOU_FACTORY_SESSION_ID"] != sessionID || env["YOU_SERVER"] != fixture.baseURL || requests[0].WorkDir != child.workingDirectory {
		t.Fatal("failed MCP command lost caller or selected host identity")
	}
	observation := requesterObservation(t, fixture, child, ctx, env["YOU_WORKER_SESSION_ID"])
	if string(observation.State) != wantState || observation.Requester == nil || observation.Requester.WorkerSessionId != parentID {
		t.Fatalf("MCP failure worker state/requester mismatch: %s", observation.State)
	}
	assertRequesterSuccessorEnvironment(t, env, observation.WorkerSessionId, parentID, token)
	return env
}
