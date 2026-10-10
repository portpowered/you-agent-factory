package customer_journeys_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// W1-S/F exercise the API-owned explicit-session Work and event read contract
// on the existing host with peer routing journeys. Each parallel leaf owns its
// Factory, provider route and public Work trace.
func runSelectedProviderRouteJourneys(t *testing.T, host *factorySessionHost) {
	t.Parallel()
	for _, failed := range []bool{false, true} {
		name := "success"
		if failed {
			name = "provider_failure"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			runSelectedProviderRoute(t, host, name, failed)
		})
	}
}

func runSelectedProviderRoute(t *testing.T, host *factorySessionHost, name string, failed bool) {
	t.Helper()
	marker := "selected-provider-route-" + name
	dir := support.ScaffoldSingleStepFactory(t, marker)
	support.WriteAgentConfig(t, dir, "processor", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "test-model"))
	support.WriteWorkstationConfig(t, dir, "process", "---\ntype: MODEL_WORKSTATION\n---\nWork: {{ (index .Inputs 0).WorkID }}\nPayload: {{ (index .Inputs 0).Payload }}\n")
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "task", TraceID: marker, Payload: []byte(`{"title":"` + marker + `"}`),
	})
	result := platformprocess.CommandResult{Stdout: []byte(marker + " COMPLETE")}
	state, outcome := "complete", factoryapi.WorkOutcomeAccepted
	complete, failures := 1, 0
	if failed {
		result = platformprocess.CommandResult{ExitCode: 77, Stderr: []byte("deterministic selected provider refusal")}
		state, outcome = "failed", factoryapi.WorkOutcomeFailed
		complete, failures = 0, 1
	}
	runner := support.NewShapedProviderCommandRunner(result)
	session, listed, events := host.Run(t, dir, runner, 15*time.Second)
	assertWorkflowWorkStates(t, listed, map[string]int{
		"task:init": 0, "task:complete": complete, "task:failed": failures,
	})
	workID, found := workIDAtCustomerState(t, listed, "task:"+state, marker)
	if !found {
		t.Fatalf("selected Work trace %q missing at %s", marker, state)
	}
	requests := runner.Requests()
	if len(requests) != 1 {
		t.Fatalf("selected runner requests = %d, want one", len(requests))
	}
	prompt := string(requests[0].Stdin)
	if requests[0].Command != "codex" || !strings.Contains(prompt, marker) || !strings.Contains(prompt, workID) || strings.Contains(prompt, "{{") {
		t.Fatalf("selected command = %q, prompt = %q; want Codex with expanded Work %q and payload %q", requests[0].Command, prompt, workID, marker)
	}
	assertSelectedProviderEvents(t, events, session.Id, workID, marker, outcome)
}

func assertSelectedProviderEvents(t *testing.T, events []factoryapi.FactoryEvent, sessionID, workID, marker string, outcome factoryapi.WorkOutcome) {
	t.Helper()
	responses := 0
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse {
			continue
		}
		if event.Context.SessionId == nil || *event.Context.SessionId != sessionID {
			t.Fatalf("dispatch session = %v, want %q", event.Context.SessionId, sessionID)
		}
		payload, err := event.Payload.AsDispatchResponseEventPayload()
		if err != nil {
			t.Fatal(err)
		}
		assertSelectedProviderResponse(t, payload, marker, outcome)
		responses++
	}
	observations := support.ObserveDispatchEvents(t, events)
	if responses != 1 || len(observations) != 1 || !support.DispatchObservationIncludesWork(observations[0], workID) {
		t.Fatalf("dispatches = %+v, response count = %d; want one correlated to Work %s", observations, responses, workID)
	}
}

func assertSelectedProviderResponse(t *testing.T, payload factoryapi.DispatchResponseEventPayload, marker string, outcome factoryapi.WorkOutcome) {
	t.Helper()
	if payload.Outcome != outcome {
		t.Fatalf("selected dispatch outcome = %s, want %s", payload.Outcome, outcome)
	}
	peerMarker := "selected-provider-route-success"
	if outcome == factoryapi.WorkOutcomeAccepted {
		peerMarker = "selected-provider-route-provider_failure"
		if payload.Output == nil || !strings.Contains(*payload.Output, marker) {
			t.Fatalf("selected dispatch output = %v, want %q", payload.Output, marker)
		}
	} else {
		// The public contract intentionally normalizes an unrecognized native
		// process error; raw provider stderr and exit details stay private.
		if payload.Error == nil {
			t.Fatal("selected failure lacks public diagnostic")
		}
		if *payload.Error != "provider execution failed" {
			t.Fatalf("selected failure diagnostic = %q, want safe provider execution failure", *payload.Error)
		}
		if payload.Output != nil && strings.TrimSpace(*payload.Output) != "" {
			t.Fatalf("failed selected dispatch published output %q", *payload.Output)
		}
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), peerMarker) {
		t.Fatalf("selected dispatch contains peer facts: %s", encoded)
	}
}

func runProviderRetryRecoveryJourneys(t *testing.T, host *factorySessionHost) {
	t.Parallel()
	t.Run("ConfigDrivenUnrecognizedProviderRefusalFailsOnce", func(t *testing.T) { runUnrecognizedProviderRefusal(t, host) })
	t.Run("ConfigDrivenRetryLoopBreakerTerminatesAfterMaxRetries", func(t *testing.T) { runRetryExhaustion(t, host) })
	t.Run("ConfigDrivenRetryLoopBreakerSucceedsBeforeLimit", func(t *testing.T) { runRetryRecovery(t, host) })
}

func runUnrecognizedProviderRefusal(t *testing.T, host *factorySessionHost) {
	t.Helper()
	t.Parallel()
	dir := support.ScaffoldFactory(t, map[string]any{
		"name": "process_failure_breaker",
		"workTypes": []any{map[string]any{
			"name": "task",
			"states": []any{
				map[string]any{"name": "init", "type": "INITIAL"},
				map[string]any{"name": "complete", "type": "TERMINAL"},
				map[string]any{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []any{map[string]any{"name": "processor"}},
		"workstations": []map[string]any{{
			"name":   "process",
			"worker": "processor",
			"inputs": []any{map[string]any{"workType": "task", "state": "init"}},
			"outputs": []any{map[string]any{
				"workType": "task",
				"state":    "complete",
			}},
			"onFailure": []any{map[string]any{
				"workType": "task",
				"state":    "init",
			}},
			"limits": map[string]any{"maxRetries": 3},
		}},
	})
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"process failure breaker"}`))
	support.WriteAgentConfig(t, dir, "processor", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "test-model"))

	runner := support.NewShapedProviderCommandRunner(
		platformprocess.CommandResult{ExitCode: 77, Stderr: []byte("future provider refusal: credential=secret")},
	)
	_, listed, events := host.Run(t, dir, runner, 15*time.Second)

	assertWorkflowWorkStates(t, listed, map[string]int{
		"task:failed":   1,
		"task:init":     0,
		"task:complete": 0,
	})
	if got := runner.CallCount(); got != 1 {
		t.Fatalf("provider command calls = %d, want one terminal process failure", got)
	}

	observations := support.ObserveDispatchEvents(t, events)
	processFailures := 0
	for _, observation := range observations {
		if observation.Request.TransitionId == "process" && observation.Response != nil {
			if observation.Response.Outcome != factoryapi.WorkOutcomeFailed {
				t.Errorf("process response outcome = %q, want FAILED", observation.Response.Outcome)
			}
			processFailures++
		}
	}
	if processFailures != 1 {
		t.Fatalf("failed process dispatches = %d, want one terminal refusal", processFailures)
	}
}

func runRetryExhaustion(t *testing.T, host *factorySessionHost) {
	t.Helper()
	t.Parallel()
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "retry_exhaustion"))

	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "Will exhaust retries"}`))

	provider := support.NewShapedProviderCommandRunner(
		platformprocess.CommandResult{Stdout: []byte("Processed. COMPLETE")},
		platformprocess.CommandResult{Stdout: []byte("Needs work")},
		platformprocess.CommandResult{Stdout: []byte("Processed. COMPLETE")},
		platformprocess.CommandResult{Stdout: []byte("Still needs work")},
		platformprocess.CommandResult{Stdout: []byte("Processed. COMPLETE")},
		platformprocess.CommandResult{Stdout: []byte("Not good enough")},
	)

	_, listed, events := host.Run(t, dir, provider, 15*time.Second)
	assertWorkflowWorkStates(t, listed, map[string]int{
		"task:failed": 1, "task:init": 0, "task:in-review": 0, "task:complete": 0,
	})

	if provider.CallCount() != 6 {
		t.Errorf("expected provider called 6 times, got %d", provider.CallCount())
	}

	assertPublicDispatchRoute(t, events, "review-exhaustion", "task:failed")
}

func runRetryRecovery(t *testing.T, host *factorySessionHost) {
	t.Helper()
	t.Parallel()
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "retry_exhaustion"))

	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "Will succeed on second try"}`))

	provider := support.NewShapedProviderCommandRunner(
		platformprocess.CommandResult{Stdout: []byte("Processed. COMPLETE")},
		platformprocess.CommandResult{Stdout: []byte("Needs work")},
		platformprocess.CommandResult{Stdout: []byte("Processed. COMPLETE")},
		platformprocess.CommandResult{Stdout: []byte("Looks good. ACCEPTED")},
	)

	_, listed, _ := host.Run(t, dir, provider, 15*time.Second)
	assertWorkflowWorkStates(t, listed, map[string]int{"task:complete": 1, "task:init": 0, "task:failed": 0})
}

func assertPublicDispatchRoute(t *testing.T, events []factoryapi.FactoryEvent, transitionID, toPlaceID string) {
	t.Helper()
	var sawDispatch bool
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse {
			continue
		}
		payload, err := event.Payload.AsDispatchResponseEventPayload()
		if err != nil {
			t.Fatalf("decode dispatch response: %v", err)
		}
		sawDispatch = sawDispatch || payload.TransitionId == transitionID
	}
	if !sawDispatch {
		t.Fatalf("public events missing transition %s before terminal place %s", transitionID, toPlaceID)
	}
}

func assertWorkflowWorkStates(t *testing.T, listed factoryapi.ListWorkResponse, wants map[string]int) {
	t.Helper()
	for placeID, want := range wants {
		if got := support.CountWorkAtCustomerState(listed, placeID); got != want {
			t.Errorf("%s Work count = %d, want %d", placeID, got, want)
		}
	}
}
