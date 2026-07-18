package runtime_api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	modelprovider "github.com/portpowered/infinite-you/pkg/models/provider"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/wire"
	"github.com/portpowered/infinite-you/pkg/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestServiceModeSmoke_StaysPubliclyReachableAcrossIdleWorkAndCancellation(t *testing.T) {
	support.SkipLongFunctional(t, "slow service-mode lifecycle smoke")
	runner := newServiceModeGatedCommandRunner()
	host, stream := startServiceModeRootRunHost(t, runner)

	assertServiceModeStatus(t, host, interfaces.RuntimeStatusIdle, 0)
	traceID := submitGeneratedWork(t, host.Endpoint(), factoryapi.SubmitWorkRequest{
		Name:         "service-mode-smoke-item",
		WorkTypeName: "task",
		Payload:      json.RawMessage(`{"title":"service-mode smoke item"}`),
	})

	runner.waitForFirstDispatch(t)
	assertDispatchRequestForTrace(t, stream, traceID, "step-one")
	assertServiceModeStatus(t, host, interfaces.RuntimeStatusActive, -1)
	assertServiceModeWorkIsNotTerminal(t, host.Endpoint(), traceID)

	close(runner.releaseFirstDispatch)
	assertAcceptedDispatchesForTrace(t, stream, traceID, "step-one", "step-two")
	assertTerminalWorkerOverrideWork(t, host.Endpoint(), traceID, "complete")
	assertServiceModeStatus(t, host, interfaces.RuntimeStatusIdle, 1)

	// A second supported read proves the continuous process remains reachable
	// after returning to idle, until the customer cancels it explicitly.
	assertServiceModeStatus(t, host, interfaces.RuntimeStatusIdle, 1)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := host.Shutdown(shutdownCtx)
	if err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	if result.Outcome != support.RootRunProcessStopped || result.ExitCode != 0 {
		t.Fatalf("Shutdown() result = %#v, want stopped exit 0", result)
	}
}

func startServiceModeRootRunHost(
	t *testing.T,
	runner workers.CommandRunner,
) (*support.RootRunFunctionalHost, *factoryEventHTTPStream) {
	t.Helper()
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "service_simple"))
	support.WriteAgentConfig(t, dir, "worker-a", support.BuildModelWorkerConfig(modelprovider.Codex, "gpt-5-codex"))
	support.WriteAgentConfig(t, dir, "worker-b", support.BuildModelWorkerConfig(modelprovider.Codex, "gpt-5-codex"))

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
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, shutdownErr := host.Shutdown(cleanupCtx); shutdownErr != nil {
			t.Errorf("cleanup Shutdown() error = %v", shutdownErr)
		}
	})

	stream := openRootRunFactoryEventHTTPStream(t, host)
	requireFunctionalEventStreamPrelude(t, stream)
	return host, stream
}

type serviceModeGatedCommandRunner struct {
	firstDispatchStarted chan struct{}
	releaseFirstDispatch chan struct{}
}

func newServiceModeGatedCommandRunner() *serviceModeGatedCommandRunner {
	return &serviceModeGatedCommandRunner{
		firstDispatchStarted: make(chan struct{}),
		releaseFirstDispatch: make(chan struct{}),
	}
}

func (r *serviceModeGatedCommandRunner) Run(ctx context.Context, request workers.CommandRequest) (workers.CommandResult, error) {
	if request.WorkstationName == "step-one" {
		close(r.firstDispatchStarted)
		select {
		case <-r.releaseFirstDispatch:
		case <-ctx.Done():
			return workers.CommandResult{}, ctx.Err()
		}
	}
	return workers.CommandResult{Stdout: []byte("service-mode step complete. COMPLETE")}, nil
}

func (r *serviceModeGatedCommandRunner) waitForFirstDispatch(t *testing.T) {
	t.Helper()
	select {
	case <-r.firstDispatchStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the first service-mode dispatch")
	}
}

func assertDispatchRequestForTrace(
	t *testing.T,
	stream *factoryEventHTTPStream,
	traceID string,
	wantTransition string,
) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		event := stream.next(time.Until(deadline))
		if event.Type != factoryapi.FactoryEventTypeDispatchRequest || !publicEventHasTrace(event, traceID) {
			continue
		}
		payload, err := event.Payload.AsDispatchRequestEventPayload()
		if err != nil {
			t.Fatalf("decode DISPATCH_REQUEST: %v", err)
		}
		if payload.TransitionId != wantTransition {
			t.Fatalf("DISPATCH_REQUEST transition = %q, want %q", payload.TransitionId, wantTransition)
		}
		return
	}
	t.Fatalf("canonical session stream did not expose DISPATCH_REQUEST for trace %q", traceID)
}

func publicEventHasTrace(event factoryapi.FactoryEvent, traceID string) bool {
	if event.Context.TraceIds == nil {
		return false
	}
	for _, candidate := range *event.Context.TraceIds {
		if candidate == traceID {
			return true
		}
	}
	return false
}

func assertServiceModeStatus(
	t *testing.T,
	host *support.RootRunFunctionalHost,
	wantRuntimeStatus interfaces.RuntimeStatus,
	wantTerminal int,
) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	response, err := host.REST().GetStatus(ctx)
	if err != nil {
		t.Fatalf("generated GetStatus() error = %v", err)
	}
	if response.StatusCode() != http.StatusOK || response.JSON200 == nil {
		t.Fatalf("generated GetStatus() response = %#v, want typed 200", response)
	}
	if response.JSON200.FactoryState != "RUNNING" || response.JSON200.RuntimeStatus != string(wantRuntimeStatus) {
		t.Fatalf("generated status state = %q runtime = %q, want RUNNING/%s", response.JSON200.FactoryState, response.JSON200.RuntimeStatus, wantRuntimeStatus)
	}
	if wantTerminal >= 0 && response.JSON200.Categories.Terminal != wantTerminal {
		t.Fatalf("generated status terminal count = %d, want %d", response.JSON200.Categories.Terminal, wantTerminal)
	}
}

func assertServiceModeWorkIsNotTerminal(t *testing.T, endpoint, traceID string) {
	t.Helper()
	work := getGeneratedJSON[factoryapi.ListWorkResponse](t, support.DefaultSessionWorkURL(endpoint, "/work"))
	item := requireGeneratedWorkByTrace(t, work, traceID)
	if generatedWorkStateType(item.State) == factoryapi.WorkStateTypeTERMINAL {
		t.Fatalf("GET /work exposed terminal state while first dispatch was blocked: %#v", item.State)
	}
}
