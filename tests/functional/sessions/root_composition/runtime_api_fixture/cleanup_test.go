package runtimeapifixture

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/portpowered/infinite-you/pkg/root"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// C06-ISOLATED CASE-43: cleanup itself owns process, listener, root, and
// tracked-lane teardown; injected lifecycle failures must remain visible while
// every independent cleanup action still runs.
func TestRuntimeAPIPackageFixtureCleanupIsIdempotentAndPreservesFailures(t *testing.T) {
	t.Run("normal cleanup probes listener and removes root", func(t *testing.T) {
		fixture, listener, process := newRuntimeAPICleanupTestFixture(t, nil, nil)
		listener.Close()

		if err := fixture.Close(); err != nil {
			t.Fatalf("fixture close: %v", err)
		}
		if err := fixture.Close(); err != nil {
			t.Fatalf("repeated fixture close: %v", err)
		}
		if got := process.closeCalls.Load(); got != 1 {
			t.Fatalf("process close calls = %d, want 1", got)
		}
		assertRuntimeAPITestRootRemoved(t, fixture.rootDir)
	})

	t.Run("injected execute and close failures remain visible", func(t *testing.T) {
		executeErr := errors.New("injected Process.Execute failure")
		closeErr := errors.New("injected application process close failure")
		fixture, listener, process := newRuntimeAPICleanupTestFixture(t, executeErr, closeErr)
		listener.Close()

		firstErr := fixture.Close()
		if !errors.Is(firstErr, executeErr) {
			t.Fatalf("fixture close error = %v, want Process.Execute cause", firstErr)
		}
		if !errors.Is(firstErr, closeErr) {
			t.Fatalf("fixture close error = %v, want process Close cause", firstErr)
		}
		secondErr := fixture.Close()
		if !errors.Is(secondErr, executeErr) || !errors.Is(secondErr, closeErr) {
			t.Fatalf("repeated fixture close error = %v, want both original causes", secondErr)
		}
		if got := process.closeCalls.Load(); got != 1 {
			t.Fatalf("process close calls = %d, want 1 after repeated cleanup", got)
		}
		assertRuntimeAPITestRootRemoved(t, fixture.rootDir)
	})

	t.Run("reachable listener fails the cleanup probe", func(t *testing.T) {
		listener := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		defer listener.Close()

		err := runtimeAPIListenerClosed(listener.URL, 1)
		if err == nil || !strings.Contains(err.Error(), "remained reachable") {
			t.Fatalf("reachable listener probe error = %v, want reachability failure", err)
		}
	})
}

type runtimeAPICleanupTestProcess struct {
	executeErr error
	closeErr   error
	closeCalls atomic.Int64
}

func (process *runtimeAPICleanupTestProcess) Execute(root.Input) error {
	return process.executeErr
}

func (process *runtimeAPICleanupTestProcess) Close(context.Context) error {
	process.closeCalls.Add(1)
	return process.closeErr
}

func (*runtimeAPICleanupTestProcess) ACPServer() support.ACPServer {
	return nil
}

func (*runtimeAPICleanupTestProcess) ProviderRegistry() support.ProviderRegistry {
	return nil
}

func (*runtimeAPICleanupTestProcess) WorkerRecordingReader() recordings.WorkerRecordingReader {
	return nil
}

func newRuntimeAPICleanupTestFixture(
	t *testing.T,
	executeErr, closeErr error,
) (*PackageFixture, *httptest.Server, *runtimeAPICleanupTestProcess) {
	t.Helper()
	rootDir := filepath.Join(t.TempDir(), "owned-runtime-api-root")
	if err := os.MkdirAll(rootDir, 0o755); err != nil {
		t.Fatalf("create cleanup test root: %v", err)
	}
	listener := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	process := &runtimeAPICleanupTestProcess{executeErr: executeErr, closeErr: closeErr}
	fixture := &PackageFixture{
		rootDir:        rootDir,
		baseURL:        listener.URL,
		process:        process,
		ledger:         newRuntimeAPICleanupLedger(),
		providerRouter: newRuntimeAPIProviderRouter(),
		commandRouter:  newRuntimeAPICommandRouter("provider"),
		scriptRouter:   newRuntimeAPICommandRouter("script"),
	}
	fixture.apiStarts.Store(1)
	fixture.processStarts.Store(1)
	inputs := support.FakeInputs(context.Background(), []string{"you", "run"})
	fixture.command = startRuntimeAPIProcessCommand(process, inputs.Input)
	return fixture, listener, process
}

func assertRuntimeAPITestRootRemoved(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cleanup test root stat error = %v, want path removed", err)
	}
}
