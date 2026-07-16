package support

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/workers"
	"github.com/portpowered/infinite-you/tests/internal/functionalhost"
)

type FunctionalAPIServerConfig = functionalhost.FunctionalAPIServerConfig
type FunctionalAPIServer = functionalhost.FunctionalAPIServer

func ConfigureWorkerCommands(
	t *testing.T,
	cfg *service.FactoryServiceConfig,
	providerRunner, scriptRunner workers.CommandRunner,
) {
	t.Helper()
	functionalhost.ConfigureWorkerCommands(t, cfg, providerRunner, scriptRunner)
}

func StartFunctionalAPIServer(t *testing.T, cfg FunctionalAPIServerConfig) *FunctionalAPIServer {
	t.Helper()
	return functionalhost.StartFunctionalAPIServer(t, cfg)
}
