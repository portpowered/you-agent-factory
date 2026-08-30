package support

import (
	"context"
	"fmt"
	"net"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocessmemory "github.com/portpowered/infinite-you/pkg/platform/processmemory"
)

// Process construction can legitimately take longer on Windows when the
// functional lane starts several root-built applications concurrently. Keep
// this as a ceiling only: readiness still returns immediately on success.
const processAPIServerReadyTimeout = 15 * time.Second

// ProcessAPIServer is an HTTP transport edge for a root-built process. It owns
// only the external server boundary and never constructs application services.
type ProcessAPIServer struct {
	// These function-valued hooks are test-harness entrypoints. The production-
	// only deadcode check cannot see callers in external functional test
	// packages, so keeping the hooks as fields avoids treating them as runtime
	// API methods.
	HoldStartUntilSignaled func(<-chan struct{})
	WaitForBoundURL        func(time.Duration) (string, error)

	startedSignal chan struct{}
	bound         chan struct{}
	ready         chan struct{}

	mu           sync.Mutex
	started      bool
	url          string
	startGate    <-chan struct{}
	shutdownGate <-chan struct{}
}

func NewProcessAPIServer() *ProcessAPIServer {
	server := &ProcessAPIServer{
		startedSignal: make(chan struct{}),
		bound:         make(chan struct{}),
		ready:         make(chan struct{}),
	}
	server.HoldStartUntilSignaled = func(gate <-chan struct{}) {
		server.mu.Lock()
		server.startGate = gate
		server.mu.Unlock()
	}
	server.WaitForBoundURL = func(timeout time.Duration) (string, error) {
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		select {
		case <-server.bound:
			server.mu.Lock()
			defer server.mu.Unlock()
			return server.url, nil
		case <-timer.C:
			return "", fmt.Errorf("timed out waiting for process API server listener after %s", timeout)
		}
	}
	return server
}

// HoldShutdownUntilSignaled defers listener teardown, once the owning
// invocation itself asks the transport to stop, until gate is closed or
// processAPIServerReadyTimeout elapses as a safety ceiling. It must be called
// before Start runs (i.e. before the owning process begins executing) so a
// caller can prove an observation happened before the server was allowed to
// close, instead of racing the invocation's own completion-triggered
// teardown.
func (server *ProcessAPIServer) HoldShutdownUntilSignaled(gate <-chan struct{}) {
	if server == nil {
		return
	}
	server.mu.Lock()
	server.shutdownGate = gate
	server.mu.Unlock()
}

func (server *ProcessAPIServer) currentShutdownGate() <-chan struct{} {
	server.mu.Lock()
	defer server.mu.Unlock()
	return server.shutdownGate
}

// Start serves the handler assembled by the injected process until its
// invocation context is cancelled.
func (server *ProcessAPIServer) Start(
	ctx context.Context,
	request platformhttpserver.StartRequest,
) error {
	if server == nil {
		return fmt.Errorf("process API server is required")
	}
	server.mu.Lock()
	if server.started {
		server.mu.Unlock()
		return fmt.Errorf("process API server already started")
	}
	server.started = true
	close(server.startedSignal)
	httpServer := httptest.NewServer(platformhttpserver.HandlerWithDiagnostics(
		request.Handler, request.Pprof, platformprocessmemory.CurrentCommit, nil,
	))
	server.url = httpServer.URL
	close(server.bound)
	startGate := server.startGate
	server.mu.Unlock()

	if startGate != nil {
		select {
		case <-startGate:
		case <-time.After(processAPIServerReadyTimeout):
		}
	}
	if request.OnBound != nil {
		boundPort := request.Port
		if address, ok := httpServer.Listener.Addr().(*net.TCPAddr); ok {
			boundPort = address.Port
		}
		request.OnBound(platformhttpserver.Binding{Port: boundPort})
	}
	server.mu.Lock()
	close(server.ready)
	server.mu.Unlock()

	<-ctx.Done()
	if gate := server.currentShutdownGate(); gate != nil {
		select {
		case <-gate:
		case <-time.After(processAPIServerReadyTimeout):
		}
	}
	httpServer.CloseClientConnections()
	httpServer.Close()
	return nil
}

// BaseURL returns the bound httptest URL after Ready has been signaled.
func (server *ProcessAPIServer) BaseURL() (string, bool) {
	if server == nil {
		return "", false
	}
	server.mu.Lock()
	defer server.mu.Unlock()
	if server.url == "" {
		return "", false
	}
	return server.url, true
}

// WaitForURL waits for the injected transport to start and returns its dynamic
// httptest URL.
func (server *ProcessAPIServer) WaitForURL(t testing.TB) string {
	t.Helper()
	baseURL, err := server.waitForURL(processAPIServerReadyTimeout)
	if err != nil {
		t.Fatal(err)
	}
	return baseURL
}

// WaitForBaseURL waits for the injected transport to start and returns its URL
// or an error. It is safe to call from background goroutines.
func (server *ProcessAPIServer) WaitForBaseURL(timeout time.Duration) (string, error) {
	return server.waitForURL(timeout)
}

func (server *ProcessAPIServer) waitForURL(timeout time.Duration) (string, error) {
	if server == nil {
		return "", fmt.Errorf("process API server is required")
	}

	readyTimer := time.NewTimer(timeout)
	defer readyTimer.Stop()
	startTimer := time.NewTimer(timeout)
	defer startTimer.Stop()

	select {
	case <-server.ready:
	case <-server.startedSignal:
		select {
		case <-server.ready:
		case <-readyTimer.C:
			return "", fmt.Errorf("timed out waiting for process API server after starter was invoked")
		}
	case <-startTimer.C:
		if server.wasStarted() {
			select {
			case <-server.ready:
			case <-readyTimer.C:
				return "", fmt.Errorf("timed out waiting for process API server after starter was invoked")
			}
		} else {
			return "", fmt.Errorf("process API server starter was never invoked; --with-server was probably omitted")
		}
	}
	if baseURL, ok := server.BaseURL(); ok {
		return baseURL, nil
	}
	return "", fmt.Errorf("process API server became ready without a URL")
}

func (server *ProcessAPIServer) wasStarted() bool {
	server.mu.Lock()
	defer server.mu.Unlock()
	return server.started
}
