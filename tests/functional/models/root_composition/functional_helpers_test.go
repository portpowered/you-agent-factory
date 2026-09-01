package root_composition_test

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/functional/internal/support/localai"
)

func functionalBuildProcess(t testing.TB, edges serviceedges.Edges) support.ApplicationProcess {
	t.Helper()
	process := support.BuildProcess(t, edges)
	support.CleanupProcess(t, process)
	return process
}

func functionalScaffoldFactory(t *testing.T, config map[string]any) string {
	t.Helper()
	return support.ScaffoldFactory(t, config)
}

func functionalTempDir(t testing.TB) string {
	t.Helper()
	return t.TempDir()
}

func functionalStartAPIServer(
	t *testing.T,
	cfg support.FunctionalAPIServerConfig,
) *support.FunctionalAPIServer {
	t.Helper()
	return support.StartFunctionalAPIServer(t, cfg)
}

func functionalStartLocalAI(t testing.TB, options ...localai.Options) *localai.Fixture {
	t.Helper()
	return localai.Start(t, options...)
}

func functionalNewHTTPServer(t testing.TB, handler http.Handler) *httptest.Server {
	t.Helper()
	return httptest.NewServer(handler)
}

func functionalListen(t testing.TB, network, address string) (net.Listener, error) {
	t.Helper()
	return net.Listen(network, address)
}
