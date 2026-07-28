package gemini

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestGeminiConductorSuccessThroughRootBuildProcess proves successful Gemini
// execution through the customer process boundary and Providers-backed command
// selection.
func TestGeminiConductorSuccessThroughRootBuildProcess(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(
		modelprovider.ProviderGemini,
		"gemini-2.5-flash",
	))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"gemini conductor success"}`))

	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout: []byte("gemini functional answer COMPLETE"),
	})

	_, listed, events, responseEvents := support.RunFactoryToCompletionWithEdgesAndResponseEvents(
		t,
		dir,
		serviceedges.Edges{ProviderCommandRunner: runner},
		20*time.Second,
	)

	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("completed work = %d, want 1; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 0 {
		t.Fatalf("failed work = %d, want 0", got)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("Gemini command runner calls = %d, want 1 through Providers path", runner.CallCount())
	}
	request := runner.LastRequest()
	if request.Command != "gemini" {
		t.Fatalf("command = %q, want gemini", request.Command)
	}
	if !containsArgPair(request.Args, "--model", "gemini-2.5-flash") {
		t.Fatalf("args = %#v, want --model gemini-2.5-flash", request.Args)
	}
	assertGeminiFinalOnlyCompletion(t, events, responseEvents, "gemini functional answer COMPLETE")
}

// TestGeminiConductorPreservesConfiguredEnvironment proves configured environment
// reaches Gemini execution through the Providers adapter.
func TestGeminiConductorPreservesConfiguredEnvironment(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(
		modelprovider.ProviderGemini,
		"gemini-2.5-flash",
	))
	support.WriteWorkstationConfig(t, dir, "process", `---
type: MODEL_WORKSTATION
env:
  GEMINI_CONTEXT_FIXTURE: configured
---
Test workstation.
`)
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"gemini conductor context"}`))

	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout: []byte("gemini context answer COMPLETE"),
	})
	_, listed, _ := support.RunFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{
		ProviderCommandRunner: runner,
	}, 20*time.Second)

	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("completed work = %d, want 1; listed=%#v", got, listed)
	}
	request := runner.LastRequest()
	if !containsEnv(request.Env, "GEMINI_CONTEXT_FIXTURE=configured") {
		t.Fatal("command environment omitted configured Gemini context")
	}
}

// TestGeminiNativeFailureThroughRootBuildProcessIsSafe proves native Gemini
// failures remain safe and observable through the customer process boundary.
func TestGeminiNativeFailureThroughRootBuildProcessIsSafe(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(
		modelprovider.ProviderGemini,
		"gemini-2.5-flash",
	))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"gemini native failure"}`))

	const leaked = "/tmp/secret-key"
	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		ExitCode: 1,
		Stderr:   []byte(`{"error":{"status":"UNAUTHENTICATED","message":"token path ` + leaked + ` leaked"}}`),
	})
	_, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{
		ProviderCommandRunner: runner,
	}, 20*time.Second)

	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
		t.Fatalf("failed work = %d, want 1; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 0 {
		t.Fatalf("completed work = %d, want 0 after native failure", got)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("Gemini command runner calls = %d, want 1", runner.CallCount())
	}
	if request := runner.LastRequest(); request.Command != "gemini" {
		t.Fatalf("command = %q, want gemini", request.Command)
	}
	encoded, err := json.Marshal(events)
	if err != nil {
		t.Fatalf("marshal Factory events: %v", err)
	}
	payload := string(encoded)
	if strings.Contains(payload, leaked) || strings.Contains(payload, "secret-key") {
		t.Fatalf("Factory events leaked unsafe Gemini failure detail: %s", payload)
	}
}

// TestGeminiCommandCancellationThroughRootBuildProcessIsCanonical proves
// cancellation returns the canonical outcome through the Providers adapter.
func TestGeminiCommandCancellationThroughRootBuildProcessIsCanonical(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(
		modelprovider.ProviderGemini,
		"gemini-2.5-flash",
	))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"gemini command cancel"}`))

	runner := &commandCancellationRunner{}
	_, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{
		ProviderCommandRunner: runner,
	}, 20*time.Second)

	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
		t.Fatalf("failed work = %d, want 1; listed=%#v", got, listed)
	}
	if runner.calls != 1 {
		t.Fatalf("Gemini command runner calls = %d, want 1", runner.calls)
	}
	if runner.lastRequest.Command != "gemini" {
		t.Fatalf("command = %q, want gemini", runner.lastRequest.Command)
	}
	encoded, err := json.Marshal(events)
	if err != nil {
		t.Fatalf("marshal Factory events: %v", err)
	}
	payload := string(encoded)
	if strings.Contains(payload, "Gemini command did not complete successfully") {
		t.Fatalf("Factory events used Gemini-local cancellation fallback: %s", payload)
	}
	if !strings.Contains(payload, "provider invocation was canceled") {
		t.Fatalf("Factory events missing canonical cancellation outcome: %s", payload)
	}
}

func assertGeminiFinalOnlyCompletion(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	responseEvents []factoryapi.FactoryResponseEvent,
	wantOutput string,
) {
	t.Helper()

	var completedMessages int
	for _, event := range responseEvents {
		switch event.Kind {
		case factoryapi.FactoryResponseEventKindMessage:
			if event.Phase == factoryapi.FactoryResponseEventPhaseDelta {
				t.Fatalf("final-only Gemini replay fabricated message delta: %#v", event)
			}
			if event.Phase == factoryapi.FactoryResponseEventPhaseCompleted {
				completedMessages++
			}
		case factoryapi.FactoryResponseEventKindTool, factoryapi.FactoryResponseEventKindUsage:
			t.Fatalf("final-only Gemini replay fabricated lifecycle: %#v", event)
		}
	}
	if completedMessages != 1 {
		t.Fatalf("completed message events = %d, want exactly one terminal result", completedMessages)
	}

	dispatchOutput := ""
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse {
			continue
		}
		payload, err := event.Payload.AsDispatchResponseEventPayload()
		if err != nil {
			t.Fatalf("decode dispatch response: %v", err)
		}
		if payload.Output != nil && *payload.Output != "" {
			dispatchOutput = *payload.Output
		}
	}
	if dispatchOutput != wantOutput {
		t.Fatalf("dispatch output = %q, want %q", dispatchOutput, wantOutput)
	}
}

type commandCancellationRunner struct {
	calls       int
	lastRequest platformprocess.CommandRequest
}

func (r *commandCancellationRunner) Run(_ context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	r.calls++
	r.lastRequest = request
	return platformprocess.CommandResult{}, context.Canceled
}

func containsArgPair(args []string, flag, value string) bool {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == flag && args[index+1] == value {
			return true
		}
	}
	return false
}

func containsEnv(env []string, expected string) bool {
	for _, value := range env {
		if value == expected {
			return true
		}
	}
	return false
}
