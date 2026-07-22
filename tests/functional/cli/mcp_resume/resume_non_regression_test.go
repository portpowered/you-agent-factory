package mcp_resume_test

import (
	"encoding/json"
	"strings"
	"testing"

	mcpfactorysession "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/mcp"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestMCPResumeSmokeLane_RuntimeBackedAsyncServeRegression(t *testing.T) {
	projectRoot := runtimeSmokeProjectRoot(t)
	client, shutdown, serveErr := startRunServeRuntimeSmokeServer(t, projectRoot)
	assertInstallSmokeInitialize(t, client)
	assertInstallSmokeDiscovery(t, client)

	sessionID := assertRuntimeSmokeAsyncStart(t, client)
	assertRuntimeSmokePollObservesRunningOrTerminal(t, client, sessionID)
	waitRuntimeSmokeTerminalCompletion(t, client, sessionID)
	shutdown()
	closeRunServeSmokeServer(t, nil, serveErr)
}

func TestMCPResumeSmokeLane_ResumeControlStaysOnCanonicalFactorySessionTools(t *testing.T) {
	harness := newMCPRuntimeResumeSmokeHarness(t)
	client, shutdown, serveErr := startRootRuntimeMCPServer(t, harness.projectRoot, harness.provider)
	assertInstallSmokeInitialize(t, client)

	toolsResult := client.call("tools/list", map[string]any{})
	toolNames := toolNamesFromListResult(t, toolsResult.Result)
	for _, want := range []string{
		mcpfactorysession.ToolStartAsync,
		mcpfactorysession.ToolGetSession,
		mcpfactorysession.ToolGetResult,
		mcpfactorysession.ToolControl,
		mcpfactorysession.ToolListDispatches,
	} {
		if !containsString(toolNames, want) {
			t.Fatalf("tools/list missing canonical resume tool %q; got %#v", want, toolNames)
		}
	}

	sessionID := startMCPRuntimeResumeSmokeInterruptedSession(t, client, harness)
	resumeResponse := mcpControlResume(t, client, sessionID)
	if resumeResponse.SessionId != sessionID {
		t.Fatalf("resume sessionId = %q, want %q", resumeResponse.SessionId, sessionID)
	}

	listed := listMCPDispatches(t, client, sessionID)
	if listed.SessionId != sessionID {
		t.Fatalf("dispatch sessionId = %q, want %q", listed.SessionId, sessionID)
	}
	if len(listed.Dispatches) == 0 {
		t.Fatal("expected dispatch summaries on shared FactorySession inspection surface")
	}

	shutdown()
	closeRunServeSmokeServer(t, nil, serveErr)
}

func TestMCPResumeSmokeLane_ResumeReadModelsUseSharedFactorySessionVocabulary(t *testing.T) {
	harness := newMCPRuntimeResumeSmokeSucceededHarness(t)
	client, shutdown, serveErr := startRootRuntimeMCPServer(t, harness.projectRoot, nil)
	assertInstallSmokeInitialize(t, client)

	sessionID := startMCPRuntimeResumeSmokeSucceededSession(t, client)
	read := readMCPSessionDurableReadModel(t, client, sessionID)
	if read.SessionId != sessionID {
		t.Fatalf("sessionId = %q, want %q", read.SessionId, sessionID)
	}
	if read.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("status = %q, want SUCCEEDED", read.Status)
	}
	if read.Lifecycle != nil && read.Lifecycle.ResumedAt != nil {
		t.Fatalf("terminal non-resume read should not expose resumedAt: %#v", read.Lifecycle)
	}

	encoded, err := json.Marshal(read)
	if err != nil {
		t.Fatalf("marshal durable read model: %v", err)
	}
	assertMCPResumeSmokeLaneOutputExcludesForbiddenVocabulary(t, string(encoded))

	shutdown()
	closeRunServeSmokeServer(t, nil, serveErr)
}

func assertMCPResumeSmokeLaneOutputExcludesForbiddenVocabulary(t *testing.T, text string) {
	t.Helper()
	lower := strings.ToLower(text)
	for _, term := range []string{"DynamicWorkflowRun", "workflow run"} {
		if strings.Contains(lower, strings.ToLower(term)) {
			t.Fatalf("output introduced forbidden vocabulary %q:\n%s", term, text)
		}
	}
}
