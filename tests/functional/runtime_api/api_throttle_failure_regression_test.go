package runtime_api

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestRetryableThrottleFailureWithoutGuardUsesDefaultRetryLimitAndFailsWork(t *testing.T) {
	dir := support.ScaffoldFactory(t, retryableFailureFactoryConfig())
	support.WriteAgentConfig(t, dir, "worker-a", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))

	runner := testutil.NewProviderCommandRunner(repeatedThrottleCommandResults(12)...)
	server := startFunctionalServerWithArgs(t, dir, false, nil,
		withWorkerCommands(runner, nil))

	server.SubmitRuntimeWork(t, work.SubmitRequest{
		Name:       "throttled-task",
		WorkTypeID: "task",
		Payload:    json.RawMessage(`{"title":"force throttle exhaustion"}`),
	})

	status := waitForThrottledFailureExhaustion(t, server, 10*time.Second)
	if status.Categories.Failed != 1 {
		t.Fatalf("failed token count = %d, want 1", status.Categories.Failed)
	}
	if status.Categories.Initial != 0 || status.Categories.Processing != 0 || status.Categories.Terminal != 0 {
		t.Fatalf(
			"non-failed token counts = initial:%d processing:%d terminal:%d, want all 0",
			status.Categories.Initial,
			status.Categories.Processing,
			status.Categories.Terminal,
		)
	}
	wantProviderCalls := 3 * 3
	if runner.CallCount() != wantProviderCalls {
		t.Fatalf("provider command runner calls = %d, want %d from default workstation retries and provider retries", runner.CallCount(), wantProviderCalls)
	}

	assertNoDanglingDispatchCreatedEvents(t, server.GetFactoryEvents(t))
}

func retryableFailureFactoryConfig() map[string]any {
	cfg := simplePipelineConfig()
	workstations := cfg["workstations"].([]map[string]any)
	workstations[0]["type"] = "MODEL_WORKSTATION"
	cfg["workstations"] = workstations
	return cfg
}

func repeatedThrottleCommandResults(count int) []platformprocess.CommandResult {
	results := make([]platformprocess.CommandResult, count)
	for i := range results {
		results[i] = platformprocess.CommandResult{
			ExitCode: 1,
			Stderr:   []byte("ERROR: selected model is at capacity"),
		}
	}
	return results
}

func waitForThrottledFailureExhaustion(
	t *testing.T,
	server *functionalAPIServer,
	timeout time.Duration,
) generated.StatusResponse {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		status := support.GetJSON[generated.StatusResponse](t, server.URL()+"/status")
		if status.RuntimeStatus == string(interfaces.RuntimeStatusIdle) &&
			status.Categories.Failed == 1 {
			return status
		}
		time.Sleep(50 * time.Millisecond)
	}

	status := support.GetJSON[generated.StatusResponse](t, server.URL()+"/status")
	t.Fatalf(
		"timed out waiting for throttled work to fail; runtime_status=%s factory_state=%s categories=%+v",
		status.RuntimeStatus,
		status.FactoryState,
		status.Categories,
	)
	return generated.StatusResponse{}
}

func assertNoDanglingDispatchCreatedEvents(t *testing.T, events []generated.FactoryEvent) {
	t.Helper()

	created := make(map[string]struct{})
	completed := make(map[string]struct{})
	for _, event := range events {
		dispatchID := event.Context.DispatchId
		if dispatchID == nil || *dispatchID == "" {
			continue
		}
		switch {
		case strings.HasPrefix(event.Id, "factory-event/dispatch-created/"):
			created[*dispatchID] = struct{}{}
		case strings.HasPrefix(event.Id, "factory-event/dispatch-completed/"):
			completed[*dispatchID] = struct{}{}
		}
	}

	for dispatchID := range created {
		if _, ok := completed[dispatchID]; !ok {
			t.Fatalf("dispatch %s has dispatch-created event without dispatch-completed event", dispatchID)
		}
	}
}
