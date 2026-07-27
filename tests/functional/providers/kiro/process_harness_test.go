package kiro

import (
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

const (
	functionalSessionID            = "675f9238-5f05-456c-9a9f-f8fe486f49e4"
	functionalReplacementSessionID = "4b11c29d-332f-4fa2-b5d3-cd0fb02f75b0"
)

func TestKiroConductorSuccessThroughRootBuildProcess(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	workerConfig := strings.Replace(
		support.BuildModelWorkerConfig(modelprovider.ProviderKiro, "kiro-auto"),
		"stopToken: COMPLETE",
		"skipPermissions: true\nstopToken: COMPLETE",
		1,
	)
	support.WriteAgentConfig(t, dir, "worker", workerConfig)
	support.WriteWorkstationConfig(t, dir, "process", `---
type: MODEL_WORKSTATION
env:
  KIRO_CONTEXT_FIXTURE: configured
---
Test workstation.
`)
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"kiro conductor success"}`))

	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout: []byte("kiro functional answer COMPLETE"),
		Stderr: []byte(`{"event":"session.created","session_id":"` + functionalSessionID + `"}`),
	})
	_, listed, _ := support.RunFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{
		ProviderCommandRunner: runner,
	}, 20*time.Second)

	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("completed work = %d, want 1; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 0 {
		t.Fatalf("failed work = %d, want 0", got)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("Kiro command runner calls = %d, want 1 through conductor", runner.CallCount())
	}
	request := runner.LastRequest()
	if request.Command != "kiro-cli" {
		t.Fatalf("command = %q, want kiro-cli", request.Command)
	}
	if len(request.Args) < 4 ||
		request.Args[0] != "chat" ||
		request.Args[1] != "--no-interactive" ||
		!containsArg(request.Args, "--trust-all-tools") {
		t.Fatalf("args = %#v, want Kiro chat and trust-all-tools flags", request.Args)
	}
	if containsArg(request.Args, "--resume-id") {
		t.Fatalf("new invocation args = %#v, must not contain --resume-id", request.Args)
	}
	prompt := request.Args[len(request.Args)-1]
	if !strings.Contains(prompt, "System instructions:") ||
		!strings.Contains(prompt, "Process the input task.") ||
		!strings.Contains(prompt, "User request:") ||
		!strings.Contains(prompt, "Test workstation.") {
		t.Fatalf("prompt = %q, want composed system instructions and work request", prompt)
	}
	if !containsEnv(request.Env, "KIRO_CONTEXT_FIXTURE=configured") {
		t.Fatal("command environment omitted configured Kiro context")
	}
}

func TestKiroConductorResumeThroughRootBuildProcess(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(
		modelprovider.ProviderKiro,
		"kiro-auto",
	))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"kiro conductor resume"}`))

	runner := testutil.NewProviderCommandRunner(
		platformprocess.CommandResult{
			ExitCode: 1,
			Stderr: []byte(
				`{"event":"session.created","session_id":"` + functionalSessionID + `"}` + "\n" +
					`{"type":"error","errorType":"ServiceUnavailableException","message":"temporary service failure"}`,
			),
		},
		platformprocess.CommandResult{
			Stdout: []byte("kiro resumed answer COMPLETE"),
			Stderr: []byte(
				`{"event":"session.created","session_id":"` + functionalReplacementSessionID + `"}`,
			),
		},
	)
	_, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{
		ProviderCommandRunner: runner,
	}, 20*time.Second)

	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("completed work = %d, want 1; listed=%#v", got, listed)
	}
	if runner.CallCount() != 2 {
		t.Fatalf("Kiro command runner calls = %d, want one failed attempt and one resumed retry", runner.CallCount())
	}
	requests := runner.Requests()
	if containsArg(requests[0].Args, "--resume-id") {
		t.Fatalf("first invocation args = %#v, must not contain --resume-id", requests[0].Args)
	}
	if !containsArgPair(requests[1].Args, "--resume-id", functionalSessionID) {
		t.Fatalf("resumed retry args = %#v, want --resume-id %s", requests[1].Args, functionalSessionID)
	}
	assertKiroRetryProviderSessions(t, events)
	encoded, err := json.Marshal(events)
	if err != nil {
		t.Fatalf("marshal Factory events: %v", err)
	}
	payload := string(encoded)
	if !strings.Contains(payload, functionalSessionID) ||
		!strings.Contains(payload, functionalReplacementSessionID) ||
		!strings.Contains(payload, `"provider":"kiro"`) {
		t.Fatalf("Factory events omitted preserved Kiro Provider Session: %s", payload)
	}
}

func assertKiroRetryProviderSessions(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()
	var failedSessionID, succeededSessionID string
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeInferenceResponse {
			continue
		}
		payload, err := event.Payload.AsInferenceResponseEventPayload()
		if err != nil {
			t.Fatalf("decode inference response: %v", err)
		}
		if payload.ProviderSession == nil || payload.ProviderSession.Id == nil {
			continue
		}
		switch payload.Outcome {
		case factoryapi.InferenceOutcomeFailed:
			failedSessionID = *payload.ProviderSession.Id
		case factoryapi.InferenceOutcomeSucceeded:
			succeededSessionID = *payload.ProviderSession.Id
		}
	}
	if failedSessionID != functionalSessionID {
		t.Fatalf("failed attempt Provider Session = %q, want preserved %q", failedSessionID, functionalSessionID)
	}
	if succeededSessionID != functionalReplacementSessionID {
		t.Fatalf(
			"successful attempt Provider Session = %q, want replacement %q",
			succeededSessionID,
			functionalReplacementSessionID,
		)
	}
}

func TestKiroRejectsUnsupportedWorkingDirectoryBeforeProviderIO(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(
		modelprovider.ProviderKiro,
		"kiro-auto",
	))
	support.WriteWorkstationConfig(t, dir, "process", `---
type: MODEL_WORKSTATION
workingDirectory: .
---
Test workstation.
`)
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"kiro invalid capability"}`))

	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout: []byte("provider must not run"),
	})
	_, listed, _ := support.RunFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{
		ProviderCommandRunner: runner,
	}, 20*time.Second)

	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
		t.Fatalf("failed work = %d, want 1 unsupported-capability failure; listed=%#v", got, listed)
	}
	if runner.CallCount() != 0 {
		t.Fatalf("Kiro command runner calls = %d, want no provider I/O", runner.CallCount())
	}
}

func TestKiroNativeFailureThroughRootBuildProcessIsSafe(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(
		modelprovider.ProviderKiro,
		"kiro-auto",
	))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"kiro native failure"}`))

	const leaked = `C:\private\kiro-token.txt`
	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		ExitCode: 1,
		Stderr: []byte(
			`{"type":"error","error":{"type":"authentication_error","message":"token path ` + leaked + ` leaked"}}`,
		),
	})
	_, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{
		ProviderCommandRunner: runner,
	}, 20*time.Second)

	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
		t.Fatalf("failed work = %d, want 1; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 0 {
		t.Fatalf("completed work = %d, want 0 after provider failure", got)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("Kiro command runner calls = %d, want 1", runner.CallCount())
	}
	encoded, err := json.Marshal(events)
	if err != nil {
		t.Fatalf("marshal Factory events: %v", err)
	}
	payload := string(encoded)
	if strings.Contains(payload, leaked) ||
		strings.Contains(payload, "kiro-token") {
		t.Fatalf("Factory events leaked unsafe Kiro failure detail: %s", payload)
	}
}

func containsArg(args []string, expected string) bool {
	for _, arg := range args {
		if arg == expected {
			return true
		}
	}
	return false
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
