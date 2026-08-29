package lifecycle_test

import (
	"context"
	"errors"
	"net"
	"net/url"
	"strings"
	"syscall"
	"testing"
	"time"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type lifecycleAPIServer struct {
	server *support.ProcessAPIServer
	closed chan struct{}
}

func newLifecycleAPIServer() *lifecycleAPIServer {
	return &lifecycleAPIServer{
		server: support.NewProcessAPIServer(),
		closed: make(chan struct{}),
	}
}

func (server *lifecycleAPIServer) HoldShutdownUntilSignaled(gate <-chan struct{}) {
	server.server.HoldShutdownUntilSignaled(gate)
}

func (server *lifecycleAPIServer) Start(
	ctx context.Context,
	request platformhttpserver.StartRequest,
) error {
	defer close(server.closed)
	return server.server.Start(ctx, request)
}

// lifecycleListenerClosed uses a positive transport-level signal: a refused
// TCP connection proves that the listener is gone, while a successful
// connection proves that it is still present. In particular, a dial timeout
// is not treated as teardown because a live listener can accept a connection
// without producing an HTTP response.
func lifecycleListenerClosed(baseURL string, closeSignal <-chan struct{}) bool {
	if closeSignal != nil {
		return isClosed(closeSignal)
	}
	if strings.TrimSpace(baseURL) == "" {
		return false
	}
	endpoint, err := url.Parse(baseURL)
	if err != nil || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.Host == "" || endpoint.Port() == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), lifecycleHTTPTimeout)
	defer cancel()
	dialer := &net.Dialer{}
	for {
		connection, err := dialer.DialContext(ctx, "tcp", endpoint.Host)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return false
			}
			return errors.Is(err, syscall.ECONNREFUSED)
		}
		_ = connection.Close()

		// Process.Close cancels the invocation-owned server, whose transport
		// starter closes its socket from another goroutine. Keep observing until
		// that positive refusal signal arrives rather than treating this brief
		// handoff window as leaked or closed.
		pollTimer := time.NewTimer(lifecyclePollInterval)
		select {
		case <-ctx.Done():
			if !pollTimer.Stop() {
				<-pollTimer.C
			}
			return false
		case <-pollTimer.C:
		}
	}
}

func TestLifecycleListenerClosedRejectsLiveNonresponsiveListener(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	accepted := make(chan struct{})
	go func() {
		connection, err := listener.Accept()
		if err != nil {
			return
		}
		close(accepted)
		<-release
		_ = connection.Close()
	}()
	t.Cleanup(func() {
		close(release)
		_ = listener.Close()
	})

	baseURL := "http://" + listener.Addr().String()
	if lifecycleListenerClosed(baseURL, nil) {
		t.Fatalf("live nonresponsive listener at %s was reported closed", baseURL)
	}
	timer := time.NewTimer(lifecycleHTTPTimeout)
	defer timer.Stop()
	select {
	case <-accepted:
	case <-timer.C:
		t.Fatal("listener probe did not reach the nonresponsive listener")
	}
}
