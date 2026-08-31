package standalone_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	mcpfactorysession "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/mcp"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const standaloneFixtureStopTimeout = 5 * time.Second

// standalonePackageFixture owns immutable application wiring for the two
// eligible fixture rows. Each row still opens a fresh MCP Process.Execute
// invocation, so its fixture service/store, stdio streams, context, and
// working directory remain scenario-owned.
type standalonePackageFixture struct {
	sync.Mutex
	process   support.ApplicationProcess
	buildErr  error
	closeOnce sync.Once
	closeErr  error
}

var sharedStandaloneFixture standalonePackageFixture
var sharedStandaloneScenarioSlot = make(chan struct{}, 1)

// standaloneTopologyLedger keeps the optimization's real resource boundary
// observable without reaching into the production fixture service. One
// Process.Execute invocation corresponds to one fixture service/store scope.
type standaloneTopologyLedger struct {
	sync.Mutex
	sharedRootBuilds          int
	isolatedRootBuilds        int
	sharedRootCloses          int
	isolatedRootCloses        int
	invocationStarts          int
	invocationReturns         int
	contextsCanceled          int
	stdioPairsOpened          int
	stdioPairsClosed          int
	workingDirectoriesMade    int
	workingDirectoriesRemoved int
}

var standaloneTopology standaloneTopologyLedger

// TestMain closes the package-scoped root after all per-invocation fixture
// services, stores, and stdio streams have been released. S03 intentionally
// keeps two separately built roots so its cross-store not-found assertion
// remains a real isolation witness.
func TestMain(m *testing.M) {
	exitCode := m.Run()

	if err := closeSharedStandaloneFixture(); err != nil {
		fmt.Fprintf(os.Stderr, "close shared standalone fixture: %v\n", err)
		if exitCode == 0 {
			exitCode = 1
		}
	}
	if err := standaloneTopology.cleanupError(); err != nil {
		fmt.Fprintf(os.Stderr, "standalone cleanup accounting: %v\n", err)
		if exitCode == 0 {
			exitCode = 1
		}
	}
	fmt.Fprintf(os.Stderr, "GATE-STAND topology: %s\n", standaloneTopology.summary())
	os.Exit(exitCode)
}

func TestBTRCP0StandaloneFixtureSuccessCharacterization(t *testing.T) {
	server := startSharedStandaloneFixtureServer(t)
	assertStandaloneInitialized(t, server.client)

	start := callStandaloneStartSync(t, server.client, standaloneSuccessRequest("req-petri-success-001", "customer-support-triage", "TKT-2002"))
	assertStandaloneSuccessResult(t, start)

	events := callStandaloneEvents(t, server.client, start.SessionId)
	assertStandaloneSuccessEvents(t, events, start.SessionId)

	result := callStandaloneResult(t, server.client, start.SessionId, true)
	assertStandaloneCompleteResult(t, result, start.SessionId)
}

func TestBTRCP0StandaloneFixtureFailureCharacterization(t *testing.T) {
	server := startSharedStandaloneFixtureServer(t)
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
	assertStandaloneSessionNotFound(
		t,
		server.client,
		"dur-sess-petri-success-001",
		"prior shared fixture invocation",
	)
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

func assertStandaloneSessionNotFound(t *testing.T, client *standaloneMCPClient, sessionID, label string) {
	t.Helper()
	response := decodeStandaloneToolResponse[factoryapi.FactorySessionDurableReadModel](t, client.callTool(
		mcpfactorysession.ToolGetSession,
		map[string]any{"sessionId": sessionID},
	))
	if response.Error == nil || response.Error.Code != "factory_session.session.not_found" {
		t.Fatalf("%s get_session response = %#v, want typed session.not_found", label, response)
	}
	if response.Error.SessionID != sessionID {
		t.Fatalf("%s get_session error sessionId = %q, want %q", label, response.Error.SessionID, sessionID)
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
	client           *standaloneMCPClient
	process          support.ApplicationProcess
	ownsProcess      bool
	cancel           context.CancelFunc
	stdinRead        *os.File
	stdinWrite       *os.File
	stdoutRead       *os.File
	stdoutWrite      *os.File
	serveErr         <-chan error
	stderr           *bytes.Buffer
	workingDirectory string
	closeOnce        sync.Once
	closeErr         error
}

func startStandaloneFixtureServer(t *testing.T, fixtureCatalog string) *standaloneMCPServer {
	t.Helper()
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{})
	if err != nil {
		t.Fatalf("BuildProcess: %v", err)
	}
	standaloneTopology.recordIsolatedRootBuild()
	return startStandaloneFixtureServerWithProcess(t, process, true, fixtureCatalog)
}

func startSharedStandaloneFixtureServer(t *testing.T) *standaloneMCPServer {
	t.Helper()
	acquireSharedStandaloneScenarioSlot(t)
	return startStandaloneFixtureServerWithProcess(
		t,
		sharedStandaloneProcess(t),
		false,
		canonicalFixtureCatalogPath(t),
	)
}

func sharedStandaloneProcess(t testing.TB) support.ApplicationProcess {
	t.Helper()

	sharedStandaloneFixture.Lock()
	defer sharedStandaloneFixture.Unlock()
	if sharedStandaloneFixture.process == nil && sharedStandaloneFixture.buildErr == nil {
		sharedStandaloneFixture.process, sharedStandaloneFixture.buildErr = support.BuildProcessWithContext(
			context.Background(), serviceedges.Edges{},
		)
		if sharedStandaloneFixture.buildErr == nil {
			standaloneTopology.recordSharedRootBuild()
		}
	}
	if sharedStandaloneFixture.buildErr != nil {
		t.Fatalf("BuildProcess() for shared standalone fixture: %v", sharedStandaloneFixture.buildErr)
	}
	if sharedStandaloneFixture.process == nil {
		t.Fatal("shared standalone fixture process is unavailable")
	}
	return sharedStandaloneFixture.process
}

func acquireSharedStandaloneScenarioSlot(t testing.TB) {
	t.Helper()
	sharedStandaloneScenarioSlot <- struct{}{}
	t.Cleanup(func() { <-sharedStandaloneScenarioSlot })
}

func closeSharedStandaloneFixture() error {
	sharedStandaloneFixture.Lock()
	process := sharedStandaloneFixture.process
	sharedStandaloneFixture.Unlock()
	if process == nil {
		return nil
	}

	sharedStandaloneFixture.closeOnce.Do(func() {
		closeContext, cancel := context.WithTimeout(context.Background(), standaloneFixtureStopTimeout)
		defer cancel()
		sharedStandaloneFixture.closeErr = process.Close(closeContext)
		standaloneTopology.recordSharedRootClose()
	})
	return sharedStandaloneFixture.closeErr
}

func startStandaloneFixtureServerWithProcess(
	t *testing.T,
	process support.ApplicationProcess,
	ownsProcess bool,
	fixtureCatalog string,
) *standaloneMCPServer {
	t.Helper()
	if process == nil {
		t.Fatal("start standalone fixture server requires an application process")
	}

	stdinRead, stdinWrite, err := os.Pipe()
	if err != nil {
		closeStandaloneProcessAfterStartFailure(process, ownsProcess)
		t.Fatalf("create MCP stdin pipe: %v", err)
	}
	stdoutRead, stdoutWrite, err := os.Pipe()
	if err != nil {
		_ = stdinRead.Close()
		_ = stdinWrite.Close()
		closeStandaloneProcessAfterStartFailure(process, ownsProcess)
		t.Fatalf("create MCP stdout pipe: %v", err)
	}
	workingDirectory, err := os.MkdirTemp("", "you-functional-sessions-standalone-")
	if err != nil {
		_ = stdinRead.Close()
		_ = stdinWrite.Close()
		_ = stdoutRead.Close()
		_ = stdoutWrite.Close()
		closeStandaloneProcessAfterStartFailure(process, ownsProcess)
		t.Fatalf("create MCP working directory: %v", err)
	}
	standaloneTopology.recordStdioPairOpened()
	standaloneTopology.recordWorkingDirectoryMade()

	ctx, cancel := context.WithCancel(context.Background())
	serveErr := make(chan error, 1)
	stderr := &bytes.Buffer{}
	standaloneTopology.recordInvocationStarted()
	go func() {
		serveErr <- process.Execute(root.Input{
			Args:             []string{"you", "server", "mcp", "--fixture-catalog", fixtureCatalog},
			Env:              os.Environ(),
			Stdin:            stdinRead,
			Stdout:           stdoutWrite,
			Stderr:           stderr,
			Context:          ctx,
			WorkingDirectory: workingDirectory,
		})
		standaloneTopology.recordInvocationReturned()
	}()
	server := &standaloneMCPServer{
		client:           newStandaloneMCPClient(t, stdinWrite, stdoutRead),
		process:          process,
		ownsProcess:      ownsProcess,
		cancel:           cancel,
		stdinRead:        stdinRead,
		stdinWrite:       stdinWrite,
		stdoutRead:       stdoutRead,
		stdoutWrite:      stdoutWrite,
		serveErr:         serveErr,
		stderr:           stderr,
		workingDirectory: workingDirectory,
	}
	t.Cleanup(func() { server.close(t) })
	return server
}

func closeStandaloneProcessAfterStartFailure(process support.ApplicationProcess, ownsProcess bool) {
	if !ownsProcess || process == nil {
		return
	}
	closeContext, cancel := context.WithTimeout(context.Background(), standaloneFixtureStopTimeout)
	_ = process.Close(closeContext)
	cancel()
	standaloneTopology.recordIsolatedRootClose()
}

func (server *standaloneMCPServer) close(t *testing.T) {
	t.Helper()
	server.closeOnce.Do(func() {
		var closeErrors []error
		server.cancel()
		standaloneTopology.recordContextCanceled()
		_ = server.stdinWrite.Close()
		select {
		case err := <-server.serveErr:
			if err != nil && !standaloneServeShutdownError(err) {
				closeErrors = append(closeErrors, fmt.Errorf(
					"fixture-backed MCP server: %w (stderr=%q)",
					err,
					strings.TrimSpace(server.stderr.String()),
				))
			}
		// A returned serveErr is the deterministic shutdown signal. This
		// timeout is only a cleanup hang guard for a stuck stdio transport.
		case <-time.After(standaloneFixtureStopTimeout):
			closeErrors = append(closeErrors, fmt.Errorf("fixture-backed MCP server did not shut down"))
		}
		_ = server.stdinRead.Close()
		_ = server.stdoutRead.Close()
		_ = server.stdoutWrite.Close()
		standaloneTopology.recordStdioPairClosed()

		if server.ownsProcess {
			closeContext, cancel := context.WithTimeout(context.Background(), standaloneFixtureStopTimeout)
			if err := server.process.Close(closeContext); err != nil {
				closeErrors = append(closeErrors, fmt.Errorf("close isolated MCP process: %w", err))
			}
			cancel()
			standaloneTopology.recordIsolatedRootClose()
		}
		if err := os.RemoveAll(server.workingDirectory); err != nil {
			closeErrors = append(closeErrors, fmt.Errorf("remove MCP working directory: %w", err))
		}
		standaloneTopology.recordWorkingDirectoryRemoved()
		server.closeErr = errors.Join(closeErrors...)
	})
	if server.closeErr != nil {
		t.Errorf("standalone fixture cleanup: %v", server.closeErr)
	}
}

func standaloneServeShutdownError(err error) bool {
	return errors.Is(err, context.Canceled) ||
		errors.Is(err, io.EOF) ||
		errors.Is(err, os.ErrClosed) ||
		strings.Contains(err.Error(), "file already closed")
}

func (ledger *standaloneTopologyLedger) recordSharedRootBuild() {
	ledger.Lock()
	ledger.sharedRootBuilds++
	ledger.Unlock()
}

func (ledger *standaloneTopologyLedger) recordIsolatedRootBuild() {
	ledger.Lock()
	ledger.isolatedRootBuilds++
	ledger.Unlock()
}

func (ledger *standaloneTopologyLedger) recordSharedRootClose() {
	ledger.Lock()
	ledger.sharedRootCloses++
	ledger.Unlock()
}

func (ledger *standaloneTopologyLedger) recordIsolatedRootClose() {
	ledger.Lock()
	ledger.isolatedRootCloses++
	ledger.Unlock()
}

func (ledger *standaloneTopologyLedger) recordInvocationStarted() {
	ledger.Lock()
	ledger.invocationStarts++
	ledger.Unlock()
}

func (ledger *standaloneTopologyLedger) recordInvocationReturned() {
	ledger.Lock()
	ledger.invocationReturns++
	ledger.Unlock()
}

func (ledger *standaloneTopologyLedger) recordContextCanceled() {
	ledger.Lock()
	ledger.contextsCanceled++
	ledger.Unlock()
}

func (ledger *standaloneTopologyLedger) recordStdioPairOpened() {
	ledger.Lock()
	ledger.stdioPairsOpened++
	ledger.Unlock()
}

func (ledger *standaloneTopologyLedger) recordStdioPairClosed() {
	ledger.Lock()
	ledger.stdioPairsClosed++
	ledger.Unlock()
}

func (ledger *standaloneTopologyLedger) recordWorkingDirectoryMade() {
	ledger.Lock()
	ledger.workingDirectoriesMade++
	ledger.Unlock()
}

func (ledger *standaloneTopologyLedger) recordWorkingDirectoryRemoved() {
	ledger.Lock()
	ledger.workingDirectoriesRemoved++
	ledger.Unlock()
}

func (ledger *standaloneTopologyLedger) cleanupError() error {
	ledger.Lock()
	defer ledger.Unlock()
	var cleanupErrors []error
	if ledger.sharedRootBuilds != ledger.sharedRootCloses {
		cleanupErrors = append(cleanupErrors, fmt.Errorf(
			"shared roots built=%d closed=%d",
			ledger.sharedRootBuilds,
			ledger.sharedRootCloses,
		))
	}
	if ledger.isolatedRootBuilds != ledger.isolatedRootCloses {
		cleanupErrors = append(cleanupErrors, fmt.Errorf(
			"isolated roots built=%d closed=%d",
			ledger.isolatedRootBuilds,
			ledger.isolatedRootCloses,
		))
	}
	if ledger.invocationStarts != ledger.invocationReturns {
		cleanupErrors = append(cleanupErrors, fmt.Errorf(
			"fixture invocations started=%d returned=%d",
			ledger.invocationStarts,
			ledger.invocationReturns,
		))
	}
	if ledger.contextsCanceled != ledger.invocationStarts {
		cleanupErrors = append(cleanupErrors, fmt.Errorf(
			"fixture contexts canceled=%d started=%d",
			ledger.contextsCanceled,
			ledger.invocationStarts,
		))
	}
	if ledger.stdioPairsOpened != ledger.stdioPairsClosed {
		cleanupErrors = append(cleanupErrors, fmt.Errorf(
			"stdio pairs opened=%d closed=%d",
			ledger.stdioPairsOpened,
			ledger.stdioPairsClosed,
		))
	}
	if ledger.workingDirectoriesMade != ledger.workingDirectoriesRemoved {
		cleanupErrors = append(cleanupErrors, fmt.Errorf(
			"working directories made=%d removed=%d",
			ledger.workingDirectoriesMade,
			ledger.workingDirectoriesRemoved,
		))
	}
	return errors.Join(cleanupErrors...)
}

func (ledger *standaloneTopologyLedger) summary() string {
	ledger.Lock()
	defer ledger.Unlock()
	return fmt.Sprintf(
		"root_builds={shared:%d isolated:%d} root_closes={shared:%d isolated:%d} fixture_scopes={started:%d returned:%d} contexts={canceled:%d} stdio_pairs={opened:%d closed:%d} working_dirs={made:%d removed:%d}",
		ledger.sharedRootBuilds,
		ledger.isolatedRootBuilds,
		ledger.sharedRootCloses,
		ledger.isolatedRootCloses,
		ledger.invocationStarts,
		ledger.invocationReturns,
		ledger.contextsCanceled,
		ledger.stdioPairsOpened,
		ledger.stdioPairsClosed,
		ledger.workingDirectoriesMade,
		ledger.workingDirectoriesRemoved,
	)
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
