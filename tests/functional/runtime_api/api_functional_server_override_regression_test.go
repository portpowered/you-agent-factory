package runtime_api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	factoryapi "github.com/portpowered/agent-factory/pkg/api/generated"
	"github.com/portpowered/agent-factory/pkg/factory"
	"github.com/portpowered/agent-factory/pkg/factory/state"
	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/petri"
	"github.com/portpowered/agent-factory/pkg/service"
	"github.com/portpowered/agent-factory/pkg/testutil"
	"github.com/portpowered/agent-factory/pkg/workers"
	"github.com/portpowered/agent-factory/tests/functional/internal/support"
)

type functionalStateCategories struct {
	Failed     int
	Initial    int
	Processing int
	Terminal   int
}

func TestFunctionalServerOverrideCompatibilityRegression_MockWorkersAndProviderOverride(t *testing.T) {
	t.Run("StartFunctionalServerMockWorkersCompletes", func(t *testing.T) {
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "service_simple"))
		testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"mock worker compatibility"}`))

		server := startFunctionalServer(t, dir, true)
		snapshot := waitForFunctionalServerCompletion(t, server, 10*time.Second)
		categories := categorizeFunctionalState(snapshot)

		if categories.Terminal != 1 {
			t.Fatalf("terminal token count = %d, want 1", categories.Terminal)
		}
		if categories.Failed != 0 {
			t.Fatalf("failed token count = %d, want 0", categories.Failed)
		}
	})

	t.Run("ProviderOverrideIsAppliedBeforeServiceBuildForHTTPRuntime", func(t *testing.T) {
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "service_simple"))
		writeAgentConfig(t, dir, "worker-a", buildModelWorkerConfig(workers.ModelProviderCodex, "gpt-5-codex"))
		writeAgentConfig(t, dir, "worker-b", buildModelWorkerConfig(workers.ModelProviderCodex, "gpt-5-codex"))

		runner := testutil.NewProviderCommandRunner(
			workers.CommandResult{Stdout: []byte("first runtime step complete. COMPLETE")},
			workers.CommandResult{Stdout: []byte("second runtime step complete. COMPLETE")},
		)
		server := startFunctionalServerWithConfig(
			t,
			dir,
			false,
			func(cfg *service.FactoryServiceConfig) {
				cfg.ProviderCommandRunnerOverride = runner
			},
			factory.WithServiceMode(),
		)

		traceID := submitFunctionalServerWork(t, server, "task", []byte(`{"title":"provider override regression"}`))
		if traceID == "" {
			t.Fatal("expected POST /work to return a trace ID")
		}

		snapshot := waitForFunctionalServerIdleTerminal(t, server, 10*time.Second)
		categories := categorizeFunctionalState(snapshot)
		if categories.Failed != 0 {
			t.Fatalf("failed token count = %d, want 0", categories.Failed)
		}

		if got := runner.CallCount(); got != 2 {
			t.Fatalf("provider command runner calls = %d, want 2", got)
		}
		for i, req := range runner.Requests() {
			if req.Command != string(workers.ModelProviderCodex) {
				t.Fatalf("provider request %d command = %q, want %q", i, req.Command, workers.ModelProviderCodex)
			}
			if req.Execution.TraceID != traceID {
				t.Fatalf("provider request %d trace ID = %q, want %q", i, req.Execution.TraceID, traceID)
			}
		}
	})
}

func submitFunctionalServerWork(t *testing.T, server *functionalAPIServer, workTypeID string, payload []byte) string {
	t.Helper()

	reqBody, err := json.Marshal(factoryapi.SubmitWorkRequest{
		WorkTypeName: workTypeID,
		Payload:      payload,
	})
	if err != nil {
		t.Fatalf("marshal submit request: %v", err)
	}

	resp, err := http.Post(server.URL()+"/work", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("POST /work: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /work: expected 201 Created, got %d", resp.StatusCode)
	}

	var result factoryapi.SubmitWorkResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode submit response: %v", err)
	}
	return result.TraceId
}

func waitForFunctionalServerCompletion(
	t *testing.T,
	server *functionalAPIServer,
	timeout time.Duration,
) *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net] {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		snapshot := server.GetEngineStateSnapshot(t)
		if snapshot.FactoryState == string(interfaces.FactoryStateCompleted) {
			return snapshot
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("factory did not reach COMPLETED within %s", timeout)
	return nil
}

func waitForFunctionalServerIdleTerminal(
	t *testing.T,
	server *functionalAPIServer,
	timeout time.Duration,
) *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net] {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		snapshot := server.GetEngineStateSnapshot(t)
		categories := categorizeFunctionalState(snapshot)
		if snapshot.FactoryState == string(interfaces.FactoryStateRunning) &&
			snapshot.RuntimeStatus == interfaces.RuntimeStatusIdle &&
			categories.Terminal == 1 {
			return snapshot
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("factory did not reach running idle terminal state within %s", timeout)
	return nil
}

func buildModelWorkerConfig(provider workers.ModelProvider, model string) string {
	return fmt.Sprintf(`---
type: MODEL_WORKER
model: %s
modelProvider: %s
stopToken: COMPLETE
---
Process the input task.
`, model, provider)
}

func categorizeFunctionalState(snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) functionalStateCategories {
	var categories functionalStateCategories
	for _, token := range snapshot.Marking.Tokens {
		if token == nil || token.Color.WorkTypeID == "" {
			continue
		}
		switch lookupFunctionalStateCategory(snapshot.Topology, token.PlaceID) {
		case state.StateCategoryFailed:
			categories.Failed++
		case state.StateCategoryTerminal:
			categories.Terminal++
		case state.StateCategoryInitial:
			categories.Initial++
		default:
			categories.Processing++
		}
	}
	return categories
}

func lookupFunctionalStateCategory(net *state.Net, placeID string) state.StateCategory {
	if net == nil {
		return state.StateCategoryProcessing
	}
	place, ok := net.Places[placeID]
	if !ok {
		return state.StateCategoryProcessing
	}
	workType, ok := net.WorkTypes[place.TypeID]
	if !ok {
		return state.StateCategoryProcessing
	}
	for _, stateConfig := range workType.States {
		if stateConfig.Value == place.State {
			return stateConfig.Category
		}
	}
	return state.StateCategoryProcessing
}
