package mock

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type directExampleDeniedRunner struct{ calls atomic.Int32 }

func (r *directExampleDeniedRunner) Run(context.Context, platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	r.calls.Add(1)
	return platformprocess.CommandResult{}, errors.New("native execution denied for published mock example")
}

// These private hosts have immutable provider policies and deny every native
// effect. Restart joins the original host before reopening its capture profile.
func TestRejectedMockCapturedOutput(t *testing.T) {
	t.Parallel()
	for _, provider := range []string{"codex", "claude"} {
		t.Run(provider, func(t *testing.T) {
			t.Parallel()
			dir := publishedDirectMockFactory(t)
			mockPath := filepath.Join(dir, "mock-workers.json")
			usageModel := "gpt-5"
			if provider == "claude" {
				usageModel = "claude-test"
			}
			if err := os.WriteFile(mockPath, []byte(`{"mockWorkers":[{"runType":"reject","usage":{"provider":"`+provider+`","model":"`+usageModel+`","inputTokens":10,"outputTokens":4,"totalTokens":14},"rejectConfig":{"stdout":"PRIVATE ordinary stdout 世界 declared-credential","stderr":"PRIVATE ordinary stderr 世界 declared-credential","exitCode":42}}]}`), 0o600); err != nil {
				t.Fatal(err)
			}
			environment := builtcliacceptance.ProcessEnvForIsolatedHome(publishedDirectMockHome(t))
			denied := &directExampleDeniedRunner{}
			cfg := support.FunctionalAPIServerConfig{
				FactoryDir: dir, WaitForServiceModeRuntime: true, Env: environment,
				Args:  []string{"--with-mock-workers", mockPath},
				Edges: serviceedges.Edges{ProviderCommandRunner: denied, ScriptCommandRunner: denied},
			}
			host := support.StartFunctionalAPIServer(t, cfg)
			path := writeRejectedDirectExecution(t, dir, provider)
			inputs := support.FakeInputs(t.Context(), []string{"you", "--server", host.URL(), "--remote", "--json", "worker-sessions", "invoke", "--execution", path, "--async"})
			inputs.Input.WorkingDirectory, inputs.Input.Env = dir, environment
			if err := host.Execute(t, inputs.Input); err != nil {
				t.Fatalf("invoke: %v %s", err, inputs.Stderr())
			}
			read := func() string {
				inputs := support.FakeInputs(t.Context(), []string{"you", "--server", host.URL(), "--json", "worker-sessions", "read", "--worker-session-id", "direct-example-session", "--view", "logs"})
				inputs.Input.WorkingDirectory, inputs.Input.Env = dir, environment
				if err := host.Execute(t, inputs.Input); err != nil {
					t.Fatalf("read: %v %s", err, inputs.Stderr())
				}
				return inputs.Stdout()
			}
			before, err := support.WaitForObservation(30*time.Second, func() (string, error) { return read(), nil }, func(s string) bool { return strings.Contains(s, `"health":"COMPLETE"`) })
			if err != nil {
				t.Fatal(err)
			}
			for _, marker := range []string{"PRIVATE ordinary stdout 世界", "PRIVATE ordinary stderr 世界"} {
				if strings.Count(before, marker) != 1 {
					t.Fatalf("output count for %q: %s", marker, before)
				}
			}
			if strings.Index(before, "ordinary stdout") > strings.Index(before, "ordinary stderr") {
				t.Fatalf("stream order: %s", before)
			}
			if strings.Contains(before, "declared-credential") || strings.Count(before, "redacted") != 2 {
				t.Fatalf("declared secret redaction: %s", before)
			}
			show := support.FakeInputs(t.Context(), []string{"you", "--server", host.URL(), "--json", "worker-sessions", "show", "--worker-session-id", "direct-example-session"})
			show.Input.WorkingDirectory, show.Input.Env = dir, environment
			if err := host.Execute(t, show.Input); err != nil || !strings.Contains(show.Stdout(), `"state":"FAILED"`) {
				t.Fatalf("failed show: %v %s %s", err, show.Stdout(), show.Stderr())
			}
			facts := assertRejectedCaptureFacts(t, host, "direct-example-session")
			assertRejectedCapturePages(t, host, "direct-example-session", environment)
			transcript := assertRejectedCaptureInspection(t, host, facts, "all", environment, "PRIVATE ordinary stdout 世界 <redacted>")
			host.Close(t)
			host = support.StartFunctionalAPIServer(t, cfg)
			if restored := assertRejectedCaptureFacts(t, host, "direct-example-session"); !equalRejectedCaptureFacts(facts, restored) {
				t.Fatalf("restart changed direct facts: before=%+v after=%+v", facts, restored)
			}
			assertRejectedCapturePages(t, host, "direct-example-session", environment)
			if restored := assertRejectedCaptureInspection(t, host, facts, "archived", environment, "PRIVATE ordinary stdout 世界 <redacted>"); !reflect.DeepEqual(transcript, restored) {
				t.Fatalf("restart changed direct transcript: before=%+v after=%+v", transcript, restored)
			}
			var original, restored any
			if err := json.Unmarshal([]byte(before), &original); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal([]byte(read()), &restored); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(original, restored) {
				t.Fatalf("restart changed captured output: before=%v after=%v", original, restored)
			}
			if denied.calls.Load() != 0 {
				t.Fatalf("native calls = %d", denied.calls.Load())
			}
		})
	}
}

func writeRejectedDirectExecution(t *testing.T, dir, provider string) string {
	t.Helper()
	document := publishedDirectExecutionDocument(t)
	execution := document["execution"].(map[string]any)
	execution["workingDirectory"] = dir
	execution["envVars"] = map[string]string{"API_KEY": "declared-credential"}
	execution["runnerId"], execution["executorProvider"], execution["modelProvider"] = provider, provider, provider
	if provider == "claude" {
		execution["model"] = "claude-test"
	}
	data, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "execution.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// Factory origin owns an explicit session and named Work. Its private immutable
// rejection policy is reused by the fresh graph after the first host joins.
func TestRejectedMockCapturedOutputFactory(t *testing.T) {
	t.Parallel()
	dir := publishedDirectMockFactory(t)
	gate := support.NewMockWorkerGate(t)
	config := map[string]any{"mockWorkers": []any{map[string]any{
		"runType": "reject", "gateConfig": gate.Config(30 * time.Second),
		"usage":        map[string]any{"provider": "codex", "model": "gpt-5", "inputTokens": 10, "outputTokens": 4, "totalTokens": 14},
		"rejectConfig": map[string]any{"stdout": "FACTORY rejected stdout 世界", "stderr": "FACTORY rejected stderr 世界", "exitCode": 42},
	}}}
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "mock-workers.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	denied := &directExampleDeniedRunner{}
	cfg := support.FunctionalAPIServerConfig{
		FactoryDir: dir, WaitForServiceModeRuntime: true,
		Env:   builtcliacceptance.ProcessEnvForIsolatedHome(publishedDirectMockHome(t)),
		Args:  []string{"--with-mock-workers", path},
		Edges: serviceedges.Edges{ProviderCommandRunner: denied, ScriptCommandRunner: denied},
	}
	host := support.StartFunctionalAPIServer(t, cfg)
	scope := support.OpenFactorySessionAt(t, host.URL(), dir).Session.Id
	name := "declared rejected Factory Work"
	submitted := support.SubmitSessionWorkAt(t, host.URL(), scope, factoryapi.SubmitWorkRequest{WorkTypeName: "task", Name: &name, Payload: "reject with declared output"})
	gate.WaitForArrival(t, 30*time.Second)
	endpoint := host.URL() + "/factory-sessions/" + scope + "/worker-sessions?workId=" + url.QueryEscape(*submitted.WorkId)
	live := support.GetJSON[factoryapi.ListWorkerSessionsResponse](t, endpoint)
	if len(live.Sessions) != 1 || live.Sessions[0].EndedAt != nil {
		t.Fatalf("gated Factory membership: %+v", live)
	}
	id := live.Sessions[0].WorkerSessionId
	gate.Release()
	status := support.WaitForSessionTerminalStatus(t, host.URL(), scope, 30*time.Second)
	if status.Categories.Failed != 1 || status.Categories.Terminal != 0 {
		t.Fatalf("rejected business outcome: %+v", status)
	}
	facts := assertRejectedCaptureFacts(t, host, id)
	if facts.Direct || facts.FactorySessionId == nil || *facts.FactorySessionId != scope || facts.WorkName == nil || *facts.WorkName != name || !reflect.DeepEqual(facts.WorkIds, live.Sessions[0].WorkIds) || facts.AttemptId != live.Sessions[0].AttemptId {
		t.Fatalf("rejected attribution: live=%+v ended=%+v", live.Sessions[0], facts)
	}
	logs := assertRejectedCapturePages(t, host, id, cfg.Env)
	transcript := assertRejectedCaptureInspection(t, host, facts, "all", cfg.Env, "FACTORY rejected stdout 世界")
	encoded, err := json.Marshal(logs.Events)
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{"FACTORY rejected stdout 世界", "FACTORY rejected stderr 世界"} {
		if strings.Count(string(encoded), marker) != 1 {
			t.Fatalf("declared Factory output %q: %s", marker, encoded)
		}
	}
	if strings.Index(string(encoded), "rejected stdout") > strings.Index(string(encoded), "rejected stderr") {
		t.Fatalf("Factory stream order: %s", encoded)
	}
	host.Close(t)
	host = support.StartFunctionalAPIServer(t, cfg)
	if restored := assertRejectedCaptureFacts(t, host, id); !equalRejectedCaptureFacts(facts, restored) {
		t.Fatalf("restart changed Factory facts: before=%+v after=%+v", facts, restored)
	}
	if restored := assertRejectedCapturePages(t, host, id, cfg.Env); !reflect.DeepEqual(logs, restored) {
		t.Fatalf("restart changed Factory logs: before=%+v after=%+v", logs, restored)
	}
	if restored := assertRejectedCaptureInspection(t, host, facts, "archived", cfg.Env, "FACTORY rejected stdout 世界"); !reflect.DeepEqual(transcript, restored) {
		t.Fatalf("restart changed Factory transcript: before=%+v after=%+v", transcript, restored)
	}
	if denied.calls.Load() != 0 {
		t.Fatalf("native calls = %d", denied.calls.Load())
	}
}

// LIST parity includes archived Factory selection as well as unscoped history.
// Transcripts project message content; stderr stays a labelled progress record
// in logs and must not become a fabricated assistant response.
func assertRejectedCaptureInspection(t *testing.T, host *support.FunctionalAPIServer, facts factoryapi.WorkerSessionObservation, history string, environment []string, stdout string) factoryapi.WorkerSessionTranscriptResponse {
	t.Helper()
	connection, closeMCP := rejectedCaptureMCP(t, host, environment)
	defer closeMCP()
	scope := "direct"
	if !facts.Direct {
		scope = "factory"
	}
	for _, scoped := range []bool{false, true} {
		query := url.Values{"history": {history}, "scope": {scope}}
		arguments := map[string]any{"action": "LIST", "history": history, "scope": scope}
		args := []string{"worker-sessions", "list", "--history", history, "--scope", scope}
		if scoped && facts.FactorySessionId != nil {
			query.Set("factorySessionId", *facts.FactorySessionId)
			// MCP LIST has no Factory Session selector; compare its supported scope on this private host.
			args = append(args, "--session", *facts.FactorySessionId)
		}
		page := support.GetJSON[factoryapi.ListWorkerSessionsResponse](t, host.URL()+"/worker-sessions?"+query.Encode())
		var cli factoryapi.ListWorkerSessionsResponse
		rejectedInspectionCLI(t, host, environment, args, &cli)
		var remote factoryapi.ListWorkerSessionsResponse
		rejectedInspectionMCP(t, connection, arguments, &remote)
		if !reflect.DeepEqual(cli, page) || !reflect.DeepEqual(remote, page) || len(page.Sessions) != 1 || !equalRejectedCaptureFacts(facts, page.Sessions[0]) {
			t.Fatalf("rejected LIST parity: HTTP=%+v CLI=%+v MCP=%+v original=%+v", page, cli, remote, facts)
		}
	}
	id := facts.WorkerSessionId
	transcript := support.GetJSON[factoryapi.WorkerSessionTranscriptResponse](t, host.URL()+"/worker-sessions/"+url.PathEscape(id)+"/transcript")
	var cli, remote factoryapi.WorkerSessionTranscriptResponse
	rejectedInspectionCLI(t, host, environment, []string{"worker-sessions", "read", "--worker-session-id", id, "--view", "transcript"}, &cli)
	rejectedInspectionMCP(t, connection, map[string]any{"action": "READ", "workerSessionId": id, "view": "transcript"}, &remote)
	if !reflect.DeepEqual(transcript, cli) || !reflect.DeepEqual(transcript, remote) || transcript.State != "FAILED" || transcript.WorkerSessionId != id || transcript.AttemptId != facts.AttemptId || !reflect.DeepEqual(transcript.WorkIds, facts.WorkIds) || len(transcript.Entries) != 1 {
		t.Fatalf("rejected transcript parity/attribution: HTTP=%+v CLI=%+v MCP=%+v", transcript, cli, remote)
	}
	entry := transcript.Entries[0]
	if entry.Text == nil || *entry.Text != stdout || entry.Type != "assistant_message" || entry.Timestamp == nil {
		t.Fatalf("rejected message projection: %+v", entry)
	}
	return transcript
}

func rejectedInspectionCLI(t *testing.T, host *support.FunctionalAPIServer, environment, args []string, result any) {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), append([]string{"you", "--server", host.URL(), "--json"}, args...))
	inputs.Input.Env = environment
	if err := host.Execute(t, inputs.Input); err != nil {
		t.Fatalf("CLI inspection: %v %s", err, inputs.Stderr())
	}
	if err := json.Unmarshal([]byte(inputs.Stdout()), result); err != nil {
		t.Fatal(err)
	}
}

func rejectedInspectionMCP(t *testing.T, connection *mcp.ClientSession, args map[string]any, result any) {
	t.Helper()
	response, err := connection.CallTool(t.Context(), &mcp.CallToolParams{Name: "you.subagent", Arguments: args})
	if err != nil || response.IsError || len(response.Content) != 1 {
		t.Fatalf("MCP inspection: %v %+v", err, response)
	}
	var envelope struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal([]byte(response.Content[0].(*mcp.TextContent).Text), &envelope); err != nil {
		t.Fatal(err)
	}
	if args["action"] == "READ" {
		var read struct {
			Transcript json.RawMessage `json:"transcript"`
		}
		if err := json.Unmarshal(envelope.Result, &read); err != nil {
			t.Fatal(err)
		}
		envelope.Result = read.Transcript
	}
	if err := json.Unmarshal(envelope.Result, result); err != nil {
		t.Fatal(err)
	}
}

func assertRejectedCaptureFacts(t *testing.T, host *support.FunctionalAPIServer, id string) factoryapi.WorkerSessionObservation {
	t.Helper()
	row := support.GetJSON[factoryapi.WorkerSessionObservation](t, host.URL()+"/worker-sessions/"+url.PathEscape(id))
	if row.State != "FAILED" || row.TerminalCause == nil || *row.TerminalCause != "FAILED" || row.RecordingHealth == nil || *row.RecordingHealth != "COMPLETE" || row.StartedAt == nil || row.EndedAt == nil || row.DurationMillis == nil || row.EndedAt.Before(*row.StartedAt) {
		t.Fatalf("rejected capture facts: %+v", row)
	}
	u := row.TokenUsage
	if u == nil || u.InputTokens == nil || *u.InputTokens != 10 || u.OutputTokens == nil || *u.OutputTokens != 4 || u.TotalTokens == nil || *u.TotalTokens != 14 || u.Origin == nil || *u.Origin != "SYNTHETIC" {
		t.Fatalf("rejected synthetic usage: %+v", row)
	}
	return row
}

// Cancel owns a private gate before the mock produces either configured stream.
// Joining and reopening verifies that cancellation cannot turn into rejection
// output during teardown or archived reconstruction.
func TestRejectedMockCapturedOutputCanceledBeforeRelease(t *testing.T) {
	t.Parallel()
	dir := publishedDirectMockFactory(t)
	gate := support.NewMockWorkerGate(t)
	config := map[string]any{"mockWorkers": []any{map[string]any{
		"runType": "reject", "gateConfig": gate.Config(30 * time.Second),
		"rejectConfig": map[string]any{"stdout": "NEVER rejected stdout", "stderr": "NEVER rejected stderr", "exitCode": 42},
	}}}
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "mock-workers.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	denied := &directExampleDeniedRunner{}
	cfg := support.FunctionalAPIServerConfig{
		FactoryDir: dir, WaitForServiceModeRuntime: true,
		Env:   builtcliacceptance.ProcessEnvForIsolatedHome(publishedDirectMockHome(t)),
		Args:  []string{"--with-mock-workers", path},
		Edges: serviceedges.Edges{ProviderCommandRunner: denied, ScriptCommandRunner: denied},
	}
	host := support.StartFunctionalAPIServer(t, cfg)
	executionPath := writeRejectedDirectExecution(t, dir, "codex")
	var admitted any
	rejectedInspectionCLI(t, host, cfg.Env, []string{"--remote", "worker-sessions", "invoke", "--execution", executionPath, "--async"}, &admitted)
	gate.WaitForArrival(t, 30*time.Second)
	id := "direct-example-session"
	endpoint := host.URL() + "/worker-sessions/" + id
	live := support.GetJSON[factoryapi.WorkerSessionObservation](t, endpoint)
	if live.State != "RUNNING" {
		t.Fatalf("gated state: %+v", live)
	}
	var canceled any
	rejectedInspectionCLI(t, host, cfg.Env, []string{"worker-sessions", "cancel", id}, &canceled)
	// The public committed capture is the terminal barrier, independent of the
	// cancel response. Waiting for that exact head cannot fabricate mock output.
	logs, err := support.WaitForObservation(30*time.Second, func() (factoryapi.WorkerSessionLogPage, error) {
		return support.GetJSON[factoryapi.WorkerSessionLogPage](t, endpoint+"/logs"), nil
	}, func(page factoryapi.WorkerSessionLogPage) bool { return page.Health == "COMPLETE" })
	if err != nil {
		t.Fatal(err)
	}
	facts := support.GetJSON[factoryapi.WorkerSessionObservation](t, endpoint)
	if facts.State != "CANCELED" || facts.TerminalCause == nil || *facts.TerminalCause != "OPERATOR_CANCEL" || facts.AttemptId != live.AttemptId {
		t.Fatalf("canceled facts: %+v", facts)
	}
	encoded, err := json.Marshal(logs)
	if err != nil || strings.Contains(string(encoded), "NEVER rejected") {
		t.Fatalf("cancellation fabricated declared output: %s %v", encoded, err)
	}
	host.Close(t)
	gate.Release()
	host = support.StartFunctionalAPIServer(t, cfg)
	restored := support.GetJSON[factoryapi.WorkerSessionLogPage](t, host.URL()+"/worker-sessions/"+id+"/logs")
	if !reflect.DeepEqual(logs, restored) {
		t.Fatalf("canceled replay changed logs: before=%+v after=%+v", logs, restored)
	}
	if denied.calls.Load() != 0 {
		t.Fatalf("native calls = %d", denied.calls.Load())
	}
}

func equalRejectedCaptureFacts(before, after factoryapi.WorkerSessionObservation) bool {
	// Restart confirms ended capture storage; that acknowledgement is not an
	// execution fact. Live failure diagnostics are reconstructed after restart.
	// Every captured identity, time, usage and terminal outcome stays equal.
	before.ConfirmationState = after.ConfirmationState
	before.Failure = after.Failure
	return reflect.DeepEqual(before, after)
}

func assertRejectedCapturePages(t *testing.T, host *support.FunctionalAPIServer, id string, environment []string) factoryapi.WorkerSessionLogPage {
	t.Helper()
	connection, closeMCP := rejectedCaptureMCP(t, host, environment)
	defer closeMCP()
	endpoint := host.URL() + "/worker-sessions/" + url.PathEscape(id) + "/logs"
	head := support.GetJSON[factoryapi.WorkerSessionLogPage](t, endpoint)
	for index, frame := range head.Events {
		if frame.Event.CapturedAt == nil || frame.WorkerSessionId != id || (index > 0 && frame.Event.Position <= head.Events[index-1].Event.Position) {
			t.Fatalf("captured event identity/time/order: %+v", head.Events)
		}
	}
	var events []factoryapi.WorkerSessionEvent
	token := ""
	for {
		query := "?limit=1"
		args := []string{"you", "--server", host.URL(), "--json", "worker-sessions", "read", "--worker-session-id", id, "--view", "logs", "--limit", "1"}
		if token != "" {
			query += "&nextToken=" + url.QueryEscape(token)
			args = append(args, "--next-token", token)
		}
		page := support.GetJSON[factoryapi.WorkerSessionLogPage](t, endpoint+query)
		arguments := map[string]any{"action": "READ", "workerSessionId": id, "view": "logs", "limit": 1}
		if token != "" {
			arguments["nextToken"] = token
		}
		result, err := connection.CallTool(t.Context(), &mcp.CallToolParams{Name: "you.subagent", Arguments: arguments})
		if err != nil || result.IsError || len(result.Content) != 1 {
			t.Fatalf("MCP rejected logs: %v %+v", err, result)
		}
		var envelope struct {
			Result struct {
				Logs factoryapi.WorkerSessionLogPage `json:"logs"`
			} `json:"result"`
		}
		if err := json.Unmarshal([]byte(result.Content[0].(*mcp.TextContent).Text), &envelope); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(envelope.Result.Logs, page) {
			t.Fatalf("MCP log parity: MCP=%+v HTTP=%+v", envelope.Result.Logs, page)
		}
		inputs := support.FakeInputs(t.Context(), args)
		inputs.Input.Env = environment
		if err := host.Execute(t, inputs.Input); err != nil {
			t.Fatalf("CLI bounded logs: %v %s", err, inputs.Stderr())
		}
		var cli factoryapi.WorkerSessionLogPage
		if err := json.Unmarshal([]byte(inputs.Stdout()), &cli); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(cli, page) || len(page.Events) > 1 || page.Health != "COMPLETE" || page.RecordingGenerationId != head.RecordingGenerationId || page.CommittedPosition != head.CommittedPosition {
			t.Fatalf("bounded log parity: HTTP=%+v CLI=%+v head=%+v", page, cli, head)
		}
		events = append(events, page.Events...)
		if len(events) > len(head.Events) {
			t.Fatal("cursor duplicated events")
		}
		if page.NextToken == nil {
			break
		}
		if *page.NextToken == token {
			t.Fatal("cursor did not advance")
		}
		token = *page.NextToken
	}
	if !reflect.DeepEqual(events, head.Events) {
		t.Fatalf("pages changed ordered events: pages=%+v head=%+v", events, head.Events)
	}
	return head
}

func rejectedCaptureMCP(t *testing.T, host *support.FunctionalAPIServer, environment []string) (*mcp.ClientSession, func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	stdin, input := io.Pipe()
	output, stdout := io.Pipe()
	done := make(chan error, 1)
	workDir := t.TempDir()
	go func() {
		err := host.Execute(t, root.Input{Context: ctx, Args: []string{"you", "--server", host.URL(), "server", "mcp"}, WorkingDirectory: workDir, Env: environment, Stdin: stdin, Stdout: stdout, Stderr: io.Discard})
		_ = stdin.CloseWithError(err)
		_ = stdout.CloseWithError(err)
		done <- err
	}()
	client := mcp.NewClient(&mcp.Implementation{Name: "rejected-capture-parity", Version: "test"}, nil)
	connection, err := client.Connect(ctx, &mcp.IOTransport{Reader: output, Writer: input}, nil)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	return connection, func() {
		_ = connection.Close()
		cancel()
		_ = input.Close()
		_ = stdin.Close()
		_ = stdout.Close()
		_ = output.Close()
		select {
		case err := <-done:
			if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrClosedPipe) {
				t.Errorf("MCP join: %v", err)
			}
		case <-time.After(30 * time.Second):
			t.Error("MCP did not join")
		}
	}
}

// Direct origin needs no submitted Work. This private host has its own idle
// Factory Session and profile, and its immutable accept policy differs from
// the named-mock fixture. All commands reuse its one root-built process.
func TestPublishedDirectWorkerSessionMockJourney(t *testing.T) {
	t.Parallel()
	dir := publishedDirectMockFactory(t)
	mockPath := filepath.Join(dir, "mock-workers.json")
	if err := os.WriteFile(mockPath, []byte(`{"unmatchedDispatchPolicy":"accept","mockWorkers":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	denied := &directExampleDeniedRunner{}
	var nativeCalls atomic.Int32
	defer func() {
		t.Logf("effect-denying boundaries: provider/script calls=%d, subprocess/ACP calls=%d", denied.calls.Load(), nativeCalls.Load())
	}()
	environment := builtcliacceptance.ProcessEnvForIsolatedHome(publishedDirectMockHome(t))
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir, WaitForServiceModeRuntime: true,
		Env:                   environment,
		Args:                  []string{"--with-mock-workers", mockPath},
		DeniedProcessAttempts: &nativeCalls,
		Edges: serviceedges.Edges{
			ProviderCommandRunner: denied, ScriptCommandRunner: denied,
			ProvidersStdioPipeFactory: func() (platformprocess.StdioChannel, error) {
				nativeCalls.Add(1)
				return nil, errors.New("ACP pipe creation denied for published mock example")
			},
		},
	})
	support.WaitForRuntimeIdle(t, server.URL(), 10*time.Second)
	document := publishedDirectExecutionDocument(t)
	document["execution"].(map[string]any)["workingDirectory"] = dir
	path := filepath.Join(dir, "execution.json")
	data, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	publishedDirectMockCommands(t, server, dir, path, environment)
	if denied.calls.Load() != 0 || nativeCalls.Load() != 0 {
		t.Fatalf("mock example attempted native effects: commands=%d subprocess/ACP=%d", denied.calls.Load(), nativeCalls.Load())
	}
}

func publishedDirectMockCommands(t *testing.T, server *support.FunctionalAPIServer, dir, path string, environment []string) {
	t.Helper()
	execute := func(args ...string) string {
		inputs := support.FakeInputs(t.Context(), append([]string{"you", "--server", server.URL(), "--json"}, args...))
		inputs.Input.WorkingDirectory, inputs.Input.Env = dir, environment
		if err := server.Execute(t, inputs.Input); err != nil || inputs.Stderr() != "" {
			t.Fatalf("%q: %v stderr=%s", args, err, inputs.Stderr())
		}
		return inputs.Stdout()
	}
	execute("factory", "config", "validate", filepath.Join(dir, "factory.json"))
	execute("worker-sessions", "--help")
	body := execute("--remote", "worker-sessions", "invoke", "--execution", path, "--user-message", "Reply with DIRECT_EXAMPLE_OK.", "--async")
	var admitted struct {
		Accepted bool   `json:"accepted"`
		ID       string `json:"workerSessionId"`
	}
	if err := json.Unmarshal([]byte(body), &admitted); err != nil || !admitted.Accepted || admitted.ID != "direct-example-session" {
		t.Fatalf("mock canonical admission: %v %s", err, body)
	}
	execute("worker-sessions", "show", "--worker-session-id", admitted.ID)
	// Capture is asynchronous. Only this private session's public committed
	// logs can establish synthetic terminal output; provider gates cannot.
	logs, err := support.WaitForObservation(30*time.Second, func() (string, error) {
		return execute("worker-sessions", "read", "--worker-session-id", admitted.ID, "--view", "logs"), nil
	}, func(body string) bool { return strings.Contains(body, `"health":"COMPLETE"`) })
	if err != nil || !strings.Contains(logs, "mock worker accepted") || !strings.Contains(logs, admitted.ID) {
		t.Fatalf("captured synthetic result: %v %s", err, logs)
	}
	if body := execute("worker-sessions", "show", "--worker-session-id", admitted.ID); !strings.Contains(body, `"state":"COMPLETED"`) || !strings.Contains(body, admitted.ID) {
		t.Fatalf("mock terminal show: %s", body)
	}
	execute("server", "stop")
	server.Close(t) // joins the private host invocation and all owned resources
}

func publishedDirectMockFactory(t *testing.T) string {
	t.Helper()
	dir := support.ScaffoldFactory(t, map[string]any{
		"name":         "direct-demo",
		"workTypes":    []map[string]any{{"name": "task", "states": []map[string]string{{"name": "init", "type": "INITIAL"}, {"name": "complete", "type": "TERMINAL"}, {"name": "failed", "type": "FAILED"}}}},
		"workers":      []map[string]string{{"name": "processor"}},
		"workstations": []map[string]any{{"name": "process-task", "worker": "processor", "inputs": []map[string]string{{"workType": "task", "state": "init"}}, "outputs": []map[string]string{{"workType": "task", "state": "complete"}}, "onFailure": []map[string]string{{"workType": "task", "state": "failed"}}}},
	})
	support.WriteAgentConfig(t, dir, "processor", "---\ntype: AGENT_WORKER\nmodelProvider: CODEX\nmodel: gpt-5\nexecutorProvider: SCRIPT_WRAP\n---\nReturn the requested short reply.\n")
	path := filepath.Join(dir, "workstations", "process-task", "AGENTS.md")
	if err := os.WriteFile(path, []byte("---\ntype: AGENT_RUN\n---\nReturn the requested short reply.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

// Keep this docs fixture in the test compilation rather than the production graph.
func publishedDirectExecutionDocument(t testing.TB) map[string]any {
	t.Helper()
	data, err := os.ReadFile(testutil.MustRepoPath(t, "docs/reference/operations.md"))
	if err != nil {
		t.Fatal(err)
	}
	_, section, ok := strings.Cut(strings.ReplaceAll(string(data), "\r\n", "\n"), "Save this complete request as `execution.json`:")
	if !ok {
		t.Fatal("published execution request is missing")
	}
	_, block, ok := strings.Cut(section, "```json\n")
	if !ok {
		t.Fatal("published execution JSON is missing")
	}
	block, _, ok = strings.Cut(block, "\n```")
	if !ok {
		t.Fatal("published execution JSON is incomplete")
	}
	var document map[string]any
	if err := json.Unmarshal([]byte(block), &document); err != nil {
		t.Fatalf("published execution JSON: %v", err)
	}
	return document
}

func publishedDirectMockHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	configDir := filepath.Join(home, ".you-agent-factory")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte(`{"defaults":{"workerModelProvider":"codex","workerModel":"gpt-5"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	return home
}
