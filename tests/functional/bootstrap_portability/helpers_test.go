package bootstrap_portability

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type functionalAPIServer struct {
	service *service.FactoryService
	*support.FunctionalAPIServer
}

func startFunctionalServerWithConfig(
	t *testing.T,
	factoryDir string,
	useMockWorkers bool,
	configure func(*service.FactoryServiceConfig),
	extraOpts ...factory.FactoryOption,
) *functionalAPIServer {
	t.Helper()
	server := &functionalAPIServer{}
	base := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		UseMockWorkers:            useMockWorkers,
		WaitForServiceModeRuntime: true,
		Configure:                 configure,
		ExtraOptions:              extraOpts,
		CaptureService: func(svc *service.FactoryService) {
			server.service = svc
		},
	})
	server.FunctionalAPIServer = base
	return server
}
