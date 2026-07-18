package runtime_api

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	modelprovider "github.com/portpowered/infinite-you/pkg/models/provider"
	"github.com/portpowered/infinite-you/pkg/orchestrators/petri"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/wire"
	"github.com/portpowered/infinite-you/pkg/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestRootRunFunctionalHostWorkerOverridesCompleteThroughPublicRuntimeAPI(t *testing.T) {
	support.SkipLongFunctional(t, "slow root-run worker override sweep")

	t.Run("MockWorkersComplete", func(t *testing.T) {
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "service_simple"))
		host, stream := startWorkerOverrideRootRunHost(t, dir, false, wire.FunctionalEdges{})

		traceID := submitGeneratedWork(t, host.Endpoint(), factoryapi.SubmitWorkRequest{
			Name:         "mock-worker-completion",
			WorkTypeName: "task",
			Payload:      json.RawMessage(`{"title":"mock worker compatibility"}`),
		})
		assertAcceptedDispatchesForTrace(t, stream, traceID, "step-one", "step-two")
		assertCompletedWorkerOverrideWork(t, host.Endpoint(), traceID)
	})

	t.Run("ProviderOverrideCompletes", func(t *testing.T) {
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "service_simple"))
		support.WriteAgentConfig(t, dir, "worker-a", support.BuildModelWorkerConfig(modelprovider.Codex, "gpt-5-codex"))
		support.WriteAgentConfig(t, dir, "worker-b", support.BuildModelWorkerConfig(modelprovider.Codex, "gpt-5-codex"))

		runner := testutil.NewProviderCommandRunner(
			workers.CommandResult{Stdout: []byte("first runtime step complete. COMPLETE")},
			workers.CommandResult{Stdout: []byte("second runtime step complete. COMPLETE")},
		)
		host, stream := startWorkerOverrideRootRunHost(t, dir, true, wire.FunctionalEdges{
			ProviderCommandRunner: runner,
		})

		traceID := submitGeneratedWork(t, host.Endpoint(), factoryapi.SubmitWorkRequest{
			Name:         "provider-override-regression",
			WorkTypeName: "task",
			Payload:      json.RawMessage(`{"title":"provider override regression"}`),
		})
		assertAcceptedDispatchesForTrace(t, stream, traceID, "step-one", "step-two")
		assertCompletedWorkerOverrideWork(t, host.Endpoint(), traceID)
	})
}

func startWorkerOverrideRootRunHost(
	t *testing.T,
	factoryRoot string,
	disableMockWorkers bool,
	edges wire.FunctionalEdges,
) (*support.RootRunFunctionalHost, *factoryEventHTTPStream) {
	t.Helper()

	host, err := support.StartRootRunFunctionalHost(context.Background(), support.RootRunFunctionalHostConfig{
		FactoryRoot:        factoryRoot,
		SystemRoot:         t.TempDir(),
		DisableMockWorkers: disableMockWorkers,
		FunctionalEdges:    edges,
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
	return host, stream
}

func assertAcceptedDispatchesForTrace(
	t *testing.T,
	stream *factoryEventHTTPStream,
	traceID string,
	wantTransitions ...string,
) {
	t.Helper()

	for index, wantTransition := range wantTransitions {
		event := terminalDispatchForTrace(t, stream, traceID)
		payload, err := event.Payload.AsDispatchResponseEventPayload()
		if err != nil {
			t.Fatalf("decode DISPATCH_RESPONSE %d: %v", index, err)
		}
		if payload.TransitionId != wantTransition || payload.Outcome != factoryapi.WorkOutcomeAccepted {
			t.Fatalf(
				"DISPATCH_RESPONSE %d = transition %q outcome %q, want transition %q outcome ACCEPTED",
				index,
				payload.TransitionId,
				payload.Outcome,
				wantTransition,
			)
		}
	}
}

func assertCompletedWorkerOverrideWork(t *testing.T, endpoint string, traceID string) {
	t.Helper()

	work := waitForGeneratedWorkComplete(t, endpoint, traceID, 10*time.Second)
	completed := requireGeneratedWorkByTrace(t, work, traceID)
	if generatedWorkStateName(completed.State) != "complete" || generatedWorkStateType(completed.State) != factoryapi.WorkStateTypeTERMINAL {
		t.Fatalf("GET /work state = %#v, want complete/TERMINAL", completed.State)
	}
}

// These legacy snapshot helpers remain for api_service_config_override_alignment_test.go
// until those construction-focused scenarios move to their owner package.
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

type functionalStateCategories struct {
	Failed     int
	Initial    int
	Processing int
	Terminal   int
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
