package http_test

import (
	"encoding/json"
	"net/http"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func assertCapturedStopCause(t *testing.T, baseURL, id, want string) {
	t.Helper()
	shown := support.GetJSON[factoryapi.WorkerSessionObservation](t, baseURL+"/worker-sessions/"+id)
	if shown.TerminalCause == nil || string(*shown.TerminalCause) != want {
		t.Fatalf("terminalCause=%v, want %s", shown.TerminalCause, want)
	}
}

// Production root composition, HTTP and real capture store; only the provider
// command is controlled. Each isolated fixture owns its direct Worker,
// readiness gate, journal, profile and terminal callback.
func TestExactStopTerminateReportsCommittedCause(t *testing.T) {
	t.Parallel()
	runner := newFunctionalWorkerGate(make(chan struct{}))
	server := startDirectWorkerSessionServer(t, runner)
	start := postDirectWorkerSession(t, t.Context(), server.URL(), "cause-request", "cause-worker", "cause-attempt")
	_ = start.Body.Close()
	if start.StatusCode != http.StatusAccepted {
		t.Fatalf("admission status=%d", start.StatusCode)
	}
	runner.waitStarted(t)
	live := support.GetJSON[factoryapi.WorkerSessionObservation](t, server.URL()+"/worker-sessions/cause-worker")
	if live.TerminalCause != nil {
		t.Fatal("live Worker has terminalCause")
	}
	stop := postWorkerSessionControl(t, server.URL(), "cause-worker", "terminate")
	_ = stop.Body.Close()
	if stop.StatusCode != http.StatusOK {
		t.Fatalf("terminate status=%d", stop.StatusCode)
	}
	runner.waitCanceled(t)
	assertCapturedStopCause(t, server.URL(), "cause-worker", "OPERATOR_TERMINATE")
	inputs := support.FakeInputs(t.Context(), []string{"you", "--server", server.URL(), "worker-sessions", "show", "--worker-session-id", "cause-worker", "--output", "json"})
	if err := server.Execute(t, inputs.Input); err != nil {
		t.Fatalf("CLI summary: %v %s", err, inputs.Stderr())
	}
	var cli factoryapi.WorkerSessionObservation
	if err := json.Unmarshal([]byte(inputs.Stdout()), &cli); err != nil || cli.TerminalCause == nil || string(*cli.TerminalCause) != "OPERATOR_TERMINATE" {
		t.Fatalf("CLI terminal cause = %v, %v", cli.TerminalCause, err)
	}
	repeat := postWorkerSessionControl(t, server.URL(), "cause-worker", "cancel")
	_ = repeat.Body.Close()
	if repeat.StatusCode != http.StatusOK {
		t.Fatalf("terminal repeat status=%d", repeat.StatusCode)
	}
	assertCapturedStopCause(t, server.URL(), "cause-worker", "OPERATOR_TERMINATE")
}
