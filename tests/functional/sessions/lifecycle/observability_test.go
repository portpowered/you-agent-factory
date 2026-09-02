package lifecycle_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const lifecycleCleanupProbeTimeout = time.Second

// lifecycleHTTPServer observes the transport role owned by the shared root
// process. It delegates all serving behavior to the functional support edge;
// the counters only make the package topology and shutdown result explicit.
type lifecycleHTTPServer struct {
	server *support.ProcessAPIServer

	done     chan struct{}
	doneOnce sync.Once
}

func newLifecycleHTTPServer() *lifecycleHTTPServer {
	return &lifecycleHTTPServer{
		server: support.NewProcessAPIServer(),
		done:   make(chan struct{}),
	}
}

func (server *lifecycleHTTPServer) start(
	ctx context.Context,
	request platformhttpserver.StartRequest,
) error {
	defer server.doneOnce.Do(func() {
		close(server.done)
	})
	return server.server.Start(ctx, request)
}

func (server *lifecycleHTTPServer) waitForBaseURL(timeout time.Duration) (string, error) {
	return server.server.WaitForBaseURL(timeout)
}

func (server *lifecycleHTTPServer) waitClosed() error {
	ctx, cancel := context.WithTimeout(context.Background(), lifecycleFixtureShutdownTimeout)
	defer cancel()
	select {
	case <-server.done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("shared lifecycle listener did not stop within %s: %w", lifecycleFixtureShutdownTimeout, ctx.Err())
	}
}

func (server *lifecycleHTTPServer) probeClosed() error {
	baseURL, ok := server.server.BaseURL()
	if !ok {
		return fmt.Errorf("shared lifecycle listener has no bound URL")
	}
	client := &http.Client{Timeout: lifecycleCleanupProbeTimeout}
	defer client.CloseIdleConnections()
	response, err := client.Get(strings.TrimSuffix(baseURL, "/") + "/status")
	if err != nil {
		return nil
	}
	defer response.Body.Close()
	body, readErr := io.ReadAll(response.Body)
	return fmt.Errorf(
		"shared lifecycle listener remains reachable after process cleanup: status=%d body=%q read error=%v",
		response.StatusCode,
		strings.TrimSpace(string(body)),
		readErr,
	)
}
