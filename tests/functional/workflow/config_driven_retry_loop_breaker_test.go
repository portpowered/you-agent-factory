package workflow

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestConfigDrivenUnrecognizedProviderRefusalFailsOnce proves an unknown provider refusal does not enter the retry loop.
func TestConfigDrivenUnrecognizedProviderRefusalFailsOnce(t *testing.T) {
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
	_, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(
		t,
		dir,
		serviceedges.Edges{ProviderCommandRunner: runner},
		15*time.Second,
	)

	assertWorkflowSessionPlaces(t, listed, map[string]int{
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

func TestConfigDrivenRetryLoopBreaker_TerminatesAfterMaxRetries(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "retry_exhaustion"))

	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "Will exhaust retries"}`))

	provider := testutil.NewMockProvider(
		workerexecution.InferenceResponse{Content: "Processed. COMPLETE"},
		workerexecution.InferenceResponse{Content: "Needs work"},
		workerexecution.InferenceResponse{Content: "Processed. COMPLETE"},
		workerexecution.InferenceResponse{Content: "Still needs work"},
		workerexecution.InferenceResponse{Content: "Processed. COMPLETE"},
		workerexecution.InferenceResponse{Content: "Not good enough"},
	)

	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir,
		Edges: serviceedges.Edges{
			ProviderOverride: provider,
		},
	})
	terminalObservation := support.OpenDefaultSessionTerminalFactoryEventObservation(t, server.URL())
	support.WaitForSessionWorkTerminalFromFactoryEvents(t, server.URL(), "~default", 15*time.Second)
	listed := support.ListDefaultSessionWork(t, server.URL())
	assertWorkflowSessionPlaces(t, listed, map[string]int{
		"task:failed": 1, "task:init": 0, "task:in-review": 0, "task:complete": 0,
	})

	if provider.CallCount() != 6 {
		t.Errorf("expected provider called 6 times, got %d", provider.CallCount())
	}

	assertPublicDispatchRoute(t, server.GetFactoryEvents(t), "review-exhaustion", "task:failed")
	server.Stop(t)
	terminalObservation.Wait(15 * time.Second)
}

func TestConfigDrivenRetryLoopBreaker_SucceedsBeforeLimit(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "retry_exhaustion"))

	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "Will succeed on second try"}`))

	provider := testutil.NewMockProvider(
		workerexecution.InferenceResponse{Content: "Processed. COMPLETE"},
		workerexecution.InferenceResponse{Content: "Needs work"},
		workerexecution.InferenceResponse{Content: "Processed. COMPLETE"},
		workerexecution.InferenceResponse{Content: "Looks good. ACCEPTED"},
	)

	_, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{ProviderOverride: provider}, 15*time.Second)
	assertWorkflowSessionPlaces(t, listed, map[string]int{"task:complete": 1, "task:init": 0, "task:failed": 0})
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

func assertWorkflowSessionPlaces(t *testing.T, listed factoryapi.ListWorkResponse, wants map[string]int) {
	t.Helper()
	for placeID, want := range wants {
		if got := support.CountWorkAtCustomerState(listed, placeID); got != want {
			t.Errorf("%s token count = %d, want %d", placeID, got, want)
		}
	}
}
