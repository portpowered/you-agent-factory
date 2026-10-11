package mock

import (
	"bytes"
	"context"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// Native assistant records and result-only declarative streams are different
// external facts. This shared command edge supplies real protocol records and
// denies every unconfigured route; it never launches a provider executable.
func TestFactoryFinalCapture(t *testing.T) {
	t.Parallel()
	routes := newSharedWorkersMockCommandRunner()
	denied := &directExampleDeniedRunner{}
	host := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: publishedDirectMockFactory(t), WaitForServiceModeRuntime: true,
		Env:   finalCaptureEnvironment(t),
		Edges: serviceedges.Edges{ProviderCommandRunner: routes, ScriptCommandRunner: denied},
	})
	for _, provider := range []string{"codex", "claude"} {
		for _, origin := range []string{"direct", "factory"} {
			t.Run(provider+"/"+origin, func(t *testing.T) {
				t.Parallel()
				dir := declaredResultFactory(t)
				environment := finalCaptureEnvironment(t)
				body := `{"decision":"ACCEPTED","output":"PRIVATE_NATIVE_FINAL"}`
				stdout := support.CodexSuccessStdout(body)
				if provider == "claude" {
					stdout = claudeNativeFinalFixture(t, body)
				}
				runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{Stdout: stdout})
				routes.setFor(dir, runner)
				t.Cleanup(func() { routes.setFor(dir, nil) })
				var workerID string
				if origin == "direct" {
					workerID = invokeNativeFinalDirect(t, host, dir, provider, environment)
				} else {
					workerID = invokeNativeFinalFactory(t, host, dir, provider, environment)
				}
				assertNativeFinalCapture(t, host, workerID, provider, body, environment)
				if len(runner.Requests()) != 1 {
					t.Fatalf("provider calls=%d, want one physical turn", len(runner.Requests()))
				}
			})
		}
	}
	for _, shape := range []string{"equal-messages", "same-item-snapshots"} {
		for _, origin := range []string{"direct", "factory"} {
			t.Run("codex/"+origin+"/"+shape, func(t *testing.T) {
				t.Parallel()
				runNativeMessageProjection(t, host, routes, origin, shape)
			})
		}
	}
	for _, provider := range []string{"codex", "claude"} {
		t.Run(provider+"/direct/accepted-private-content", func(t *testing.T) {
			t.Parallel()
			dir, environment := declaredResultFactory(t), finalCaptureEnvironment(t)
			body := "PRIVATE accepted declared-credential"
			stdout := support.CodexSuccessStdout(body)
			if provider == "claude" {
				stdout = claudeNativeFinalFixture(t, body)
			}
			routes.setFor(dir, testutil.NewProviderCommandRunner(platformprocess.CommandResult{Stdout: stdout}))
			t.Cleanup(func() { routes.setFor(dir, nil) })
			id := invokeNativeFinalDirect(t, host, dir, provider, environment)
			assertNativeFinalCapture(t, host, id, provider, "PRIVATE accepted <redacted>", environment)
		})
	}
	// The second graph owns a different durable profile. Its immutable edges
	// deny execution; knowing an ID must not disclose another profile's capture.
	foreignEnv := finalCaptureEnvironment(t)
	foreign := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: declaredResultFactory(t), WaitForServiceModeRuntime: true, Env: foreignEnv,
		Edges: serviceedges.Edges{ProviderCommandRunner: denied, ScriptCommandRunner: denied},
	})
	for _, provider := range []string{"codex", "claude"} {
		t.Run(provider+"/factory/foreign-capture-scope", func(t *testing.T) {
			t.Parallel()
			runFinalCaptureScopeScenario(t, host, foreign, routes, denied, provider, foreignEnv)
		})
	}
	t.Run("foreign-worker-cursor", func(t *testing.T) {
		t.Parallel()
		var ids []string
		var environment []string
		for _, marker := range []string{"PRIVATE_CURSOR_SOURCE", "PRIVATE_CURSOR_PEER"} {
			dir := declaredResultFactory(t)
			environment = finalCaptureEnvironment(t)
			routes.setFor(dir, testutil.NewProviderCommandRunner(platformprocess.CommandResult{Stdout: support.CodexSuccessStdout(marker)}))
			t.Cleanup(func() { routes.setFor(dir, nil) })
			id := invokeNativeFinalDirect(t, host, dir, "codex", environment)
			assertNativeFinalCapture(t, host, id, "codex", marker, environment)
			ids = append(ids, id)
		}
		assertMockUsagePeerCursor(t, host, t.TempDir(), environment, ids[0], ids[1])
	})
	t.Run("claude/factory/earlier-completion-final-fallback", func(t *testing.T) {
		t.Parallel()
		runEarlierFinalCapture(t, host, routes)
	})
}

// A retryable physical turn can publish a native message before failing. The
// successful retry has only a result record, so the earlier completion cannot
// suppress its truthful fallback. Both turns use the actual runner and harness.
func runEarlierFinalCapture(t *testing.T, host *support.FunctionalAPIServer, routes *sharedWorkersMockCommandRunner) {
	t.Helper()
	dir, environment := declaredResultFactory(t), finalCaptureEnvironment(t)
	body := `{"decision":"ACCEPTED","output":"PRIVATE_NATIVE_FINAL"}`
	native := claudeNativeFinalFixture(t, "PRIVATE_EARLIER_MESSAGE")
	fallback, err := json.Marshal(map[string]any{"type": "result", "subtype": "success", "is_error": false,
		"result": body, "session_id": "private-native-claude"})
	if err != nil {
		t.Fatal(err)
	}
	runner := testutil.NewProviderCommandRunner(
		platformprocess.CommandResult{Stdout: native, Stderr: []byte("service unavailable"), ExitCode: 1},
		platformprocess.CommandResult{Stdout: append(fallback, '\n')},
	)
	routes.setFor(dir, runner)
	t.Cleanup(func() { routes.setFor(dir, nil) })
	id := invokeNativeFinalFactory(t, host, dir, "claude", environment)
	waitFinalCaptureComplete(t, host, id)
	completed := completedAssistantDrafts(t, assertRejectedCapturePages(t, host, id, environment))
	if len(completed) != 2 || len(runner.Requests()) != 2 {
		t.Fatalf("messages=%d calls=%d, want two retained turns", len(completed), len(runner.Requests()))
	}
	assertCompletedEnvelope(t, completed[0], "PRIVATE_EARLIER_MESSAGE")
	assertCompletedEnvelope(t, completed[1], body)
	if completed[0].Provenance.Delivery != workers.DeliveryNativeStream || completed[0].TurnID != "agent-turn-1" ||
		completed[1].Provenance.Delivery != workers.DeliverySynthesized || completed[1].Provenance.Fidelity != workers.FidelityFinalOnly ||
		completed[1].Provenance.Provider != "agent-run" || completed[1].TurnID != "agent-turn-2" {
		t.Fatalf("turn provenance: %+v %+v", completed[0], completed[1])
	}
}

func runFinalCaptureScopeScenario(t *testing.T, host, foreign *support.FunctionalAPIServer, routes *sharedWorkersMockCommandRunner, denied *directExampleDeniedRunner, provider string, foreignEnv []string) {
	t.Helper()
	dir, environment := declaredResultFactory(t), finalCaptureEnvironment(t)
	body := `{"decision":"ACCEPTED","output":"PRIVATE_NATIVE_FINAL"}`
	stdout := support.CodexSuccessStdout(body)
	if provider == "claude" {
		stdout = claudeNativeFinalFixture(t, body)
	}
	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{Stdout: stdout})
	routes.setFor(dir, runner)
	t.Cleanup(func() { routes.setFor(dir, nil) })
	id := invokeNativeFinalFactory(t, host, dir, provider, environment)
	assertNativeFinalCapture(t, host, id, provider, body, environment)
	before := assertRejectedCapturePages(t, host, id, environment)
	peerDir := declaredResultFactory(t)
	peerScope := support.OpenFactorySessionAt(t, host.URL(), peerDir).Session.Id
	assertFinalCaptureScopeDenial(t, host, environment, id, peerScope)
	assertFinalCaptureScopeDenial(t, foreign, foreignEnv, id, "")
	if after := assertRejectedCapturePages(t, host, id, environment); !reflect.DeepEqual(before, after) {
		t.Fatal("denied reads changed committed capture")
	}
	if len(runner.Requests()) != 1 || denied.calls.Load() != 0 {
		t.Fatalf("denied read effects: provider=%d forbidden=%d", len(runner.Requests()), denied.calls.Load())
	}
}

func assertFinalCaptureScopeDenial(t *testing.T, host *support.FunctionalAPIServer, environment []string, id, scope string) {
	t.Helper()
	connection, ctx := mockUsageMCP(t, host, t.TempDir(), environment)
	views := []string{"summary", "transcript", "logs"}
	if scope != "" {
		// Logs uses the top-level Worker ID; there is no Factory-scoped route.
		views = []string{"summary", "transcript"}
	}
	for _, view := range views {
		args := []string{"you", "--json", "--server", host.URL(), "worker-sessions", "read", "--worker-session-id", id, "--view", view}
		if view == "summary" {
			args = []string{"you", "--json", "--server", host.URL(), "worker-sessions", "show", "--worker-session-id", id}
		}
		arguments := map[string]any{"action": "READ", "workerSessionId": id, "view": view}
		mcpCode := "worker_session.not_found"
		path := "/worker-sessions/" + url.PathEscape(id)
		code := factoryapi.ErrorResponseCodeNOTFOUND
		if view != "summary" {
			path += "/" + view
			if view == "logs" {
				code = "WORKER_SESSION_NOT_FOUND"
			}
		}
		if scope != "" {
			args = append(args, "--session", scope)
			arguments["factorySessionId"] = scope
			// MCP READ is ID-scoped and rejects a Factory selector rather than
			// accepting it as authority. Preserve that published contract.
			mcpCode = "worker_session.invalid_request"
			path = "/factory-sessions/" + url.PathEscape(scope) + path
		}
		inputs := support.FakeInputs(ctx, args)
		inputs.Input.Env, inputs.Input.WorkingDirectory = environment, t.TempDir()
		if err := host.Execute(t, inputs.Input); err == nil || inputs.Stdout() != "" {
			t.Fatalf("foreign capture CLI disclosed %s: %v %s", view, err, inputs.Stdout())
		}
		var cliFailure factoryapi.ErrorResponse
		if err := json.Unmarshal([]byte(inputs.Stderr()), &cliFailure); err != nil || cliFailure.Code != "WORKER_SESSION_NOT_FOUND" {
			t.Fatalf("foreign capture CLI error: %s %v", inputs.Stderr(), err)
		}
		assertMockUsageHTTPNotFound(t, host, path, code)
		result, err := connection.CallTool(ctx, &mcp.CallToolParams{Name: "you.subagent", Arguments: arguments})
		if err != nil || !result.IsError {
			t.Fatalf("foreign capture MCP disclosed %s: %+v %v", view, result, err)
		}
		assertFinalCaptureMCPDenial(t, result, mcpCode)
	}
}

func assertFinalCaptureMCPDenial(t *testing.T, result *mcp.CallToolResult, code string) {
	t.Helper()
	var failure struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	encoded, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(encoded, &failure); err != nil || failure.Error.Code != code {
		t.Fatalf("foreign capture MCP error: %s %v", encoded, err)
	}
}

// Equal text is not a duplicate identity. Conversely, successive snapshots of
// one item remain in logs while the transcript projects its final content once.
func runNativeMessageProjection(t *testing.T, host *support.FunctionalAPIServer, routes *sharedWorkersMockCommandRunner, origin, shape string) {
	t.Helper()
	dir, environment := declaredResultFactory(t), finalCaptureEnvironment(t)
	body := `{"decision":"ACCEPTED","output":"PRIVATE_NATIVE_FINAL"}`
	stdout := "{\"type\":\"thread.started\",\"thread_id\":\"private-projection-thread\"}\n{\"type\":\"turn.started\"}\n"
	texts, types, ids := []string{body, body}, []string{"item.completed", "item.completed"}, []string{"message-one", "message-two"}
	if shape == "same-item-snapshots" {
		texts = []string{"PRIVATE draft", "PRIVATE updated", body}
		types, ids = []string{"item.started", "item.updated", "item.completed"}, []string{"message-one", "message-one", "message-one"}
	}
	for index, text := range texts {
		record, err := json.Marshal(map[string]any{"type": types[index], "item": map[string]string{"id": ids[index], "type": "agent_message", "text": text}})
		if err != nil {
			t.Fatal(err)
		}
		stdout += string(record) + "\n"
	}
	stdout += "{\"type\":\"turn.completed\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}\n"
	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{Stdout: []byte(stdout)})
	routes.setFor(dir, runner)
	t.Cleanup(func() { routes.setFor(dir, nil) })
	id := ""
	if origin == "direct" {
		id = invokeNativeFinalDirect(t, host, dir, "codex", environment)
	} else {
		id = invokeNativeFinalFactory(t, host, dir, "codex", environment)
	}
	facts, err := support.WaitForObservation(30*time.Second, func() (factoryapi.WorkerSessionObservation, error) {
		return support.GetJSON[factoryapi.WorkerSessionObservation](t, host.URL()+"/worker-sessions/"+id), nil
	}, func(o factoryapi.WorkerSessionObservation) bool { return o.EndedAt != nil })
	if err != nil || facts.State != "COMPLETED" {
		t.Fatalf("physical outcome: %+v %v", facts, err)
	}
	page := assertRejectedCapturePages(t, host, id, environment)
	assertNativeMessageProjection(t, page, ids, types, texts)
	transcript := support.GetJSON[factoryapi.WorkerSessionTranscriptResponse](t, host.URL()+"/worker-sessions/"+id+"/transcript")
	want := 2
	if shape == "same-item-snapshots" {
		want = 1
	}
	if transcript.State != "COMPLETED" || len(transcript.Entries) != want {
		t.Fatalf("genuine messages/snapshot projection: %+v want %d entries", transcript, want)
	}
	for _, entry := range transcript.Entries {
		if entry.Text == nil || *entry.Text != body || entry.Type != "assistant_message" {
			t.Fatalf("final transcript content: %+v", entry)
		}
	}
	if len(runner.Requests()) != 1 {
		t.Fatalf("provider calls=%d, want one physical turn", len(runner.Requests()))
	}
}

func assertNativeMessageProjection(t *testing.T, page factoryapi.WorkerSessionLogPage, ids, types, texts []string) {
	t.Helper()
	index := 0
	for _, event := range page.Events {
		encoded, err := json.Marshal(event.Event.Payload)
		if err != nil {
			t.Fatal(err)
		}
		var draft workers.Draft
		if err := json.Unmarshal(encoded, &draft); err != nil {
			t.Fatal(err)
		}
		if draft.Kind != workers.KindMessage {
			continue
		}
		nativeType := strings.Replace(types[min(index, len(types)-1)], "item.", "message.", 1)
		if nativeType == "message.updated" {
			nativeType = "message.delta"
		}
		if index >= len(ids) || draft.ItemID != ids[index] || draft.Provenance.NativeEventType != nativeType || draft.Provenance.Delivery != workers.DeliveryNativeStream {
			t.Fatalf("native message identity/source order at %d: %+v", index, draft)
		}
		if draft.Phase == workers.PhaseDelta {
			var payload workers.MessageDeltaPayload
			if err := json.Unmarshal(draft.Payload, &payload); err != nil || payload.TextDelta != texts[index] {
				t.Fatalf("native delta content: %+v %v", payload, err)
			}
		} else {
			assertCompletedEnvelope(t, draft, texts[index])
		}
		index++
	}
	if index != len(ids) {
		t.Fatalf("native messages=%d, want %d; capture=%+v", index, len(ids), page)
	}
}

func claudeNativeFinalFixture(t *testing.T, body string) []byte {
	t.Helper()
	var lines []string
	for _, record := range []any{
		map[string]any{"type": "system", "subtype": "init", "session_id": "private-native-claude"},
		map[string]any{"type": "assistant", "session_id": "private-native-claude", "message": map[string]any{
			"id": "private-native-message", "role": "assistant", "content": []map[string]string{{"type": "text", "text": body}},
		}},
		map[string]any{"type": "result", "subtype": "success", "is_error": false, "result": body, "session_id": "private-native-claude"},
	} {
		data, err := json.Marshal(record)
		if err != nil {
			t.Fatal(err)
		}
		lines = append(lines, string(data))
	}
	return []byte(strings.Join(lines, "\n") + "\n")
}

func invokeNativeFinalDirect(t *testing.T, host *support.FunctionalAPIServer, dir, provider string, environment []string) string {
	t.Helper()
	path := writeRejectedDirectExecution(t, dir, provider)
	inputs := support.FakeInputs(t.Context(), []string{"you", "--server", host.URL(), "--remote", "--json", "worker-sessions", "invoke", "--execution", path, "--async"})
	inputs.Input.WorkingDirectory, inputs.Input.Env = dir, environment
	// The public example has a fixed ID. Give each parallel direct execution
	// its own identity in the same public request shape.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	id := uuid.NewString()
	document["workerSessionId"], document["requestId"] = id, id+"-request"
	dispatch := document["execution"].(map[string]any)["dispatch"].(map[string]any)
	dispatch["dispatchId"] = id + "-attempt"
	dispatch["execution"].(map[string]any)["requestId"] = id + "-request"
	data, err = json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := host.Execute(t, inputs.Input); err != nil {
		t.Fatalf("direct invoke: %v %s", err, inputs.Stderr())
	}
	return id
}

func invokeNativeFinalFactory(t *testing.T, host *support.FunctionalAPIServer, dir, provider string, environment []string) string {
	t.Helper()
	model := "gpt-5"
	if provider == "claude" {
		model = "claude-sonnet-4-6"
	}
	support.WriteAgentConfig(t, dir, "processor", "---\ntype: AGENT_WORKER\nmodelProvider: "+strings.ToUpper(provider)+"\nmodel: "+model+"\nexecutorProvider: "+strings.ToUpper(provider)+"\n---\nReturn the requested reply.\n")
	id := uuid.NewString()
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{WorkID: "target", WorkTypeID: "task", TraceID: id, Payload: []byte("PRIVATE_NATIVE_INPUT")})
	ctx, cancel := context.WithCancel(t.Context())
	inputs := support.FakeInputs(ctx, []string{"you", "run", "--session=" + id, "--dir", dir, "--continuously", "--quiet", "--no-record"})
	inputs.Input.WorkingDirectory, inputs.Input.Env = dir, environment
	joined := make(chan struct{})
	go func() { defer close(joined); _ = host.Execute(t, inputs.Input) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-joined:
		case <-time.After(30 * time.Second):
			t.Error("native Factory invocation did not join")
		}
	})
	support.WaitForSessionTerminalStatus(t, host.URL(), id, 20*time.Second)
	base := host.URL() + "/factory-sessions/" + id
	listed := support.GetJSON[factoryapi.ListWorkResponse](t, base+"/work")
	if support.CountWorkAtCustomerState(listed, "task:complete") != 1 {
		t.Fatalf("Factory Work outcome: %+v; stderr=%s", listed, inputs.Stderr())
	}
	dispatches := support.ObserveDispatchEvents(t, support.GetFactoryEventsForSessionAt(t, host.URL(), id))
	if len(dispatches) != 1 || dispatches[0].Response == nil || support.StringPointerValue(dispatches[0].Response.Output) != "PRIVATE_NATIVE_FINAL" {
		t.Fatalf("native envelope did not normalize to Work output: %+v", dispatches)
	}
	rows := support.GetJSON[factoryapi.ListWorkerSessionsResponse](t, base+"/worker-sessions?workId=target")
	if len(rows.Sessions) != 1 || rows.Sessions[0].FactorySessionId == nil || *rows.Sessions[0].FactorySessionId != id {
		t.Fatalf("Factory Worker association: %+v", rows)
	}
	return rows.Sessions[0].WorkerSessionId
}

func assertNativeFinalCapture(t *testing.T, host *support.FunctionalAPIServer, id, provider, body string, environment []string) {
	t.Helper()
	facts, err := support.WaitForObservation(10*time.Second, func() (factoryapi.WorkerSessionObservation, error) {
		return support.GetJSON[factoryapi.WorkerSessionObservation](t, host.URL()+"/worker-sessions/"+id), nil
	}, func(o factoryapi.WorkerSessionObservation) bool { return o.EndedAt != nil })
	if err != nil || facts.State != "COMPLETED" || facts.Provider == nil || *facts.Provider != provider {
		t.Fatalf("native physical completion: %+v %v", facts, err)
	}
	page := assertRejectedCapturePages(t, host, id, environment)
	completed := completedAssistantDrafts(t, page)
	if len(completed) != 1 {
		t.Fatalf("completed native messages=%d, want one", len(completed))
	}
	assertCompletedEnvelope(t, completed[0], body)
	if completed[0].Provenance.Provider != provider || completed[0].Provenance.Delivery != workers.DeliveryNativeStream {
		t.Fatalf("native provenance changed: %+v", completed[0].Provenance)
	}
}

func completedAssistantDrafts(t *testing.T, page factoryapi.WorkerSessionLogPage) []workers.Draft {
	t.Helper()
	var completed []workers.Draft
	for _, event := range page.Events {
		raw, err := json.Marshal(event.Event.Payload)
		if err != nil {
			t.Fatal(err)
		}
		var draft workers.Draft
		if err := json.Unmarshal(raw, &draft); err != nil {
			t.Fatal(err)
		}
		if draft.Kind == workers.KindMessage && draft.Phase == workers.PhaseCompleted {
			var payload workers.MessagePayload
			if err := json.Unmarshal(draft.Payload, &payload); err != nil {
				t.Fatal(err)
			}
			if payload.Role == "assistant" {
				completed = append(completed, draft)
			}
		}
	}
	return completed
}

func assertCompletedEnvelope(t *testing.T, draft workers.Draft, body string) {
	t.Helper()
	var payload workers.MessagePayload
	if err := json.Unmarshal(draft.Payload, &payload); err != nil || payload.Role != "assistant" || len(payload.ContentBlocks) != 1 || payload.ContentBlocks[0].Text != body {
		t.Fatalf("native envelope changed: %+v %v", payload, err)
	}
}

// Each provider/rejection shape is a distinct immutable mock policy. Its direct
// and Factory cases share one root. Separate policies also isolate the mock's
// fixed third-party session ID, which direct execution treats as session-owned.
func TestFactoryFinalCaptureRejected(t *testing.T) {
	t.Parallel()
	for _, provider := range []string{"codex", "claude"} {
		for _, shape := range []string{"stdout", "stderr", "both", "absent"} {
			t.Run(provider+"/"+shape, func(t *testing.T) {
				t.Parallel()
				entry := finalRejectionEntry(provider, shape)
				entry.WorkstationName = ""
				path := writeBatchMockWorkersConfig(t, workers.MockWorkersConfig{MockWorkers: []workers.MockWorkerConfig{entry}})
				denied := &directExampleDeniedRunner{}
				hostEnvironment := append(finalCaptureEnvironment(t), "API_KEY=declared-credential")
				host := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
					FactoryDir: publishedDirectMockFactory(t), WaitForServiceModeRuntime: true,
					Env: hostEnvironment, Args: []string{"--with-mock-workers=" + path},
					Edges: serviceedges.Edges{ProviderCommandRunner: denied, ScriptCommandRunner: denied},
				})
				t.Cleanup(func() {
					if denied.calls.Load() != 0 {
						t.Errorf("native/script calls=%d", denied.calls.Load())
					}
				})
				for _, origin := range []string{"direct", "factory"} {
					t.Run(origin, func(t *testing.T) {
						t.Parallel()
						dir, environment := declaredResultFactory(t), finalCaptureEnvironment(t)
						environment = append(environment, "API_KEY=declared-credential")
						id := ""
						if origin == "direct" {
							id = invokeNativeFinalDirect(t, host, dir, provider, environment)
						} else {
							id = invokeRejectedFinalFactory(t, host, dir, provider, environment, entry)
						}
						assertFinalRejection(t, host, id, provider, environment, entry.RejectConfig)
					})
				}
			})
		}
	}
}
func finalRejectionEntry(provider, shape string) workers.MockWorkerConfig {
	exit, tokens, zero := 42, int64(11), int64(0)
	rejection := &workers.MockWorkerRejectConfig{ExitCode: &exit}
	if shape == "stdout" || shape == "both" {
		rejection.Stdout = "PRIVATE final stdout 世界 declared-credential"
	}
	if shape == "stderr" || shape == "both" {
		rejection.Stderr = "PRIVATE final stderr 世界 declared-credential"
	}
	return workers.MockWorkerConfig{
		RunType: workers.MockWorkerRunTypeReject, RejectConfig: rejection,
		Usage: &workers.MockWorkerUsageConfig{Provider: provider, Model: "private-usage-model", InputTokens: &tokens, OutputTokens: &zero},
	}
}

func invokeRejectedFinalFactory(t *testing.T, host *support.FunctionalAPIServer, dir, provider string, environment []string, entry workers.MockWorkerConfig) string {
	t.Helper()
	model := "gpt-5"
	if provider == "claude" {
		model = "claude-sonnet-4-6"
	}
	support.WriteAgentConfig(t, dir, "processor", "---\ntype: AGENT_WORKER\nmodelProvider: "+strings.ToUpper(provider)+"\nmodel: "+model+"\nexecutorProvider: "+strings.ToUpper(provider)+"\n---\nReturn the requested reply.\n")
	entry.WorkstationName = ""
	// Declare the same secret at the Factory execution boundary as the direct
	// request's envVars. Invocation-local CLI environment is not worker policy.
	factoryPath := filepath.Join(dir, "factory.json")
	data, err := os.ReadFile(factoryPath)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	document["workstations"].([]any)[0].(map[string]any)["env"] = map[string]string{"API_KEY": "declared-credential"}
	data, err = json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(factoryPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	path := writeBatchMockWorkersConfig(t, workers.MockWorkersConfig{MockWorkers: []workers.MockWorkerConfig{entry}})
	id := uuid.NewString()
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{WorkID: "target", WorkTypeID: "task", TraceID: id, Payload: []byte("PRIVATE rejected input")})
	ctx, cancel := context.WithCancel(t.Context())
	inputs := support.FakeInputs(ctx, []string{"you", "run", "--session=" + id, "--dir", dir, "--with-mock-workers=" + path, "--continuously", "--quiet", "--no-record"})
	inputs.Input.WorkingDirectory, inputs.Input.Env = dir, environment
	joined := make(chan struct{})
	go func() { defer close(joined); _ = host.Execute(t, inputs.Input) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-joined:
		case <-time.After(30 * time.Second):
			t.Error("rejected Factory invocation did not join")
		}
	})
	support.WaitForSessionTerminalStatus(t, host.URL(), id, 30*time.Second)
	base := host.URL() + "/factory-sessions/" + id
	listed := support.GetJSON[factoryapi.ListWorkResponse](t, base+"/work")
	if len(listed.Results) != 1 || listed.Results[0].State == nil || listed.Results[0].State.Type != factoryapi.WorkStateTypeFAILED {
		t.Fatalf("rejected Work outcome: %+v", listed)
	}
	rows := support.GetJSON[factoryapi.ListWorkerSessionsResponse](t, base+"/worker-sessions?workId=target")
	if len(rows.Sessions) != 1 || rows.Sessions[0].FactorySessionId == nil || *rows.Sessions[0].FactorySessionId != id {
		t.Fatalf("rejected Factory Worker identity: %+v", rows)
	}
	return rows.Sessions[0].WorkerSessionId
}

func assertFinalRejection(t *testing.T, host *support.FunctionalAPIServer, id, provider string, environment []string, rejection *workers.MockWorkerRejectConfig) {
	t.Helper()
	facts, err := support.WaitForObservation(30*time.Second, func() (factoryapi.WorkerSessionObservation, error) {
		return support.GetJSON[factoryapi.WorkerSessionObservation](t, host.URL()+"/worker-sessions/"+id), nil
	}, func(o factoryapi.WorkerSessionObservation) bool {
		return o.EndedAt != nil && o.RecordingHealth != nil && *o.RecordingHealth == "COMPLETE"
	})
	if err != nil {
		t.Fatal(err)
	}
	assertFinalRejectionSelection(t, facts, provider)
	assertDeclaredUsage(t, facts.TokenUsage, 11)
	page := assertRejectedCapturePages(t, host, id, environment)
	assertFinalRejectedUsage(t, page, provider)
	var captured bytes.Buffer
	encoder := json.NewEncoder(&captured)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(page.Events); err != nil {
		t.Fatal(err)
	}
	encoded := captured.String()
	if strings.Contains(encoded, "declared-credential") || strings.Contains(encoded, "agent_final_response") || len(completedAssistantDrafts(t, page)) != 0 {
		t.Fatalf("failed capture: leaked=%v fallback=%v completed=%d", strings.Contains(encoded, "declared-credential"), strings.Contains(encoded, "agent_final_response"), len(completedAssistantDrafts(t, page)))
	}
	assertFinalRejectedOutput(t, encoded, rejection)
}

func assertFinalRejectedUsage(t *testing.T, page factoryapi.WorkerSessionLogPage, provider string) {
	t.Helper()
	want := map[string]any{"origin": "SYNTHETIC", "model": "private-usage-model",
		"inputTokens": float64(11), "outputTokens": float64(0), "totalTokens": float64(11)}
	count := 0
	for _, frame := range page.Events {
		data, err := json.Marshal(frame.Event.Payload)
		if err != nil {
			t.Fatal(err)
		}
		var draft workers.Draft
		if err := json.Unmarshal(data, &draft); err != nil {
			t.Fatal(err)
		}
		if draft.Kind != workers.KindUsage {
			continue
		}
		count++
		var fact map[string]any
		if err := json.Unmarshal(draft.Payload, &fact); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(fact, want) || draft.Provenance.Provider != provider || draft.Provenance.Delivery != workers.DeliverySynthesized {
			t.Fatalf("rejected synthetic usage/explicit zero changed: %+v %+v", fact, draft.Provenance)
		}
	}
	if count != 1 {
		t.Fatalf("rejected usage observations=%d, want one", count)
	}
}

func assertFinalRejectionSelection(t *testing.T, facts factoryapi.WorkerSessionObservation, provider string) {
	t.Helper()
	if facts.State != "FAILED" || facts.TerminalCause == nil || *facts.TerminalCause != "FAILED" || facts.Provider == nil || *facts.Provider != provider {
		t.Fatalf("rejected physical outcome/selection: %+v", facts)
	}
	model := "gpt-5"
	if provider == "claude" {
		model = "claude-sonnet-4-6"
		if facts.Direct {
			model = "claude-test"
		}
	}
	if facts.Model == nil || *facts.Model != model {
		t.Fatalf("execution model=%v, want %s separate from configured usage", facts.Model, model)
	}
}

func assertFinalRejectedOutput(t *testing.T, encoded string, rejection *workers.MockWorkerRejectConfig) {
	t.Helper()
	for _, text := range []string{"PRIVATE final stdout 世界", "PRIVATE final stderr 世界"} {
		want := 0
		if strings.Contains(rejection.Stdout+rejection.Stderr, text) {
			want = 1
		}
		if strings.Count(encoded, text) != want {
			t.Fatalf("rejected output count for %q: %s want %d", text, encoded, want)
		}
		if strings.Count(encoded, text+" <redacted>") != want {
			t.Fatalf("rejected sensitive content was not redacted for %q", text)
		}
	}
	if rejection.Stdout != "" && rejection.Stderr != "" && strings.Index(encoded, "final stdout") > strings.Index(encoded, "final stderr") {
		t.Fatalf("configured output order changed: %s", encoded)
	}
}

// Each independent scenario owns a profile and runs in parallel. Within a
// scenario, shutdown must join before reopening the same durable directory:
// concurrent hosts cannot prove clean-restart capture continuity.
func TestFactoryFinalCaptureRestart(t *testing.T) {
	t.Parallel()
	for _, shape := range []string{"native-envelope", "result-only", "business-invalid"} {
		t.Run(shape, func(t *testing.T) {
			t.Parallel()
			dir, environment := declaredResultFactory(t), finalCaptureEnvironment(t)
			support.WriteAgentConfig(t, dir, "processor", "---\ntype: AGENT_WORKER\nmodelProvider: CLAUDE\nmodel: claude-sonnet-4-6\nexecutorProvider: CLAUDE\n---\nReturn the requested reply.\n")
			body, stdout := finalRestartFixture(t, shape)
			runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{Stdout: stdout})
			denied := &directExampleDeniedRunner{}
			cfg := support.FunctionalAPIServerConfig{FactoryDir: dir, WaitForServiceModeRuntime: true,
				Env: environment, Edges: serviceedges.Edges{ProviderCommandRunner: runner, ScriptCommandRunner: denied}}
			host := support.StartFunctionalAPIServer(t, cfg)
			scope := support.OpenFactorySessionAt(t, host.URL(), dir).Session.Id
			submitted := support.SubmitSessionWorkAt(t, host.URL(), scope, factoryapi.SubmitWorkRequest{WorkTypeName: "task", Payload: "PRIVATE_RESTART_INPUT"})
			status := support.WaitForSessionTerminalStatus(t, host.URL(), scope, 30*time.Second)
			wantFailed, wantTerminal := 0, 1
			if shape == "business-invalid" {
				wantFailed, wantTerminal = 1, 0
			}
			if status.Categories.Failed != wantFailed || status.Categories.Terminal != wantTerminal {
				t.Fatalf("business outcome for %s: %+v", shape, status)
			}
			rows := support.GetJSON[factoryapi.ListWorkerSessionsResponse](t, host.URL()+"/factory-sessions/"+scope+"/worker-sessions?workId="+*submitted.WorkId)
			if len(rows.Sessions) != 1 {
				t.Fatalf("Factory membership: %+v", rows)
			}
			id := rows.Sessions[0].WorkerSessionId
			facts := waitFinalCaptureComplete(t, host, id)
			if facts.FactorySessionId == nil || *facts.FactorySessionId != scope || len(facts.WorkIds) != 1 || facts.WorkIds[0] != *submitted.WorkId {
				t.Fatalf("capture attribution: %+v", facts)
			}
			logs := assertRejectedCapturePages(t, host, id, environment)
			assertFinalRestartMessage(t, logs, shape, body)
			transcript := assertFinalCaptureInspection(t, host, facts, "all", environment)
			host.Close(t)
			host = support.StartFunctionalAPIServer(t, cfg)
			restored := waitFinalCaptureComplete(t, host, id)
			if !equalRejectedCaptureFacts(facts, restored) {
				t.Fatalf("restart changed physical facts: before=%+v after=%+v", facts, restored)
			}
			if got := assertRejectedCapturePages(t, host, id, environment); !reflect.DeepEqual(logs, got) {
				t.Fatalf("restart changed acknowledged ordered capture: before=%+v after=%+v", logs, got)
			}
			if got := assertFinalCaptureInspection(t, host, restored, "archived", environment); !reflect.DeepEqual(transcript, got) {
				t.Fatalf("restart changed transcript: before=%+v after=%+v", transcript, got)
			}
			if len(runner.Requests()) != 1 || denied.calls.Load() != 0 {
				t.Fatalf("restart execution effects: provider=%d script=%d", len(runner.Requests()), denied.calls.Load())
			}
		})
	}
}

func finalRestartFixture(t *testing.T, shape string) (string, []byte) {
	t.Helper()
	body := `{"decision":"ACCEPTED","output":"PRIVATE_RESTART_FINAL"}`
	if shape == "business-invalid" {
		invalid, _ := businessInvalidResultExample(t)
		body = string(invalid)
	}
	if shape != "result-only" {
		return body, claudeNativeFinalFixture(t, body)
	}
	record, err := json.Marshal(map[string]any{"type": "result", "subtype": "success", "is_error": false, "result": body, "session_id": "private-restart-claude"})
	if err != nil {
		t.Fatal(err)
	}
	return body, append(record, '\n')
}

func assertFinalRestartMessage(t *testing.T, logs factoryapi.WorkerSessionLogPage, shape, body string) {
	t.Helper()
	completed := completedAssistantDrafts(t, logs)
	if len(completed) != 1 {
		t.Fatalf("completions=%d, want one: %+v", len(completed), logs)
	}
	assertCompletedEnvelope(t, completed[0], body)
	provenance := completed[0].Provenance
	if shape == "result-only" {
		if provenance.Delivery != workers.DeliverySynthesized || provenance.Fidelity != workers.FidelityFinalOnly || provenance.Provider != "agent-run" {
			t.Fatalf("fallback provenance: %+v", provenance)
		}
	} else if provenance.Delivery != workers.DeliveryNativeStream || provenance.Provider != "claude" {
		t.Fatalf("native provenance: %+v", provenance)
	}
}

func waitFinalCaptureComplete(t *testing.T, host *support.FunctionalAPIServer, id string) factoryapi.WorkerSessionObservation {
	t.Helper()
	facts, err := support.WaitForObservation(30*time.Second, func() (factoryapi.WorkerSessionObservation, error) {
		return support.GetJSON[factoryapi.WorkerSessionObservation](t, host.URL()+"/worker-sessions/"+id), nil
	}, func(o factoryapi.WorkerSessionObservation) bool {
		return o.EndedAt != nil && o.RecordingHealth != nil && *o.RecordingHealth == "COMPLETE"
	})
	if err != nil || facts.State != "COMPLETED" || facts.TerminalCause == nil || *facts.TerminalCause != "COMPLETED" {
		t.Fatalf("completed physical capture: %+v %v", facts, err)
	}
	return facts
}

func assertFinalCaptureInspection(t *testing.T, host *support.FunctionalAPIServer, facts factoryapi.WorkerSessionObservation, history string, environment []string) factoryapi.WorkerSessionTranscriptResponse {
	t.Helper()
	connection, closeMCP := rejectedCaptureMCP(t, host, environment)
	defer closeMCP()
	assertRejectedCaptureList(t, host, connection, facts, history, environment)
	id := facts.WorkerSessionId
	transcript := support.GetJSON[factoryapi.WorkerSessionTranscriptResponse](t, host.URL()+"/worker-sessions/"+id+"/transcript")
	var cli, remote factoryapi.WorkerSessionTranscriptResponse
	rejectedInspectionCLI(t, host, environment, []string{"worker-sessions", "read", "--worker-session-id", id, "--view", "transcript"}, &cli)
	rejectedInspectionMCP(t, connection, map[string]any{"action": "READ", "workerSessionId": id, "view": "transcript"}, &remote)
	if !reflect.DeepEqual(transcript, cli) || !reflect.DeepEqual(transcript, remote) || transcript.State != "COMPLETED" || transcript.WorkerSessionId != id || transcript.AttemptId != facts.AttemptId || !reflect.DeepEqual(transcript.WorkIds, facts.WorkIds) || len(transcript.Entries) != 1 {
		t.Fatalf("completed transcript parity/attribution: HTTP=%+v CLI=%+v MCP=%+v", transcript, cli, remote)
	}
	return transcript
}
