package bootstrap_portability

import (
	"context"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type functionalAPIServer struct {
	*support.FunctionalAPIServer
}

func startFunctionalServer(
	t *testing.T,
	factoryDir string,
	useMockWorkers bool,
	externalEdges ...serviceedges.Edges,
) *functionalAPIServer {
	t.Helper()
	cfg := support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		UseMockWorkers:            useMockWorkers,
		WaitForServiceModeRuntime: true,
	}
	if len(externalEdges) > 0 {
		cfg.Edges = externalEdges[0]
	}
	base := support.StartFunctionalAPIServer(t, cfg)
	return &functionalAPIServer{FunctionalAPIServer: base}
}

func waitForCurrentFactoryRuntimeIdle(t *testing.T, serverURL string, timeout time.Duration) {
	t.Helper()
	support.WaitForRuntimeIdle(t, serverURL, timeout)
}

type HTTPNamedFactoryReadback struct {
	t         *testing.T
	serverURL string
}

func (readback HTTPNamedFactoryReadback) GetCurrentFactory(context.Context) (factoryapi.Factory, error) {
	return getCurrentFactory(readback.t, readback.serverURL), nil
}
