package support

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/pkg/workers"
)

// FunctionalAPIServerConfig and FunctionalAPIServer retain the legacy
// functional helper surface while product construction remains in pkg/testutil.
type FunctionalAPIServerConfig = testutil.FunctionalAPIServerConfig
type FunctionalAPIServer = testutil.FunctionalAPIServer

func ConfigureWorkerCommands(
	t *testing.T,
	cfg *service.FactoryServiceConfig,
	providerRunner, scriptRunner workers.CommandRunner,
) {
	t.Helper()
	testutil.ConfigureWorkerCommands(t, cfg, providerRunner, scriptRunner)
}

func StartFunctionalAPIServer(t *testing.T, cfg FunctionalAPIServerConfig) *FunctionalAPIServer {
	t.Helper()
	return testutil.StartFunctionalAPIServer(t, cfg)
}
