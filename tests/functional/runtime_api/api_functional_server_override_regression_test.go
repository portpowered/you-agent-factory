package runtime_api

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	modelprovider "github.com/portpowered/infinite-you/pkg/models/provider"
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
		assertTerminalWorkerOverrideWork(t, host.Endpoint(), traceID, "complete")
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
		assertTerminalWorkerOverrideWork(t, host.Endpoint(), traceID, "complete")
	})

	t.Run("ScriptOverrideCompletes", func(t *testing.T) {
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))
		runner := support.NewRecordingCommandRunner("script alignment output")
		host, stream := startWorkerOverrideRootRunHost(t, dir, true, wire.FunctionalEdges{
			ScriptCommandRunner: runner,
		})

		traceID := submitGeneratedWork(t, host.Endpoint(), factoryapi.SubmitWorkRequest{
			Name:         "script-override-regression",
			WorkTypeName: "task",
			Payload:      json.RawMessage(`"script override regression"`),
		})
		assertAcceptedDispatchesForTrace(t, stream, traceID, "run-script")
		assertTerminalWorkerOverrideWork(t, host.Endpoint(), traceID, "done")
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

func assertTerminalWorkerOverrideWork(t *testing.T, endpoint string, traceID string, wantState string) {
	t.Helper()

	work := getGeneratedJSON[factoryapi.ListWorkResponse](t, support.DefaultSessionWorkURL(endpoint, "/work"))
	completed := requireGeneratedWorkByTrace(t, work, traceID)
	if generatedWorkStateName(completed.State) != wantState || generatedWorkStateType(completed.State) != factoryapi.WorkStateTypeTERMINAL {
		t.Fatalf("GET /work state = %#v, want %s/TERMINAL", completed.State, wantState)
	}
}
