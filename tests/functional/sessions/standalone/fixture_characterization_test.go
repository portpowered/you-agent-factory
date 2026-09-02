package standalone_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	mcpfactorysession "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/mcp"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestBTRCP0StandaloneFixtureSuccessCharacterization(t *testing.T) {
	server := startStandaloneFixtureServer(t, canonicalFixtureCatalogPath(t))
	assertStandaloneInitialized(t, server.client)

	start := callStandaloneStartSync(t, server.client, standaloneSuccessRequest("req-petri-success-001", "customer-support-triage", "TKT-2002"))
	assertStandaloneSuccessResult(t, start)

	events := callStandaloneEvents(t, server.client, start.SessionId)
	assertStandaloneSuccessEvents(t, events, start.SessionId)

	result := callStandaloneResult(t, server.client, start.SessionId, true)
	assertStandaloneCompleteResult(t, result, start.SessionId)
}

func TestBTRCP0StandaloneFixtureFailureCharacterization(t *testing.T) {
	server := startStandaloneFixtureServer(t, canonicalFixtureCatalogPath(t))
	assertStandaloneInitialized(t, server.client)

	workflowName := "does-not-exist"
	start := callStandaloneStartSync(t, server.client, factoryapi.FactorySessionExecutionRequest{
		RequestId: "req-missing-source-001",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowName,
			WorkflowName: &workflowName,
		},
	})
	assertStandaloneFailureResult(t, start)
	failureResult := callStandaloneResult(t, server.client, start.SessionId, false)
	if failureResult.ResultStatus != factoryapi.FactorySessionResultStatusUnavailable || failureResult.SessionStatus == nil || *failureResult.SessionStatus != factoryapi.FactorySessionDurableLifecycleStatusFailed || failureResult.PrimaryResult != nil {
		t.Fatalf("standalone failed get_result = %#v, want UNAVAILABLE/FAILED without primary result", failureResult)
	}

	events := callStandaloneEvents(t, server.client, start.SessionId)
	assertStandaloneFailureEvents(t, events, start.SessionId)
	assertStandaloneFailedSessionHasNoLiveDispatches(t, server.client, start.SessionId)
}

func TestBTRCP0StandaloneFixtureRunsRemainIsolated(t *testing.T) {
	catalogA := writeStandaloneFixtureVariant(t, "dur-sess-petri-success-isolated-a", "fixture result A")
	catalogB := writeStandaloneFixtureVariant(t, "dur-sess-petri-success-isolated-b", "fixture result B")
	serverA := startStandaloneFixtureServer(t, catalogA)
	serverB := startStandaloneFixtureServer(t, catalogB)
	assertStandaloneInitialized(t, serverA.client)
	assertStandaloneInitialized(t, serverB.client)

	request := func() factoryapi.FactorySessionExecutionRequest {
		return standaloneSuccessRequest("req-petri-success-001", "customer-support-triage", "TKT-2002")
	}
	startA := callStandaloneStartSync(t, serverA.client, request())
	startB := callStandaloneStartSync(t, serverB.client, request())
	if startA.SessionId == startB.SessionId {
		t.Fatalf("isolated fixture session IDs = %q and %q, want independent identities", startA.SessionId, startB.SessionId)
	}
	assertStandalonePrimaryText(t, startA.Result, "fixture result A", "isolated A start result")
	assertStandalonePrimaryText(t, startB.Result, "fixture result B", "isolated B start result")

	eventsA := callStandaloneEvents(t, serverA.client, startA.SessionId)
	eventsB := callStandaloneEvents(t, serverB.client, startB.SessionId)
	assertStandaloneEventSession(t, eventsA, startA.SessionId, "isolated A")
	assertStandaloneEventSession(t, eventsB, startB.SessionId, "isolated B")
	if eventsA[0].Id == eventsB[0].Id {
		t.Fatalf("isolated event IDs both equal %q, want session-scoped histories", eventsA[0].Id)
	}

	resultA := callStandaloneResult(t, serverA.client, startA.SessionId, false)
	resultB := callStandaloneResult(t, serverB.client, startB.SessionId, false)
	assertStandalonePrimaryText(t, resultA, "fixture result A", "isolated A read result")
	assertStandalonePrimaryText(t, resultB, "fixture result B", "isolated B read result")

	crossRead := decodeStandaloneToolResponse[mcpfactorysession.ReadEventsResult](t, serverA.client.callTool(
		mcpfactorysession.ToolReadEvents,
		map[string]any{"sessionId": startB.SessionId},
	))
	if crossRead.Error == nil || crossRead.Error.Code != "factory_session.session.not_found" {
		t.Fatalf("cross-store read response = %#v, want typed session.not_found", crossRead)
	}
	if crossRead.Error.SessionID != startB.SessionId {
		t.Fatalf("cross-store error sessionId = %q, want %q", crossRead.Error.SessionID, startB.SessionId)
	}
}

func canonicalFixtureCatalogPath(t *testing.T) string {
	t.Helper()
	return testutil.MustRepoPath(t, "pkg/transports/http/testdata/durable-session-contract-fixtures.json")
}

func standaloneSuccessRequest(requestID, factoryID, ticketID string) factoryapi.FactorySessionExecutionRequest {
	args := map[string]interface{}{"ticketId": ticketID}
	return factoryapi.FactorySessionExecutionRequest{
		RequestId: requestID,
		Args:      &args,
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:      factoryapi.FactorySessionExecutionSourceKindFactoryId,
			FactoryId: stringPtr(factoryID),
		},
	}
}

func callStandaloneStartSync(
	t *testing.T,
	client *standaloneMCPClient,
	request factoryapi.FactorySessionExecutionRequest,
) factoryapi.FactorySessionSyncExecutionResponse {
	t.Helper()
	response := decodeStandaloneToolResponse[factoryapi.FactorySessionSyncExecutionResponse](t, client.callTool(
		mcpfactorysession.ToolStartSync,
		request,
	))
	if response.Error != nil || response.Result == nil {
		t.Fatalf("standalone start_sync = %#v, want typed result", response)
	}
	return *response.Result
}

func callStandaloneEvents(t *testing.T, client *standaloneMCPClient, sessionID string) []factoryapi.FactoryEvent {
	t.Helper()
	response := decodeStandaloneToolResponse[mcpfactorysession.ReadEventsResult](t, client.callTool(
		mcpfactorysession.ToolReadEvents,
		map[string]any{"sessionId": sessionID},
	))
	if response.Error != nil || response.Result == nil {
		t.Fatalf("standalone read_events(%q) = %#v, want typed result", sessionID, response)
	}
	return response.Result.Events
}

func callStandaloneResult(t *testing.T, client *standaloneMCPClient, sessionID string, includeArtifacts bool) *factoryapi.FactorySessionResult {
	t.Helper()
	mode := factoryapi.FactorySessionResultModeFinal
	response := decodeStandaloneToolResponse[factoryapi.FactorySessionResult](t, client.callTool(
		mcpfactorysession.ToolGetResult,
		map[string]any{
			"sessionId":        sessionID,
			"mode":             mode,
			"includeArtifacts": includeArtifacts,
		},
	))
	if response.Error != nil || response.Result == nil {
		t.Fatalf("standalone get_result(%q) = %#v, want typed result", sessionID, response)
	}
	return response.Result
}

func assertStandaloneSuccessResult(t *testing.T, result factoryapi.FactorySessionSyncExecutionResponse) {
	t.Helper()
	if result.SessionId != "dur-sess-petri-success-001" || result.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("standalone success identity/status = (%q, %q), want durable fixture success", result.SessionId, result.Status)
	}
	if result.SyncOutcome != factoryapi.FactorySessionSyncExecutionOutcomeCompleted || result.Result == nil {
		t.Fatalf("standalone success sync result = %#v, want COMPLETED with terminal result", result)
	}
	if result.Result.ResultStatus != factoryapi.FactorySessionResultStatusFinal || result.Result.SessionId != result.SessionId {
		t.Fatalf("standalone success terminal result = %#v, want FINAL for %q", result.Result, result.SessionId)
	}
	assertStandalonePrimaryText(t, result.Result, "Ticket triaged and resolved.", "standalone success result")
}

func assertStandaloneCompleteResult(t *testing.T, result *factoryapi.FactorySessionResult, sessionID string) {
	t.Helper()
	if result.SessionId != sessionID || result.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("standalone get_result = %#v, want FINAL for %q", result, sessionID)
	}
	assertStandalonePrimaryText(t, result, "Ticket triaged and resolved.", "standalone complete result")
	if result.ArtifactRefs == nil || len(*result.ArtifactRefs) != 1 || (*result.ArtifactRefs)[0].Id != "art-petri-final-001" {
		t.Fatalf("standalone artifact refs = %#v, want art-petri-final-001", result.ArtifactRefs)
	}
}

func assertStandalonePrimaryText(t *testing.T, result *factoryapi.FactorySessionResult, want, context string) {
	t.Helper()
	if result == nil || result.PrimaryResult == nil || len(*result.PrimaryResult) != 1 {
		t.Fatalf("%s primaryResult = %#v, want one text part", context, result)
	}
	part, err := (*result.PrimaryResult)[0].AsWorkTextContentPart()
	if err != nil || part.Text != want {
		t.Fatalf("%s primaryResult text = (%q, %v), want %q", context, part.Text, err, want)
	}
}

func assertStandaloneSuccessEvents(t *testing.T, events []factoryapi.FactoryEvent, sessionID string) {
	t.Helper()
	wantTypes := []factoryapi.FactoryEventType{
		factoryapi.FactoryEventTypeSessionStarted,
		factoryapi.FactoryEventTypeSessionResultUpdated,
		factoryapi.FactoryEventTypeSessionCompleted,
	}
	wantIDs := []string{
		"session-started/" + sessionID,
		"session-result-updated/" + sessionID,
		"session-completed/" + sessionID,
	}
	assertStandaloneEventSequence(t, events, sessionID, wantTypes, wantIDs)

	updated, err := events[1].Payload.AsSessionResultUpdatedEventPayload()
	if err != nil || updated.ResultStatus != factoryapi.FactoryEventSessionResultStatusFinal {
		t.Fatalf("standalone result-updated payload = (%#v, %v), want FINAL", updated, err)
	}
	completed, err := events[2].Payload.AsSessionCompletedEventPayload()
	if err != nil || completed.FinalStatus != factoryapi.FactorySessionDurableLifecycleStatusSucceeded || completed.ResultStatus == nil || *completed.ResultStatus != factoryapi.FactoryEventSessionResultStatusFinal {
		t.Fatalf("standalone completed payload = (%#v, %v), want SUCCEEDED/FINAL", completed, err)
	}
}

func assertStandaloneFailureResult(t *testing.T, result factoryapi.FactorySessionSyncExecutionResponse) {
	t.Helper()
	if result.SessionId != "dur-sess-missing-source-001" || result.Status != factoryapi.FactorySessionDurableLifecycleStatusFailed {
		t.Fatalf("standalone failure identity/status = (%q, %q), want missing-source FAILED", result.SessionId, result.Status)
	}
	if result.SyncOutcome != factoryapi.FactorySessionSyncExecutionOutcomeCompleted || result.Result == nil {
		t.Fatalf("standalone failure sync result = %#v, want COMPLETED with typed result", result)
	}
	if result.Result.ResultStatus != factoryapi.FactorySessionResultStatusUnavailable || result.Result.SessionStatus == nil || *result.Result.SessionStatus != factoryapi.FactorySessionDurableLifecycleStatusFailed {
		t.Fatalf("standalone failure terminal result = %#v, want UNAVAILABLE/FAILED", result.Result)
	}
	if result.Result.PrimaryResult != nil {
		t.Fatalf("standalone failure primaryResult = %#v, want nil", result.Result.PrimaryResult)
	}
}

func assertStandaloneFailureEvents(t *testing.T, events []factoryapi.FactoryEvent, sessionID string) {
	t.Helper()
	assertStandaloneEventSequence(t, events, sessionID,
		[]factoryapi.FactoryEventType{
			factoryapi.FactoryEventTypeSessionStarted,
			factoryapi.FactoryEventTypeSessionResultUpdated,
			factoryapi.FactoryEventTypeSessionCompleted,
		},
		[]string{
			"session-started/" + sessionID,
			"session-result-updated/" + sessionID,
			"session-completed/" + sessionID,
		},
	)
	completed, err := events[2].Payload.AsSessionCompletedEventPayload()
	if err != nil || completed.FinalStatus != factoryapi.FactorySessionDurableLifecycleStatusFailed || completed.ResultStatus == nil || *completed.ResultStatus != factoryapi.FactoryEventSessionResultStatusUnavailable {
		t.Fatalf("standalone failed completed payload = (%#v, %v), want FAILED/UNAVAILABLE", completed, err)
	}
}

func assertStandaloneEventSequence(t *testing.T, events []factoryapi.FactoryEvent, sessionID string, wantTypes []factoryapi.FactoryEventType, wantIDs []string) {
	t.Helper()
	if len(events) != len(wantTypes) {
		t.Fatalf("standalone events = %d, want literal sequence of %d", len(events), len(wantTypes))
	}
	for index, event := range events {
		if event.Type != wantTypes[index] || event.Id != wantIDs[index] {
			t.Fatalf("standalone event %d = (%q, %q), want (%q, %q)", index, event.Type, event.Id, wantTypes[index], wantIDs[index])
		}
		if event.Context.SessionId == nil || *event.Context.SessionId != sessionID || event.Context.Sequence != index+1 || event.Context.Tick != index+1 || event.Context.SessionSequence == nil || *event.Context.SessionSequence != index {
			t.Fatalf("standalone event %d context = %#v, want session %q and sequence/tick %d/%d", index, event.Context, sessionID, index+1, index+1)
		}
	}
}

func assertStandaloneEventSession(t *testing.T, events []factoryapi.FactoryEvent, sessionID, label string) {
	t.Helper()
	if len(events) != 3 {
		t.Fatalf("%s events = %d, want three canonical lifecycle events", label, len(events))
	}
	for _, event := range events {
		if event.Context.SessionId == nil || *event.Context.SessionId != sessionID {
			t.Fatalf("%s event %q sessionId = %#v, want %q", label, event.Id, event.Context.SessionId, sessionID)
		}
	}
}

func assertStandaloneFailedSessionHasNoLiveDispatches(t *testing.T, client *standaloneMCPClient, sessionID string) {
	t.Helper()
	session := decodeStandaloneToolResponse[factoryapi.FactorySessionDurableReadModel](t, client.callTool(
		mcpfactorysession.ToolGetSession,
		map[string]any{"sessionId": sessionID},
	))
	if session.Error != nil || session.Result == nil || session.Result.Status != factoryapi.FactorySessionDurableLifecycleStatusFailed {
		t.Fatalf("failed standalone get = %#v, want terminal FAILED session", session)
	}
	if session.Result.FailureDetail == nil || session.Result.FailureDetail.Reason != "unknown" {
		t.Fatalf("failed standalone session failure = %#v, want typed failure", session.Result.FailureDetail)
	}

	dispatches := decodeStandaloneToolResponse[factoryapi.ListFactorySessionDispatchesResponse](t, client.callTool(
		mcpfactorysession.ToolListDispatches,
		map[string]any{"sessionId": sessionID},
	))
	if dispatches.Error != nil || dispatches.Result == nil || len(dispatches.Result.Dispatches) != 0 {
		t.Fatalf("failed standalone dispatches = %#v, want no lingering dispatches", dispatches)
	}
}

func writeStandaloneFixtureVariant(t *testing.T, sessionID, resultText string) string {
	t.Helper()
	raw, err := os.ReadFile(canonicalFixtureCatalogPath(t))
	if err != nil {
		t.Fatalf("read canonical fixture catalog: %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("decode canonical fixture catalog: %v", err)
	}
	scenarios, ok := document["scenarios"].([]any)
	if !ok {
		t.Fatal("canonical fixture catalog scenarios is not an array")
	}
	for _, item := range scenarios {
		scenario, ok := item.(map[string]any)
		if !ok || scenario["id"] != "petri-succeeded-one-dispatch" {
			continue
		}
		setStandaloneFixtureSessionID(scenario, sessionID)
		setStandaloneFixtureResultText(scenario, resultText)
		break
	}
	encoded, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		t.Fatalf("encode fixture variant: %v", err)
	}
	path := filepath.Join(t.TempDir(), "durable-session-contract-fixtures.json")
	if err := os.WriteFile(path, append(encoded, '\n'), 0o600); err != nil {
		t.Fatalf("write fixture variant: %v", err)
	}
	return path
}

func setStandaloneFixtureSessionID(scenario map[string]any, sessionID string) {
	oldSessionID := ""
	if session, ok := scenario["session"].(map[string]any); ok {
		oldSessionID, _ = session["sessionId"].(string)
	}
	if oldSessionID != "" {
		replaceStandaloneFixtureStrings(scenario, oldSessionID, sessionID)
	}
	if session, ok := scenario["session"].(map[string]any); ok {
		session["sessionId"] = sessionID
	}
	if response, ok := scenario["syncResponse"].(map[string]any); ok {
		response["sessionId"] = sessionID
		if result, ok := response["result"].(map[string]any); ok {
			result["sessionId"] = sessionID
		}
	}
	if result, ok := scenario["result"].(map[string]any); ok {
		result["sessionId"] = sessionID
	}
}

func replaceStandaloneFixtureStrings(value any, oldValue, newValue string) {
	switch typed := value.(type) {
	case map[string]any:
		for key, item := range typed {
			switch nested := item.(type) {
			case string:
				typed[key] = strings.ReplaceAll(nested, oldValue, newValue)
			default:
				replaceStandaloneFixtureStrings(nested, oldValue, newValue)
			}
		}
	case []any:
		for _, item := range typed {
			replaceStandaloneFixtureStrings(item, oldValue, newValue)
		}
	}
}

func setStandaloneFixtureResultText(scenario map[string]any, resultText string) {
	setPrimaryText := func(document map[string]any) {
		document["primaryResult"] = []any{map[string]any{"type": "text", "text": resultText}}
	}
	if response, ok := scenario["syncResponse"].(map[string]any); ok {
		if result, ok := response["result"].(map[string]any); ok {
			setPrimaryText(result)
		}
	}
	if result, ok := scenario["result"].(map[string]any); ok {
		setPrimaryText(result)
	}
}

func stringPtr(value string) *string { return &value }

type standaloneMCPServer struct {
	client     *standaloneMCPClient
	cancel     context.CancelFunc
	stdinWrite *os.File
	serveErr   <-chan error
	closeOnce  sync.Once
}

func startStandaloneFixtureServer(t *testing.T, fixtureCatalog string) *standaloneMCPServer {
	t.Helper()
	process, err := support.BuildProcessWithContext(t.Context(), serviceedges.Edges{})
	if err != nil {
		t.Fatalf("BuildProcess: %v", err)
	}
	stdinRead, stdinWrite, err := os.Pipe()
	if err != nil {
		t.Fatalf("create MCP stdin pipe: %v", err)
	}
	stdoutRead, stdoutWrite, err := os.Pipe()
	if err != nil {
		t.Fatalf("create MCP stdout pipe: %v", err)
	}
	t.Cleanup(func() {
		_ = stdinRead.Close()
		_ = stdinWrite.Close()
		_ = stdoutRead.Close()
		_ = stdoutWrite.Close()
	})

	ctx, cancel := context.WithCancel(t.Context())
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- process.Execute(root.Input{
			Args:             []string{"you", "server", "mcp", "--fixture-catalog", fixtureCatalog},
			Env:              builtcliacceptance.ProcessEnvForIsolatedHome(t.TempDir()),
			Stdin:            stdinRead,
			Stdout:           stdoutWrite,
			Context:          ctx,
			WorkingDirectory: t.TempDir(),
		})
	}()
	server := &standaloneMCPServer{
		client:     newStandaloneMCPClient(t, stdinWrite, stdoutRead),
		cancel:     cancel,
		stdinWrite: stdinWrite,
		serveErr:   serveErr,
	}
	t.Cleanup(func() { server.close(t) })
	return server
}

func (server *standaloneMCPServer) close(t *testing.T) {
	t.Helper()
	server.closeOnce.Do(func() {
		server.cancel()
		_ = server.stdinWrite.Close()
		err := <-server.serveErr
		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, io.EOF) && !strings.Contains(err.Error(), "file already closed") {
			t.Errorf("fixture-backed MCP server shutdown: %v", err)
		}
	})
}

type standaloneMCPClient struct {
	t      *testing.T
	stdin  io.Writer
	stdout *bufio.Reader
	nextID int
}

func newStandaloneMCPClient(t *testing.T, stdin io.Writer, stdout io.Reader) *standaloneMCPClient {
	t.Helper()
	return &standaloneMCPClient{t: t, stdin: stdin, stdout: bufio.NewReader(stdout)}
}

func (client *standaloneMCPClient) call(method string, params any) standaloneRPCResponse {
	client.t.Helper()
	client.nextID++
	payload, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      client.nextID,
		"method":  method,
		"params":  params,
	})
	if err != nil {
		client.t.Fatalf("marshal MCP %s request: %v", method, err)
	}
	if _, err := client.stdin.Write(append(payload, '\n')); err != nil {
		client.t.Fatalf("write MCP %s request: %v", method, err)
	}
	line, err := client.stdout.ReadString('\n')
	if err != nil {
		client.t.Fatalf("read MCP %s response: %v", method, err)
	}
	var response standaloneRPCResponse
	if err := json.Unmarshal([]byte(line), &response); err != nil {
		client.t.Fatalf("decode MCP %s response: %v", method, err)
	}
	if response.ID != client.nextID {
		client.t.Fatalf("MCP %s response id = %d, want %d", method, response.ID, client.nextID)
	}
	return response
}

func (client *standaloneMCPClient) callTool(name string, arguments any) standaloneRPCResponse {
	client.t.Helper()
	encoded, err := json.Marshal(arguments)
	if err != nil {
		client.t.Fatalf("marshal %s arguments: %v", name, err)
	}
	return client.call("tools/call", map[string]any{
		"name":      name,
		"arguments": json.RawMessage(encoded),
	})
}

func assertStandaloneInitialized(t *testing.T, client *standaloneMCPClient) {
	t.Helper()
	response := client.call("initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "btrc-p0-standalone", "version": "test"},
	})
	if response.Error != nil {
		t.Fatalf("MCP initialize error = %#v", response.Error)
	}
	if response.Result["protocolVersion"] != "2024-11-05" {
		t.Fatalf("MCP protocolVersion = %#v, want 2024-11-05", response.Result["protocolVersion"])
	}
}

func decodeStandaloneToolResponse[T any](t *testing.T, response standaloneRPCResponse) mcpfactorysession.ToolResponse[T] {
	t.Helper()
	if response.Error != nil {
		t.Fatalf("MCP tools/call protocol error = %#v", response.Error)
	}
	content, ok := response.Result["content"].([]any)
	if !ok || len(content) == 0 {
		t.Fatalf("MCP tools/call content = %#v, want one result item", response.Result)
	}
	item, ok := content[0].(map[string]any)
	if !ok {
		t.Fatalf("MCP tools/call content[0] = %#v, want object", content[0])
	}
	text, ok := item["text"].(string)
	if !ok {
		t.Fatalf("MCP tools/call content[0].text = %#v, want JSON text", item["text"])
	}
	var decoded mcpfactorysession.ToolResponse[T]
	if err := json.Unmarshal([]byte(text), &decoded); err != nil {
		t.Fatalf("decode MCP tool response: %v", err)
	}
	return decoded
}

type standaloneRPCResponse struct {
	ID     int            `json:"id"`
	Result map[string]any `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}
