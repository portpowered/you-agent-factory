package sessionexecution_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface/factorysession"
	"github.com/portpowered/infinite-you/pkg/cli/sessionexecution"
	fse "github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution/fixtures"
)

func TestRunSync_SuccessFixtureHumanOutput(t *testing.T) {
	service := newContractFakeService(t)
	var output bytes.Buffer
	err := sessionexecution.RunSync(context.Background(), sessionexecution.RunConfig{
		StartConfig: sessionexecution.StartConfig{
			Mode:      sessionexecution.ExecutionModeSync,
			RequestID: "req-petri-success-001",
			FactoryID: "customer-support-triage",
		},
		Output:  &output,
		Service: service,
	})
	if err != nil {
		t.Fatalf("RunSync: %v", err)
	}

	text := output.String()
	for _, want := range []string{
		"Factory session dur-sess-petri-success-001 completed (SUCCEEDED).",
		"Source hash: sha256:petri-factory-001",
		"Primary result: Ticket triaged and resolved.",
		"Session link: /factory-sessions/dur-sess-petri-success-001",
		"Results link: /factory-sessions/dur-sess-petri-success-001/results",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}
}

func TestRunSync_SuccessFixtureJSONOutputIsDeterministic(t *testing.T) {
	service := newContractFakeService(t)
	want := expectedSyncSuccessAPIResponse(t)

	var firstOutput bytes.Buffer
	if err := sessionexecution.RunSync(context.Background(), sessionexecution.RunConfig{
		StartConfig: sessionexecution.StartConfig{
			Mode:      sessionexecution.ExecutionModeSync,
			RequestID: "req-petri-success-001",
			FactoryID: "customer-support-triage",
		},
		JSON:    true,
		Output:  &firstOutput,
		Service: service,
	}); err != nil {
		t.Fatalf("first RunSync: %v", err)
	}

	var gotResponse factoryapi.FactorySessionSyncExecutionResponse
	if err := json.Unmarshal(bytes.TrimSpace(firstOutput.Bytes()), &gotResponse); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	assertSyncSuccessAPIResponse(t, gotResponse, want)

	var secondOutput bytes.Buffer
	if err := sessionexecution.RunSync(context.Background(), sessionexecution.RunConfig{
		StartConfig: sessionexecution.StartConfig{
			Mode:      sessionexecution.ExecutionModeSync,
			RequestID: "req-petri-success-001",
			FactoryID: "customer-support-triage",
		},
		JSON:    true,
		Output:  &secondOutput,
		Service: service,
	}); err != nil {
		t.Fatalf("second RunSync: %v", err)
	}
	if !bytes.Equal(bytes.TrimSpace(firstOutput.Bytes()), bytes.TrimSpace(secondOutput.Bytes())) {
		t.Fatalf("json output is not deterministic across equivalent runs")
	}
}

func TestRunSync_ReplaysSameRequestIDWithoutNewSession(t *testing.T) {
	service := newContractFakeService(t)
	base := sessionexecution.RunConfig{
		StartConfig: sessionexecution.StartConfig{
			Mode:      sessionexecution.ExecutionModeSync,
			RequestID: "req-petri-success-001",
			FactoryID: "customer-support-triage",
		},
		JSON:    true,
		Service: service,
	}

	var firstOutput bytes.Buffer
	first := base
	first.Output = &firstOutput
	if err := sessionexecution.RunSync(context.Background(), first); err != nil {
		t.Fatalf("first RunSync: %v", err)
	}

	var secondOutput bytes.Buffer
	second := base
	second.Output = &secondOutput
	if err := sessionexecution.RunSync(context.Background(), second); err != nil {
		t.Fatalf("replay RunSync: %v", err)
	}

	if !bytes.Equal(bytes.TrimSpace(firstOutput.Bytes()), bytes.TrimSpace(secondOutput.Bytes())) {
		t.Fatalf("replay output changed:\nfirst: %s\nsecond: %s", firstOutput.Bytes(), secondOutput.Bytes())
	}
}

func TestRunSync_RejectsAsyncMode(t *testing.T) {
	service := newContractFakeService(t)
	var output bytes.Buffer
	err := sessionexecution.RunSync(context.Background(), sessionexecution.RunConfig{
		StartConfig: sessionexecution.StartConfig{
			Mode:      sessionexecution.ExecutionModeAsync,
			RequestID: "req-petri-success-001",
			FactoryID: "customer-support-triage",
		},
		Output:  &output,
		Service: service,
	})
	if err == nil {
		t.Fatal("RunSync = nil, want unsupported mode error")
	}
	if !strings.Contains(output.String(), sessionexecution.ErrorCodeUnsupportedMode) {
		t.Fatalf("output = %q, want unsupported mode code", output.String())
	}
}

func assertSyncSuccessAPIResponse(
	t *testing.T,
	got, want factoryapi.FactorySessionSyncExecutionResponse,
) {
	t.Helper()
	assertSyncSuccessIdentity(t, got, want)
	assertSyncSuccessResultAndLinks(t, got, want)
}

func assertSyncSuccessIdentity(
	t *testing.T,
	got, want factoryapi.FactorySessionSyncExecutionResponse,
) {
	t.Helper()
	if got.SessionId != want.SessionId {
		t.Fatalf("sessionId = %q, want %q", got.SessionId, want.SessionId)
	}
	if got.Status != want.Status {
		t.Fatalf("status = %q, want %q", got.Status, want.Status)
	}
	if got.SyncOutcome != want.SyncOutcome {
		t.Fatalf("syncOutcome = %q, want %q", got.SyncOutcome, want.SyncOutcome)
	}
	if got.SourceHash == nil || want.SourceHash == nil || *got.SourceHash != *want.SourceHash {
		t.Fatalf("sourceHash = %#v, want %#v", got.SourceHash, want.SourceHash)
	}
}

func assertSyncSuccessResultAndLinks(
	t *testing.T,
	got, want factoryapi.FactorySessionSyncExecutionResponse,
) {
	t.Helper()
	if got.Result == nil || want.Result == nil {
		t.Fatalf("result = %#v, want %#v", got.Result, want.Result)
	}
	if got.Result.ResultStatus != want.Result.ResultStatus {
		t.Fatalf("resultStatus = %q, want %q", got.Result.ResultStatus, want.Result.ResultStatus)
	}
	if got.Links == nil || want.Links == nil {
		t.Fatalf("links = %#v, want %#v", got.Links, want.Links)
	}
	if got.Links.Session == nil || want.Links.Session == nil || *got.Links.Session != *want.Links.Session {
		t.Fatalf("session link = %#v, want %#v", got.Links.Session, want.Links.Session)
	}
	if got.Links.Results == nil || want.Links.Results == nil || *got.Links.Results != *want.Links.Results {
		t.Fatalf("results link = %#v, want %#v", got.Links.Results, want.Links.Results)
	}
	if primaryResultText(got.Result) != primaryResultText(want.Result) {
		t.Fatalf("primaryResult text = %q, want %q", primaryResultText(got.Result), primaryResultText(want.Result))
	}
}

func primaryResultText(result *factoryapi.FactorySessionResult) string {
	if result == nil || result.PrimaryResult == nil {
		return ""
	}
	for _, part := range *result.PrimaryResult {
		textPart, err := part.AsWorkTextContentPart()
		if err != nil {
			continue
		}
		if trimmed := strings.TrimSpace(textPart.Text); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func expectedSyncSuccessAPIResponse(t *testing.T) factoryapi.FactorySessionSyncExecutionResponse {
	t.Helper()
	raw, err := os.ReadFile(contractFixtureCatalogPath(t))
	if err != nil {
		t.Fatalf("read fixture catalog: %v", err)
	}
	var catalog struct {
		Scenarios []struct {
			ID           string          `json:"id"`
			SyncResponse json.RawMessage `json:"syncResponse"`
		} `json:"scenarios"`
	}
	if err := json.Unmarshal(raw, &catalog); err != nil {
		t.Fatalf("decode fixture catalog: %v", err)
	}
	for _, scenario := range catalog.Scenarios {
		if scenario.ID != fixtures.FixtureScenarioSyncSuccess {
			continue
		}
		var response factoryapi.FactorySessionSyncExecutionResponse
		if err := json.Unmarshal(scenario.SyncResponse, &response); err != nil {
			t.Fatalf("decode sync response fixture: %v", err)
		}
		return response
	}
	t.Fatalf("missing sync response fixture for %q", fixtures.FixtureScenarioSyncSuccess)
	return factoryapi.FactorySessionSyncExecutionResponse{}
}

func newContractFakeService(t *testing.T) fse.Service {
	t.Helper()
	service, err := fse.NewFakeServiceFromContractFixtures(contractFixtureCatalogPath(t))
	if err != nil {
		t.Fatalf("NewFakeServiceFromContractFixtures: %v", err)
	}
	return service
}

func contractFixtureCatalogPath(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "..", "api", "testdata", "durable-session-contract-fixtures.json")
}

func TestRunSync_TimeoutFixtureHumanOutput(t *testing.T) {
	service := newContractFakeService(t)
	timeoutMillis := int64(30000)
	var output bytes.Buffer
	if err := sessionexecution.RunSync(context.Background(), sessionexecution.RunConfig{
		StartConfig: sessionexecution.StartConfig{
			Mode:              sessionexecution.ExecutionModeSync,
			RequestID:         "req-js-timeout-001",
			WorkflowName:      "long-running-audit",
			WaitTimeoutMillis: &timeoutMillis,
		},
		Output:  &output,
		Service: service,
	}); err != nil {
		t.Fatalf("RunSync: %v", err)
	}

	text := output.String()
	for _, want := range []string{
		"Factory session dur-sess-js-timeout-001 timed out (RUNNING).",
		"Timed out: true",
		"Source ref: workflow/long-running-audit",
		"Status link: /factory-sessions/dur-sess-js-timeout-001",
		"Session link: /factory-sessions/dur-sess-js-timeout-001",
		"Follow-up: you workflow status dur-sess-js-timeout-001",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}
	for _, absent := range []string{
		"completed (SUCCEEDED)",
		"Primary result:",
		"Cancel on timeout:",
		"Session canceled by timeout:",
	} {
		if strings.Contains(text, absent) {
			t.Fatalf("output should not contain %q:\n%s", absent, text)
		}
	}
}

func TestRunSync_TimeoutFixtureJSONOutputIsDeterministic(t *testing.T) {
	service := newContractFakeService(t)
	timeoutMillis := int64(30000)
	base := sessionexecution.RunConfig{
		StartConfig: sessionexecution.StartConfig{
			Mode:              sessionexecution.ExecutionModeSync,
			RequestID:         "req-js-timeout-001",
			WorkflowName:      "long-running-audit",
			WaitTimeoutMillis: &timeoutMillis,
		},
		JSON:    true,
		Service: service,
	}

	var firstOutput bytes.Buffer
	first := base
	first.Output = &firstOutput
	if err := sessionexecution.RunSync(context.Background(), first); err != nil {
		t.Fatalf("first RunSync: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(firstOutput.Bytes()), &payload); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if payload["sessionId"] != "dur-sess-js-timeout-001" {
		t.Fatalf("sessionId = %#v", payload["sessionId"])
	}
	if payload["status"] != "RUNNING" {
		t.Fatalf("status = %#v, want RUNNING", payload["status"])
	}
	if payload["syncOutcome"] != "TIMED_OUT" {
		t.Fatalf("syncOutcome = %#v, want TIMED_OUT", payload["syncOutcome"])
	}
	if payload["timedOut"] != true {
		t.Fatalf("timedOut = %#v, want true", payload["timedOut"])
	}
	if payload["requestId"] != "req-js-timeout-001" {
		t.Fatalf("requestId = %#v", payload["requestId"])
	}
	if payload["resultAvailability"] != "NOT_READY" {
		t.Fatalf("resultAvailability = %#v, want NOT_READY", payload["resultAvailability"])
	}
	if payload["result"] != nil {
		t.Fatalf("result = %#v, want nil for timeout", payload["result"])
	}
	if payload["cancelOnTimeout"] == true {
		t.Fatalf("cancelOnTimeout = %#v, want false or absent", payload["cancelOnTimeout"])
	}
	links, ok := payload["links"].(map[string]any)
	if !ok {
		t.Fatalf("links = %#v", payload["links"])
	}
	if links["status"] != "/factory-sessions/dur-sess-js-timeout-001" {
		t.Fatalf("status link = %#v", links["status"])
	}

	var secondOutput bytes.Buffer
	second := base
	second.Output = &secondOutput
	if err := sessionexecution.RunSync(context.Background(), second); err != nil {
		t.Fatalf("second RunSync: %v", err)
	}
	if !bytes.Equal(bytes.TrimSpace(firstOutput.Bytes()), bytes.TrimSpace(secondOutput.Bytes())) {
		t.Fatalf("json output is not deterministic across equivalent timeout runs")
	}
}

func TestRunSync_TimeoutWithCancelOnTimeoutHumanAndJSON(t *testing.T) {
	service := newContractFakeService(t)
	timeoutMillis := int64(30000)
	base := sessionexecution.RunConfig{
		StartConfig: sessionexecution.StartConfig{
			Mode:              sessionexecution.ExecutionModeSync,
			RequestID:         "req-js-timeout-001",
			WorkflowName:      "long-running-audit",
			WaitTimeoutMillis: &timeoutMillis,
			CancelOnTimeout:   true,
		},
		Service: service,
	}

	var humanOutput bytes.Buffer
	human := base
	human.Output = &humanOutput
	if err := sessionexecution.RunSync(context.Background(), human); err != nil {
		t.Fatalf("RunSync human: %v", err)
	}
	text := humanOutput.String()
	for _, want := range []string{
		"Factory session dur-sess-js-timeout-001 timed out (CANCELING).",
		"Cancel on timeout: requested",
		"Session canceled by timeout: true",
		"Timed out: true",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}

	var jsonOutput bytes.Buffer
	jsonRun := base
	jsonRun.JSON = true
	jsonRun.Output = &jsonOutput
	if err := sessionexecution.RunSync(context.Background(), jsonRun); err != nil {
		t.Fatalf("RunSync json: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(jsonOutput.Bytes()), &payload); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if payload["sessionCanceledByTimeout"] != true {
		t.Fatalf("sessionCanceledByTimeout = %#v, want true", payload["sessionCanceledByTimeout"])
	}
	if payload["cancelOnTimeout"] != true {
		t.Fatalf("cancelOnTimeout = %#v, want true", payload["cancelOnTimeout"])
	}
	if payload["status"] != "CANCELING" {
		t.Fatalf("status = %#v, want CANCELING", payload["status"])
	}
	if payload["resultAvailability"] != "UNAVAILABLE" {
		t.Fatalf("resultAvailability = %#v, want UNAVAILABLE", payload["resultAvailability"])
	}
}

func TestRunSync_TimeoutSessionInspectableViaStatusAndResult(t *testing.T) {
	service := newContractFakeService(t)
	seedTimeoutSession(t, service, false)

	var statusOutput bytes.Buffer
	if err := sessionexecution.RunStatus(context.Background(), sessionexecution.StatusConfig{
		SessionID: "dur-sess-js-timeout-001",
		Output:    &statusOutput,
		Service:   service,
	}); err != nil {
		t.Fatalf("RunStatus: %v", err)
	}
	if !strings.Contains(statusOutput.String(), "Factory session dur-sess-js-timeout-001 is RUNNING.") {
		t.Fatalf("status output = %q, want running session", statusOutput.String())
	}

	var resultOutput bytes.Buffer
	err := sessionexecution.RunResult(context.Background(), sessionexecution.ResultConfig{
		SessionID: "dur-sess-js-timeout-001",
		Output:    &resultOutput,
		Service:   service,
	})
	var outcome *sessionexecution.ResultOutcomeError
	if !errors.As(err, &outcome) {
		t.Fatalf("RunResult error = %v, want ResultOutcomeError", err)
	}
	if outcome.Status != factoryapi.FactorySessionResultStatusNotReady {
		t.Fatalf("outcome status = %q, want NOT_READY", outcome.Status)
	}
	if !strings.Contains(resultOutput.String(), "Availability reason: SYNC_WAIT_TIMED_OUT") {
		t.Fatalf("result output = %q, want SYNC_WAIT_TIMED_OUT availability", resultOutput.String())
	}
}

func TestRunSync_TimeoutWithCancelOnTimeoutSessionInspectable(t *testing.T) {
	service := newContractFakeService(t)
	seedTimeoutSession(t, service, true)

	var statusOutput bytes.Buffer
	if err := sessionexecution.RunStatus(context.Background(), sessionexecution.StatusConfig{
		SessionID: "dur-sess-js-timeout-001",
		Output:    &statusOutput,
		Service:   service,
	}); err != nil {
		t.Fatalf("RunStatus: %v", err)
	}
	if !strings.Contains(statusOutput.String(), "Factory session dur-sess-js-timeout-001 is CANCELING.") {
		t.Fatalf("status output = %q, want canceling session", statusOutput.String())
	}

	var resultOutput bytes.Buffer
	err := sessionexecution.RunResult(context.Background(), sessionexecution.ResultConfig{
		SessionID: "dur-sess-js-timeout-001",
		Output:    &resultOutput,
		Service:   service,
	})
	var outcome *sessionexecution.ResultOutcomeError
	if !errors.As(err, &outcome) {
		t.Fatalf("RunResult error = %v, want ResultOutcomeError", err)
	}
	if outcome.Status != factoryapi.FactorySessionResultStatusUnavailable {
		t.Fatalf("outcome status = %q, want UNAVAILABLE", outcome.Status)
	}
	if !strings.Contains(resultOutput.String(), "Availability reason: SESSION_CANCELED") {
		t.Fatalf("result output = %q, want SESSION_CANCELED availability", resultOutput.String())
	}
}

func seedTimeoutSession(t *testing.T, service fse.Service, cancelOnTimeout bool) {
	t.Helper()
	timeoutMillis := int64(30000)
	if err := sessionexecution.RunSync(context.Background(), sessionexecution.RunConfig{
		StartConfig: sessionexecution.StartConfig{
			Mode:              sessionexecution.ExecutionModeSync,
			RequestID:         "req-js-timeout-001",
			WorkflowName:      "long-running-audit",
			WaitTimeoutMillis: &timeoutMillis,
			CancelOnTimeout:   cancelOnTimeout,
		},
		Output:  ioDiscard{},
		Service: service,
	}); err != nil {
		t.Fatalf("seed RunSync: %v", err)
	}
}

func TestRunSync_UsesSharedServicePrimaryResultProjection(t *testing.T) {
	service := newContractFakeService(t)
	normalized, _, err := sessionexecution.NormalizeStartRequest(sessionexecution.StartConfig{
		Mode:      sessionexecution.ExecutionModeSync,
		RequestID: "req-petri-success-001",
		FactoryID: "customer-support-triage",
	})
	if err != nil {
		t.Fatalf("NormalizeStartRequest: %v", err)
	}

	var output bytes.Buffer
	if err := sessionexecution.RunSync(context.Background(), sessionexecution.RunConfig{
		StartConfig: sessionexecution.StartConfig{
			Mode:      sessionexecution.ExecutionModeSync,
			RequestID: "req-petri-success-001",
			FactoryID: "customer-support-triage",
		},
		JSON:    true,
		Output:  &output,
		Service: service,
	}); err != nil {
		t.Fatalf("RunSync: %v", err)
	}

	var mapped factoryapi.FactorySessionSyncExecutionResponse
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &mapped); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if mapped.Result == nil || mapped.Result.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("result = %#v, want FINAL primary result from shared sync start projection", mapped.Result)
	}

	direct, err := service.StartSync(context.Background(), normalized)
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}
	want := factorysession.SyncStartResponseToAPI(direct)
	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal direct projection: %v", err)
	}
	if !bytes.Equal(bytes.TrimSpace(output.Bytes()), wantJSON) {
		t.Fatalf("CLI output diverged from shared SyncStartResponseToAPI projection")
	}
}

// TestRunSync_LiveProviderJavaScriptSession_ReReadStatusAndResult proves CLI
// live-dispatch smoke through the shared execution-service path only. MCP host
// setup and website inspection are deferred follow-up cells (see
// follow-up-cell-cli-live-dispatch-smoke-deferred.md).
func TestRunSync_LiveProviderJavaScriptSession_ReReadStatusAndResult(t *testing.T) {
	service, projectRoot := newLiveChildCLIJavaScriptRuntimeService(t)

	var runOutput bytes.Buffer
	if err := sessionexecution.RunSync(context.Background(), sessionexecution.RunConfig{
		StartConfig: sessionexecution.StartConfig{
			Mode:              sessionexecution.ExecutionModeSync,
			RequestID:         "req-cli-live-child-smoke-001",
			WorkflowName:      "agent-run-fake-child",
			ArgsJSON:          `{"subject":"workflows"}`,
			ChildExecutorMode: fse.ChildExecutorModeLive,
		},
		ExecutionBackendConfig: sessionexecution.ExecutionBackendConfig{
			Provider:    string(fse.ExecutionProviderJavaScriptRuntime),
			ProjectRoot: projectRoot,
		},
		JSON:    true,
		Output:  &runOutput,
		Service: service,
	}); err != nil {
		t.Fatalf("RunSync: %v", err)
	}

	var runResponse factoryapi.FactorySessionSyncExecutionResponse
	if err := json.Unmarshal(bytes.TrimSpace(runOutput.Bytes()), &runResponse); err != nil {
		t.Fatalf("decode run output: %v", err)
	}
	if runResponse.SessionId == "" {
		t.Fatalf("sessionId = %q, want non-empty durable session id", runResponse.SessionId)
	}
	if runResponse.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("status = %q, want SUCCEEDED", runResponse.Status)
	}
	if runResponse.SyncOutcome != factoryapi.FactorySessionSyncExecutionOutcomeCompleted {
		t.Fatalf("syncOutcome = %q, want COMPLETED", runResponse.SyncOutcome)
	}

	var statusOutput bytes.Buffer
	if err := sessionexecution.RunStatus(context.Background(), sessionexecution.StatusConfig{
		SessionID: runResponse.SessionId,
		ExecutionBackendConfig: sessionexecution.ExecutionBackendConfig{
			Provider:    string(fse.ExecutionProviderJavaScriptRuntime),
			ProjectRoot: projectRoot,
		},
		JSON:    true,
		Output:  &statusOutput,
		Service: service,
	}); err != nil {
		t.Fatalf("RunStatus: %v", err)
	}

	var statusResponse factoryapi.FactorySessionDurableReadModel
	if err := json.Unmarshal(bytes.TrimSpace(statusOutput.Bytes()), &statusResponse); err != nil {
		t.Fatalf("decode status output: %v", err)
	}
	if statusResponse.SessionId != runResponse.SessionId {
		t.Fatalf("status sessionId = %q, want %q", statusResponse.SessionId, runResponse.SessionId)
	}
	if statusResponse.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("status read = %q, want SUCCEEDED", statusResponse.Status)
	}

	var resultOutput bytes.Buffer
	if err := sessionexecution.RunResult(context.Background(), sessionexecution.ResultConfig{
		SessionID: runResponse.SessionId,
		ExecutionBackendConfig: sessionexecution.ExecutionBackendConfig{
			Provider:    string(fse.ExecutionProviderJavaScriptRuntime),
			ProjectRoot: projectRoot,
		},
		JSON:    true,
		Output:  &resultOutput,
		Service: service,
	}); err != nil {
		t.Fatalf("RunResult: %v", err)
	}

	var resultResponse factoryapi.FactorySessionResult
	if err := json.Unmarshal(bytes.TrimSpace(resultOutput.Bytes()), &resultResponse); err != nil {
		t.Fatalf("decode result output: %v", err)
	}
	if resultResponse.SessionId != runResponse.SessionId {
		t.Fatalf("result sessionId = %q, want %q", resultResponse.SessionId, runResponse.SessionId)
	}
	if resultResponse.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("resultStatus = %q, want FINAL", resultResponse.ResultStatus)
	}
	if resultResponse.SessionStatus == nil || *resultResponse.SessionStatus != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("sessionStatus = %#v, want SUCCEEDED", resultResponse.SessionStatus)
	}

	dispatchDetail, err := service.GetDispatch(context.Background(), runResponse.SessionId, "dispatch-1")
	if err != nil {
		t.Fatalf("GetDispatch: %v", err)
	}
	if dispatchDetail.JavaScript == nil || dispatchDetail.JavaScript.ExecutionMode != "live-provider" {
		t.Fatalf("dispatch javascript = %#v, want live-provider execution mode", dispatchDetail.JavaScript)
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

	var runOutput bytes.Buffer
	if err := sessionexecution.RunSync(context.Background(), sessionexecution.RunConfig{
		StartConfig: sessionexecution.StartConfig{
			Mode:              sessionexecution.ExecutionModeSync,
			RequestID:         "req-cli-live-child-dispatch-smoke-001",
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
	sessionID := runResponse.SessionId
	if sessionID == "" {
		t.Fatal("sessionId = empty, want durable session id")
	}

	var dispatchesHuman bytes.Buffer
	if err := sessionexecution.RunDispatches(context.Background(), sessionexecution.DispatchesConfig{
		SessionID:              sessionID,
		ExecutionBackendConfig: backend,
		Output:                 &dispatchesHuman,
		Service:                service,
	}); err != nil {
		t.Fatalf("RunDispatches human: %v", err)
	}
	dispatchText := dispatchesHuman.String()
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

	var dispatchList factoryapi.ListFactorySessionDispatchesResponse
	if err := json.Unmarshal(bytes.TrimSpace(dispatchesJSON.Bytes()), &dispatchList); err != nil {
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

	listed, err := service.ListDispatches(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("ListDispatches: %v", err)
	}
	wantDispatchJSON, err := json.Marshal(factorysession.ListDispatchesResponseToAPI(listed))
	if err != nil {
		t.Fatalf("marshal shared projection: %v", err)
	}
	if !bytes.Equal(bytes.TrimSpace(dispatchesJSON.Bytes()), wantDispatchJSON) {
		t.Fatalf("CLI dispatches JSON diverged from shared ListDispatchesResponseToAPI projection")
	}

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

func TestRunSync_JavaScriptRuntimeBackend_UsesRealExecutionServiceWithoutFixtureStub(t *testing.T) {
	projectRoot := setupCLIAgentRunWorkflowFixture(t)

	var runOutput bytes.Buffer
	if err := sessionexecution.RunSync(context.Background(), sessionexecution.RunConfig{
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
	}); err != nil {
		t.Fatalf("RunSync: %v", err)
	}

	var runResponse factoryapi.FactorySessionSyncExecutionResponse
	if err := json.Unmarshal(bytes.TrimSpace(runOutput.Bytes()), &runResponse); err != nil {
		t.Fatalf("decode run output: %v", err)
	}
	if !strings.HasPrefix(runResponse.SessionId, "dur-sess-") {
		t.Fatalf("sessionId = %q, want runtime-backed durable session id", runResponse.SessionId)
	}
	if runResponse.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("status = %q, want SUCCEEDED", runResponse.Status)
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
		},
	)
	if err != nil {
		t.Fatalf("NewExecutionService: %v", err)
	}
	return service, projectRoot
}

func TestRunSync_JavaScriptRuntimeFakeChildCLIInspectionRegression(t *testing.T) {
	projectRoot := setupCLIAgentRunWorkflowFixture(t)
	service, err := fse.NewExecutionService(
		fse.ExecutionProviderJavaScriptRuntime,
		fse.ServiceConfig{ProjectRoot: projectRoot},
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

func setupCLIAgentRunWorkflowFixture(t *testing.T) string {
	t.Helper()
	projectRoot := t.TempDir()
	workflowDir := filepath.Join(projectRoot, ".claude", "workflows")
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}
	sourcePath := filepath.Join("..", "..", "orchestrators", "javascript", "runtime", "testdata", "agent-run-fake-child.workflow.js")
	source, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read workflow fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workflowDir, "agent-run-fake-child.js"), source, 0o600); err != nil {
		t.Fatalf("write workflow fixture: %v", err)
	}
	return projectRoot
}
