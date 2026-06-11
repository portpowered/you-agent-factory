package sessionexecution_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/cli/sessionexecution"
)

func TestRunAsync_AsyncRunningFixtureHumanOutput(t *testing.T) {
	service := newContractFakeService(t)
	var output bytes.Buffer
	err := sessionexecution.RunAsync(context.Background(), sessionexecution.RunConfig{
		StartConfig: sessionexecution.StartConfig{
			Mode:         sessionexecution.ExecutionModeAsync,
			RequestID:    "req-js-run-n-001",
			WorkflowName: "release-train",
			ArgsJSON:     `{"release":"2026.06"}`,
		},
		Output:  &output,
		Service: service,
	})
	if err != nil {
		t.Fatalf("RunAsync: %v", err)
	}

	text := output.String()
	for _, want := range []string{
		"Factory session dur-sess-js-run-n-001 started (RUNNING).",
		"Request id: req-js-run-n-001",
		"Source ref: workflow/release-train",
		"Status link: /factory-sessions/dur-sess-js-run-n-001",
		"Results link: /factory-sessions/dur-sess-js-run-n-001/results",
		"Follow-up: you workflow status dur-sess-js-run-n-001",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}
}

func TestRunAsync_AsyncRunningFixtureJSONOutputIsDeterministic(t *testing.T) {
	service := newContractFakeService(t)
	base := sessionexecution.RunConfig{
		StartConfig: sessionexecution.StartConfig{
			Mode:         sessionexecution.ExecutionModeAsync,
			RequestID:    "req-js-run-n-001",
			WorkflowName: "release-train",
			ArgsJSON:     `{"release":"2026.06"}`,
		},
		JSON:    true,
		Service: service,
	}

	var firstOutput bytes.Buffer
	first := base
	first.Output = &firstOutput
	if err := sessionexecution.RunAsync(context.Background(), first); err != nil {
		t.Fatalf("first RunAsync: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(firstOutput.Bytes()), &payload); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if payload["sessionId"] != "dur-sess-js-run-n-001" {
		t.Fatalf("sessionId = %#v", payload["sessionId"])
	}
	if payload["status"] != "RUNNING" {
		t.Fatalf("status = %#v, want RUNNING", payload["status"])
	}
	if payload["requestId"] != "req-js-run-n-001" {
		t.Fatalf("requestId = %#v", payload["requestId"])
	}
	if payload["resultAvailability"] != "PARTIAL" {
		t.Fatalf("resultAvailability = %#v, want PARTIAL", payload["resultAvailability"])
	}
	links, ok := payload["links"].(map[string]any)
	if !ok {
		t.Fatalf("links = %#v", payload["links"])
	}
	if links["status"] != "/factory-sessions/dur-sess-js-run-n-001" {
		t.Fatalf("status link = %#v", links["status"])
	}
	if links["results"] != "/factory-sessions/dur-sess-js-run-n-001/results" {
		t.Fatalf("results link = %#v", links["results"])
	}

	var secondOutput bytes.Buffer
	second := base
	second.Output = &secondOutput
	if err := sessionexecution.RunAsync(context.Background(), second); err != nil {
		t.Fatalf("second RunAsync: %v", err)
	}
	if !bytes.Equal(bytes.TrimSpace(firstOutput.Bytes()), bytes.TrimSpace(secondOutput.Bytes())) {
		t.Fatalf("json output is not deterministic across equivalent starts")
	}
}

func TestRunAsync_RejectsSyncMode(t *testing.T) {
	service := newContractFakeService(t)
	var output bytes.Buffer
	err := sessionexecution.RunAsync(context.Background(), sessionexecution.RunConfig{
		StartConfig: sessionexecution.StartConfig{
			Mode:      sessionexecution.ExecutionModeSync,
			RequestID: "req-js-run-n-001",
			FactoryID: "customer-support-triage",
		},
		Output:  &output,
		Service: service,
	})
	if err == nil {
		t.Fatal("RunAsync = nil, want unsupported mode error")
	}
	if !strings.Contains(output.String(), sessionexecution.ErrorCodeUnsupportedMode) {
		t.Fatalf("output = %q, want unsupported mode code", output.String())
	}
}

func TestRunAsyncThenStatus_ReportsSameMockProviderState(t *testing.T) {
	service := newContractFakeService(t)
	startCfg := sessionexecution.RunConfig{
		StartConfig: sessionexecution.StartConfig{
			Mode:         sessionexecution.ExecutionModeAsync,
			RequestID:    "req-js-run-n-001",
			WorkflowName: "release-train",
			ArgsJSON:     `{"release":"2026.06"}`,
		},
		JSON:    true,
		Service: service,
	}
	var startOutput bytes.Buffer
	startCfg.Output = &startOutput
	if err := sessionexecution.RunAsync(context.Background(), startCfg); err != nil {
		t.Fatalf("RunAsync: %v", err)
	}
	var started map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(startOutput.Bytes()), &started); err != nil {
		t.Fatalf("decode start output: %v", err)
	}
	sessionID, _ := started["sessionId"].(string)
	if sessionID == "" {
		t.Fatalf("start output missing sessionId: %#v", started)
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

	var status factoryapi.FactorySessionDurableReadModel
	if err := json.Unmarshal(bytes.TrimSpace(statusOutput.Bytes()), &status); err != nil {
		t.Fatalf("decode status output: %v", err)
	}
	if status.SessionId != sessionID {
		t.Fatalf("sessionId = %q, want %q", status.SessionId, sessionID)
	}
	if status.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("status = %q, want RUNNING", status.Status)
	}
	if status.Phase == nil || *status.Phase != "verify" {
		t.Fatalf("phase = %#v, want verify", status.Phase)
	}
	if status.Progress == nil || status.Progress.TotalDispatches == nil || *status.Progress.TotalDispatches != 3 {
		t.Fatalf("progress = %#v", status.Progress)
	}
	if status.Links == nil || status.Links.Results == nil || *status.Links.Results != "/factory-sessions/dur-sess-js-run-n-001/results" {
		t.Fatalf("links = %#v", status.Links)
	}
}
