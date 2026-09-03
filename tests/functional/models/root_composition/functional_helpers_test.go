package root_composition_test

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/functional/internal/support/localai"
)

var (
	functionalDefaultProcess     support.ApplicationProcess
	functionalDefaultEnvironment []string
)

func TestMain(m *testing.M) {
	fixtureRoot, err := os.MkdirTemp("", "models-root-composition-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "create shared models fixture:", err)
		os.Exit(1)
	}
	homeDir := filepath.Join(fixtureRoot, "home")
	cacheDir := filepath.Join(fixtureRoot, "model-cache")
	for _, path := range []string{homeDir, cacheDir} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			fmt.Fprintln(os.Stderr, "create shared models fixture path:", err)
			_ = os.RemoveAll(fixtureRoot)
			os.Exit(1)
		}
	}
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{})
	if err != nil {
		fmt.Fprintln(os.Stderr, "build shared models process:", err)
		_ = os.RemoveAll(fixtureRoot)
		os.Exit(1)
	}
	functionalDefaultProcess = process
	functionalDefaultEnvironment = append(
		functionalHomeEnvironment(homeDir),
		runcli.ModelCacheDirEnvironment+"="+cacheDir,
	)

	code := m.Run()

	ctx, cancelClose := context.WithTimeout(context.Background(), 15*time.Second)
	if err := process.Close(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "close shared models process:", err)
		code = 1
	}
	cancelClose()
	if err := os.RemoveAll(fixtureRoot); err != nil {
		fmt.Fprintln(os.Stderr, "remove shared models fixture:", err)
		code = 1
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
	if functionalDefaultProcess == nil {
		t.Fatal("shared Models process is not initialized")
	}
	return functionalDefaultProcess
}

func functionalSharedDefaultEnvironment() []string {
	return append([]string(nil), functionalDefaultEnvironment...)
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
