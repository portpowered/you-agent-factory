package cli_rest_journeys_test

import (
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// Retained-history and stream journeys share bootstrap, but every case owns
// its Factory Session and authored directory. Parent cleanup follows children.
func newFactoryEventSessionHost(t *testing.T) *support.FunctionalAPIServer {
	t.Helper()
	idle := support.ScaffoldSingleStepFactory(t, "factory-event-host")
	support.ClearSeedInputs(t, idle)
	return support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: idle, WaitForServiceModeRuntime: true,
		Edges: serviceedges.Edges{ProviderCommandRunner: support.NewStaticSuccessCommandRunner("factory event provider COMPLETE")},
	})
}

func openFactoryEventSession(t *testing.T, host *support.FunctionalAPIServer, dir string) string {
	t.Helper()
	opened := support.OpenFactorySessionAt(t, host.URL(), dir)
	id := opened.Session.Id
	t.Cleanup(func() { support.CloseFactorySessionAt(t, host.URL(), id) })
	return id
}
