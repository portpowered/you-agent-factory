package root_composition_test

import (
	"context"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// A distinct immutable failing transport edge needs its own process. Both
// attempts share that process; its default-session inventory is isolated from
// other parallel fixtures. Opening uses the CLI, and HTTP observes the public
// session inventory and controls rather than private runtime state.
func TestProcessGatewayStartFailureLeavesNoLiveSessionAndRetrySucceeds(t *testing.T) {
	t.Parallel()
	acquireRootCompositionFixtureSlot(t)
	failure := errors.New("injected gateway transport startup failure")
	router := &reusableRootAPIServerStarter{}
	listenerContexts := make(chan context.Context, 2)
	failedRequests := make(chan platformhttpserver.StartRequest, 1)
	var failed atomic.Bool
	process := support.BuildProcess(t, serviceedges.Edges{
		APIServerStarter: func(ctx context.Context, request platformhttpserver.StartRequest) error {
			listenerContexts <- ctx
			if failed.CompareAndSwap(false, true) {
				failedRequests <- request
				return failure
			}
			return router.start(ctx, request)
		},
	})
	support.CleanupProcess(t, process)
	dir := support.ScaffoldFactory(t, processExecuteRuntimeOpeningFactoryConfig())
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "run", "--factory", filepath.Join(dir, "factory.json"),
		"--continuously", "--with-server", "--quiet", "--no-record",
	})
	home := t.TempDir()
	inputs.Input.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	inputs.Input.WorkingDirectory = dir
	if err := process.Execute(inputs.Input); !errors.Is(err, failure) {
		t.Fatalf("failed startup error = %v, want injected cause; stdout=%q stderr=%q", err, inputs.Stdout(), inputs.Stderr())
	}
	assertGatewayListenerCancelled(t, <-listenerContexts)
	request := <-failedRequests
	api := httptest.NewServer(request.Handler)
	t.Cleanup(api.Close)
	assertGatewayLiveInventoryEmpty(t, api.URL)
	testProcessExecuteOpensRequestedFactorySessionThroughRoot(t, process, router)
	assertGatewayListenerCancelled(t, <-listenerContexts)
	assertGatewayLiveInventoryEmpty(t, api.URL)
}

func assertGatewayListenerCancelled(t *testing.T, ctx context.Context) {
	t.Helper()
	if !errors.Is(ctx.Err(), context.Canceled) {
		t.Fatalf("listener context error = %v, want cancellation before Execute joins", ctx.Err())
	}
}

func assertGatewayLiveInventoryEmpty(t *testing.T, baseURL string) {
	t.Helper()
	listed := support.GetJSON[factoryapi.ListFactorySessionsResponse](t, baseURL+"/factory-sessions?scope=live")
	if len(listed.Sessions) != 0 {
		t.Fatalf("live sessions after joined invocation = %#v, want no unpublished or retired session", listed.Sessions)
	}
}
