package workersessions_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/internal/functionalevidence"
)

// A reusable production process serves independent stdio sessions. The host is
// a schema-shaped external HTTP fixture; this proves transport composition,
// not runtime control policy. No binary or provider is executed by this suite.
const observationJSON = `{"workerSessionId":"host-worker","direct":true,"providerSessionAvailable":true,"providerSession":{"id":"fixture-provider-session","kind":"session","provider":"controlled"},"workIds":[],"turnId":null,"attemptId":"attempt-host","state":"COMPLETED","confirmationState":"CONFIRMED","startedAt":null,"endedAt":null,"durationMillis":null,"durationBasis":"UNAVAILABLE","transcript":"AVAILABLE","parse":{"errors":[],"eventCount":0,"malformedLineCount":0,"unknownEventCount":0}}`
const transcriptJSON = `{"workerSessionId":"host-worker","providerSession":{"id":"fixture-provider-session","kind":"session","provider":"controlled"},"workIds":[],"turnId":null,"attemptId":"attempt-host","state":"COMPLETED","entries":[]}`
const summaryFrameJSON = `{"workerSessionId":"host-worker","providerSession":{"id":"fixture-provider-session","kind":"session","provider":"controlled"},"workIds":[],"delivery":"REPLAY_SUMMARY","errorCode":null,"errorMessage":null,"event":{"cursor":{"position":0},"position":0,"payload":{},"schemaId":"worker-session-replay-summary","sourceEventId":"summary","sourceId":"host-worker","sourceSequence":0,"sourceType":"worker"},"replaySummary":{"complete":true,"eventsEmitted":0,"kind":"replay-summary","reason":"COMPLETED"}}`
const interruptJSON = `{"accepted":true,"phase":"SUCCESSOR_ADMISSION","requestId":"request","sourceWorkerSessionId":"host-worker","successorWorkerSessionId":"successor","source":{"workerSessionId":"host-worker","state":"CANCELED","eventTopic":"worker/host-worker"},"successor":{"workerSessionId":"successor","state":"STARTING","eventTopic":"worker/successor"}}`

func TestWorkerSessionMCPParity(t *testing.T) {
	t.Run("parity", runSelectedHostScenarios)
	functionalevidence.Covers(t, "mcp/mcp.tool.you.subagent",
		"cli/you.worker-sessions.terminate", "rest/terminateWorkerSession")
}

func runSelectedHostScenarios(t *testing.T) {
	process := newSelectedHostClientProcess(t)
	t.Run("run subagent", func(t *testing.T) {
		t.Parallel()
		workDir := filepath.Join(t.TempDir(), "run-subagent")
		if err := os.MkdirAll(workDir, 0o755); err != nil {
			t.Fatal(err)
		}
		env := append([]string(nil), process.environment...)
		source := support.InstallPackagedFactoryWithProcess(t, process, env, workDir, "@you/subagent")
		support.CreateNamedFactoryAtRootWithProcess(t, process, env, workDir, filepath.Join(workDir, "factory"), "@you/subagent", filepath.Join(source, "factory.json"))
		session, ctx, _ := startCancellableMCP(t, process, "http://127.0.0.1:1", workDir)
		for _, action := range []string{"", "RUN"} {
			args := map[string]any{"prompt": "Return the controlled answer", "provider": "codex", "model": "test-model", "workingRoot": workDir}
			if action != "" {
				args["action"] = action
			}
			result := callTool(t, ctx, session, "you.subagent", args)
			if result.IsError || len(result.Content) != 1 || !strings.Contains(result.Content[0].(*mcp.TextContent).Text, "controlled subagent answer") {
				t.Fatalf("RUN action=%q result=%#v", action, result)
			}
		}
	})
	t.Run("unknown action has no effects", func(t *testing.T) {
		t.Parallel()
		host := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			t.Error("unknown action reached selected host")
		}))
		t.Cleanup(host.Close)
		session, ctx := startMCP(t, process, host.URL)
		assertToolError(t, callTool(t, ctx, session, "you.subagent", map[string]any{"action": "UNKNOWN", "prompt": "must not run"}), "worker_session.invalid_request", false)
	})
	t.Run("real host exact target controls", func(t *testing.T) {
		t.Parallel()
		runRealHostControls(t, process)
	})
	t.Run("real host force target controls", func(t *testing.T) {
		t.Parallel()
		runRealHostForceControls(t, process)
	})
	for _, mode := range []string{"unattached", "declined", "failed"} {
		t.Run("real host force "+mode, func(t *testing.T) {
			t.Parallel()
			runForceHostFailure(t, process, mode)
		})
	}
	t.Run("real host force caller disconnect", func(t *testing.T) {
		t.Parallel()
		runDetachedForceControl(t, process)
	})
	for _, phase := range []string{"intent", "result"} {
		for _, mode := range []string{"confirmed", "failed"} {
			t.Run("real host force persistence "+phase+" "+mode, func(t *testing.T) {
				t.Parallel()
				runForcePersistenceFailure(t, process, phase, mode)
			})
		}
	}
	t.Run("real host history snapshots", func(t *testing.T) {
		t.Parallel()
		runRealHostHistory(t, process)
	})
	t.Run("real host history recovery", func(t *testing.T) {
		t.Parallel()
		runRealHostHistoryRecovery(t, process)
	})
	t.Run("real host captured metadata recovery", func(t *testing.T) {
		t.Parallel()
		runCapturedMetadataRecovery(t, process)
	})
	t.Run("real host captured logs polling", func(t *testing.T) {
		t.Parallel()
		runCapturedLogsPolling(t, process)
	})
	t.Run("real host captured logs cursor isolation", func(t *testing.T) {
		t.Parallel()
		runCapturedLogsCursorIsolation(t, process)
	})
	for _, mode := range []string{"", "provider", "recorded"} {
		t.Run("real host interrupt "+mode, func(t *testing.T) {
			t.Parallel()
			runRealHostInterrupt(t, process, mode)
		})
	}
	for _, mode := range []string{"provider", "recorded"} {
		t.Run("real host partial interrupt "+mode, func(t *testing.T) {
			t.Parallel()
			runRealHostPartialInterrupt(t, process, mode)
		})
	}
	t.Run("real host Factory discovery and reads", func(t *testing.T) {
		t.Parallel()
		runRealHostFactory(t, process)
	})
	t.Run("selected host request cancels with MCP session", func(t *testing.T) {
		t.Parallel()
		runHostCancellation(t, process)
	})
	t.Run("discovery and reads", func(t *testing.T) {
		t.Parallel()
		queries := make(chan string, 2)
		host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/worker-sessions" && r.URL.Query().Has("history") {
				if r.Method != http.MethodGet || r.URL.Query().Get("scope") != "all" {
					t.Errorf("documented LIST request = %s %s", r.Method, r.URL)
				}
				queries <- r.URL.Query().Get("history")
			}
			readHost(w, r)
		}))
		t.Cleanup(host.Close)
		args := packagedOperationsListInput(t, process)
		session, ctx := startMCP(t, process, host.URL)
		assertWorkerDiscovery(t, ctx, session)
		for _, history := range []string{"archived", "all"} {
			result := callTool(t, ctx, session, "you.subagent", args)
			if result.IsError || len(result.Content) != 1 {
				t.Fatalf("packaged LIST history=%s: %#v", history, result)
			}
			var listed map[string]any
			if err := json.Unmarshal([]byte(result.Content[0].(*mcp.TextContent).Text), &listed); err != nil {
				t.Fatal(err)
			}
			assertJSONEqual(t, listed["result"], getHost(t, host.URL+"/worker-sessions"))
			select {
			case got := <-queries:
				if got != history {
					t.Fatalf("selected host history=%q, want %q", got, history)
				}
			default:
				t.Fatal("documented LIST did not query the selected host")
			}
			delete(args, "history")
		}
		for _, view := range []string{"summary", "transcript", "events"} {
			result := callWorker(t, ctx, session, "read", map[string]any{"workerSessionId": "host-worker", "view": view})["result"].(map[string]any)
			assertJSONEqual(t, result["session"], getHost(t, host.URL+"/worker-sessions/host-worker"))
			assertReadView(t, result, view, host.URL)
		}
		legacy := callTool(t, ctx, session, "you.factory_session.get", map[string]any{"sessionId": "missing-local"})
		assertToolError(t, legacy, "factory_session.session.not_found", false)
	})
	t.Run("control response parity", func(t *testing.T) {
		t.Parallel()
		host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost || !strings.HasPrefix(r.URL.Path, "/worker-sessions/host-worker/") {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			if strings.HasSuffix(r.URL.Path, "/interrupt") {
				w.WriteHeader(http.StatusAccepted)
				_, _ = io.WriteString(w, interruptJSON)
				return
			}
			action := strings.ToUpper(strings.TrimPrefix(r.URL.Path, "/worker-sessions/host-worker/"))
			_ = json.NewEncoder(w).Encode(map[string]any{"workerSessionId": "host-worker", "dispatchId": "attempt-host", "action": action, "outcome": "NOOP", "state": "CANCELED"})
		}))
		t.Cleanup(host.Close)
		session, ctx := startMCP(t, process, host.URL)
		for _, operation := range []string{"CANCEL", "TERMINATE", "INTERRUPT"} {
			args := map[string]any{"workerSessionId": "host-worker", "operation": operation}
			if operation == "INTERRUPT" {
				args["requestId"], args["successorWorkerSessionId"], args["replacementMessage"] = "request", "successor", "replacement"
			}
			result := callWorker(t, ctx, session, "control", args)["result"]
			assertJSONEqual(t, result, requestHost(t, http.MethodPost, host.URL+"/worker-sessions/host-worker/"+strings.ToLower(operation)))
		}
	})
	t.Run("invalid mode makes no HTTP request", func(t *testing.T) {
		t.Parallel()
		host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Error("invalid mode reached HTTP")
			w.WriteHeader(http.StatusInternalServerError)
		}))
		t.Cleanup(host.Close)
		session, ctx := startMCP(t, process, host.URL)
		for _, mode := range []any{"", "unknown", "Provider", nil, 1, true} {
			args := interruptPayload("replacement")
			args["workerSessionId"], args["operation"], args["resumeMode"] = "source", "INTERRUPT", mode
			assertToolError(t, callAction(t, ctx, session, "CONTROL", args), "worker_session.invalid_request", false)
		}
		for _, op := range []string{"CANCEL", "TERMINATE", "KILL"} {
			assertToolError(t, callAction(t, ctx, session, "CONTROL", map[string]any{"workerSessionId": "source", "operation": op, "resumeMode": "recorded"}), "worker_session.invalid_request", false)
		}
	})
	t.Run("typed errors and unavailable host", func(t *testing.T) {
		t.Parallel()
		host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			status := http.StatusNotFound
			if strings.HasSuffix(r.URL.Path, "/denied") {
				status = http.StatusForbidden
			} else if strings.HasSuffix(r.URL.Path, "/unavailable") {
				status = http.StatusServiceUnavailable
			}
			w.WriteHeader(status)
			_, _ = io.WriteString(w, `{"message":"secret-from-host"}`)
		}))
		t.Cleanup(host.Close)
		session, ctx := startMCP(t, process, host.URL)
		for id, code := range map[string]string{"missing": "worker_session.not_found", "denied": "worker_session.permission_denied"} {
			result := callAction(t, ctx, session, "READ", map[string]any{"workerSessionId": id})
			assertToolError(t, result, code, false)
		}
		assertToolError(t, callAction(t, ctx, session, "READ", map[string]any{"workerSessionId": "unavailable"}), "worker_session.unavailable", true)
		for _, mode := range []string{"", "provider", "recorded"} {
			args := interruptModePayload("replacement", mode)
			args["workerSessionId"], args["operation"] = "missing", "INTERRUPT"
			assertToolError(t, callAction(t, ctx, session, "CONTROL", args), "worker_session.not_found", false)
		}
		host.Close()
		for _, mode := range []string{"", "provider", "recorded"} {
			args := interruptModePayload("replacement", mode)
			args["workerSessionId"], args["operation"] = "host-worker", "INTERRUPT"
			assertToolError(t, callAction(t, ctx, session, "CONTROL", args), "worker_session.host_unavailable", true)
		}
		for _, tool := range []string{"list", "read", "control"} {
			args := map[string]any{}
			if tool != "list" {
				args["workerSessionId"] = "host-worker"
			}
			if tool == "control" {
				args["operation"] = "CANCEL"
			}
			assertToolError(t, callAction(t, ctx, session, strings.ToUpper(tool), args), "worker_session.host_unavailable", true)
		}
	})
}

type rejectLocalProvider struct{ t *testing.T }

type subagentScenarioRunner struct{ t *testing.T }

func (r subagentScenarioRunner) Run(ctx context.Context, req platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	if strings.Contains(filepath.ToSlash(req.WorkDir)+"/", "/run-subagent/") {
		return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout("controlled subagent answer")}, nil
	}
	return (rejectLocalProvider{t: r.t}).Run(ctx, req)
}

func (r rejectLocalProvider) Run(context.Context, platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	r.t.Error("selected-host MCP executed a local provider")
	return platformprocess.CommandResult{}, errors.New("local provider execution forbidden")
}

func startMCP(t *testing.T, process support.Process, host string) (*mcp.ClientSession, context.Context) {
	t.Helper()
	session, ctx, _ := startCancellableMCP(t, process, host)
	return session, ctx
}

func startCancellableMCP(t *testing.T, process support.Process, host string, workspace ...string) (*mcp.ClientSession, context.Context, context.CancelFunc) {
	t.Helper()
	// The session spans host shutdown/recovery and independent profile setup.
	// Bound individual protocol operations, rather than expiring the transport
	// while the scenario is preparing its next host.
	ctx, cancel := context.WithCancel(t.Context())
	stdinRead, stdinWrite := io.Pipe()
	stdoutRead, stdoutWrite := io.Pipe()
	done := make(chan error, 1)
	workDir := t.TempDir()
	if len(workspace) > 0 {
		workDir = workspace[0]
	}
	go func() {
		done <- process.Execute(root.Input{
			Args: []string{"you", "--server", host, "server", "mcp"}, Context: ctx,
			WorkingDirectory: workDir,
			Stdin:            stdinRead, Stdout: stdoutWrite, Stderr: io.Discard,
		})
	}()
	t.Cleanup(func() {
		cancel()
		_ = stdinWrite.Close()
		_ = stdinRead.Close()
		_ = stdoutWrite.Close()
		_ = stdoutRead.Close()
		select {
		case err := <-done:
			if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrClosedPipe) {
				t.Errorf("MCP execute: %v", err)
			}
		case <-time.After(30 * time.Second):
			t.Error("MCP execute did not join")
		}
	})
	client := mcp.NewClient(&mcp.Implementation{Name: "selected-host-parity", Version: "test"}, nil)
	connectCtx, cancelConnect := context.WithTimeout(ctx, 30*time.Second)
	defer cancelConnect()
	session, err := client.Connect(connectCtx, &mcp.IOTransport{Reader: stdoutRead, Writer: stdinWrite}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session, ctx, cancel
}

func packagedOperationsListInput(t *testing.T, process support.Process) map[string]any {
	t.Helper()
	workDir := t.TempDir()
	home := filepath.Join(workDir, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := process.Execute(root.Input{
		Args: []string{"you", "docs", "operations"}, Context: context.Background(),
		Env: append(os.Environ(), "HOME="+home, "USERPROFILE="+home), WorkingDirectory: workDir,
		Stdin: strings.NewReader(""), Stdout: &output, Stderr: io.Discard,
	}); err != nil {
		t.Fatal(err)
	}
	for _, block := range strings.Split(output.String(), "```json\n")[1:] {
		var args map[string]any
		if err := json.Unmarshal([]byte(strings.SplitN(block, "```", 2)[0]), &args); err == nil && args["action"] == "LIST" {
			if args["history"] != "archived" || args["scope"] != "all" {
				t.Fatalf("packaged archived LIST input = %v", args)
			}
			return args
		}
	}
	t.Fatal("packaged operations has no JSON LIST example")
	return nil
}

func readHost(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch r.URL.Path {
	case "/worker-sessions":
		_, _ = io.WriteString(w, `{"workerSessions":[`+observationJSON+`],"nextToken":null}`)
	case "/worker-sessions/host-worker":
		_, _ = io.WriteString(w, observationJSON)
	case "/worker-sessions/host-worker/transcript":
		_, _ = io.WriteString(w, transcriptJSON)
	case "/worker-sessions/host-worker/events":
		if r.URL.Query().Get("replayOnly") != "true" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: "+summaryFrameJSON+"\n\n")
	default:
		http.NotFound(w, r)
	}
}

func callTool(t *testing.T, ctx context.Context, session *mcp.ClientSession, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return result
}

func callAction(t *testing.T, ctx context.Context, session *mcp.ClientSession, action string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	input := make(map[string]any, len(args)+1)
	for key, value := range args {
		input[key] = value
	}
	input["action"] = action
	return callTool(t, ctx, session, "you.subagent", input)
}

func callWorker(t *testing.T, ctx context.Context, session *mcp.ClientSession, tool string, args map[string]any) map[string]any {
	t.Helper()
	result := callAction(t, ctx, session, strings.ToUpper(tool), args)
	if result.IsError || len(result.Content) != 1 {
		t.Fatalf("%s result: %#v", tool, result)
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(result.Content[0].(*mcp.TextContent).Text), &envelope); err != nil {
		t.Fatal(err)
	}
	return envelope
}

func getHost(t *testing.T, url string) any {
	t.Helper()
	return requestHost(t, http.MethodGet, url)
}

func requestHost(t *testing.T, method, url string) any {
	t.Helper()
	request, err := http.NewRequest(method, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var value any
	if err := json.NewDecoder(response.Body).Decode(&value); err != nil {
		t.Fatal(err)
	}
	return value
}

func assertJSONEqual(t *testing.T, actual, expected any) {
	t.Helper()
	left, _ := json.Marshal(actual)
	right, _ := json.Marshal(expected)
	var normalized any
	if err := json.Unmarshal(left, &normalized); err != nil {
		t.Fatal(err)
	}
	left, _ = json.Marshal(normalized)
	normalized = nil
	if err := json.Unmarshal(right, &normalized); err != nil {
		t.Fatal(err)
	}
	right, _ = json.Marshal(normalized)
	if !bytes.Equal(left, right) {
		t.Fatalf("MCP=%s HTTP=%s", left, right)
	}
}

func assertToolError(t *testing.T, result *mcp.CallToolResult, code string, retryable bool) {
	t.Helper()
	encoded, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Error struct {
			Code      string `json:"code"`
			Retryable bool   `json:"retryable"`
		} `json:"error"`
	}
	if err := json.Unmarshal(encoded, &envelope); err != nil {
		t.Fatal(err)
	}
	if !result.IsError || envelope.Error.Code != code || envelope.Error.Retryable != retryable || strings.Contains(string(encoded), "secret-from-host") {
		t.Fatalf("error result = %s isError=%v", encoded, result.IsError)
	}
}

func assertReadView(t *testing.T, result map[string]any, view, host string) {
	t.Helper()
	switch view {
	case "summary":
		if len(result) != 1 {
			t.Fatalf("summary includes extra view: %v", result)
		}
	case "transcript":
		assertJSONEqual(t, result["transcript"], getHost(t, host+"/worker-sessions/host-worker/transcript"))
	case "events":
		assertJSONEqual(t, result["events"], map[string]any{"events": []any{json.RawMessage(summaryFrameJSON)}, "truncated": false})
	}
}

func assertWorkerDiscovery(t *testing.T, ctx context.Context, session *mcp.ClientSession) {
	t.Helper()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	result, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tool := range result.Tools {
		if strings.HasPrefix(tool.Name, "you.worker_session.") {
			t.Fatalf("retired tool remains discoverable: %s", tool.Name)
		}
		if tool.Name == "you.subagent" {
			found = true
			if tool.Annotations == nil {
				t.Fatalf("missing hints: %s", tool.Name)
			}
		}
	}
	if !found || len(result.Tools) != 11 {
		t.Fatalf("subagent discovery: found=%v tools=%d", found, len(result.Tools))
	}
}

func runHostCancellation(t *testing.T, process support.Process) {
	t.Helper()
	started, canceled := make(chan struct{}), make(chan struct{})
	host := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		close(started)
		<-request.Context().Done()
		close(canceled)
	}))
	t.Cleanup(host.Close)
	session, ctx, cancel := startCancellableMCP(t, process, host.URL)
	done := make(chan error, 1)
	go func() {
		_, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "you.subagent", Arguments: map[string]any{"action": "LIST"}})
		done <- err
	}()
	waitControlSignal(t, started)
	cancel()
	waitControlSignal(t, canceled)
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled MCP call: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("canceled MCP call did not join")
	}
}
