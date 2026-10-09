package customer_commands_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
)

// TestCLIHelpJourneys owns its immutable fixture until all customer scenarios finish.
func TestCLIHelpJourneys(t *testing.T) {
	t.Parallel()
	initializeHelpFixture(t)
	t.Run("WorkerSessionHelpAndUsage", testWorkerSessionHelpAndUsage)
	t.Run("TestCLIRunHelpShowsInvocationSignatureForNamedFactory", testHelpCLIRunHelpShowsInvocationSignatureForNamedFactory)
	t.Run("TestCLIRunHelpDistinguishesRequiredAndOptionalParameters", testHelpCLIRunHelpDistinguishesRequiredAndOptionalParameters)
	t.Run("TestCLIRunHelpDoesNotDispatchExternalWork", testHelpCLIRunHelpDoesNotDispatchExternalWork)
	t.Run("TestCLISessionHelpPublishesRunnablePlacementExamples", testHelpCLISessionHelpPublishesRunnablePlacementExamples)
	t.Run("TestCLIRunHelpCoversGenericAndExplicitFactorySelections", testHelpCLIRunHelpCoversGenericAndExplicitFactorySelections)
	t.Run("TestCLIRunHelpResetsEmptyAndInvalidSelections", testHelpCLIRunHelpResetsEmptyAndInvalidSelections)
}

func testWorkerSessionHelpAndUsage(t *testing.T) {
	t.Parallel()
	fixture := helpPackageFixtureForTest(t)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(server.Close)
	for _, args := range [][]string{
		{"--server", server.URL, "worker-sessions", "--help"},
		{"worker-sessions", "--help", "--server", server.URL},
		{"worker-sessions", "--server", server.URL, "-h"},
		{"worker-sessions"}, {"help", "worker-sessions"},
		{"worker-sessions", "invoke", "--help"},
		{"--server", server.URL, "worker-sessions", "missing"},
		{"worker-sessions", "missing", "--server", server.URL},
	} {
		result := fixture.execute(t, append([]string{"you"}, args...)...)
		if strings.Contains(strings.Join(args, " "), "missing") {
			var usage *clidiag.UsageError
			if !errors.As(result.err, &usage) || !strings.Contains(usage.Error(), `unknown command "missing"`) {
				t.Fatalf("%q: expected typed actual-token usage error, got %v", args, result.err)
			}
			if result.inputs.Stdout() != "" || !strings.Contains(result.inputs.Stderr(), `unknown command "missing"`) || !strings.Contains(result.inputs.Stderr(), "--help") {
				t.Fatalf("usage streams: stdout=%q stderr=%q", result.inputs.Stdout(), result.inputs.Stderr())
			}
		} else if result.err != nil || strings.Count(result.inputs.Stdout(), "Usage:") != 1 || result.inputs.Stderr() != "" {
			t.Fatalf("%q: err=%v stdout=%q stderr=%q", args, result.err, result.inputs.Stdout(), result.inputs.Stderr())
		}
	}
	if calls.Load() != 0 || fixture.providerRunner.CallCount() != 0 || fixture.hostStarts.Load() != 0 {
		t.Fatalf("help/usage external effects: HTTP=%d provider=%d host-start=%d", calls.Load(), fixture.providerRunner.CallCount(), fixture.hostStarts.Load())
	}
}
