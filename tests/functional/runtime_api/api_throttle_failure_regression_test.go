package runtime_api

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	modelprovider "github.com/portpowered/infinite-you/pkg/models/provider"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/wire"
	"github.com/portpowered/infinite-you/pkg/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestRetryableThrottleFailureWithoutGuardUsesDefaultRetryLimitAndFailsWork(t *testing.T) {
	dir := support.ScaffoldFactory(t, retryableFailureFactoryConfig())
	support.WriteAgentConfig(t, dir, "worker-a", support.BuildModelWorkerConfig(modelprovider.Codex, "gpt-5-codex"))

	runner := testutil.NewProviderCommandRunner(repeatedThrottleCommandResults(12)...)
	host, err := support.StartRootRunFunctionalHost(context.Background(), support.RootRunFunctionalHostConfig{
		FactoryRoot:        dir,
		SystemRoot:         t.TempDir(),
		DisableMockWorkers: true,
		FunctionalEdges: wire.FunctionalEdges{
			ProviderCommandRunner: runner,
		},
	})
	if err != nil {
		t.Fatalf("StartRootRunFunctionalHost() error = %v", err)
	}
	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, shutdownErr := host.Shutdown(shutdownCtx); shutdownErr != nil {
			t.Errorf("Shutdown() error = %v", shutdownErr)
		}
	})

	stream := openRootRunFactoryEventHTTPStream(t, host)
	requireFunctionalEventStreamPrelude(t, stream)
	traceID := submitGeneratedWork(t, host.Endpoint(), factoryapi.SubmitWorkRequest{
		Name:         "throttled-task",
		WorkTypeName: "task",
		Payload:      json.RawMessage(`{"title":"force throttle exhaustion"}`),
	})

	terminalEvent := terminalDispatchForTrace(t, stream, traceID)
	terminalPayload, err := terminalEvent.Payload.AsDispatchResponseEventPayload()
	if err != nil {
		t.Fatalf("decode terminal DISPATCH_RESPONSE payload: %v", err)
	}
	if terminalPayload.Outcome != factoryapi.WorkOutcomeFailed {
		t.Fatalf("terminal DISPATCH_RESPONSE outcome = %q, want FAILED", terminalPayload.Outcome)
	}
	if terminalPayload.Error == nil || !strings.Contains(*terminalPayload.Error, "temporarily unavailable") {
		t.Fatalf("terminal DISPATCH_RESPONSE error = %q, want safe throttle diagnostic", support.StringPointerValue(terminalPayload.Error))
	}

	failed := waitForGeneratedFailedWorkByTrace(t, host.Endpoint(), traceID, 10*time.Second)
	if generatedWorkStateName(failed.State) != "failed" || generatedWorkStateType(failed.State) != factoryapi.WorkStateTypeFAILED {
		t.Fatalf("GET /work state = %#v, want failed/FAILED", failed.State)
	}
	if generatedWorkStateName(failed.State) == "complete" {
		t.Fatalf("GET /work state = %#v, must not expose successful completion after throttle exhaustion", failed.State)
	}
}

func waitForGeneratedFailedWorkByTrace(t *testing.T, baseURL, traceID string, timeout time.Duration) factoryapi.Work {
	t.Helper()

	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for time.Now().Before(deadline) {
		work := getGeneratedJSON[factoryapi.ListWorkResponse](t, support.DefaultSessionWorkURL(baseURL, "/work"))
		for _, item := range work.Results {
			if support.StringPointerValue(item.TraceId) == traceID && generatedWorkStateName(item.State) == "failed" {
				return item
			}
		}
		<-ticker.C
	}

	work := getGeneratedJSON[factoryapi.ListWorkResponse](t, support.DefaultSessionWorkURL(baseURL, "/work"))
	t.Fatalf("timed out waiting for GET /work failure for trace %q; last response: %#v", traceID, work)
	return factoryapi.Work{}
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
