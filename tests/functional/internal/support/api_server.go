package support

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/workers"
	"github.com/portpowered/infinite-you/tests/internal/functionalhost"
)

type FunctionalAPIServerConfig = functionalhost.FunctionalAPIServerConfig
type FunctionalAPIServer = functionalhost.FunctionalAPIServer

// ConfigureWorkerCommands installs typed functional command edges before the
// service graph is assembled.
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

func StartFunctionalAPIServiceModeServer(t *testing.T, factoryDir string, useMockWorkers bool) *FunctionalAPIServer {
	t.Helper()
	return StartFunctionalAPIServer(t, FunctionalAPIServerConfig{
		FactoryDir:     factoryDir,
		UseMockWorkers: useMockWorkers,
		ExtraOptions:   []factory.FactoryOption{factory.WithServiceMode()},
	})
}
