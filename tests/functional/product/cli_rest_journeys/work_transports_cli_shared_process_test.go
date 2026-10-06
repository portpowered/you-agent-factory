package cli_rest_journeys_test

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/root"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

var (
	workCLIProcess support.Process
	workCLIServer  *support.FunctionalAPIServer
	workCLIHome    string
)

// The parent owns the initialized host until every parallel child has closed
// its explicit Factory Session. Work IDs and request IDs are session-scoped.
func initializeWorktransportscliFixture(t *testing.T) {
	workCLIHome = t.TempDir()
	workCLIServer = support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                support.ScaffoldFactory(t, workListFiltersCountsFactoryConfig()),
		Env:                       workListFiltersCountsEnvironment(workCLIHome),
		WaitForServiceModeRuntime: true,
		BeforeStart: func(_ testing.TB, process support.Process, _ root.Input) {
			workCLIProcess = process
		},
	})
}

func openWorkCLISession(t *testing.T, factoryDir string) string {
	t.Helper()
	id := support.OpenFactorySessionAt(t, workCLIServer.URL(), factoryDir).Session.Id
	t.Cleanup(func() { support.CloseFactorySessionAt(t, workCLIServer.URL(), id) })
	return id
}

func resetworktransportscli1State() {
	workCLIProcess = nil
	workCLIServer = nil
	workCLIHome = ""
}
