package sessionexecution_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	fse "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	"github.com/portpowered/infinite-you/pkg/transports/cli/sessionexecution"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
)

func TestRunResult_SuccessFixtureFinalResultHumanOutput(t *testing.T) {
	service := newContractFakeService(t)
	seedSyncSuccessSession(t, service)

	var output bytes.Buffer
	if err := sessionexecution.RunResult(context.Background(), sessionexecution.ResultConfig{
		SessionID: "dur-sess-petri-success-001",
		Output:    &output,
		Service:   service,
	}); err != nil {
		t.Fatalf("RunResult: %v", err)
	}

	text := output.String()
	for _, want := range []string{
		"Factory session dur-sess-petri-success-001 result is FINAL.",
		"Session status: SUCCEEDED",
		"Primary result: Ticket triaged and resolved.",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}
}

func TestRunResult_SuccessFixtureJSONOutputIsDeterministic(t *testing.T) {
	service := newContractFakeService(t)
	seedSyncSuccessSession(t, service)

	base := sessionexecution.ResultConfig{
		SessionID: "dur-sess-petri-success-001",
		JSON:      true,
		Service:   service,
	}

	var firstOutput bytes.Buffer
	first := base
	first.Output = &firstOutput
	if err := sessionexecution.RunResult(context.Background(), first); err != nil {
		t.Fatalf("first RunResult: %v", err)
	}

	var result factoryapi.FactorySessionResult
	if err := json.Unmarshal(bytes.TrimSpace(firstOutput.Bytes()), &result); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if result.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("resultStatus = %q, want FINAL", result.ResultStatus)
	}
	if resultDisplayText(&result) != "Ticket triaged and resolved." {
		t.Fatalf("primary result = %q", resultDisplayText(&result))
	}

	var secondOutput bytes.Buffer
	second := base
	second.Output = &secondOutput
	if err := sessionexecution.RunResult(context.Background(), second); err != nil {
		t.Fatalf("second RunResult: %v", err)
	}
	if !bytes.Equal(bytes.TrimSpace(firstOutput.Bytes()), bytes.TrimSpace(secondOutput.Bytes())) {
		t.Fatalf("json output is not deterministic across equivalent result reads")
	}
}

func TestRunResult_AsyncRunningFinalModeReportsNotReady(t *testing.T) {
	service := newContractFakeService(t)
	seedAsyncRunningSession(t, service)

	var output bytes.Buffer
	err := sessionexecution.RunResult(context.Background(), sessionexecution.ResultConfig{
		SessionID: "dur-sess-js-run-n-001",
		Output:    &output,
		Service:   service,
	})
	var outcome *sessionexecution.ResultOutcomeError
	if !errors.As(err, &outcome) {
		t.Fatalf("RunResult error = %v, want ResultOutcomeError", err)
	}
	if outcome.Status != factoryapi.FactorySessionResultStatusNotReady {
		t.Fatalf("outcome status = %q, want NOT_READY", outcome.Status)
	}

	text := output.String()
	for _, want := range []string{
		"Factory session dur-sess-js-run-n-001 result is NOT_READY.",
		"Session status: RUNNING",
		"Availability reason: RESULT_NOT_READY",
		"Availability message: Session is still running.",
		"Retryable: true",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}
}

func TestRunResult_AsyncRunningPartialModeReturnsPartialData(t *testing.T) {
	service := newContractFakeService(t)
	seedAsyncRunningSession(t, service)

	var output bytes.Buffer
	if err := sessionexecution.RunResult(context.Background(), sessionexecution.ResultConfig{
		SessionID: "dur-sess-js-run-n-001",
		Mode:      "partial",
		Output:    &output,
		Service:   service,
	}); err != nil {
		t.Fatalf("RunResult: %v", err)
	}

	text := output.String()
	for _, want := range []string{
		"Factory session dur-sess-js-run-n-001 result is PARTIAL.",
		"Session status: RUNNING",
		`Primary result: {"completedSteps":1,"phase":"verify"}`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}
}

func TestRunResult_MissingSessionReturnsDeterministicError(t *testing.T) {
	service := newContractFakeService(t)
	var output bytes.Buffer
	err := sessionexecution.RunResult(context.Background(), sessionexecution.ResultConfig{
		SessionID: "dur-sess-missing-001",
		JSON:      true,
		Output:    &output,
		Service:   service,
	})
	if err == nil {
		t.Fatal("RunResult = nil, want missing session error")
	}
	if !strings.Contains(output.String(), sessionexecution.ErrorCodeSessionNotFound) {
		t.Fatalf("output = %q, want session not found code", output.String())
	}
}

func TestRunResult_UsesSharedServiceProjection(t *testing.T) {
	service := newContractFakeService(t)
	seedSyncSuccessSession(t, service)

	direct, err := service.GetResult(context.Background(), "dur-sess-petri-success-001", fse.ResultRequest{
		Mode: fse.ResultModeFinal,
	})
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	want := factorysession.ResultResponseToAPI(direct)
	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal direct projection: %v", err)
	}

	var output bytes.Buffer
	if err := sessionexecution.RunResult(context.Background(), sessionexecution.ResultConfig{
		SessionID: "dur-sess-petri-success-001",
		JSON:      true,
		Output:    &output,
		Service:   service,
	}); err != nil {
		t.Fatalf("RunResult: %v", err)
	}
	if !bytes.Equal(bytes.TrimSpace(output.Bytes()), wantJSON) {
		t.Fatalf("CLI output diverged from shared ResultResponseToAPI projection")
	}
}

func seedSyncSuccessSession(t *testing.T, service fse.Service) {
	t.Helper()
	if err := sessionexecution.RunSync(context.Background(), sessionexecution.RunConfig{
		StartConfig: sessionexecution.StartConfig{
			Mode:      sessionexecution.ExecutionModeSync,
			RequestID: "req-petri-success-001",
			FactoryID: "customer-support-triage",
		},
		Output:  ioDiscard{},
		Service: service,
	}); err != nil {
		t.Fatalf("seed RunSync: %v", err)
	}
}

func resultDisplayText(result *factoryapi.FactorySessionResult) string {
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
