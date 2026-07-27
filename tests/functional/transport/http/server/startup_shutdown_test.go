package server_test

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestAPIServerStartsOnConfiguredListenerAndServesStatus proves the public API
// server becomes reachable on its configured loopback listener and serves a
// non-empty readiness status observation after start.
func TestAPIServerStartsOnConfiguredListenerAndServesStatus(t *testing.T) {
	configuredURL := reserveConfiguredLoopbackURL(t)
	dir := support.ScaffoldFactory(t, startupShutdownTestFactoryConfig())
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		UseMockWorkers:            true,
		WaitForServiceModeRuntime: true,
		Args:                      []string{"--server", configuredURL},
	})
	defer server.Stop(t)

	listenerURL := server.URL()
	if listenerURL == "" {
		t.Fatal("started API server returned empty listener URL")
	}
	if !strings.HasPrefix(listenerURL, "http://127.0.0.1:") &&
		!strings.HasPrefix(listenerURL, "http://localhost:") {
		t.Fatalf("listener URL = %q, want loopback HTTP URL", listenerURL)
	}

	response, err := http.Get(listenerURL + "/status")
	if err != nil {
		t.Fatalf("GET %s/status: %v", listenerURL, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET %s/status status = %d, want %d", listenerURL, response.StatusCode, http.StatusOK)
	}

	status := support.GetJSON[factoryapi.StatusResponse](t, listenerURL+"/status")
	if status.FactoryState == "" {
		t.Fatal("GET /status returned empty factoryState")
	}
	if status.RuntimeStatus == "" {
		t.Fatal("GET /status returned empty runtimeStatus")
	}
}

// TestAPIServerShutdownClosesListenerAndActiveStreams proves shutdown through the
// public API server lifecycle closes the listener and terminates active public streams.
func TestAPIServerShutdownClosesListenerAndActiveStreams(t *testing.T) {
	dir := scaffoldConcurrentRequestsFactory(t)
	blocking := newBlockingInvocationRunner()
	edges := serviceedges.Edges{}
	support.ConfigureWorkerCommands(t, &edges, blocking, nil)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Edges:                     edges,
	})

	listenerURL := server.URL()
	parsed, err := url.Parse(listenerURL)
	if err != nil || parsed.Host == "" {
		t.Fatalf("parse listener URL %q: %v", listenerURL, err)
	}

	session := getFactorySession(t, listenerURL, factorysessions.DefaultSessionID)
	activeRequest := startActiveBlockingInvocation(t, listenerURL, session.Id, blocking)

	statusResp, err := http.Get(listenerURL + "/status")
	if err != nil {
		t.Fatalf("GET %s/status before shutdown: %v", listenerURL, err)
	}
	_ = statusResp.Body.Close()
	if statusResp.StatusCode != http.StatusOK {
		t.Fatalf(
			"GET %s/status before shutdown status = %d, want %d",
			listenerURL,
			statusResp.StatusCode,
			http.StatusOK,
		)
	}

	stopDone := stopFunctionalAPIServerAsync(t, server)
	waitForListenerClosed(t, parsed.Host, listenerURL, 10*time.Second)
	assertActiveRequestTerminatedAfterShutdown(t, activeRequest)

	select {
	case <-stopDone:
	case <-time.After(10 * time.Second):
		activeRequest.cancel()
		t.Fatal("timed out waiting for API server shutdown to complete")
	}
}

// TestAPIServerBindFailureUnwindsStartedLifecycleRoles proves bind failure through
// the public API server lifecycle reports a documented failure and leaves no
// leaked listeners or readiness side effects.
func TestAPIServerBindFailureUnwindsStartedLifecycleRoles(t *testing.T) {
	dir := support.ScaffoldFactory(t, startupShutdownTestFactoryConfig())
	requestedURL := "http://127.0.0.1:65534"

	var attempts []string
	starter, err := platformhttpserver.NewStarter(func(_ string, address string) (net.Listener, error) {
		attempts = append(attempts, address)
		return nil, errors.New("address unavailable")
	})
	if err != nil {
		t.Fatalf("NewStarter() error = %v", err)
	}

	var browserCalls, readinessCalls atomic.Int32
	edges := serviceedges.Edges{
		APIServerStarter: starter,
		BrowserOpener: func(context.Context, string) error {
			browserCalls.Add(1)
			return nil
		},
		RuntimeHostObserver: func(factorysessions.RuntimeHostBinding) {
			readinessCalls.Add(1)
		},
	}
	process := support.BuildProcess(t, edges)

	home := t.TempDir()
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--server", requestedURL,
		"run", "--dir", dir,
		"--continuously", "--with-server", "--quiet", "--no-record",
	})
	inputs.Input.WorkingDirectory = dir
	inputs.Input.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)

	if err := process.Execute(inputs.Input); err == nil {
		t.Fatalf(
			"Process.Execute(run with bind failure) error = nil; stdout=%q stderr=%q",
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}
	for _, forbidden := range []string{"Factory initiated:", "Dashboard URL:"} {
		if strings.Contains(inputs.Stdout(), forbidden) {
			t.Fatalf("run stdout exposed readiness %q before bind failure:\n%s", forbidden, inputs.Stdout())
		}
	}

	var response factoryapi.ErrorResponse
	if err := json.Unmarshal([]byte(inputs.Stderr()), &response); err != nil {
		t.Fatalf("run stderr is not exactly one ErrorResponse: %v\n%s", err, inputs.Stderr())
	}
	if response.Code != factoryapi.ErrorResponseCode("SERVER_BIND_FAILED") {
		t.Fatalf("ErrorResponse = %#v, want SERVER_BIND_FAILED", response)
	}
	if got, want := strings.Join(attempts, ","), "127.0.0.1:65534,127.0.0.1:65535"; got != want {
		t.Fatalf("listener attempts = %q, want %q", got, want)
	}
	if browserCalls.Load() != 0 || readinessCalls.Load() != 0 {
		t.Fatalf(
			"post-failure effects = browser:%d readiness:%d, want none",
			browserCalls.Load(),
			readinessCalls.Load(),
		)
	}

	parsed, err := url.Parse(requestedURL)
	if err != nil {
		t.Fatalf("parse requested listener URL %q: %v", requestedURL, err)
	}
	rebound, err := net.Listen("tcp4", parsed.Host)
	if err != nil {
		t.Fatalf("requested listener address %s remained unavailable after bind failure: %v", parsed.Host, err)
	}
	_ = rebound.Close()

	client := &http.Client{Timeout: 500 * time.Millisecond}
	statusResponse, err := client.Get(requestedURL + "/status")
	if err == nil {
		_ = statusResponse.Body.Close()
		t.Fatalf("GET %s/status succeeded after bind failure", requestedURL)
	}
}

func stopFunctionalAPIServerAsync(t *testing.T, server *support.FunctionalAPIServer) <-chan struct{} {
	t.Helper()

	done := make(chan struct{})
	go func() {
		defer close(done)
		server.Stop(t)
	}()
	return done
}

type activeBlockingInvocation struct {
	cancel context.CancelFunc
	done   chan error
}

func startActiveBlockingInvocation(
	t *testing.T,
	baseURL string,
	sessionID string,
	blocking *blockingInvocationRunner,
) *activeBlockingInvocation {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	request := &activeBlockingInvocation{
		cancel: cancel,
		done:   make(chan error, 1),
	}
	go func() {
		request.done <- postBlockingInvocation(ctx, baseURL, sessionID, blocking)
	}()

	select {
	case <-blocking.started:
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("timed out waiting for active blocking invocation")
	}

	return request
}

func assertActiveRequestTerminatedAfterShutdown(t *testing.T, request *activeBlockingInvocation) {
	t.Helper()

	select {
	case err := <-request.done:
		if err == nil {
			t.Fatal("active blocking invocation completed without error after shutdown")
		}
		if !errors.Is(err, context.Canceled) && !isClosedConnectionError(err) {
			t.Fatalf("active blocking invocation after shutdown error = %v, want closed stream", err)
		}
	case <-time.After(5 * time.Second):
		request.cancel()
		t.Fatal("active blocking invocation remained open after shutdown")
	}
}

func waitForListenerClosed(t *testing.T, listenerHost, listenerURL string, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		client := &http.Client{Timeout: 500 * time.Millisecond}
		response, err := client.Get(listenerURL + "/status")
		if err == nil {
			_ = response.Body.Close()
		} else {
			rebound, listenErr := net.Listen("tcp4", listenerHost)
			if listenErr == nil {
				_ = rebound.Close()
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("listener %s remained reachable after %s", listenerHost, timeout)
}

func isClosedConnectionError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "connection reset") ||
		strings.Contains(message, "broken pipe") ||
		strings.Contains(message, "use of closed network connection") ||
		strings.Contains(message, "connection refused") ||
		strings.Contains(message, "eof")
}

func reserveConfiguredLoopbackURL(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve configured loopback port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatalf("release configured loopback port: %v", err)
	}
	return "http://127.0.0.1:" + strconv.Itoa(port)
}

func startupShutdownTestFactoryConfig() map[string]any {
	return map[string]any{
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]string{{"name": "worker-a"}},
		"workstations": []map[string]any{{
			"name":      "process",
			"worker":    "worker-a",
			"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
		}},
	}
}
