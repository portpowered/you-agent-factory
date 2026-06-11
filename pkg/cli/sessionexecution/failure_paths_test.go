package sessionexecution_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/cli/sessionexecution"
	fse "github.com/portpowered/infinite-you/pkg/factorysessionexecution"
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
