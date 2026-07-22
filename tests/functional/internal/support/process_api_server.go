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
)

const processAPIServerReadyTimeout = 5 * time.Second

// ProcessAPIServer is an HTTP transport edge for a root-built process. It owns
// only the external server boundary and never constructs application services.
type ProcessAPIServer struct {
	ready chan struct{}

	mu      sync.Mutex
	started bool
	url     string
}

func NewProcessAPIServer() *ProcessAPIServer {
	return &ProcessAPIServer{ready: make(chan struct{})}
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
	httpServer := httptest.NewServer(request.Handler)
	server.url = httpServer.URL
	if request.OnBound != nil {
		boundPort := request.Port
		if address, ok := httpServer.Listener.Addr().(*net.TCPAddr); ok {
			boundPort = address.Port
		}
		request.OnBound(platformhttpserver.Binding{Port: boundPort})
	}
	close(server.ready)
	server.mu.Unlock()

	<-ctx.Done()
	httpServer.Close()
	return nil
}

// Ready is closed after the httptest server is accepting requests.
func (server *ProcessAPIServer) Ready() <-chan struct{} {
	if server == nil {
		closed := make(chan struct{})
		close(closed)
		return closed
	}
	return server.ready
}

// WaitForURL waits for the injected transport to start and returns its dynamic
// httptest URL.
func (server *ProcessAPIServer) WaitForURL(t testing.TB) string {
	t.Helper()
	if server == nil {
		t.Fatal("process API server is required")
	}
	select {
	case <-server.ready:
	case <-time.After(processAPIServerReadyTimeout):
		t.Fatal("timed out waiting for process API server")
	}
	server.mu.Lock()
	defer server.mu.Unlock()
	if server.url == "" {
		t.Fatal("process API server became ready without a URL")
	}
	return server.url
}
