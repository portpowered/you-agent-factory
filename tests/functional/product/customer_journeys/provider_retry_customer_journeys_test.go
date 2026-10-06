package customer_journeys_test

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestProviderRetryRecoveryJourneys(t *testing.T) {
	t.Parallel()
	host := support.NewFactorySessionHost(t)
	t.Run("ConfigDrivenUnrecognizedProviderRefusalFailsOnce", func(t *testing.T) { runUnrecognizedProviderRefusal(t, host) })
	t.Run("ConfigDrivenRetryLoopBreakerTerminatesAfterMaxRetries", func(t *testing.T) { runRetryExhaustion(t, host) })
	t.Run("ConfigDrivenRetryLoopBreakerSucceedsBeforeLimit", func(t *testing.T) { runRetryRecovery(t, host) })
}

func runUnrecognizedProviderRefusal(t *testing.T, host *support.FactorySessionHost) {
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

func runRetryExhaustion(t *testing.T, host *support.FactorySessionHost) {
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

func runRetryRecovery(t *testing.T, host *support.FactorySessionHost) {
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
