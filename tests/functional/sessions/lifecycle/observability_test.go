package lifecycle_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const lifecycleCleanupProbeTimeout = time.Second

// lifecycleHTTPServer observes the transport role owned by the shared root
// process. It delegates all serving behavior to the functional support edge;
// the counters only make the package topology and shutdown result explicit.
type lifecycleHTTPServer struct {
	server *support.ProcessAPIServer

	mu       sync.Mutex
	starts   int
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
	server.mu.Lock()
	server.starts++
	server.mu.Unlock()
	defer server.doneOnce.Do(func() { close(server.done) })
	return server.server.Start(ctx, request)
}

func (server *lifecycleHTTPServer) startCount() int {
	server.mu.Lock()
	defer server.mu.Unlock()
	return server.starts
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

// lifecycleProcessConstructor records successful root.BuildProcess calls at
// the construction boundary. The shared fixture uses it for both roles, so
// the role observation does not depend on inspecting a manually entered count.
type lifecycleProcessConstructor struct {
	mu        sync.Mutex
	roleNames []string
}

func (constructor *lifecycleProcessConstructor) build(
	ctx context.Context,
	role string,
	edges serviceedges.Edges,
) (support.ApplicationProcess, error) {
	process, err := support.BuildProcessWithContext(ctx, edges)
	if err != nil {
		return nil, err
	}
	constructor.mu.Lock()
	constructor.roleNames = append(constructor.roleNames, role)
	constructor.mu.Unlock()
	return process, nil
}

func (constructor *lifecycleProcessConstructor) roles() []string {
	constructor.mu.Lock()
	defer constructor.mu.Unlock()
	return append([]string(nil), constructor.roleNames...)
}

// TestFactorySessionCleanupRunsAfterEarlySubtestExit proves that a session
// admitted by a child is cleaned by its registered t.Cleanup callback even
// when the child returns before its normal explicit cleanup path. The parent
// then observes the public not-found contract through the real shared server.
func TestFactorySessionCleanupRunsAfterEarlySubtestExit(t *testing.T) {
	baseURL := sharedLifecycleServerURL(t)
	var sessionID string

	if !t.Run("setup exits before explicit cleanup", func(t *testing.T) {
		factoryDir := filepath.Join(t.TempDir(), "session-lifecycle-cleanup-probe-factory")
		if err := os.Mkdir(factoryDir, 0o755); err != nil {
			t.Fatalf("create cleanup probe factory directory: %v", err)
		}
		initNewFactory := true
		opened := postSessionLifecycleJSON[factoryapi.OpenFactorySessionResponse](
			t,
			baseURL+"/factory-sessions",
			factoryapi.OpenFactorySessionRequest{
				FolderPath:     factoryDir,
				InitNewFactory: &initNewFactory,
			},
			"open cleanup probe Factory Session",
		)
		if opened.Session == nil || strings.TrimSpace(opened.Session.Id) == "" {
			t.Fatalf("cleanup probe response missing session id: %#v", opened)
		}
		sessionID = opened.Session.Id
		registerLifecycleSessionCleanup(t, baseURL, sessionID)

		// Returning here models a setup/assertion exit before the normal explicit
		// close path. The registered cleanup must still run before t.Run returns.
		return
	}) {
		t.Fatal("cleanup probe subtest failed")
	}
	if strings.TrimSpace(sessionID) == "" {
		t.Fatal("cleanup probe did not admit a session")
	}
	assertAPISessionNotFound(t, baseURL, sessionID)
}
