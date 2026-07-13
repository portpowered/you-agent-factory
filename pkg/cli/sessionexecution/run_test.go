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
	fse "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/execution/fixtures"
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
