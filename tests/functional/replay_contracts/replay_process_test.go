package replay_contracts

import (
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type replayProcessObservation struct {
	Events  []factoryapi.FactoryEvent
	Session factoryapi.FactorySession
	Work    factoryapi.ListWorkResponse
}

func observeReplayThroughRoot(
	t *testing.T,
	artifactPath string,
	timeout time.Duration,
) replayProcessObservation {
	t.Helper()

	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: t.TempDir(),
		Args:       []string{"--replay", artifactPath},
	})
	support.WaitForTerminalStatus(t, server.URL(), timeout)
	observation := replayProcessObservation{
		Events:  server.GetFactoryEvents(t),
		Session: support.GetDefaultSession(t, server.URL()),
		Work:    support.ListDefaultSessionWork(t, server.URL()),
	}
	server.Stop(t)
	return observation
}

func assertReplayPlaceCounts(
	t testing.TB,
	session factoryapi.FactorySession,
	wants map[string]int,
) {
	t.Helper()
	for placeID, want := range wants {
		if got := support.SessionPlaceTokenCount(session, placeID); got != want {
			t.Errorf("%s token count = %d, want %d", placeID, got, want)
		}
	}
}

func replayWorkIncludesID(work factoryapi.ListWorkResponse, workID string) bool {
	for _, item := range work.Results {
		if stringPointerValue(item.WorkId) == workID {
			return true
		}
	}
	return false
}
