package sessionexecution_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/factory"
	fse "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	"github.com/portpowered/infinite-you/pkg/transports/cli/sessionexecution"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
)

// TestRunSync_LiveProviderJavaScriptSession_ReReadStatusAndResult proves CLI
// live-dispatch smoke through the shared execution-service path only. MCP host
// setup and website inspection are deferred follow-up cells (see
// follow-up-cell-cli-live-dispatch-smoke-deferred.md).
func TestRunSync_LiveProviderJavaScriptSession_ReReadStatusAndResult(t *testing.T) {
	service, projectRoot := newLiveChildCLIJavaScriptRuntimeService(t)
	backend := liveProviderJavaScriptBackend(projectRoot)
	runResponse := runLiveProviderCLIJSONSync(t, service, backend, "req-cli-live-child-smoke-001")
	assertLiveProviderSucceededRunResponse(t, runResponse)
	assertLiveProviderCLIStatusMatchesRun(t, service, backend, runResponse.SessionId)
	assertLiveProviderCLIResultMatchesRun(t, service, backend, runResponse.SessionId)
	assertLiveProviderDispatchExecutionMode(t, service, runResponse.SessionId)
}

// TestRunSync_LiveProviderJavaScriptSession_RequiresConfiguredProvider proves
// the CLI does not silently install a fake provider when live execution is
// requested without an injected execution service.
func TestRunSync_LiveProviderJavaScriptSession_RequiresConfiguredProvider(t *testing.T) {
	projectRoot := setupCLIAgentRunWorkflowFixture(t)
	backend := sessionexecution.ExecutionBackendConfig{
		Provider:    string(fse.ExecutionProviderJavaScriptRuntime),
		ProjectRoot: projectRoot,
	}

	var runOutput bytes.Buffer
	err := sessionexecution.RunSync(context.Background(), sessionexecution.RunConfig{
		StartConfig: sessionexecution.StartConfig{
			Mode:              sessionexecution.ExecutionModeSync,
			RequestID:         "req-cli-live-child-fresh-service-001",
			WorkflowName:      "agent-run-fake-child",
			ArgsJSON:          `{"subject":"workflows"}`,
			ChildExecutorMode: fse.ChildExecutorModeLive,
		},
		ExecutionBackendConfig: backend,
		JSON:                   true,
		Output:                 &runOutput,
	})
	if err == nil || !strings.Contains(err.Error(), "durable execution service is required") {
		t.Fatalf("RunSync error = %v, want missing injected service", err)
	}
}

// TestLiveProviderJavaScriptSession_DispatchAndArtifactCLIInspection proves
// bridged-child dispatch and artifact linkage through direct CLI reads backed by
// shared ListDispatchesResponseToAPI / ListArtifactsResponseToAPI projections.
func TestLiveProviderJavaScriptSession_DispatchAndArtifactCLIInspection(t *testing.T) {
	service, projectRoot := newLiveChildCLIJavaScriptRuntimeService(t)
	backend := sessionexecution.ExecutionBackendConfig{
		Provider:    string(fse.ExecutionProviderJavaScriptRuntime),
		ProjectRoot: projectRoot,
	}
	sessionID := runLiveProviderSmokeSession(t, service, backend, "req-cli-live-child-dispatch-smoke-001")
	assertLiveProviderDispatchCLIOutput(t, service, sessionID, backend)
	assertLiveProviderArtifactCLIOutput(t, service, sessionID, backend)
}

func TestRunSync_JavaScriptRuntimeBackend_DoesNotInstallLiveFixtureProvider(t *testing.T) {
	projectRoot := setupCLIAgentRunWorkflowFixture(t)

	var runOutput bytes.Buffer
	err := sessionexecution.RunSync(context.Background(), sessionexecution.RunConfig{
		StartConfig: sessionexecution.StartConfig{
			Mode:              sessionexecution.ExecutionModeSync,
			RequestID:         "req-cli-live-child-resolver-smoke-001",
			WorkflowName:      "agent-run-fake-child",
			ArgsJSON:          `{"subject":"workflows"}`,
			ChildExecutorMode: fse.ChildExecutorModeLive,
		},
		ExecutionBackendConfig: sessionexecution.ExecutionBackendConfig{
			Provider:    string(fse.ExecutionProviderJavaScriptRuntime),
			ProjectRoot: projectRoot,
		},
		JSON:   true,
		Output: &runOutput,
	})
	if err == nil || !strings.Contains(err.Error(), "durable execution service is required") {
		t.Fatalf("RunSync error = %v, want missing injected service", err)
	}
}

func TestRunSync_JavaScriptRuntimeFakeChildCLIInspectionRegression(t *testing.T) {
	projectRoot := setupCLIAgentRunWorkflowFixture(t)
	service, err := fse.NewExecutionService(
		fse.ExecutionProviderJavaScriptRuntime,
		fse.ServiceConfig{
			ProjectRoot: projectRoot, Persistence: fse.DisabledPersistence(), Clock: factory.EnsureClock(nil),
		},
	)
	if err != nil {
		t.Fatalf("NewExecutionService: %v", err)
	}
	backend := sessionexecution.ExecutionBackendConfig{
		Provider:    string(fse.ExecutionProviderJavaScriptRuntime),
		ProjectRoot: projectRoot,
	}

	var runOutput bytes.Buffer
	if err := sessionexecution.RunSync(context.Background(), sessionexecution.RunConfig{
		StartConfig: sessionexecution.StartConfig{
			Mode:         sessionexecution.ExecutionModeSync,
			RequestID:    "req-cli-fake-child-regression-001",
			WorkflowName: "agent-run-fake-child",
			ArgsJSON:     `{"subject":"workflows"}`,
		},
		ExecutionBackendConfig: backend,
		JSON:                   true,
		Output:                 &runOutput,
		Service:                service,
	}); err != nil {
		t.Fatalf("RunSync: %v", err)
	}

	var runResponse factoryapi.FactorySessionSyncExecutionResponse
	if err := json.Unmarshal(bytes.TrimSpace(runOutput.Bytes()), &runResponse); err != nil {
		t.Fatalf("decode run output: %v", err)
	}
	sessionID := runResponse.SessionId
	if sessionID == "" {
		t.Fatal("sessionId = empty, want runtime-backed durable session id")
	}

	dispatchDetail, err := service.GetDispatch(context.Background(), sessionID, "dispatch-1")
	if err != nil {
		t.Fatalf("GetDispatch: %v", err)
	}
	if dispatchDetail.JavaScript == nil || dispatchDetail.JavaScript.ExecutionMode != fse.ChildExecutorModeFake {
		t.Fatalf("dispatch javascript = %#v, want fake execution mode", dispatchDetail.JavaScript)
	}

	var dispatchesHuman bytes.Buffer
	if err := sessionexecution.RunDispatches(context.Background(), sessionexecution.DispatchesConfig{
		SessionID:              sessionID,
		ExecutionBackendConfig: backend,
		Output:                 &dispatchesHuman,
		Service:                service,
	}); err != nil {
		t.Fatalf("RunDispatches: %v", err)
	}
	dispatchText := dispatchesHuman.String()
	if strings.Contains(dispatchText, "live-provider-session-1") {
		t.Fatalf("fake-child dispatches leaked live-provider markers:\n%s", dispatchText)
	}
	for _, want := range []string{
		"dispatches (1):",
		"- dispatch-1 COMPLETED",
	} {
		if !strings.Contains(dispatchText, want) {
			t.Fatalf("dispatch human output missing %q:\n%s", want, dispatchText)
		}
	}

	var resultOutput bytes.Buffer
	if err := sessionexecution.RunResult(context.Background(), sessionexecution.ResultConfig{
		SessionID:              sessionID,
		ExecutionBackendConfig: backend,
		JSON:                   true,
		Output:                 &resultOutput,
		Service:                service,
	}); err != nil {
		t.Fatalf("RunResult: %v", err)
	}
	var resultResponse factoryapi.FactorySessionResult
	if err := json.Unmarshal(bytes.TrimSpace(resultOutput.Bytes()), &resultResponse); err != nil {
		t.Fatalf("decode result output: %v", err)
	}
	if resultResponse.SessionId != sessionID {
		t.Fatalf("result sessionId = %q, want %q", resultResponse.SessionId, sessionID)
	}
}

func TestRunSync_ExplicitFakeChildMode_OverridesLiveConfiguredServiceCLI(t *testing.T) {
	service, projectRoot := newLiveChildCLIJavaScriptRuntimeService(t)
	backend := sessionexecution.ExecutionBackendConfig{
		Provider:    string(fse.ExecutionProviderJavaScriptRuntime),
		ProjectRoot: projectRoot,
	}

	var runOutput bytes.Buffer
	if err := sessionexecution.RunSync(context.Background(), sessionexecution.RunConfig{
		StartConfig: sessionexecution.StartConfig{
			Mode:              sessionexecution.ExecutionModeSync,
			RequestID:         "req-cli-explicit-fake-child-override-001",
			WorkflowName:      "agent-run-fake-child",
			ArgsJSON:          `{"subject":"workflows"}`,
			ChildExecutorMode: fse.ChildExecutorModeFake,
		},
		ExecutionBackendConfig: backend,
		JSON:                   true,
		Output:                 &runOutput,
		Service:                service,
	}); err != nil {
		t.Fatalf("RunSync: %v", err)
	}

	var runResponse factoryapi.FactorySessionSyncExecutionResponse
	if err := json.Unmarshal(bytes.TrimSpace(runOutput.Bytes()), &runResponse); err != nil {
		t.Fatalf("decode run output: %v", err)
	}

	dispatchDetail, err := service.GetDispatch(context.Background(), runResponse.SessionId, "dispatch-1")
	if err != nil {
		t.Fatalf("GetDispatch: %v", err)
	}
	if dispatchDetail.JavaScript == nil || dispatchDetail.JavaScript.ExecutionMode != fse.ChildExecutorModeFake {
		t.Fatalf("dispatch javascript = %#v, want fake execution mode override", dispatchDetail.JavaScript)
	}

	var dispatchesHuman bytes.Buffer
	if err := sessionexecution.RunDispatches(context.Background(), sessionexecution.DispatchesConfig{
		SessionID:              runResponse.SessionId,
		ExecutionBackendConfig: backend,
		Output:                 &dispatchesHuman,
		Service:                service,
	}); err != nil {
		t.Fatalf("RunDispatches: %v", err)
	}
	if strings.Contains(dispatchesHuman.String(), "live-provider-session-1") {
		t.Fatalf("explicit fake override leaked live-provider markers:\n%s", dispatchesHuman.String())
	}
}

func liveProviderJavaScriptBackend(projectRoot string) sessionexecution.ExecutionBackendConfig {
	return sessionexecution.ExecutionBackendConfig{
		Provider:    string(fse.ExecutionProviderJavaScriptRuntime),
		ProjectRoot: projectRoot,
	}
}

func runLiveProviderCLIJSONSync(
	t *testing.T,
	service fse.Service,
	backend sessionexecution.ExecutionBackendConfig,
	requestID string,
) factoryapi.FactorySessionSyncExecutionResponse {
	t.Helper()
	var runOutput bytes.Buffer
	if err := sessionexecution.RunSync(context.Background(), sessionexecution.RunConfig{
		StartConfig: sessionexecution.StartConfig{
			Mode:              sessionexecution.ExecutionModeSync,
			RequestID:         requestID,
			WorkflowName:      "agent-run-fake-child",
			ArgsJSON:          `{"subject":"workflows"}`,
			ChildExecutorMode: fse.ChildExecutorModeLive,
		},
		ExecutionBackendConfig: backend,
		JSON:                   true,
		Output:                 &runOutput,
		Service:                service,
	}); err != nil {
		t.Fatalf("RunSync: %v", err)
	}
	var runResponse factoryapi.FactorySessionSyncExecutionResponse
	if err := json.Unmarshal(bytes.TrimSpace(runOutput.Bytes()), &runResponse); err != nil {
		t.Fatalf("decode run output: %v", err)
	}
	return runResponse
}

func assertLiveProviderSucceededRunResponse(t *testing.T, runResponse factoryapi.FactorySessionSyncExecutionResponse) {
	t.Helper()
	if runResponse.SessionId == "" {
		t.Fatalf("sessionId = %q, want non-empty durable session id", runResponse.SessionId)
	}
	if runResponse.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("status = %q, want SUCCEEDED", runResponse.Status)
	}
	if runResponse.SyncOutcome != factoryapi.FactorySessionSyncExecutionOutcomeCompleted {
		t.Fatalf("syncOutcome = %q, want COMPLETED", runResponse.SyncOutcome)
	}
}

func assertLiveProviderCLIStatusMatchesRun(
	t *testing.T,
	service fse.Service,
	backend sessionexecution.ExecutionBackendConfig,
	sessionID string,
) {
	t.Helper()
	var statusOutput bytes.Buffer
	if err := sessionexecution.RunStatus(context.Background(), sessionexecution.StatusConfig{
		SessionID:              sessionID,
		ExecutionBackendConfig: backend,
		JSON:                   true,
		Output:                 &statusOutput,
		Service:                service,
	}); err != nil {
		t.Fatalf("RunStatus: %v", err)
	}
	var statusResponse factoryapi.FactorySessionDurableReadModel
	if err := json.Unmarshal(bytes.TrimSpace(statusOutput.Bytes()), &statusResponse); err != nil {
		t.Fatalf("decode status output: %v", err)
	}
	if statusResponse.SessionId != sessionID {
		t.Fatalf("status sessionId = %q, want %q", statusResponse.SessionId, sessionID)
	}
	if statusResponse.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("status read = %q, want SUCCEEDED", statusResponse.Status)
	}
}

func assertLiveProviderCLIResultMatchesRun(
	t *testing.T,
	service fse.Service,
	backend sessionexecution.ExecutionBackendConfig,
	sessionID string,
) {
	t.Helper()
	var resultOutput bytes.Buffer
	if err := sessionexecution.RunResult(context.Background(), sessionexecution.ResultConfig{
		SessionID:              sessionID,
		ExecutionBackendConfig: backend,
		JSON:                   true,
		Output:                 &resultOutput,
		Service:                service,
	}); err != nil {
		t.Fatalf("RunResult: %v", err)
	}
	var resultResponse factoryapi.FactorySessionResult
	if err := json.Unmarshal(bytes.TrimSpace(resultOutput.Bytes()), &resultResponse); err != nil {
		t.Fatalf("decode result output: %v", err)
	}
	if resultResponse.SessionId != sessionID {
		t.Fatalf("result sessionId = %q, want %q", resultResponse.SessionId, sessionID)
	}
	if resultResponse.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("resultStatus = %q, want FINAL", resultResponse.ResultStatus)
	}
	if resultResponse.SessionStatus == nil || *resultResponse.SessionStatus != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("sessionStatus = %#v, want SUCCEEDED", resultResponse.SessionStatus)
	}
}

func assertLiveProviderDispatchExecutionMode(t *testing.T, service fse.Service, sessionID string) {
	t.Helper()
	dispatchDetail, err := service.GetDispatch(context.Background(), sessionID, "dispatch-1")
	if err != nil {
		t.Fatalf("GetDispatch: %v", err)
	}
	if dispatchDetail.JavaScript == nil || dispatchDetail.JavaScript.ExecutionMode != "live-provider" {
		t.Fatalf("dispatch javascript = %#v, want live-provider execution mode", dispatchDetail.JavaScript)
	}
}

func newLiveChildCLIJavaScriptRuntimeService(t *testing.T) (fse.Service, string) {
	t.Helper()
	projectRoot := setupCLIAgentRunWorkflowFixture(t)
	service, err := fse.NewExecutionService(
		fse.ExecutionProviderJavaScriptRuntime,
		fse.ServiceConfig{
			ProjectRoot:       projectRoot,
			ChildExecutorMode: fse.ChildExecutorModeLive,
			Provider:          fse.SmokeLiveChildProvider(),
			Persistence:       fse.DisabledPersistence(),
			Clock:             factory.EnsureClock(nil),
		},
	)
	if err != nil {
		t.Fatalf("NewExecutionService: %v", err)
	}
	return service, projectRoot
}

func setupCLIAgentRunWorkflowFixture(t *testing.T) string {
	t.Helper()
	projectRoot := t.TempDir()
	workflowDir := filepath.Join(projectRoot, ".claude", "workflows")
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}
	sourcePath := filepath.Join("..", "..", "..", "orchestrators", "javascript", "runtime", "testdata", "agent-run-fake-child.workflow.js")
	source, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read workflow fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workflowDir, "agent-run-fake-child.js"), source, 0o600); err != nil {
		t.Fatalf("write workflow fixture: %v", err)
	}
	return projectRoot
}

func runLiveProviderSmokeSession(
	t *testing.T,
	service fse.Service,
	backend sessionexecution.ExecutionBackendConfig,
	requestID string,
) string {
	t.Helper()
	var runOutput bytes.Buffer
	if err := sessionexecution.RunSync(context.Background(), sessionexecution.RunConfig{
		StartConfig: sessionexecution.StartConfig{
			Mode:              sessionexecution.ExecutionModeSync,
			RequestID:         requestID,
			WorkflowName:      "agent-run-fake-child",
			ArgsJSON:          `{"subject":"workflows"}`,
			ChildExecutorMode: fse.ChildExecutorModeLive,
		},
		ExecutionBackendConfig: backend,
		JSON:                   true,
		Output:                 &runOutput,
		Service:                service,
	}); err != nil {
		t.Fatalf("RunSync: %v", err)
	}
	var runResponse factoryapi.FactorySessionSyncExecutionResponse
	if err := json.Unmarshal(bytes.TrimSpace(runOutput.Bytes()), &runResponse); err != nil {
		t.Fatalf("decode run output: %v", err)
	}
	if runResponse.SessionId == "" {
		t.Fatal("sessionId = empty, want durable session id")
	}
	return runResponse.SessionId
}

func assertLiveProviderDispatchCLIOutput(
	t *testing.T,
	service fse.Service,
	sessionID string,
	backend sessionexecution.ExecutionBackendConfig,
) {
	t.Helper()
	dispatchText := runLiveProviderDispatchesHuman(t, service, sessionID, backend)
	assertLiveProviderDispatchHumanMarkers(t, dispatchText)
	dispatchesJSON := runLiveProviderDispatchesJSON(t, service, sessionID, backend)
	assertLiveProviderDispatchJSONFields(t, sessionID, dispatchesJSON)
	assertCLIDispatchesMatchProjection(t, service, sessionID, dispatchesJSON)
}

func runLiveProviderDispatchesHuman(
	t *testing.T,
	service fse.Service,
	sessionID string,
	backend sessionexecution.ExecutionBackendConfig,
) string {
	t.Helper()
	var dispatchesHuman bytes.Buffer
	if err := sessionexecution.RunDispatches(context.Background(), sessionexecution.DispatchesConfig{
		SessionID:              sessionID,
		ExecutionBackendConfig: backend,
		Output:                 &dispatchesHuman,
		Service:                service,
	}); err != nil {
		t.Fatalf("RunDispatches human: %v", err)
	}
	return dispatchesHuman.String()
}

func assertLiveProviderDispatchHumanMarkers(t *testing.T, dispatchText string) {
	t.Helper()
	for _, want := range []string{
		"dispatches (1):",
		"- dispatch-1 COMPLETED",
		"provider=mock",
		"provider session: live-provider-session-1",
		"artifacts=child-artifact-1",
	} {
		if !strings.Contains(dispatchText, want) {
			t.Fatalf("dispatch human output missing %q:\n%s", want, dispatchText)
		}
	}
}

func runLiveProviderDispatchesJSON(
	t *testing.T,
	service fse.Service,
	sessionID string,
	backend sessionexecution.ExecutionBackendConfig,
) []byte {
	t.Helper()
	var dispatchesJSON bytes.Buffer
	if err := sessionexecution.RunDispatches(context.Background(), sessionexecution.DispatchesConfig{
		SessionID:              sessionID,
		ExecutionBackendConfig: backend,
		JSON:                   true,
		Output:                 &dispatchesJSON,
		Service:                service,
	}); err != nil {
		t.Fatalf("RunDispatches json: %v", err)
	}
	return bytes.TrimSpace(dispatchesJSON.Bytes())
}

func assertLiveProviderDispatchJSONFields(t *testing.T, sessionID string, dispatchesJSON []byte) {
	t.Helper()
	var dispatchList factoryapi.ListFactorySessionDispatchesResponse
	if err := json.Unmarshal(dispatchesJSON, &dispatchList); err != nil {
		t.Fatalf("decode dispatches json: %v", err)
	}
	if dispatchList.SessionId != sessionID {
		t.Fatalf("dispatch sessionId = %q, want %q", dispatchList.SessionId, sessionID)
	}
	if dispatchList.Dispatches == nil || len(dispatchList.Dispatches) != 1 {
		t.Fatalf("dispatches = %#v, want one dispatch", dispatchList.Dispatches)
	}
	dispatch := dispatchList.Dispatches[0]
	if dispatch.Id != "dispatch-1" {
		t.Fatalf("dispatch id = %q, want dispatch-1", dispatch.Id)
	}
	if dispatch.Provider == nil || *dispatch.Provider != "mock" {
		t.Fatalf("dispatch provider = %#v, want mock", dispatch.Provider)
	}
	if dispatch.ProviderSessionRefs == nil || len(*dispatch.ProviderSessionRefs) != 1 ||
		(*dispatch.ProviderSessionRefs)[0].Id != "live-provider-session-1" {
		t.Fatalf("providerSessionRefs = %#v", dispatch.ProviderSessionRefs)
	}
	if dispatch.OutputArtifactIds == nil || len(*dispatch.OutputArtifactIds) != 1 ||
		(*dispatch.OutputArtifactIds)[0] != "child-artifact-1" {
		t.Fatalf("outputArtifactIds = %#v, want [child-artifact-1]", dispatch.OutputArtifactIds)
	}
}

func assertCLIDispatchesMatchProjection(t *testing.T, service fse.Service, sessionID string, dispatchesJSON []byte) {
	t.Helper()
	listed, err := service.ListDispatches(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("ListDispatches: %v", err)
	}
	wantDispatchJSON, err := json.Marshal(factorysession.ListDispatchesResponseToAPI(listed))
	if err != nil {
		t.Fatalf("marshal shared projection: %v", err)
	}
	if !bytes.Equal(dispatchesJSON, wantDispatchJSON) {
		t.Fatalf("CLI dispatches JSON diverged from shared ListDispatchesResponseToAPI projection")
	}
}

func assertLiveProviderArtifactCLIOutput(
	t *testing.T,
	service fse.Service,
	sessionID string,
	backend sessionexecution.ExecutionBackendConfig,
) {
	t.Helper()
	var artifactsHuman bytes.Buffer
	if err := sessionexecution.RunArtifacts(context.Background(), sessionexecution.ArtifactsConfig{
		SessionID:              sessionID,
		ExecutionBackendConfig: backend,
		Output:                 &artifactsHuman,
		Service:                service,
	}); err != nil {
		t.Fatalf("RunArtifacts human: %v", err)
	}
	artifactText := artifactsHuman.String()
	wantArtifactHref := "/factory-sessions/" + sessionID + "/artifacts/child-artifact-1"
	for _, want := range []string{
		"artifacts (1):",
		"- child-artifact-1",
		"dispatch=dispatch-1",
		wantArtifactHref,
	} {
		if !strings.Contains(artifactText, want) {
			t.Fatalf("artifact human output missing %q:\n%s", want, artifactText)
		}
	}

	var artifactsJSON bytes.Buffer
	if err := sessionexecution.RunArtifacts(context.Background(), sessionexecution.ArtifactsConfig{
		SessionID:              sessionID,
		ExecutionBackendConfig: backend,
		JSON:                   true,
		Output:                 &artifactsJSON,
		Service:                service,
	}); err != nil {
		t.Fatalf("RunArtifacts json: %v", err)
	}

	var artifactList factoryapi.ListFactorySessionArtifactsResponse
	if err := json.Unmarshal(bytes.TrimSpace(artifactsJSON.Bytes()), &artifactList); err != nil {
		t.Fatalf("decode artifacts json: %v", err)
	}
	if artifactList.SessionId != sessionID {
		t.Fatalf("artifact sessionId = %q, want %q", artifactList.SessionId, sessionID)
	}
	if artifactList.Artifacts == nil || len(artifactList.Artifacts) != 1 {
		t.Fatalf("artifacts = %#v, want one artifact", artifactList.Artifacts)
	}
	artifact := artifactList.Artifacts[0]
	if artifact.Id != "child-artifact-1" {
		t.Fatalf("artifact id = %q, want child-artifact-1", artifact.Id)
	}
	if artifact.DispatchId == nil || *artifact.DispatchId != "dispatch-1" {
		t.Fatalf("artifact dispatchId = %#v, want dispatch-1", artifact.DispatchId)
	}

	listedArtifacts, err := service.ListArtifacts(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("ListArtifacts: %v", err)
	}
	wantArtifactJSON, err := json.Marshal(factorysession.ListArtifactsResponseToAPI(listedArtifacts))
	if err != nil {
		t.Fatalf("marshal shared artifact projection: %v", err)
	}
	if !bytes.Equal(bytes.TrimSpace(artifactsJSON.Bytes()), wantArtifactJSON) {
		t.Fatalf("CLI artifacts JSON diverged from shared ListArtifactsResponseToAPI projection")
	}
}
