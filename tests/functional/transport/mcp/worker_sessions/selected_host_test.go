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
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
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
	functionalevidence.Covers(t, "mcp/mcp.tool.you.worker_session.list", "mcp/mcp.tool.you.worker_session.read", "mcp/mcp.tool.you.worker_session.control")
}

func runSelectedHostScenarios(t *testing.T) {
	process := support.BuildProcess(t, serviceedges.Edges{ProviderCommandRunner: rejectLocalProvider{t: t}})
	t.Run("real host exact target controls", func(t *testing.T) {
		t.Parallel()
		runRealHostControls(t, process)
	})
	t.Run("real host interrupt and transcript", func(t *testing.T) {
		t.Parallel()
		runRealHostInterrupt(t, process)
	})
	t.Run("real host partial interrupt", func(t *testing.T) {
		t.Parallel()
		runRealHostPartialInterrupt(t, process)
	})
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
		host := httptest.NewServer(http.HandlerFunc(readHost))
		t.Cleanup(host.Close)
		session, ctx := startMCP(t, process, host.URL)
		assertWorkerDiscovery(t, ctx, session)
		listed := callWorker(t, ctx, session, "list", map[string]any{"scope": "all"})
		assertJSONEqual(t, listed["result"], getHost(t, host.URL+"/worker-sessions"))
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
			result := callTool(t, ctx, session, "you.worker_session.read", map[string]any{"workerSessionId": id})
			assertToolError(t, result, code, false)
		}
		assertToolError(t, callTool(t, ctx, session, "you.worker_session.read", map[string]any{"workerSessionId": "unavailable"}), "worker_session.unavailable", true)
		host.Close()
		for _, tool := range []string{"list", "read", "control"} {
			args := map[string]any{}
			if tool != "list" {
				args["workerSessionId"] = "host-worker"
			}
			if tool == "control" {
				args["operation"] = "CANCEL"
			}
			assertToolError(t, callTool(t, ctx, session, "you.worker_session."+tool, args), "worker_session.host_unavailable", true)
		}
	})
}

type rejectLocalProvider struct{ t *testing.T }

func (r rejectLocalProvider) Run(context.Context, platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	r.t.Error("selected-host MCP executed a local provider")
	return platformprocess.CommandResult{}, errors.New("local provider execution forbidden")
}

func startMCP(t *testing.T, process support.Process, host string) (*mcp.ClientSession, context.Context) {
	t.Helper()
	session, ctx, _ := startCancellableMCP(t, process, host)
	return session, ctx
}

func startCancellableMCP(t *testing.T, process support.Process, host string) (*mcp.ClientSession, context.Context, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	stdinRead, stdinWrite := io.Pipe()
	stdoutRead, stdoutWrite := io.Pipe()
	done := make(chan error, 1)
	workDir := t.TempDir()
	homeDir := filepath.Join(workDir, "home")
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	go func() {
		done <- process.Execute(root.Input{
			Args: []string{"you", "--server", host, "server", "mcp"}, Context: ctx,
			Env: append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir), WorkingDirectory: workDir,
			Stdin: stdinRead, Stdout: stdoutWrite, Stderr: io.Discard,
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
	session, err := client.Connect(ctx, &mcp.IOTransport{Reader: stdoutRead, Writer: stdinWrite}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session, ctx, cancel
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
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return result
}

func callWorker(t *testing.T, ctx context.Context, session *mcp.ClientSession, tool string, args map[string]any) map[string]any {
	t.Helper()
	result := callTool(t, ctx, session, "you.worker_session."+tool, args)
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
	result, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tool := range result.Tools {
		if strings.HasPrefix(tool.Name, "you.worker_session.") {
			names = append(names, tool.Name)
			if tool.Annotations == nil {
				t.Fatalf("missing hints: %s", tool.Name)
			}
		}
	}
	if !reflect.DeepEqual(names, []string{"you.worker_session.control", "you.worker_session.list", "you.worker_session.read"}) {
		t.Fatalf("Worker Session discovery: %v", names)
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
		_, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "you.worker_session.list", Arguments: map[string]any{}})
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
