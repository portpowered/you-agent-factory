package mcp_test

import (
	"testing"
	"time"

	mcpfactorysession "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/mcp"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// TestMCPPauseResumeAndCancelTargetCanonicalFactorySession proves public MCP
// Factory Session lifecycle controls target one durable session id: pause,
// resume, and cancel through you.factory_session.control keep the same
// canonical sessionId on control responses and subsequent MCP session reads.
func TestMCPPauseResumeAndCancelTargetCanonicalFactorySession(t *testing.T) {
	projectRoot := setupBusyLoopWorkflowFixture(t)
	client, shutdown, serveErr := startRootRuntimeMCPServer(t, projectRoot, nil)
	assertMCPInitialized(t, client)

	toolsResult := client.call("tools/list", map[string]any{})
	toolNames := toolNamesFromListResult(t, toolsResult.Result)
	for _, want := range []string{
		mcpfactorysession.ToolStartAsync,
		mcpfactorysession.ToolGetSession,
		mcpfactorysession.ToolControl,
	} {
		if !containsString(toolNames, want) {
			t.Fatalf("tools/list missing canonical control tool %q; got %#v", want, toolNames)
		}
	}

	sessionID := startMCPRunningSession(t, client)
	startedRead := readMCPSessionDurableReadModel(t, client, sessionID)
	assertCanonicalSessionID(t, startedRead.SessionId, sessionID, "start read")

	pauseResponse := mcpControl(
		t,
		client,
		sessionID,
		factoryapi.FactorySessionLifecycleControlKindPause,
		factoryapi.FactorySessionLifecycleControlOutcomeAccepted,
	)
	if pauseResponse.Operation != factoryapi.FactorySessionLifecycleControlKindPause {
		t.Fatalf("pause operation = %q, want PAUSE", pauseResponse.Operation)
	}
	if pauseResponse.Status != factoryapi.FactorySessionDurableLifecycleStatusPaused {
		t.Fatalf("pause status = %q, want PAUSED", pauseResponse.Status)
	}
	pausedRead := readMCPSessionDurableReadModel(t, client, sessionID)
	assertCanonicalSessionID(t, pausedRead.SessionId, sessionID, "pause read")
	if pausedRead.Status != factoryapi.FactorySessionDurableLifecycleStatusPaused {
		t.Fatalf("pause read status = %q, want PAUSED", pausedRead.Status)
	}

	resumeResponse := mcpControl(
		t,
		client,
		sessionID,
		factoryapi.FactorySessionLifecycleControlKindResume,
		factoryapi.FactorySessionLifecycleControlOutcomeAccepted,
	)
	if resumeResponse.Operation != factoryapi.FactorySessionLifecycleControlKindResume {
		t.Fatalf("resume operation = %q, want RESUME", resumeResponse.Operation)
	}
	if resumeResponse.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("resume status = %q, want RUNNING", resumeResponse.Status)
	}
	resumedRead := readMCPSessionDurableReadModel(t, client, sessionID)
	assertCanonicalSessionID(t, resumedRead.SessionId, sessionID, "resume read")
	if resumedRead.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("resume read status = %q, want RUNNING", resumedRead.Status)
	}

	cancelResponse := mcpControl(
		t,
		client,
		sessionID,
		factoryapi.FactorySessionLifecycleControlKindCancel,
		factoryapi.FactorySessionLifecycleControlOutcomeAccepted,
	)
	if cancelResponse.Operation != factoryapi.FactorySessionLifecycleControlKindCancel {
		t.Fatalf("cancel operation = %q, want CANCEL", cancelResponse.Operation)
	}
	canceledRead := waitForMCPSessionStatus(
		t,
		client,
		sessionID,
		factoryapi.FactorySessionDurableLifecycleStatusCanceled,
		8*time.Second,
	)
	assertCanonicalSessionID(t, canceledRead.SessionId, sessionID, "cancel read")

	shutdown()
	closeMCPServer(t, nil, serveErr)
}

// TestMCPControlledSessionIsReadableThroughAPI proves an MCP-controlled Factory
// Session remains readable through the public API with the same session id and
// shared Factory Session vocabulary after MCP start and lifecycle interaction.
func TestMCPControlledSessionIsReadableThroughAPI(t *testing.T) {
	projectRoot := setupBusyLoopWorkflowFixture(t)
	apiServer := startFunctionalAPIServerForMCPControls(t, projectRoot)
	baseURL := apiServer.URL()

	client, shutdown, serveErr := startRootRuntimeMCPServer(t, projectRoot, nil)
	assertMCPInitialized(t, client)

	sessionID := startMCPRunningSession(t, client)
	mcpControl(
		t,
		client,
		sessionID,
		factoryapi.FactorySessionLifecycleControlKindPause,
		factoryapi.FactorySessionLifecycleControlOutcomeAccepted,
	)

	mcpRead := readMCPSessionDurableReadModel(t, client, sessionID)
	assertCanonicalSessionID(t, mcpRead.SessionId, sessionID, "mcp read")
	if mcpRead.Status != factoryapi.FactorySessionDurableLifecycleStatusPaused {
		t.Fatalf("mcp read status = %q, want PAUSED", mcpRead.Status)
	}

	apiRead := readAPIFactorySessionDurableReadModel(t, baseURL, sessionID)
	assertAPIReadMatchesMCPSharedFactorySessionVocabulary(t, mcpRead, apiRead)

	shutdown()
	closeMCPServer(t, nil, serveErr)
}

// TestMCPSynchronousFactorySessionReturnsTerminalResult proves public MCP
// you.factory_session.start_sync returns a terminal success result for a
// successful Factory Session run without requiring async polling.
func TestMCPSynchronousFactorySessionReturnsTerminalResult(t *testing.T) {
	projectRoot := setupSimpleFinalWorkflowFixture(t)
	client, shutdown, serveErr := startRootRuntimeMCPServer(t, projectRoot, nil)
	assertMCPInitialized(t, client)

	toolsResult := client.call("tools/list", map[string]any{})
	toolNames := toolNamesFromListResult(t, toolsResult.Result)
	if !containsString(toolNames, mcpfactorysession.ToolStartSync) {
		t.Fatalf("tools/list missing sync start tool %q; got %#v", mcpfactorysession.ToolStartSync, toolNames)
	}

	syncResult := startMCPSyncSucceededSession(t, client)
	sessionRead := readMCPSessionDurableReadModel(t, client, syncResult.SessionId)
	assertCanonicalSessionID(t, sessionRead.SessionId, syncResult.SessionId, "sync session read")
	if sessionRead.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("sync session read status = %q, want SUCCEEDED", sessionRead.Status)
	}
	if sessionRead.ResultSummary == nil ||
		sessionRead.ResultSummary.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("sync session read resultSummary = %#v, want FINAL", sessionRead.ResultSummary)
	}

	shutdown()
	closeMCPServer(t, nil, serveErr)
}

// TestMCPSynchronousFailureReturnsStructuredFailure proves public MCP
// you.factory_session.start_sync surfaces a structured sync failure for a
// failing Factory Session run without presenting a terminal success primary
// result on the sync response or subsequent MCP session read.
func TestMCPSynchronousFailureReturnsStructuredFailure(t *testing.T) {
	projectRoot := setupThrowErrorWorkflowFixture(t)
	client, shutdown, serveErr := startRootRuntimeMCPServer(t, projectRoot, nil)
	assertMCPInitialized(t, client)

	syncResult := startMCPSyncFailedSession(t, client)
	sessionRead := readMCPSessionDurableReadModel(t, client, syncResult.SessionId)
	assertCanonicalSessionID(t, sessionRead.SessionId, syncResult.SessionId, "sync failure session read")
	if sessionRead.Status != factoryapi.FactorySessionDurableLifecycleStatusFailed {
		t.Fatalf("sync session read status = %q, want FAILED", sessionRead.Status)
	}
	assertMCPStructuredFailureDetail(t, sessionRead.FailureDetail, "sync failure session read")
	if sessionRead.ResultSummary != nil &&
		sessionRead.ResultSummary.ResultStatus == factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("sync session read resultSummary = %#v, want non-FINAL failure availability", sessionRead.ResultSummary)
	}

	shutdown()
	closeMCPServer(t, nil, serveErr)
}

// TestMCPAsyncFactorySessionCanBePolledToSuccess proves public MCP
// you.factory_session.start_async returns a session id and subsequent MCP get
// and get_result polling observe progress until a terminal success result.
func TestMCPAsyncFactorySessionCanBePolledToSuccess(t *testing.T) {
	projectRoot := setupSimpleFinalWorkflowFixture(t)
	client, shutdown, serveErr := startRootRuntimeMCPServer(t, projectRoot, nil)
	assertMCPInitialized(t, client)

	toolsResult := client.call("tools/list", map[string]any{})
	toolNames := toolNamesFromListResult(t, toolsResult.Result)
	for _, want := range []string{
		mcpfactorysession.ToolStartAsync,
		mcpfactorysession.ToolGetSession,
		mcpfactorysession.ToolGetResult,
	} {
		if !containsString(toolNames, want) {
			t.Fatalf("tools/list missing async poll tool %q; got %#v", want, toolNames)
		}
	}

	sessionID, asyncStart := startMCPAsyncSucceededSession(t, client)
	assertCanonicalSessionID(t, asyncStart.SessionId, sessionID, "async start")

	sessionRead, terminalResult := pollMCPAsyncSessionToTerminalSuccess(t, client, sessionID, 8*time.Second)
	assertCanonicalSessionID(t, sessionRead.SessionId, sessionID, "async poll session read")
	if sessionRead.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("async poll session status = %q, want SUCCEEDED", sessionRead.Status)
	}
	if sessionRead.ResultSummary == nil ||
		sessionRead.ResultSummary.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("async poll session resultSummary = %#v, want FINAL", sessionRead.ResultSummary)
	}
	if terminalResult.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("async poll result status = %q, want FINAL", terminalResult.ResultStatus)
	}

	shutdown()
	closeMCPServer(t, nil, serveErr)
}

// TestMCPAsyncFactorySessionCanBePolledToFailure proves public MCP
// you.factory_session.start_async plus get and get_result polling reach a
// terminal failure outcome without presenting a terminal success result.
func TestMCPAsyncFactorySessionCanBePolledToFailure(t *testing.T) {
	projectRoot := setupAsyncThrowErrorWorkflowFixture(t)
	client, shutdown, serveErr := startRootRuntimeMCPServer(t, projectRoot, nil)
	assertMCPInitialized(t, client)

	toolsResult := client.call("tools/list", map[string]any{})
	toolNames := toolNamesFromListResult(t, toolsResult.Result)
	for _, want := range []string{
		mcpfactorysession.ToolStartAsync,
		mcpfactorysession.ToolGetSession,
		mcpfactorysession.ToolGetResult,
	} {
		if !containsString(toolNames, want) {
			t.Fatalf("tools/list missing async poll tool %q; got %#v", want, toolNames)
		}
	}

	sessionID, asyncStart := startMCPAsyncFailedSession(t, client)
	assertCanonicalSessionID(t, asyncStart.SessionId, sessionID, "async failure start")

	sessionRead, terminalResult := pollMCPAsyncSessionToTerminalFailure(t, client, sessionID, 8*time.Second)
	assertCanonicalSessionID(t, sessionRead.SessionId, sessionID, "async failure poll session read")
	if sessionRead.Status != factoryapi.FactorySessionDurableLifecycleStatusFailed {
		t.Fatalf("async failure poll session status = %q, want FAILED", sessionRead.Status)
	}
	assertMCPStructuredFailureDetail(t, sessionRead.FailureDetail, "async failure poll session read")
	if sessionRead.ResultSummary != nil &&
		sessionRead.ResultSummary.ResultStatus == factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("async failure poll session resultSummary = %#v, want non-FINAL failure availability", sessionRead.ResultSummary)
	}
	assertMCPAsyncFailureNotTerminalSuccess(t, terminalResult)

	shutdown()
	closeMCPServer(t, nil, serveErr)
}

func toolNamesFromListResult(t *testing.T, result map[string]any) []string {
	t.Helper()
	rawTools, ok := result["tools"].([]any)
	if !ok {
		t.Fatalf("tools/list result missing tools array: %#v", result)
	}
	names := make([]string, 0, len(rawTools))
	for _, raw := range rawTools {
		tool, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("tool entry = %#v, want object", raw)
		}
		name, _ := tool["name"].(string)
		names = append(names, name)
	}
	return names
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
