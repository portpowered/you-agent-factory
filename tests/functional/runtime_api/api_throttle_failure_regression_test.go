package runtime_api

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/factory"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	modelprovider "github.com/portpowered/infinite-you/pkg/models/provider"
	"github.com/portpowered/infinite-you/pkg/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/work"
	"github.com/portpowered/infinite-you/pkg/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestRetryableThrottleFailureWithoutGuardUsesDefaultRetryLimitAndFailsWork(t *testing.T) {
	dir := support.ScaffoldFactory(t, retryableFailureFactoryConfig())
	support.WriteAgentConfig(t, dir, "worker-a", support.BuildModelWorkerConfig(modelprovider.Codex, "gpt-5-codex"))

	runner := testutil.NewProviderCommandRunner(repeatedThrottleCommandResults(12)...)
	server := startFunctionalServerWithConfig(t, dir, false, func(cfg *service.FactoryServiceConfig) {
		support.ConfigureWorkerCommands(t, cfg, runner, nil)
	}, factory.WithServiceMode())

	server.SubmitRuntimeWork(t, work.SubmitRequest{
		Name:       "throttled-task",
		WorkTypeID: "task",
		Payload:    json.RawMessage(`{"title":"force throttle exhaustion"}`),
	})

	snapshot := waitForThrottledFailureExhaustion(t, server, 10*time.Second)
	categories := categorizeFunctionalState(snapshot)
	if categories.Failed != 1 {
		t.Fatalf("failed token count = %d, want 1", categories.Failed)
	}
	if categories.Initial != 0 || categories.Processing != 0 || categories.Terminal != 0 {
		t.Fatalf("non-failed token counts = initial:%d processing:%d terminal:%d, want all 0", categories.Initial, categories.Processing, categories.Terminal)
	}
	if snapshot.InFlightCount != 0 {
		t.Fatalf("in-flight dispatch count = %d, want 0", snapshot.InFlightCount)
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

func repeatedThrottleCommandResults(count int) []workers.CommandResult {
	results := make([]workers.CommandResult, count)
	for i := range results {
		results[i] = workers.CommandResult{
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
) *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net] {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		snapshot := server.GetEngineStateSnapshot(t)
		categories := categorizeFunctionalState(snapshot)
		if snapshot.RuntimeStatus == interfaces.RuntimeStatusIdle &&
			categories.Failed == 1 &&
			snapshot.InFlightCount == 0 {
			return snapshot
		}
		time.Sleep(50 * time.Millisecond)
	}

	snapshot := server.GetEngineStateSnapshot(t)
	categories := categorizeFunctionalState(snapshot)
	t.Fatalf(
		"timed out waiting for throttled work to fail; runtime_status=%s factory_state=%s active_dispatches=%d categories=%+v",
		snapshot.RuntimeStatus,
		snapshot.FactoryState,
		snapshot.InFlightCount,
		categories,
	)
	return nil
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
