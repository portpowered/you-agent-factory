package mcpcli_test

import (
	"encoding/json"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution/fixtures"
	mcpfactorysession "github.com/portpowered/infinite-you/pkg/mcp/factorysession"
)

func TestMCPResumeSmokeLane_FixtureBackedInstallSmokeRegression(t *testing.T) {
	service := newContractFixtureService(t)
	projectRoot := writeValidWorkflowFixture(t)

	client, stdinWrite, serveErr := startRunServeSmokeServer(t, service)
	assertInstallSmokeInitialize(t, client)
	assertInstallSmokeDiscovery(t, client)
	assertInstallSmokeValidateSuccess(t, client, projectRoot)
	sessionID := assertInstallSmokeAsyncStart(t, client)
	assertInstallSmokeRunningPoll(t, client, sessionID)
	closeRunServeSmokeServer(t, stdinWrite, serveErr)
}

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
	client, shutdown, serveErr := startRunServeWithRuntimeService(t, harness.service)
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

func TestMCPResumeSmokeLane_ToolDiscoveryAvoidsForbiddenVocabulary(t *testing.T) {
	service := newContractFixtureService(t)
	client, stdinWrite, serveErr := startRunServeSmokeServer(t, service)
	assertInstallSmokeInitialize(t, client)

	toolsResult := client.call("tools/list", map[string]any{})
	encoded, err := json.Marshal(toolsResult.Result)
	if err != nil {
		t.Fatalf("marshal tools/list: %v", err)
	}
	assertMCPResumeSmokeLaneOutputExcludesForbiddenVocabulary(t, string(encoded))

	closeRunServeSmokeServer(t, stdinWrite, serveErr)
}

func TestMCPResumeSmokeLane_ResumeReadModelsUseSharedFactorySessionVocabulary(t *testing.T) {
	harness := newMCPRuntimeResumeSmokeSucceededHarness(t)
	client, shutdown, serveErr := startRunServeWithRuntimeService(t, harness.service)
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
	for _, term := range fixtures.ForbiddenFixtureVocabularyTerms() {
		if strings.Contains(lower, strings.ToLower(term)) {
			t.Fatalf("output introduced forbidden vocabulary %q:\n%s", term, text)
		}
	}
}
