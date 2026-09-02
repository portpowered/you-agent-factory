package root_composition_test

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/functional/internal/support/localai"
)

var (
	functionalDefaultProcessMu sync.Mutex
	functionalDefaultProcess   support.ApplicationProcess
)

func TestMain(m *testing.M) {
	code := m.Run()

	functionalDefaultProcessMu.Lock()
	process := functionalDefaultProcess
	functionalDefaultProcessMu.Unlock()
	if process != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		if err := process.Close(ctx); err != nil {
			fmt.Fprintln(os.Stderr, "close shared models process:", err)
			code = 1
		}
		cancel()
	}
	os.Exit(code)
}

func functionalBuildProcess(t testing.TB, edges serviceedges.Edges) support.ApplicationProcess {
	t.Helper()
	process := support.BuildProcess(t, edges)
	support.CleanupProcess(t, process)
	return process
}

// functionalSharedDefaultProcess owns the immutable empty-edge application
// graph used by independent local and remote diagnostic commands. Each caller
// still owns its profile, working directory, inputs, and server endpoint.
func functionalSharedDefaultProcess(t testing.TB) support.ApplicationProcess {
	t.Helper()
	functionalDefaultProcessMu.Lock()
	defer functionalDefaultProcessMu.Unlock()
	if functionalDefaultProcess == nil {
		functionalDefaultProcess = support.BuildProcess(t, serviceedges.Edges{})
	}
	return functionalDefaultProcess
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
