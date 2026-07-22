package support

import (
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
	session, _, _ := runFactoryToCompletion(t, dir, overrides, timeout)
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
	session, work, _ := runFactoryToCompletion(t, dir, overrides, timeout)
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
	return runFactoryToCompletion(t, dir, overrides, timeout)
}

func runFactoryToCompletion(
	t testing.TB,
	dir string,
	overrides serviceedges.Edges,
	timeout time.Duration,
) (factoryapi.FactorySession, factoryapi.ListWorkResponse, []factoryapi.FactoryEvent) {
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
	daemon.Stop(t)
	return session, work, events
}

// SessionPlaceTokenCount reads one authored place from the public Petri
// projection. It deliberately does not inspect an internal runtime marking.
func SessionPlaceTokenCount(session factoryapi.FactorySession, placeID string) int {
	if session.Runtime.Petri == nil {
		return 0
	}
	count := 0
	for _, token := range session.Runtime.Petri.Marking {
		if token.PlaceId == placeID {
			count++
		}
	}
	return count
}

// SessionHasWorkAtPlace reads one Work location from the generated public
// Factory Session projection.
func SessionHasWorkAtPlace(session factoryapi.FactorySession, workID, placeID string) bool {
	if session.Runtime.Petri == nil {
		return false
	}
	for _, token := range session.Runtime.Petri.Marking {
		if token.WorkId == workID && token.PlaceId == placeID {
			return true
		}
	}
	return false
}
