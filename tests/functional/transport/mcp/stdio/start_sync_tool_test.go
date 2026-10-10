package stdio_test

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionmcp "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/mcp"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// M01-M06: each connection selects its own working root and owns its streams
// and profile. The immutable command edge attributes results by working root,
// so simultaneous invocations need no mutable provider selector or long lock.
func TestMCPStartSyncRunsFactorySessionThroughComposedProcess(t *testing.T) {
	t.Parallel()
	runner := &mcpRootResultRunner{}
	process, err := root.BuildProcess(t.Context(), serviceedges.Edges{ProviderCommandRunner: runner})
	if err != nil {
		t.Fatalf("BuildProcess: %v", err)
	}
	support.CleanupProcess(t, process)
	sessions, ok := process.FactorySessions().FactorySessions().(factorysessions.Service)
	if !ok {
		t.Fatal("composed process did not expose Factory Sessions")
	}
	t.Run("completion and ordered events", func(t *testing.T) {
		t.Parallel()
		server := startComposedMemoryMCP(t, process)
		initializeMCPClient(t, server.client)
		sessionID := assertComposedMCPSync(t, server, "first")
		assertComposedMCPEvents(t, server.client, sessionID)
		server.closeInput(t)
	})
	for _, name := range []string{"reconnect suffix", "reconnect tail", "unknown session", "absent cursor"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assertComposedReconnect(t, process, name)
		})
	}
	t.Run("closing one selection leaves its peer usable", func(t *testing.T) {
		t.Parallel()
		assertComposedPeerLifetime(t, process, runner, sessions)
	})
	t.Run("configured subagent provider selection", func(t *testing.T) {
		t.Parallel()
		assertMCPConfiguredProviderSelection(t, process, runner)
	})
}

type mcpRootResultRunner struct {
	gates    sync.Map
	requests sync.Map
}

type mcpCommandGate struct {
	entered chan struct{}
	release chan struct{}
}

func (runner *mcpRootResultRunner) gate(t *testing.T, root string) *mcpCommandGate {
	t.Helper()
	gate := &mcpCommandGate{entered: make(chan struct{}), release: make(chan struct{})}
	runner.gates.Store(filepath.Clean(root), gate)
	t.Cleanup(func() { runner.gates.Delete(filepath.Clean(root)) })
	return gate
}

func (runner *mcpRootResultRunner) Run(ctx context.Context, req platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	runner.requests.Store(filepath.Clean(req.WorkDir), req)
	if value, ok := runner.gates.LoadAndDelete(filepath.Clean(req.WorkDir)); ok {
		gate := value.(*mcpCommandGate)
		close(gate.entered)
		select {
		case <-gate.release:
		case <-ctx.Done():
			return platformprocess.CommandResult{}, ctx.Err()
		}
	}
	return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout("completed at " + filepath.Clean(req.WorkDir))}, nil
}

type composedMemoryMCP struct {
	client *stdioMCPClient
	root   string
	home   string
	input  *io.PipeWriter
	done   <-chan error
}

func startComposedMemoryMCP(t *testing.T, process support.Process) *composedMemoryMCP {
	return startComposedMemoryMCPWithLifetime(t, t, process)
}

// A non-canceling sync timeout can outlive its request. Its directories and
// connection context belong to the cohort that joins the reusable process.
func startComposedMemoryMCPWithLifetime(t, lifetime *testing.T, process support.Process) *composedMemoryMCP {
	t.Helper()
	projectRoot := support.ScaffoldSingleStepFactory(lifetime, "composed-mcp")
	stdin, input := io.Pipe()
	output, stdout := io.Pipe()
	ctx, cancel := context.WithTimeout(lifetime.Context(), time.Minute)
	done := make(chan error, 1)
	t.Cleanup(func() {
		cancel()
		_ = input.Close()
		_ = stdin.Close()
		_ = stdout.Close()
		_ = output.Close()
	})
	home := lifetime.TempDir()
	go func() {
		err := process.Execute(root.Input{
			Args:             []string{"you", "server", "mcp", "--project-root", projectRoot},
			Env:              builtcliacceptance.ProcessEnvForIsolatedHome(home),
			WorkingDirectory: projectRoot, Context: ctx,
			Stdin: stdin, Stdout: stdout, Stderr: io.Discard,
		})
		_ = stdout.Close()
		done <- err
	}()
	return &composedMemoryMCP{client: newStdioMCPClient(t, input, output), root: projectRoot, home: home, input: input, done: done}
}

// The connection owns its profile and working root. Rejected selections must
// not enter the provider command edge; a subsequent valid request remains usable.
func assertMCPConfiguredProviderSelection(t *testing.T, process support.Process, runner *mcpRootResultRunner) {
	t.Helper()
	server := startComposedMemoryMCP(t, process)
	initializeMCPClient(t, server.client)
	env := builtcliacceptance.ProcessEnvForIsolatedHome(server.home)
	source := support.InstallPackagedFactoryWithProcess(t, process, env, server.root, "@you/subagent")
	support.CreateNamedFactoryAtRootWithProcess(t, process, env, server.root,
		filepath.Join(server.root, "factory"), "@you/subagent", filepath.Join(source, "factory.json"))
	configPath := filepath.Join(server.home, ".you-agent-factory", "config.json")
	before, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { runner.requests.Delete(filepath.Clean(server.root)) })
	call := func(provider string) mcpJSONRPCResponse {
		return server.client.call("tools/call", map[string]any{
			"name": "you.subagent", "arguments": map[string]any{
				"prompt": "Complete the selected request", "provider": provider, "model": "gpt-5-codex",
			},
		})
	}
	unknown := decodeComposedEnvelope[factorysessionmcp.SubagentResult](t, call("unknown-wfc-provider"))
	if unknown.Error == nil || unknown.Result != nil || unknown.Error.Code != "factory_session.subagent.provider_not_found" || unknown.Error.Retryable {
		t.Fatalf("unknown provider result = %#v", unknown)
	}
	if err := os.WriteFile(configPath, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	broken := call("codex")
	if broken.Error == nil && broken.Result["isError"] != true {
		t.Fatalf("malformed configuration returned success: %#v", broken)
	}
	if _, invoked := runner.requests.Load(filepath.Clean(server.root)); invoked {
		t.Fatal("rejected provider or malformed profile admitted a provider attempt")
	}
	if err := os.WriteFile(configPath, before, 0o600); err != nil {
		t.Fatal(err)
	}
	assertMCPHealthyProviderRecovery(t, call, runner, server.root)
	server.closeInput(t)
}

func assertMCPHealthyProviderRecovery(t *testing.T, call func(string) mcpJSONRPCResponse, runner *mcpRootResultRunner, workingRoot string) {
	t.Helper()
	result := decodeComposedTool[factorysessionmcp.SubagentResult](t, call("codex"))
	if result.SessionID == "" || result.Status != "COMPLETED" || !strings.Contains(result.Text, "completed at "+filepath.Clean(workingRoot)) {
		t.Fatalf("configured provider result = %#v", result)
	}
	request, invoked := runner.requests.LoadAndDelete(filepath.Clean(workingRoot))
	if !invoked || request.(platformprocess.CommandRequest).Command != "codex" {
		t.Fatalf("selected provider request = %#v", request)
	}
}

func (server *composedMemoryMCP) closeInput(t *testing.T) {
	t.Helper()
	if err := server.input.Close(); err != nil {
		t.Fatalf("close caller input: %v", err)
	}
	// Completion is observed directly; this deadline only bounds a stuck test.
	select {
	case err := <-server.done:
		if err != nil {
			t.Fatalf("Process.Execute after input close: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("Process.Execute did not finish after caller input closed")
	}
}

func assertComposedMCPSync(t *testing.T, server *composedMemoryMCP, requestID string) string {
	t.Helper()
	workName, _ := json.Marshal("work-" + requestID)
	workflow := `return (async function () { return await agent.run({label: ` + string(workName) + `, prompt: ` + string(workName) + `, modelProvider: "codex", model: "gpt-5-codex"}); })();`
	response := server.client.call("tools/call", map[string]any{
		"name": factorysessionmcp.ToolStartSync,
		"arguments": map[string]any{
			"requestId": requestID + "-" + uuid.NewString(),
			"source":    map[string]any{"kind": "INLINE_WORKFLOW", "inlineWorkflow": map[string]any{"inlineSource": map[string]any{"encoding": "utf-8", "inline": workflow}}},
		},
	})
	result := decodeComposedTool[factoryapi.FactorySessionSyncExecutionResponse](t, response)
	if result.SyncOutcome != factoryapi.FactorySessionSyncExecutionOutcomeCompleted || result.SessionId == "" {
		t.Fatalf("start_sync = %#v, want COMPLETED and session identity", result)
	}
	encoded, err := json.Marshal(result.Result)
	if err != nil || !strings.Contains(string(encoded), "completed at ") || !strings.Contains(string(encoded), strings.ReplaceAll(server.root, `\`, `\\`)) {
		t.Fatalf("start_sync result = %s (%v), want provider output from working root %q", encoded, err, server.root)
	}
	return result.SessionId
}

func assertComposedMCPEvents(t *testing.T, client *stdioMCPClient, sessionID string) factorysessionmcp.ReadEventsResult {
	t.Helper()
	result := decodeComposedTool[factorysessionmcp.ReadEventsResult](t, client.call("tools/call", map[string]any{
		"name": factorysessionmcp.ToolReadEvents, "arguments": map[string]any{"sessionId": sessionID},
	}))
	if result.SessionID != sessionID || len(result.Events) == 0 {
		t.Fatalf("read_events = %#v, want nonempty events for %q", result, sessionID)
	}
	for index, event := range result.Events {
		if event.Context.SessionId == nil || *event.Context.SessionId != sessionID {
			t.Fatalf("event %d belongs to %#v, want %q", index, event.Context.SessionId, sessionID)
		}
		if index > 0 && event.Context.Sequence <= result.Events[index-1].Context.Sequence {
			t.Fatalf("events are not ordered at %d: %#v", index, result.Events)
		}
	}
	return result
}

func decodeComposedTool[T any](t *testing.T, response mcpJSONRPCResponse) T {
	t.Helper()
	envelope := decodeComposedEnvelope[T](t, response)
	if envelope.Error != nil || envelope.Result == nil {
		t.Fatalf("MCP tool outcome = %#v, want success", envelope)
	}
	return *envelope.Result
}

func decodeComposedEnvelope[T any](t *testing.T, response mcpJSONRPCResponse) factorysessionmcp.ToolResponse[T] {
	t.Helper()
	if response.Error != nil {
		t.Fatalf("MCP protocol error: %#v", response.Error)
	}
	var envelope factorysessionmcp.ToolResponse[T]
	encoded, err := json.Marshal(response.Result["structuredContent"])
	if err != nil {
		t.Fatalf("encode structured tool result: %v", err)
	}
	if string(encoded) == "null" {
		encoded = []byte(mcpToolResultText(t, response.Result))
	}
	if err := json.Unmarshal(encoded, &envelope); err != nil {
		t.Fatalf("decode tool result: %v; response=%#v", err, response.Result)
	}
	return envelope
}

func composedEventRead(client *stdioMCPClient, input factorysessionmcp.ReadEventsInput) mcpJSONRPCResponse {
	return client.call("tools/call", map[string]any{"name": factorysessionmcp.ToolReadEvents, "arguments": input})
}

func assertComposedReconnect(t *testing.T, process support.Process, name string) {
	t.Helper()
	server := startComposedMemoryMCP(t, process)
	initializeMCPClient(t, server.client)
	sessionID := assertComposedMCPSync(t, server, name)
	all := assertComposedMCPEvents(t, server.client, sessionID)
	if len(all.Events) < 2 {
		t.Fatal("reconnect witness requires at least two events")
	}
	input := factorysessionmcp.ReadEventsInput{SessionID: sessionID, AfterEventID: all.Events[0].Id}
	want := all.Events[1:]
	switch name {
	case "reconnect tail":
		input.AfterEventID = all.Events[len(all.Events)-1].Id
		want = nil
	case "unknown session":
		unknown := "dur-sess-" + strings.ReplaceAll(uuid.NewString(), "-", "")
		assertComposedReadError(t, server.client, factorysessionmcp.ReadEventsInput{SessionID: unknown}, "factory_session.session.not_found", "")
	case "absent cursor":
		assertComposedReadError(t, server.client, factorysessionmcp.ReadEventsInput{SessionID: sessionID, AfterEventID: uuid.NewString()}, "factory_session.events.reconnect_cursor_not_found", "RECONNECT_CURSOR_NOT_FOUND")
		negative := -1
		assertComposedReadError(t, server.client, factorysessionmcp.ReadEventsInput{SessionID: sessionID, AfterSequence: &negative}, "BAD_REQUEST", "")
	}
	got := decodeComposedTool[factorysessionmcp.ReadEventsResult](t, composedEventRead(server.client, input))
	if got.SessionID != sessionID || len(got.Events) != len(want) {
		t.Fatalf("reconnect = %#v, want %d same-session events", got, len(want))
	}
	if len(want) > 0 && !reflect.DeepEqual(got.Events, want) {
		t.Fatalf("reconnect changed canonical suffix: %#v vs %#v", got.Events, want)
	}
	// Failed or empty reads cannot poison the connection or its canonical history.
	if reread := assertComposedMCPEvents(t, server.client, sessionID); !reflect.DeepEqual(reread, all) {
		t.Fatal("subsequent read changed canonical history")
	}
	server.closeInput(t)
}

func assertComposedReadError(t *testing.T, client *stdioMCPClient, input factorysessionmcp.ReadEventsInput, code, reason string) {
	t.Helper()
	got := decodeComposedEnvelope[factorysessionmcp.ReadEventsResult](t, composedEventRead(client, input))
	if got.Result != nil || got.Error == nil || got.Error.Code != code || got.Error.Retryable {
		t.Fatalf("read error = %#v, want %s and retryable=false", got, code)
	}
	if code != "BAD_REQUEST" && got.Error.SessionID != input.SessionID {
		t.Fatalf("error lost requested identity: %#v", got.Error)
	}
	if reason != "" && got.Error.Details["reason"] != reason {
		t.Fatalf("error reason = %#v, want %s", got.Error.Details, reason)
	}
}

func assertComposedPeerLifetime(t *testing.T, process support.Process, runner *mcpRootResultRunner, sessions factorysessions.Service) {
	t.Helper()
	first := startComposedMemoryMCP(t, process)
	second := startComposedMemoryMCP(t, process)
	initializeMCPClient(t, first.client)
	initializeMCPClient(t, second.client)
	firstGate := runner.gate(t, first.root)
	secondGate := runner.gate(t, second.root)
	firstResult, secondResult := make(chan string, 1), make(chan string, 1)
	go func() { firstResult <- assertComposedMCPSync(t, first, "first") }()
	go func() { secondResult <- assertComposedMCPSync(t, second, "second") }()
	// Both commands must enter before either is released: this proves live overlap.
	awaitComposedSignal(t, firstGate.entered)
	awaitComposedSignal(t, secondGate.entered)
	close(firstGate.release)
	firstID := awaitComposedSession(t, firstResult)
	firstEvents := assertComposedMCPEvents(t, first.client, firstID)
	assertComposedLedger(t, sessions, firstEvents, "work-first", "work-second")
	first.closeInput(t)
	select {
	case err := <-second.done:
		t.Fatalf("peer Execute joined while its command was gated: %v", err)
	default:
	}
	close(secondGate.release)
	secondID := awaitComposedSession(t, secondResult)
	if firstID == secondID {
		t.Fatal("isolated requests shared a Factory Session identity")
	}
	secondEvents := assertComposedMCPEvents(t, second.client, secondID)
	assertComposedLedger(t, sessions, secondEvents, "work-second", "work-first")
	nextID := assertComposedMCPSync(t, second, "after-peer-close")
	if nextID == firstID || nextID == secondID {
		t.Fatal("new request reused a Factory Session identity")
	}
	assertComposedLedger(t, sessions, assertComposedMCPEvents(t, second.client, nextID), "work-after-peer-close", "work-first")
	second.closeInput(t)
}

// Literal event IDs may repeat across Sessions. Compare each complete ordered
// result to its own canonical ledger, including Session identity and payload.
func assertComposedLedger(t *testing.T, sessions factorysessions.Service, got factorysessionmcp.ReadEventsResult, ownWork, peerWork string) {
	t.Helper()
	ledger, err := sessions.ReadEvents(t.Context(), got.SessionID, factorysessions.EventReconnectRequest{})
	if err != nil {
		t.Fatalf("canonical ledger read: %v", err)
	}
	if ledger.SessionID != got.SessionID || len(ledger.Events) != len(got.Events) {
		t.Fatal("MCP read differs from its own canonical ledger")
	}
	foundOwn := false
	for i, raw := range ledger.Events {
		var want factoryapi.FactoryEvent
		if err := json.Unmarshal(raw, &want); err != nil {
			t.Fatalf("decode canonical ledger: %v", err)
		}
		if !reflect.DeepEqual(got.Events[i], want) {
			t.Fatalf("MCP event %d differs from its own canonical fact: %#v vs %#v", i, got.Events[i], want)
		}
		encoded, err := json.Marshal(got.Events[i].Payload)
		if err != nil {
			t.Fatalf("encode event payload: %v", err)
		}
		payload := string(encoded)
		if strings.Contains(payload, peerWork) {
			t.Fatalf("peer work leaked in event %s: %s", got.Events[i].Id, payload)
		}
		foundOwn = foundOwn || strings.Contains(payload, ownWork)
	}
	if !foundOwn {
		t.Fatalf("canonical history contains no own work %q", ownWork)
	}
}

func awaitComposedSignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	// Signals synchronize execution; the deadline only bounds a broken fixture.
	select {
	case <-signal:
	case <-t.Context().Done():
		t.Fatal("context ended before provider entered")
	case <-time.After(30 * time.Second):
		t.Fatal("provider did not enter its owned gate")
	}
}

func awaitComposedSession(t *testing.T, result <-chan string) string {
	t.Helper()
	select {
	case id := <-result:
		return id
	case <-time.After(30 * time.Second):
		t.Fatal("released provider did not complete start_sync")
		return ""
	}
}
