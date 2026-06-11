package sessionexecution_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/cli/sessionexecution"
	fse "github.com/portpowered/infinite-you/pkg/factorysessionexecution"
)

func TestRunStatus_AsyncRunningFixtureHumanOutput(t *testing.T) {
	service := newContractFakeService(t)
	seedAsyncRunningSession(t, service)

	var output bytes.Buffer
	if err := sessionexecution.RunStatus(context.Background(), sessionexecution.StatusConfig{
		SessionID: "dur-sess-js-run-n-001",
		Output:    &output,
		Service:   service,
	}); err != nil {
		t.Fatalf("RunStatus: %v", err)
	}

	text := output.String()
	for _, want := range []string{
		"Factory session dur-sess-js-run-n-001 is RUNNING.",
		"Phase: verify",
		"Progress: total dispatches 3, completed 1, in flight 1",
		"Results link: /factory-sessions/dur-sess-js-run-n-001/results",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}
}

func TestRunStatus_AsyncRunningFixtureJSONOutputIsDeterministic(t *testing.T) {
	service := newContractFakeService(t)
	seedAsyncRunningSession(t, service)

	base := sessionexecution.StatusConfig{
		SessionID: "dur-sess-js-run-n-001",
		JSON:      true,
		Service:   service,
	}

	var firstOutput bytes.Buffer
	first := base
	first.Output = &firstOutput
	if err := sessionexecution.RunStatus(context.Background(), first); err != nil {
		t.Fatalf("first RunStatus: %v", err)
	}

	var status factoryapi.FactorySessionDurableReadModel
	if err := json.Unmarshal(bytes.TrimSpace(firstOutput.Bytes()), &status); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if status.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("status = %q, want RUNNING", status.Status)
	}
	if status.Progress == nil || status.Progress.InFlightDispatches == nil || *status.Progress.InFlightDispatches != 1 {
		t.Fatalf("progress = %#v", status.Progress)
	}
	if status.Links == nil || status.Links.Status == nil {
		t.Fatalf("links = %#v", status.Links)
	}

	var secondOutput bytes.Buffer
	second := base
	second.Output = &secondOutput
	if err := sessionexecution.RunStatus(context.Background(), second); err != nil {
		t.Fatalf("second RunStatus: %v", err)
	}
	if !bytes.Equal(bytes.TrimSpace(firstOutput.Bytes()), bytes.TrimSpace(secondOutput.Bytes())) {
		t.Fatalf("json output is not deterministic across equivalent status reads")
	}
}

func TestRunStatus_MissingSessionReturnsDeterministicError(t *testing.T) {
	service := newContractFakeService(t)
	var output bytes.Buffer
	err := sessionexecution.RunStatus(context.Background(), sessionexecution.StatusConfig{
		SessionID: "dur-sess-missing-001",
		JSON:      true,
		Output:    &output,
		Service:   service,
	})
	if err == nil {
		t.Fatal("RunStatus = nil, want missing session error")
	}
	if !strings.Contains(output.String(), sessionexecution.ErrorCodeSessionNotFound) {
		t.Fatalf("output = %q, want session not found code", output.String())
	}
}

func seedAsyncRunningSession(t *testing.T, service fse.Service) {
	t.Helper()
	if err := sessionexecution.RunAsync(context.Background(), sessionexecution.RunConfig{
		StartConfig: sessionexecution.StartConfig{
			Mode:         sessionexecution.ExecutionModeAsync,
			RequestID:    "req-js-run-n-001",
			WorkflowName: "release-train",
			ArgsJSON:     `{"release":"2026.06"}`,
		},
		Output:  ioDiscard{},
		Service: service,
	}); err != nil {
		t.Fatalf("seed RunAsync: %v", err)
	}
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }
