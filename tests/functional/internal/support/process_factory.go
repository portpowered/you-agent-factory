package support

import (
	"os"
	"strings"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	workerprovider "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// RunFactoryToCompletion executes the customer daemon command through the
// canonical root-built process and returns its public default-session
// projection after every visible work token becomes terminal.
func RunFactoryToCompletion(
	t testing.TB,
	dir string,
	provider workerprovider.Provider,
	timeout time.Duration,
) factoryapi.FactorySession {
	return RunFactoryToCompletionWithEdges(t, dir, serviceedges.Edges{
		ProviderOverride: provider,
	}, timeout)
}

// RunFactoryToCompletionWithEdges executes the same customer daemon while
// allowing a test to replace external process boundaries such as command
// execution.
func RunFactoryToCompletionWithEdges(
	t testing.TB,
	dir string,
	overrides serviceedges.Edges,
	timeout time.Duration,
) factoryapi.FactorySession {
	session, _, _, _ := runFactoryToCompletion(t, dir, overrides, timeout, false)
	return session
}

// RunFactoryToCompletionWithEdgesAndWork also returns the customer-visible
// Work listing captured before the daemon stops.
func RunFactoryToCompletionWithEdgesAndWork(
	t testing.TB,
	dir string,
	overrides serviceedges.Edges,
	timeout time.Duration,
) (factoryapi.FactorySession, factoryapi.ListWorkResponse) {
	session, work, _, _ := runFactoryToCompletion(t, dir, overrides, timeout, false)
	return session, work
}

// RunFactoryToCompletionWithEdgesAndObservations also returns the public Work
// listing and retained Factory Event history captured before the daemon stops.
func RunFactoryToCompletionWithEdgesAndObservations(
	t testing.TB,
	dir string,
	overrides serviceedges.Edges,
	timeout time.Duration,
) (factoryapi.FactorySession, factoryapi.ListWorkResponse, []factoryapi.FactoryEvent) {
	session, work, events, _ := runFactoryToCompletion(t, dir, overrides, timeout, false)
	return session, work, events
}

// RunFactoryToCompletionWithEdgesAndResponseEvents also reads the public
// ephemeral response-event stream before stopping the root-built process.
func RunFactoryToCompletionWithEdgesAndResponseEvents(
	t testing.TB,
	dir string,
	overrides serviceedges.Edges,
	timeout time.Duration,
) (
	factoryapi.FactorySession,
	factoryapi.ListWorkResponse,
	[]factoryapi.FactoryEvent,
	[]factoryapi.FactoryResponseEvent,
) {
	return runFactoryToCompletion(t, dir, overrides, timeout, true)
}

func runFactoryToCompletion(
	t testing.TB,
	dir string,
	overrides serviceedges.Edges,
	timeout time.Duration,
	captureResponseEvents bool,
) (
	factoryapi.FactorySession,
	factoryapi.ListWorkResponse,
	[]factoryapi.FactoryEvent,
	[]factoryapi.FactoryResponseEvent,
) {
	t.Helper()

	server := NewProcessAPIServer()
	overrides.APIServerStarter = server.Start
	process := BuildProcess(t, overrides)
	inputs := FakeInputs(t.Context(), []string{
		"you", "run",
		"--dir", dir,
		"--continuously",
		"--server", "http://127.0.0.1:1",
		"--quiet",
		"--no-record",
	})
	homeDir := t.TempDir()
	inputs.Input.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	inputs.Input.WorkingDirectory = dir
	t.Cleanup(func() {
		if !t.Failed() {
			return
		}
		if stderr := strings.TrimSpace(inputs.Stderr()); stderr != "" {
			t.Logf("daemon stderr:\n%s", stderr)
		}
		if stdout := strings.TrimSpace(inputs.Stdout()); stdout != "" {
			t.Logf("daemon stdout:\n%s", stdout)
		}
	})
	daemon := StartProcessCommand(t, process, inputs.Input)
	baseURL := server.WaitForURL(t)
	WaitForTerminalStatus(t, baseURL, timeout)

	session := GetDefaultSession(t, baseURL)
	work := ListDefaultSessionWork(t, baseURL)
	events := GetFactoryEventsAt(t, baseURL)
	var responseEvents []factoryapi.FactoryResponseEvent
	if captureResponseEvents {
		responseEvents = GetFactoryResponseEventsAt(t, baseURL, session.Id)
	}
	daemon.Stop(t)
	return session, work, events, responseEvents
}
