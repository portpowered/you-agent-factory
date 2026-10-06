package inference_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
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
	functionalDefaultFixtureRoot string
	functionalDefaultFixtureOnce sync.Once
	functionalDefaultFixtureErr  error
)

func TestMain(m *testing.M) {
	code := m.Run()
	if closeFunctionalDefaultFixture() != nil {
		code = 1
	}
	os.Exit(code)
}

func FunctionalMonolithCleanup(t *testing.T) {
	t.Helper()
	if err := closeFunctionalDefaultFixture(); err != nil {
		t.Errorf("close shared Models fixture: %v", err)
	}
}

func initializeFunctionalDefaultFixture() {
	fixtureRoot, err := os.MkdirTemp("", "models-local-inference-")
	if err != nil {
		functionalDefaultFixtureErr = err
		return
	}
	functionalDefaultFixtureRoot = fixtureRoot
	homeDir := filepath.Join(fixtureRoot, "home")
	cacheDir := filepath.Join(fixtureRoot, "model-cache")
	for _, path := range []string{homeDir, cacheDir} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			functionalDefaultFixtureErr = err
			return
		}
	}
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{})
	if err != nil {
		functionalDefaultFixtureErr = err
		return
	}
	functionalDefaultProcess = process
	functionalDefaultEnvironment = append(functionalHomeEnvironment(homeDir), runcli.ModelCacheDirEnvironment+"="+cacheDir)
}

func closeFunctionalDefaultFixture() error {
	var closeErr error
	if functionalDefaultProcess != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		closeErr = functionalDefaultProcess.Close(ctx)
		cancel()
	}
	if functionalDefaultFixtureRoot != "" {
		closeErr = errors.Join(closeErr, os.RemoveAll(functionalDefaultFixtureRoot))
	}
	if closeErr != nil {
		fmt.Fprintln(os.Stderr, "close shared Models fixture:", closeErr)
	}
	return closeErr
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
	functionalDefaultFixtureOnce.Do(initializeFunctionalDefaultFixture)
	if functionalDefaultFixtureErr != nil {
		t.Fatalf("initialize shared Models fixture: %v", functionalDefaultFixtureErr)
	}
	return functionalDefaultProcess
}

func functionalSharedDefaultEnvironment(t testing.TB) []string {
	t.Helper()
	functionalSharedDefaultProcess(t)
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
