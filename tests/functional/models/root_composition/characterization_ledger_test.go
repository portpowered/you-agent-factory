package root_composition_test

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/functional/internal/support/localai"
)

// c06CharacterizationLedger records actual package-process setup operations.
// It is deliberately a test-only measurement surface: it does not change
// production wiring, route behavior, or the external-effect edges used by the
// Models witnesses.
var c06Ledger = &c06CharacterizationLedger{}

type c06CharacterizationLedger struct {
	rootBuilds          atomic.Int64
	apiServers          atomic.Int64
	httptestServers     atomic.Int64
	localAIStarts       atomic.Int64
	tcpListeners        atomic.Int64
	factoryRoots        atomic.Int64
	tempDirs            atomic.Int64
	hostStarts          atomic.Int64
	assetHTTPCalls      atomic.Int64
	hostHTTPCalls       atomic.Int64
	localHTTPCalls      atomic.Int64
	sharedSessionOpens  atomic.Int64
	sharedSessionCloses atomic.Int64
}

func TestMain(m *testing.M) {
	exitCode := m.Run()
	if err := closeSharedModelsFixture(); err != nil {
		fmt.Fprintf(os.Stderr, "C06-002 shared Models fixture cleanup failed: %v\n", err)
		exitCode = 1
	}
	sharedRootBuilds, sharedAPIStarts, sharedSessionOpens, sharedSessionCloses := sharedModelsFixtureCounters()
	fmt.Fprintf(
		os.Stderr,
		"CHAR-001 ledger root_builds=%d api_servers=%d httptest_servers=%d localai_starts=%d tcp_listeners=%d factory_roots=%d temp_dirs=%d host_starts=%d asset_http_calls=%d host_http_calls=%d local_http_calls=%d\n",
		c06Ledger.rootBuilds.Load(),
		c06Ledger.apiServers.Load(),
		c06Ledger.httptestServers.Load(),
		c06Ledger.localAIStarts.Load(),
		c06Ledger.tcpListeners.Load(),
		c06Ledger.factoryRoots.Load(),
		c06Ledger.tempDirs.Load(),
		c06Ledger.hostStarts.Load(),
		c06Ledger.assetHTTPCalls.Load(),
		c06Ledger.hostHTTPCalls.Load(),
		c06Ledger.localHTTPCalls.Load(),
	)
	fmt.Fprintf(
		os.Stderr,
		"C06-002 shared_models_root_builds=%d shared_models_api_starts=%d shared_models_session_opens=%d shared_models_session_closes=%d\n",
		sharedRootBuilds,
		sharedAPIStarts,
		sharedSessionOpens,
		sharedSessionCloses,
	)
	os.Exit(exitCode)
}

func characterizationBuildProcess(t testing.TB, edges serviceedges.Edges) support.ApplicationProcess {
	t.Helper()
	process := support.BuildProcess(t, edges)
	c06Ledger.rootBuilds.Add(1)
	return process
}

func characterizationScaffoldFactory(t *testing.T, config map[string]any) string {
	t.Helper()
	dir := support.ScaffoldFactory(t, config)
	c06Ledger.factoryRoots.Add(1)
	return dir
}

func characterizationTempDir(t testing.TB) string {
	t.Helper()
	dir := t.TempDir()
	c06Ledger.tempDirs.Add(1)
	return dir
}

func characterizationStartFunctionalAPIServer(
	t *testing.T,
	cfg support.FunctionalAPIServerConfig,
) *support.FunctionalAPIServer {
	t.Helper()
	server := support.StartFunctionalAPIServer(t, cfg)
	c06Ledger.apiServers.Add(1)
	return server
}

func characterizationStartLocalAI(t testing.TB, options ...localai.Options) *localai.Fixture {
	t.Helper()
	fixture := localai.Start(t, options...)
	c06Ledger.localAIStarts.Add(1)
	return fixture
}

func characterizationNewHTTPServer(t testing.TB, handler http.Handler) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		c06Ledger.localHTTPCalls.Add(1)
		handler.ServeHTTP(writer, request)
	}))
	c06Ledger.httptestServers.Add(1)
	return server
}

func characterizationListen(t testing.TB, network, address string) (net.Listener, error) {
	t.Helper()
	listener, err := net.Listen(network, address)
	if err == nil {
		c06Ledger.tcpListeners.Add(1)
	}
	return listener, err
}
