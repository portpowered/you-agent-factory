package sessionexecution_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/apisurface/factorysession"
	fse "github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution/fixtures"
	"github.com/portpowered/infinite-you/pkg/transports/cli/sessionexecution"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestFailurePaths_MissingSessionStatusHumanAndJSON(t *testing.T) {
	service := newContractFakeService(t)

	var humanOutput bytes.Buffer
	err := sessionexecution.RunStatus(context.Background(), sessionexecution.StatusConfig{
		SessionID: "dur-sess-missing-001",
		Output:    &humanOutput,
		Service:   service,
	})
	assertMissingSessionCLIError(t, err, humanOutput.String(), sessionexecution.ErrorCodeSessionNotFound, false)

	var jsonOutput bytes.Buffer
	err = sessionexecution.RunStatus(context.Background(), sessionexecution.StatusConfig{
		SessionID: "dur-sess-missing-001",
		JSON:      true,
		Output:    &jsonOutput,
		Service:   service,
	})
	assertMissingSessionCLIError(t, err, jsonOutput.String(), sessionexecution.ErrorCodeSessionNotFound, true)
}

func TestFailurePaths_MissingSessionResultHumanAndJSON(t *testing.T) {
	service := newContractFakeService(t)

	var humanOutput bytes.Buffer
	err := sessionexecution.RunResult(context.Background(), sessionexecution.ResultConfig{
		SessionID: "dur-sess-missing-001",
		Output:    &humanOutput,
		Service:   service,
	})
	assertMissingSessionCLIError(t, err, humanOutput.String(), sessionexecution.ErrorCodeSessionNotFound, false)

	var jsonOutput bytes.Buffer
	err = sessionexecution.RunResult(context.Background(), sessionexecution.ResultConfig{
		SessionID: "dur-sess-missing-001",
		JSON:      true,
		Output:    &jsonOutput,
		Service:   service,
	})
	assertMissingSessionCLIError(t, err, jsonOutput.String(), sessionexecution.ErrorCodeSessionNotFound, true)
}

func TestFailurePaths_UnsupportedModeRunAndStartHumanAndJSON(t *testing.T) {
	service := newContractFakeService(t)
	before := liveSessionCount(t, service)

	var runHuman bytes.Buffer
	err := sessionexecution.RunSync(context.Background(), sessionexecution.RunConfig{
		StartConfig: sessionexecution.StartConfig{
			Mode:      sessionexecution.ExecutionModeAsync,
			RequestID: "req-petri-success-001",
			FactoryID: "customer-support-triage",
		},
		Output:  &runHuman,
		Service: service,
	})
	assertCLIExecutionError(t, err, runHuman.String(), sessionexecution.ErrorCodeUnsupportedMode, "mode", false)
	if after := liveSessionCount(t, service); after != before {
		t.Fatalf("live session count = %d, want %d after unsupported run mode", after, before)
	}

	var runJSON bytes.Buffer
	err = sessionexecution.RunSync(context.Background(), sessionexecution.RunConfig{
		StartConfig: sessionexecution.StartConfig{
			Mode:      sessionexecution.ExecutionModeAsync,
			RequestID: "req-petri-success-001",
			FactoryID: "customer-support-triage",
		},
		JSON:    true,
		Output:  &runJSON,
		Service: service,
	})
	assertCLIExecutionError(t, err, runJSON.String(), sessionexecution.ErrorCodeUnsupportedMode, "mode", true)

	var startHuman bytes.Buffer
	err = sessionexecution.RunAsync(context.Background(), sessionexecution.RunConfig{
		StartConfig: sessionexecution.StartConfig{
			Mode:      sessionexecution.ExecutionModeSync,
			RequestID: "req-js-run-n-001",
			FactoryID: "customer-support-triage",
		},
		Output:  &startHuman,
		Service: service,
	})
	assertCLIExecutionError(t, err, startHuman.String(), sessionexecution.ErrorCodeUnsupportedMode, "mode", false)
	if after := liveSessionCount(t, service); after != before {
		t.Fatalf("live session count = %d, want %d after unsupported start mode", after, before)
	}

	var startJSON bytes.Buffer
	err = sessionexecution.RunAsync(context.Background(), sessionexecution.RunConfig{
		StartConfig: sessionexecution.StartConfig{
			Mode:      sessionexecution.ExecutionModeSync,
			RequestID: "req-js-run-n-001",
			FactoryID: "customer-support-triage",
		},
		JSON:    true,
		Output:  &startJSON,
		Service: service,
	})
	assertCLIExecutionError(t, err, startJSON.String(), sessionexecution.ErrorCodeUnsupportedMode, "mode", true)
}

func TestFailurePaths_BadSourceDoesNotCreateSessions(t *testing.T) {
	service := newContractFakeService(t)
	before := liveSessionCount(t, service)

	var missingSource bytes.Buffer
	err := sessionexecution.RunSync(context.Background(), sessionexecution.RunConfig{
		StartConfig: sessionexecution.StartConfig{
			Mode:      sessionexecution.ExecutionModeSync,
			RequestID: "req-001",
			StdinIsTTY: func() bool {
				return true
			},
		},
		JSON:    true,
		Output:  &missingSource,
		Service: service,
	})
	assertCLIExecutionError(t, err, missingSource.String(), sessionexecution.ErrorCodeMissingSource, "source", true)
	if after := liveSessionCount(t, service); after != before {
		t.Fatalf("live session count = %d, want %d after missing source", after, before)
	}

	var conflictingSource bytes.Buffer
	err = sessionexecution.RunAsync(context.Background(), sessionexecution.RunConfig{
		StartConfig: sessionexecution.StartConfig{
			Mode:         sessionexecution.ExecutionModeAsync,
			RequestID:    "req-001",
			FactoryID:    "customer-support-triage",
			WorkflowName: "review",
		},
		Output:  &conflictingSource,
		Service: service,
	})
	assertCLIExecutionError(t, err, conflictingSource.String(), sessionexecution.ErrorCodeSourceConflict, "source", false)
	if after := liveSessionCount(t, service); after != before {
		t.Fatalf("live session count = %d, want %d after conflicting source", after, before)
	}
}

func TestFailurePaths_NotReadyResultHumanAndJSON(t *testing.T) {
	service := newContractFakeService(t)
	seedAsyncRunningSession(t, service)

	var humanOutput bytes.Buffer
	err := sessionexecution.RunResult(context.Background(), sessionexecution.ResultConfig{
		SessionID: "dur-sess-js-run-n-001",
		Output:    &humanOutput,
		Service:   service,
	})
	var outcome *sessionexecution.ResultOutcomeError
	if !errors.As(err, &outcome) {
		t.Fatalf("RunResult error = %v, want ResultOutcomeError", err)
	}
	if outcome.Status != factoryapi.FactorySessionResultStatusNotReady {
		t.Fatalf("outcome status = %q, want NOT_READY", outcome.Status)
	}
	if !strings.Contains(humanOutput.String(), "NOT_READY") {
		t.Fatalf("human output = %q, want NOT_READY summary", humanOutput.String())
	}

	var jsonOutput bytes.Buffer
	err = sessionexecution.RunResult(context.Background(), sessionexecution.ResultConfig{
		SessionID: "dur-sess-js-run-n-001",
		JSON:      true,
		Output:    &jsonOutput,
		Service:   service,
	})
	if !errors.As(err, &outcome) {
		t.Fatalf("RunResult error = %v, want ResultOutcomeError", err)
	}
	var payload factoryapi.FactorySessionResult
	if decodeErr := json.Unmarshal(bytes.TrimSpace(jsonOutput.Bytes()), &payload); decodeErr != nil {
		t.Fatalf("decode json: %v", decodeErr)
	}
	if payload.ResultStatus != factoryapi.FactorySessionResultStatusNotReady {
		t.Fatalf("resultStatus = %q, want NOT_READY", payload.ResultStatus)
	}
	if payload.Availability == nil || payload.Availability.Reason == nil || *payload.Availability.Reason != "RESULT_NOT_READY" {
		t.Fatalf("availability = %#v, want RESULT_NOT_READY", payload.Availability)
	}
}

func TestFailurePaths_RequestIDReplayAsyncAndSync(t *testing.T) {
	service := newContractFakeService(t)

	asyncBase := sessionexecution.RunConfig{
		StartConfig: sessionexecution.StartConfig{
			Mode:         sessionexecution.ExecutionModeAsync,
			RequestID:    "req-idempotent-replay-001",
			WorkflowFile: ".claude/workflows/idempotent.yaml",
			ArgsJSON:     `{"task":"replay"}`,
			PolicyHash:   "req-policy-idempotent",
		},
		JSON:    true,
		Service: service,
	}
	var firstAsync bytes.Buffer
	first := asyncBase
	first.Output = &firstAsync
	if err := sessionexecution.RunAsync(context.Background(), first); err != nil {
		t.Fatalf("first RunAsync: %v", err)
	}
	var secondAsync bytes.Buffer
	second := asyncBase
	second.Output = &secondAsync
	if err := sessionexecution.RunAsync(context.Background(), second); err != nil {
		t.Fatalf("replay RunAsync: %v", err)
	}
	if !bytes.Equal(bytes.TrimSpace(firstAsync.Bytes()), bytes.TrimSpace(secondAsync.Bytes())) {
		t.Fatalf("async replay output changed:\nfirst: %s\nsecond: %s", firstAsync.Bytes(), secondAsync.Bytes())
	}

	syncBase := sessionexecution.RunConfig{
		StartConfig: sessionexecution.StartConfig{
			Mode:      sessionexecution.ExecutionModeSync,
			RequestID: "req-petri-success-001",
			FactoryID: "customer-support-triage",
		},
		JSON:    true,
		Service: service,
	}
	var firstSync bytes.Buffer
	firstSyncCfg := syncBase
	firstSyncCfg.Output = &firstSync
	if err := sessionexecution.RunSync(context.Background(), firstSyncCfg); err != nil {
		t.Fatalf("first RunSync: %v", err)
	}
	var secondSync bytes.Buffer
	secondSyncCfg := syncBase
	secondSyncCfg.Output = &secondSync
	if err := sessionexecution.RunSync(context.Background(), secondSyncCfg); err != nil {
		t.Fatalf("replay RunSync: %v", err)
	}
	if !bytes.Equal(bytes.TrimSpace(firstSync.Bytes()), bytes.TrimSpace(secondSync.Bytes())) {
		t.Fatalf("sync replay output changed:\nfirst: %s\nsecond: %s", firstSync.Bytes(), secondSync.Bytes())
	}
}

func TestFailurePaths_RequestIDConflictAsyncAndSyncHumanAndJSON(t *testing.T) {
	service := newContractFakeService(t)

	asyncBase := sessionexecution.StartConfig{
		Mode:         sessionexecution.ExecutionModeAsync,
		RequestID:    "req-idempotent-replay-001",
		WorkflowFile: ".claude/workflows/idempotent.yaml",
		ArgsJSON:     `{"task":"replay"}`,
		PolicyHash:   "req-policy-idempotent",
	}
	if err := sessionexecution.RunAsync(context.Background(), sessionexecution.RunConfig{
		StartConfig: asyncBase,
		Output:      ioDiscard{},
		Service:     service,
	}); err != nil {
		t.Fatalf("seed RunAsync: %v", err)
	}
	before := liveSessionCount(t, service)

	conflictAsync := asyncBase
	conflictAsync.ArgsJSON = `{"task":"different"}`
	var asyncHuman bytes.Buffer
	err := sessionexecution.RunAsync(context.Background(), sessionexecution.RunConfig{
		StartConfig: conflictAsync,
		Output:      &asyncHuman,
		Service:     service,
	})
	assertCLIExecutionError(t, err, asyncHuman.String(), sessionexecution.ErrorCodeRequestIDConflict, "requestId", false)
	if after := liveSessionCount(t, service); after != before {
		t.Fatalf("live session count = %d, want %d after async request-id conflict", after, before)
	}

	var asyncJSON bytes.Buffer
	err = sessionexecution.RunAsync(context.Background(), sessionexecution.RunConfig{
		StartConfig: conflictAsync,
		JSON:        true,
		Output:      &asyncJSON,
		Service:     service,
	})
	assertCLIExecutionError(t, err, asyncJSON.String(), sessionexecution.ErrorCodeRequestIDConflict, "requestId", true)

	syncBase := sessionexecution.StartConfig{
		Mode:      sessionexecution.ExecutionModeSync,
		RequestID: "req-petri-success-001",
		FactoryID: "customer-support-triage",
	}
	if err := sessionexecution.RunSync(context.Background(), sessionexecution.RunConfig{
		StartConfig: syncBase,
		Output:      ioDiscard{},
		Service:     service,
	}); err != nil {
		t.Fatalf("seed RunSync: %v", err)
	}
	before = liveSessionCount(t, service)

	conflictSync := syncBase
	conflictSync.FactoryID = "long-running-audit"
	var syncHuman bytes.Buffer
	err = sessionexecution.RunSync(context.Background(), sessionexecution.RunConfig{
		StartConfig: conflictSync,
		Output:      &syncHuman,
		Service:     service,
	})
	assertCLIExecutionError(t, err, syncHuman.String(), sessionexecution.ErrorCodeRequestIDConflict, "requestId", false)
	if after := liveSessionCount(t, service); after != before {
		t.Fatalf("live session count = %d, want %d after sync request-id conflict", after, before)
	}

	var syncJSON bytes.Buffer
	err = sessionexecution.RunSync(context.Background(), sessionexecution.RunConfig{
		StartConfig: conflictSync,
		JSON:        true,
		Output:      &syncJSON,
		Service:     service,
	})
	assertCLIExecutionError(t, err, syncJSON.String(), sessionexecution.ErrorCodeRequestIDConflict, "requestId", true)
}

func liveSessionCount(t *testing.T, service fse.Service) int {
	t.Helper()
	result, err := service.ListSessions(context.Background(), fse.ListSessionsRequest{
		Scope: fse.SessionListScopeLive,
	})
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	return len(result.LiveSessions)
}

func assertMissingSessionCLIError(t *testing.T, err error, output, code string, jsonOutput bool) {
	t.Helper()
	if err == nil {
		t.Fatalf("error = nil, want %s", code)
	}
	if !errors.Is(err, fse.ErrSessionNotFound) {
		t.Fatalf("error = %v, want ErrSessionNotFound", err)
	}
	assertCLIExecutionError(t, err, output, code, "sessionId", jsonOutput)
}

func assertCLIExecutionError(t *testing.T, err error, output, code, field string, jsonOutput bool) {
	t.Helper()
	if err == nil {
		t.Fatalf("error = nil, want %s", code)
	}
	if jsonOutput {
		var payload struct {
			Code    string `json:"code"`
			Message string `json:"message"`
			Field   string `json:"field"`
		}
		if decodeErr := json.Unmarshal(bytes.TrimSpace([]byte(output)), &payload); decodeErr != nil {
			t.Fatalf("decode json output %q: %v", output, decodeErr)
		}
		if payload.Code != code {
			t.Fatalf("code = %q, want %q", payload.Code, code)
		}
		if payload.Field != field {
			t.Fatalf("field = %q, want %q", payload.Field, field)
		}
		if strings.TrimSpace(payload.Message) == "" {
			t.Fatalf("message = %q, want non-empty", payload.Message)
		}
		return
	}
	if !strings.Contains(output, code) {
		t.Fatalf("output = %q, want code %q", output, code)
	}
	wantPrefix := code + ":"
	if !strings.HasPrefix(strings.TrimSpace(output), wantPrefix) {
		t.Fatalf("output = %q, want human prefix %q", output, wantPrefix)
	}
	if field != "" && !strings.Contains(output, "("+field+")") {
		t.Fatalf("output = %q, want field %q", output, field)
	}
}

func TestRunDispatches_SuccessFixtureHumanOutput(t *testing.T) {
	service := newContractFakeService(t)
	seedSyncSuccessSession(t, service)

	var output bytes.Buffer
	if err := sessionexecution.RunDispatches(context.Background(), sessionexecution.DispatchesConfig{
		SessionID: "dur-sess-petri-success-001",
		Output:    &output,
		Service:   service,
	}); err != nil {
		t.Fatalf("RunDispatches: %v", err)
	}

	text := output.String()
	for _, want := range []string{
		"Factory session dur-sess-petri-success-001 dispatches (1):",
		"- disp-petri-success-001 COMPLETED PETRI_TRANSITION",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}
}

func TestRunDispatches_AsyncPetriRunFixtureIncludesProviderSessionRefs(t *testing.T) {
	service := newContractFakeService(t)
	if err := sessionexecution.RunAsync(context.Background(), sessionexecution.RunConfig{
		StartConfig: sessionexecution.StartConfig{
			Mode:      sessionexecution.ExecutionModeAsync,
			RequestID: "req-petri-run-001",
			FactoryID: "customer-support-triage",
		},
		Output:  ioDiscard{},
		Service: service,
	}); err != nil {
		t.Fatalf("seed RunAsync: %v", err)
	}

	var output bytes.Buffer
	if err := sessionexecution.RunDispatches(context.Background(), sessionexecution.DispatchesConfig{
		SessionID: "dur-sess-petri-run-001",
		Output:    &output,
		Service:   service,
	}); err != nil {
		t.Fatalf("RunDispatches: %v", err)
	}

	text := output.String()
	if !strings.Contains(text, "dispatches (1):") {
		t.Fatalf("output = %q, want one dispatch", text)
	}
	if !strings.Contains(text, "provider session: prov-sess-disp-petri-001") {
		t.Fatalf("output missing provider session ref:\n%s", text)
	}
}

func TestRunDispatches_SuccessFixtureJSONMatchesFixtureHash(t *testing.T) {
	service := newContractFakeService(t)
	seedSyncSuccessSession(t, service)

	base := sessionexecution.DispatchesConfig{
		SessionID: "dur-sess-petri-success-001",
		JSON:      true,
		Service:   service,
	}

	var firstOutput bytes.Buffer
	first := base
	first.Output = &firstOutput
	if err := sessionexecution.RunDispatches(context.Background(), first); err != nil {
		t.Fatalf("first RunDispatches: %v", err)
	}

	listed, err := service.ListDispatches(context.Background(), "dur-sess-petri-success-001")
	if err != nil {
		t.Fatalf("ListDispatches: %v", err)
	}
	wantHash, err := fixtures.ListDispatchesResultHash(listed)
	if err != nil {
		t.Fatalf("ListDispatchesResultHash: %v", err)
	}
	if wantHash != "sha256:a32d5d0f136dcfef8061746c8f270702163c92a04e3c9f75eb9248e19bebd34a" {
		t.Fatalf("fixture hash drifted to %q", wantHash)
	}

	var mapped factoryapi.ListFactorySessionDispatchesResponse
	if err := json.Unmarshal(bytes.TrimSpace(firstOutput.Bytes()), &mapped); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if mapped.SessionId != "dur-sess-petri-success-001" {
		t.Fatalf("sessionId = %q", mapped.SessionId)
	}
	if mapped.Dispatches == nil || len(mapped.Dispatches) != 1 {
		t.Fatalf("dispatches = %#v", mapped.Dispatches)
	}
	if mapped.Dispatches[0].Id != "disp-petri-success-001" {
		t.Fatalf("dispatch id = %q", mapped.Dispatches[0].Id)
	}

	var secondOutput bytes.Buffer
	second := base
	second.Output = &secondOutput
	if err := sessionexecution.RunDispatches(context.Background(), second); err != nil {
		t.Fatalf("second RunDispatches: %v", err)
	}
	if !bytes.Equal(bytes.TrimSpace(firstOutput.Bytes()), bytes.TrimSpace(secondOutput.Bytes())) {
		t.Fatalf("json output is not deterministic across equivalent dispatch reads")
	}

	wantJSON, err := json.Marshal(factorysession.ListDispatchesResponseToAPI(listed))
	if err != nil {
		t.Fatalf("marshal shared projection: %v", err)
	}
	if !bytes.Equal(bytes.TrimSpace(firstOutput.Bytes()), wantJSON) {
		t.Fatalf("CLI output diverged from shared ListDispatchesResponseToAPI projection")
	}
}

func TestRunArtifacts_SuccessFixtureHumanOutput(t *testing.T) {
	service := newContractFakeService(t)
	seedSyncSuccessSession(t, service)

	var output bytes.Buffer
	if err := sessionexecution.RunArtifacts(context.Background(), sessionexecution.ArtifactsConfig{
		SessionID: "dur-sess-petri-success-001",
		Output:    &output,
		Service:   service,
	}); err != nil {
		t.Fatalf("RunArtifacts: %v", err)
	}

	text := output.String()
	for _, want := range []string{
		"Factory session dur-sess-petri-success-001 artifacts (1):",
		"- art-petri-final-001 Triage summary FINAL_RESULT",
		"/factory-sessions/dur-sess-petri-success-001/artifacts/art-petri-final-001",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}
}

func TestRunArtifacts_SuccessFixtureJSONMatchesFixtureHash(t *testing.T) {
	service := newContractFakeService(t)
	seedSyncSuccessSession(t, service)

	base := sessionexecution.ArtifactsConfig{
		SessionID: "dur-sess-petri-success-001",
		JSON:      true,
		Service:   service,
	}

	var firstOutput bytes.Buffer
	first := base
	first.Output = &firstOutput
	if err := sessionexecution.RunArtifacts(context.Background(), first); err != nil {
		t.Fatalf("first RunArtifacts: %v", err)
	}

	listed, err := service.ListArtifacts(context.Background(), "dur-sess-petri-success-001")
	if err != nil {
		t.Fatalf("ListArtifacts: %v", err)
	}
	wantHash, err := fixtures.ListArtifactsResultHash(listed)
	if err != nil {
		t.Fatalf("ListArtifactsResultHash: %v", err)
	}
	if wantHash != "sha256:c42d891189b507df18e127e6cf10deeacf3d56a97c48786491d0ddfd3ed65fce" {
		t.Fatalf("fixture hash drifted to %q", wantHash)
	}

	var secondOutput bytes.Buffer
	second := base
	second.Output = &secondOutput
	if err := sessionexecution.RunArtifacts(context.Background(), second); err != nil {
		t.Fatalf("second RunArtifacts: %v", err)
	}
	if !bytes.Equal(bytes.TrimSpace(firstOutput.Bytes()), bytes.TrimSpace(secondOutput.Bytes())) {
		t.Fatalf("json output is not deterministic across equivalent artifact reads")
	}

	wantJSON, err := json.Marshal(factorysession.ListArtifactsResponseToAPI(listed))
	if err != nil {
		t.Fatalf("marshal shared projection: %v", err)
	}
	if !bytes.Equal(bytes.TrimSpace(firstOutput.Bytes()), wantJSON) {
		t.Fatalf("CLI output diverged from shared ListArtifactsResponseToAPI projection")
	}
}

func TestRunEvents_AsyncRunningFixtureHumanAndReconnectCursor(t *testing.T) {
	service := newContractFakeService(t)
	seedAsyncRunningSession(t, service)

	var allOutput bytes.Buffer
	if err := sessionexecution.RunEvents(context.Background(), sessionexecution.EventsConfig{
		SessionID: "dur-sess-js-run-n-001",
		Output:    &allOutput,
		Service:   service,
	}); err != nil {
		t.Fatalf("RunEvents all: %v", err)
	}
	allText := allOutput.String()
	for _, want := range []string{
		"Factory session dur-sess-js-run-n-001 events (2):",
		"SESSION_STARTED session-started/dur-sess-js-run-n-001",
	} {
		if !strings.Contains(allText, want) {
			t.Fatalf("all events output missing %q:\n%s", want, allText)
		}
	}

	var afterOutput bytes.Buffer
	if err := sessionexecution.RunEvents(context.Background(), sessionexecution.EventsConfig{
		SessionID:    "dur-sess-js-run-n-001",
		AfterEventID: "session-started/dur-sess-js-run-n-001",
		Output:       &afterOutput,
		Service:      service,
	}); err != nil {
		t.Fatalf("RunEvents reconnect: %v", err)
	}
	afterText := afterOutput.String()
	if !strings.Contains(afterText, "events (1):") {
		t.Fatalf("reconnect output = %q, want one trailing event", afterText)
	}
	if strings.Contains(afterText, "SESSION_STARTED") {
		t.Fatalf("reconnect output should omit acknowledged event:\n%s", afterText)
	}
}

func TestRunEvents_AsyncRunningFixtureJSONMatchesFixtureHash(t *testing.T) {
	service := newContractFakeService(t)
	seedAsyncRunningSession(t, service)

	base := sessionexecution.EventsConfig{
		SessionID: "dur-sess-js-run-n-001",
		JSON:      true,
		Service:   service,
	}

	var firstOutput bytes.Buffer
	first := base
	first.Output = &firstOutput
	if err := sessionexecution.RunEvents(context.Background(), first); err != nil {
		t.Fatalf("first RunEvents: %v", err)
	}

	read, err := service.ReadEvents(context.Background(), "dur-sess-js-run-n-001", fse.EventReconnectRequest{})
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	wantHash, err := fixtures.EventReadResultHash(read)
	if err != nil {
		t.Fatalf("EventReadResultHash: %v", err)
	}
	if wantHash != "sha256:11a22ce83ca44464c5a8d90062542e6bf9f16d4350005808795b95df7e461c65" {
		t.Fatalf("fixture hash drifted to %q", wantHash)
	}

	var secondOutput bytes.Buffer
	second := base
	second.Output = &secondOutput
	if err := sessionexecution.RunEvents(context.Background(), second); err != nil {
		t.Fatalf("second RunEvents: %v", err)
	}
	if !bytes.Equal(bytes.TrimSpace(firstOutput.Bytes()), bytes.TrimSpace(secondOutput.Bytes())) {
		t.Fatalf("json output is not deterministic across equivalent event reads")
	}

	wantJSON, err := json.Marshal(factorysession.EventReadResponseToAPI(read))
	if err != nil {
		t.Fatalf("marshal shared projection: %v", err)
	}
	if !bytes.Equal(bytes.TrimSpace(firstOutput.Bytes()), wantJSON) {
		t.Fatalf("CLI output diverged from shared EventReadResponseToAPI projection")
	}
}

func TestRunEvents_MissingReconnectCursorReturnsDeterministicError(t *testing.T) {
	service := newContractFakeService(t)
	seedAsyncRunningSession(t, service)

	var output bytes.Buffer
	err := sessionexecution.RunEvents(context.Background(), sessionexecution.EventsConfig{
		SessionID:    "dur-sess-js-run-n-001",
		AfterEventID: "missing-event-cursor",
		JSON:         true,
		Output:       &output,
		Service:      service,
	})
	if err == nil {
		t.Fatal("RunEvents = nil, want reconnect cursor error")
	}
	if !strings.Contains(output.String(), sessionexecution.ErrorCodeReconnectCursorNotFound) {
		t.Fatalf("output = %q, want reconnect cursor code", output.String())
	}
}

func TestRunDispatches_MissingSessionReturnsDeterministicError(t *testing.T) {
	service := newContractFakeService(t)
	var output bytes.Buffer
	err := sessionexecution.RunDispatches(context.Background(), sessionexecution.DispatchesConfig{
		SessionID: "dur-sess-missing-001",
		JSON:      true,
		Output:    &output,
		Service:   service,
	})
	if err == nil {
		t.Fatal("RunDispatches = nil, want missing session error")
	}
	if !strings.Contains(output.String(), sessionexecution.ErrorCodeSessionNotFound) {
		t.Fatalf("output = %q, want session not found code", output.String())
	}
}

// TestFixtureBackedCLIInspectionRegression_FullLoopWithoutLiveProviderFlags guards
// the default fixture-backed CLI inspection path while the additive live-provider
// smoke lane stays opt-in via --execution-provider and --child-executor-mode.
func assertFixtureBackedDispatchesRegression(
	t *testing.T,
	service fse.Service,
	sessionID string,
	dispatchesOutput []byte,
) {
	t.Helper()
	listedDispatches, err := service.ListDispatches(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("ListDispatches: %v", err)
	}
	wantDispatchHash, err := fixtures.ListDispatchesResultHash(listedDispatches)
	if err != nil {
		t.Fatalf("ListDispatchesResultHash: %v", err)
	}
	if wantDispatchHash != "sha256:a32d5d0f136dcfef8061746c8f270702163c92a04e3c9f75eb9248e19bebd34a" {
		t.Fatalf("fixture dispatch hash drifted to %q", wantDispatchHash)
	}
	wantDispatchJSON, err := json.Marshal(factorysession.ListDispatchesResponseToAPI(listedDispatches))
	if err != nil {
		t.Fatalf("marshal dispatch projection: %v", err)
	}
	if !bytes.Equal(bytes.TrimSpace(dispatchesOutput), wantDispatchJSON) {
		t.Fatalf("CLI dispatches JSON diverged from shared projection")
	}
	if strings.Contains(string(dispatchesOutput), "live-provider-session-1") {
		t.Fatalf("fixture-backed dispatches leaked live-provider markers:\n%s", dispatchesOutput)
	}
}

func assertFixtureBackedArtifactsRegression(
	t *testing.T,
	service fse.Service,
	sessionID string,
	artifactsOutput []byte,
) {
	t.Helper()
	listedArtifacts, err := service.ListArtifacts(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("ListArtifacts: %v", err)
	}
	wantArtifactHash, err := fixtures.ListArtifactsResultHash(listedArtifacts)
	if err != nil {
		t.Fatalf("ListArtifactsResultHash: %v", err)
	}
	if wantArtifactHash != "sha256:c42d891189b507df18e127e6cf10deeacf3d56a97c48786491d0ddfd3ed65fce" {
		t.Fatalf("fixture artifact hash drifted to %q", wantArtifactHash)
	}
	wantArtifactJSON, err := json.Marshal(factorysession.ListArtifactsResponseToAPI(listedArtifacts))
	if err != nil {
		t.Fatalf("marshal artifact projection: %v", err)
	}
	if !bytes.Equal(bytes.TrimSpace(artifactsOutput), wantArtifactJSON) {
		t.Fatalf("CLI artifacts JSON diverged from shared projection")
	}
}

func TestFixtureBackedCLIInspectionRegression_FullLoopWithoutLiveProviderFlags(t *testing.T) {
	service := newContractFakeService(t)
	sessionID := "dur-sess-petri-success-001"

	var runOutput bytes.Buffer
	if err := sessionexecution.RunSync(context.Background(), sessionexecution.RunConfig{
		StartConfig: sessionexecution.StartConfig{
			Mode:      sessionexecution.ExecutionModeSync,
			RequestID: "req-petri-success-001",
			FactoryID: "customer-support-triage",
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
	if runResponse.SessionId != sessionID {
		t.Fatalf("sessionId = %q, want %q", runResponse.SessionId, sessionID)
	}

	var statusOutput bytes.Buffer
	if err := sessionexecution.RunStatus(context.Background(), sessionexecution.StatusConfig{
		SessionID: sessionID,
		JSON:      true,
		Output:    &statusOutput,
		Service:   service,
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

	var resultOutput bytes.Buffer
	if err := sessionexecution.RunResult(context.Background(), sessionexecution.ResultConfig{
		SessionID: sessionID,
		JSON:      true,
		Output:    &resultOutput,
		Service:   service,
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

	var dispatchesOutput bytes.Buffer
	if err := sessionexecution.RunDispatches(context.Background(), sessionexecution.DispatchesConfig{
		SessionID: sessionID,
		JSON:      true,
		Output:    &dispatchesOutput,
		Service:   service,
	}); err != nil {
		t.Fatalf("RunDispatches: %v", err)
	}
	assertFixtureBackedDispatchesRegression(t, service, sessionID, dispatchesOutput.Bytes())

	var artifactsOutput bytes.Buffer
	if err := sessionexecution.RunArtifacts(context.Background(), sessionexecution.ArtifactsConfig{
		SessionID: sessionID,
		JSON:      true,
		Output:    &artifactsOutput,
		Service:   service,
	}); err != nil {
		t.Fatalf("RunArtifacts: %v", err)
	}
	assertFixtureBackedArtifactsRegression(t, service, sessionID, artifactsOutput.Bytes())
}
